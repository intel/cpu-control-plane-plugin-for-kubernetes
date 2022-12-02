// Package cpudaemon implements allocation logic of pods and containers
package cpudaemon

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

// CGroupDriver stores cgroup driver used by kubelet.
type CGroupDriver int

// CGroup drivers as defined in kubelet.
const (
	DriverSystemd CGroupDriver = iota
	DriverCgroupfs
)

// DError custom error type.
type DError int

// Possible  Daeomon errors.
const (
	CpusNotAvailable DError = iota
	PodNotFound
	PodSpecError
	ContainerNotFound
	MissingCgroup
	UnknownTopology
	RuntimeError
	NotImplemented
)

// QoS pod and containers quality of service type.
type QoS int

// QoS classes as defined in K8s.
const (
	Guaranteed QoS = iota
	BestEffort
	Burstable
)

// QoSFromLimit returns QoS class based on limits set on pod cpu.
func QoSFromLimit[T int | int32 | int64](limitCpu, requestCpu T) QoS {
	if limitCpu > 0 || requestCpu > 0 {
		if limitCpu == requestCpu {
			return Guaranteed
		}
		if requestCpu < limitCpu {
			return Burstable
		}
	}
	return BestEffort
}

// DaemonError Custom Error type.
type DaemonError struct {
	ErrorType    DError
	ErrorMessage string
}

// Error implements error interface.
func (d DaemonError) Error() string {
	return "Daemon Error: " + d.ErrorMessage
}

type failedContainer struct {
	cid string
	err error
}

type failedContainersErrors []failedContainer

func (e failedContainersErrors) ErrorOrNil() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

func (e failedContainersErrors) Error() string {
	fails := make([]string, 0, len(e))
	for _, fail := range e {
		fails = append(fails, fmt.Sprintf("cid: %s, err: %s", fail.cid, fail.err))
	}
	return fmt.Sprintf("multiple errors: %s", strings.Join(fails, ";"))
}

// PodMetadata represent a pod resource in the daemon.
type PodMetadata struct {
	PID        string
	Name       string
	Namespace  string
	Containers []Container
}

// ContainerRuntime represents different CRI used by k8s.
type ContainerRuntime int

// Supported runtimes.
const (
	Docker ContainerRuntime = iota
	ContainerdRunc
	Kind
)

func (cr ContainerRuntime) String() string {
	return []string{
		"Docker",
		"Containerd+Runc",
		"Kind",
	}[cr]
}

// Container Represents a container in the Daemon.
type Container struct {
	CID  string
	PID  string
	Name string
	Cpus int
	QS   QoS
}

// Daemon holds a state of the daemon.
type Daemon struct {
	state   DaemonState
	policy  Policy
	stateMu sync.Mutex
	logger  logr.Logger
}

type containerUpdated struct {
	current Container
	wanted  Container
}

// GetState Daemon State getter.
func (d *Daemon) GetState() string {
	return fmt.Sprint(d.state)
}

// New constrcuts a new daemon.
func New(cPath, numaPath, statePath string, p Policy, logger logr.Logger) (*Daemon, error) {
	s, err := newState(cPath, numaPath, statePath)
	if err != nil {
		return nil, err
	}
	d := Daemon{
		state:  *s,
		policy: p,
		logger: logger.WithName("daemon"),
	}

	return &d, nil
}

func (d *Daemon) rollbackContainers(podID string, containers []*ctlplaneapi.ContainerInfo) {
	for _, container := range containers {
		c := containerFromRequest(container, podID)
		d.logger.Info("rolling back container", "cid", container.ContainerId)
		err := d.policy.ClearContainer(c, &d.state)
		d.logger.Error(err, "failed to roll back container", "cid", container.ContainerId)
	}
}

// CreatePod Creates a pod with given resource allocation for the parent pod and all.
// Error handling: either all containers were added successfully or pod creation fails.
func (d *Daemon) CreatePod(req *ctlplaneapi.CreatePodRequest) (*ctlplaneapi.AllocatedPodResources, error) {
	if err := ctlplaneapi.ValidateCreatePodRequest(req); err != nil {
		d.logger.Error(err, "validation error")
		return nil, DaemonError{ErrorType: PodSpecError, ErrorMessage: err.Error()}
	}

	d.stateMu.Lock()
	defer d.stateMu.Unlock()

	d.logger.Info("create pod allocation", "request", req)

	podMeta := PodMetadata{
		PID:       req.PodId,
		Name:      req.PodName,
		Namespace: req.PodNamespace,
	}

	d.state.Pods[req.PodId] = podMeta
	containersCpus := []ctlplaneapi.AllocatedContainerResource{}

	for i, it := range req.Containers {
		c := containerFromRequest(it, req.PodId)
		err := d.policy.AssignContainer(c, &d.state)

		if err != nil {
			d.logger.Error(err, "cannot assign container", "container", c)
			d.rollbackContainers(req.PodId, req.Containers[:i])
			delete(d.state.Pods, req.PodId)
			return nil, err
		}

		containersCpus = append(containersCpus, ctlplaneapi.AllocatedContainerResource{
			ContainerID: it.ContainerId,
			CPUSet:      d.state.Allocated[it.ContainerId],
		})
		podMeta.Containers = append(podMeta.Containers, c)
		d.state.Pods[req.PodId] = podMeta
	}

	if err := d.saveState(); err != nil {
		return nil, *err
	}

	d.logger.Info("pod allocation created")
	return &ctlplaneapi.AllocatedPodResources{
		ContainerResources: containersCpus,
	}, nil
}

// DeletePod Deletes pod and children containers allocations.
// Error handling: all containers are deleted from the state, event if some error happens before.
func (d *Daemon) DeletePod(req *ctlplaneapi.DeletePodRequest) error {
	if err := ctlplaneapi.ValidateDeletePodRequest(req); err != nil {
		d.logger.Error(err, "validation error")
		return DaemonError{ErrorType: PodSpecError, ErrorMessage: err.Error()}
	}
	d.stateMu.Lock()
	defer d.stateMu.Unlock()

	d.logger.Info("delete pod allocation", "request", req)
	pod, ok := d.state.Pods[req.PodId]
	if !ok {
		err := DaemonError{
			ErrorType:    PodNotFound,
			ErrorMessage: "Pod not found in CPU State",
		}
		d.logger.Error(err, "cannot delete pod")
		return err
	}

	var err error
	if err = d.deleteContainers(pod.Containers); err != nil {
		d.logger.Error(err, "cannot delete containers") // ignore deletion errors
	}

	delete(d.state.Pods, req.PodId)

	if err := d.saveState(); err != nil {
		d.logger.Error(err, "cannot save state")
	}

	d.logger.Info("pod allocation deleted")
	return err
}

// UpdatePod Creates a pod with given resource allocation for the parent pod and all.
// Error handling: this function is reentrant.
func (d *Daemon) UpdatePod(req *ctlplaneapi.UpdatePodRequest) (*ctlplaneapi.AllocatedPodResources, error) {
	if err := ctlplaneapi.ValidateUpdatePodRequest(req); err != nil {
		d.logger.Error(err, "validation error")
		return nil, DaemonError{ErrorType: PodSpecError, ErrorMessage: err.Error()}
	}
	if _, ok := d.state.Pods[req.PodId]; !ok {
		err := DaemonError{
			ErrorType:    PodNotFound,
			ErrorMessage: fmt.Sprintf("Pod %s does not exist, cannot update", req.PodId),
		}
		d.logger.Error(err, "cannot update pod")
		return nil, err
	}

	d.stateMu.Lock()
	defer d.stateMu.Unlock()

	containersCpus := []ctlplaneapi.AllocatedContainerResource{}

	d.logger.Info("update pod allocation", "request", req)

	pod := d.state.Pods[req.PodId]
	pC := pod.Containers

	// pods present in current set, not present in request
	deleted := getDeletedContainers(pC, req.Containers)
	d.logger.V(2).Info("deleted containers", "containers", deleted)
	deletedErr := d.deleteContainers(deleted)

	// pods present in current set, and present in request, but with different parameters
	updated := getChangedContainers(pC, req.Containers)
	d.logger.V(2).Info("updated containers", "containers", updated)
	cpus, updatedContainers, updatedErr := d.updateContainers(updated)
	containersCpus = append(containersCpus, cpus...)

	// pods not present in current set, present in request
	added := getAddedContainers(pC, req.Containers, req.PodId)
	d.logger.V(2).Info("added containers", "containers", added)
	cpus, addedContainers, addedErr := d.addContainers(added)
	containersCpus = append(containersCpus, cpus...)

	pod.Containers = make([]Container, 0, len(req.Containers))
	pod.Containers = append(pod.Containers, getNotModifiedContainers(pC, req.Containers)...)
	pod.Containers = append(pod.Containers, updatedContainers...)
	pod.Containers = append(pod.Containers, addedContainers...)
	d.state.Pods[req.PodId] = pod

	if err := d.saveState(); err != nil {
		return nil, *err
	}
	d.logger.Info("pod allocation updated")

	if deletedErr != nil || addedErr != nil || updatedErr != nil {
		return &ctlplaneapi.AllocatedPodResources{ContainerResources: containersCpus}, DaemonError{
			ErrorMessage: fmt.Sprintf("Delete errors: %s, Add errors: %s, Update errors: %s",
				errOrNil(deletedErr),
				errOrNil(addedErr),
				errOrNil(updatedErr),
			),
			ErrorType: RuntimeError,
		}
	}
	return &ctlplaneapi.AllocatedPodResources{
		ContainerResources: containersCpus,
	}, nil
}

func errOrNil(err error) string {
	if err != nil {
		return err.Error()
	}
	return "nil"
}

func (d *Daemon) saveState() *DaemonError {
	d.logger.Info("saving state")
	if err := d.state.SaveState(); err != nil {
		d.logger.Error(err, "cannot save daemon state")
		return &DaemonError{RuntimeError, "Cannot save daemon state: " + err.Error()}
	}
	return nil
}

func (d *Daemon) deleteContainers(deleted []Container) error {
	failed := failedContainersErrors{}
	for _, it := range deleted {
		if err := d.policy.DeleteContainer(it, &d.state); err != nil {
			failed = append(failed, failedContainer{it.CID, err})
		}
	}
	return failed.ErrorOrNil()
}

func (d *Daemon) updateContainers(updated []containerUpdated) ([]ctlplaneapi.AllocatedContainerResource, []Container, error) {
	allocatedContainers := []ctlplaneapi.AllocatedContainerResource{}
	failed := failedContainersErrors{}
	updatedContainers := []Container{}

	for _, it := range updated {
		err := d.policy.DeleteContainer(it.current, &d.state)
		if err != nil {
			failed = append(failed, failedContainer{it.current.CID, err})
			continue
		}
		err = d.policy.AssignContainer(it.wanted, &d.state)
		if err != nil {
			failed = append(failed, failedContainer{it.current.CID, err})
			continue
		}
		allocatedContainers = append(allocatedContainers, ctlplaneapi.AllocatedContainerResource{
			ContainerID: it.wanted.CID,
			CPUSet:      d.state.Allocated[it.wanted.CID],
		})
		updatedContainers = append(updatedContainers, it.wanted)
	}
	return allocatedContainers, updatedContainers, failed.ErrorOrNil()
}

func (d *Daemon) addContainers(added []Container) ([]ctlplaneapi.AllocatedContainerResource, []Container, error) {
	allocatedContainers := []ctlplaneapi.AllocatedContainerResource{}
	addedContainers := []Container{}
	failed := failedContainersErrors{}

	for _, it := range added {
		err := d.policy.AssignContainer(it, &d.state)
		if err != nil {
			failed = append(failed, failedContainer{it.CID, err})
			continue
		}
		allocatedContainers = append(allocatedContainers, ctlplaneapi.AllocatedContainerResource{
			ContainerID: it.CID,
			CPUSet:      d.state.Allocated[it.CID],
		})
		addedContainers = append(addedContainers, it)
	}
	return allocatedContainers, addedContainers, failed.ErrorOrNil()
}

func getDeletedContainers(current []Container, wanted []*ctlplaneapi.ContainerInfo) []Container {
	deleted := make([]Container, 0, len(current))
	for _, cc := range current {
		exist := false
		for _, oc := range wanted {
			if oc.ContainerId == cc.CID {
				exist = true
				break
			}
		}
		if !exist {
			deleted = append(deleted, cc)
		}
	}
	return deleted
}

func getChangedContainers(current []Container, wanted []*ctlplaneapi.ContainerInfo) []containerUpdated {
	changed := make([]containerUpdated, 0, len(wanted))
	for _, cc := range wanted {
		for _, oc := range current {
			if oc.CID == cc.ContainerId {
				if ccr := containerFromRequest(cc, oc.PID); oc != ccr {
					changed = append(changed, containerUpdated{
						current: oc,
						wanted:  ccr,
					})
				}
			}
		}
	}
	return changed
}

func getNotModifiedContainers(current []Container, wanted []*ctlplaneapi.ContainerInfo) []Container {
	notChanged := make([]Container, 0, len(wanted))
	for _, cc := range wanted {
		for _, oc := range current {
			if oc.CID == cc.ContainerId {
				if ccr := containerFromRequest(cc, oc.PID); oc == ccr {
					notChanged = append(notChanged, oc)
				}
			}
		}
	}
	return notChanged
}

func getAddedContainers(current []Container, wanted []*ctlplaneapi.ContainerInfo, podID string) []Container {
	added := make([]Container, 0, len(wanted))
	for _, cc := range wanted {
		exist := false
		for _, oc := range current {
			if oc.CID == cc.ContainerId {
				exist = true
				break
			}
		}
		if !exist {
			added = append(added, containerFromRequest(cc, podID))
		}
	}
	return added
}

func containerFromRequest(req *ctlplaneapi.ContainerInfo, podID string) Container {
	qs := BestEffort

	if req.Resources.RequestedCpus == req.Resources.LimitCpus &&
		req.Resources.RequestedMemory == req.Resources.LimitMemory &&
		req.Resources.RequestedCpus > 0 {
		qs = Guaranteed
	} else if req.Resources.RequestedCpus < req.Resources.LimitCpus ||
		req.Resources.RequestedMemory < req.Resources.LimitMemory {
		qs = Burstable
	}

	return Container{
		CID:  req.ContainerId,
		PID:  podID,
		Name: req.ContainerName,
		Cpus: int(req.Resources.RequestedCpus),
		QS:   qs,
	}
}
