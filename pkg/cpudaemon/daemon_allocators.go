package cpudaemon

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/utils"

	"github.com/containerd/cgroups"
	cgroupsv2 "github.com/containerd/cgroups/v2"
	"github.com/go-logr/logr"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// ResourceNotSet is used as default resource allocation in CgroupController.UpdateCPUSet invocations.
const ResourceNotSet = ""

// Allocator interface to take cpu.
type Allocator interface {
	takeCpus(c Container, s *DaemonState) error
	freeCpus(c Container, s *DaemonState) error
	clearCpus(c Container, s *DaemonState) error
}

// CgroupControllerImpl CgroupController interface implementation.
type CgroupControllerImpl struct {
	containerRuntime ContainerRuntime
	cgroupDriver     CGroupDriver
	logger           logr.Logger
}

// NewCgroupController returns initialized CgroupControllerImpl instance.
func NewCgroupController(containerRuntime ContainerRuntime, cgroupDriver CGroupDriver, logger logr.Logger) CgroupControllerImpl {
	return CgroupControllerImpl{containerRuntime, cgroupDriver, logger.WithName("cgroupController")}
}

// CgroupController interface to cgroup library to control cpusets.
type CgroupController interface {
	UpdateCPUSet(path string, sPath string, c Container, cpuSet string, memSet string) error
}

var _ CgroupController = CgroupControllerImpl{}

// DefaultAllocator simple static allocator without NUMA.
type DefaultAllocator struct {
	ctrl CgroupController
}

var _ Allocator = &DefaultAllocator{}

// NewDefaultAllocator constructs default cpu allocator.
func NewDefaultAllocator(controller CgroupController) *DefaultAllocator {
	return newAllocator(controller)
}

func newAllocator(ct CgroupController) *DefaultAllocator {
	d := DefaultAllocator{
		ctrl: ct,
	}
	return &d
}

// SliceName returns path to container cgroup leaf slice in cgroupfs.
func SliceName(c Container, r ContainerRuntime, d CGroupDriver) string {
	if r == Kind {
		return sliceNameKind(c)
	}
	if d == DriverSystemd {
		return sliceNameDockerContainerdWithSystemd(c, r)
	}
	return sliceNameDockerContainerdWithCgroupfs(c, r)
}

func sliceNameKind(c Container) string {
	podType := [3]string{"", "besteffort/", "burstable/"}
	return fmt.Sprintf(
		"kubelet/kubepods/%spod%s/%s",
		podType[c.QS],
		c.PID,
		strings.ReplaceAll(c.CID, "containerd://", ""),
	)
}

func sliceNameDockerContainerdWithSystemd(c Container, r ContainerRuntime) string {
	sliceType := [3]string{"", "kubepods-besteffort.slice/", "kubepods-burstable.slice/"}
	podType := [3]string{"", "-besteffort", "-burstable"}
	runtimeTypePrefix := [2]string{"docker", "cri-containerd"}
	runtimeURLPrefix := [2]string{"docker://", "containerd://"}
	return fmt.Sprintf(
		"/kubepods.slice/%skubepods%s-pod%s.slice/%s-%s.scope",
		sliceType[c.QS],
		podType[c.QS],
		strings.ReplaceAll(c.PID, "-", "_"),
		runtimeTypePrefix[r],
		strings.ReplaceAll(c.CID, runtimeURLPrefix[r], ""),
	)
}

func sliceNameDockerContainerdWithCgroupfs(c Container, r ContainerRuntime) string {
	sliceType := [3]string{"", "besteffort/", "burstable/"}
	runtimeURLPrefix := [2]string{"docker://", "containerd://"}
	return fmt.Sprintf(
		"/kubepods/%spod%s/%s",
		sliceType[c.QS],
		c.PID,
		strings.ReplaceAll(c.CID, runtimeURLPrefix[r], ""),
	)
}

func (d *DefaultAllocator) takeCpus(c Container, s *DaemonState) error {
	if c.QS != Guaranteed {
		return nil
	}
	for i, b := range s.AvailableCPUs {
		if b.EndCPU-b.StartCPU+1-c.Cpus > 0 {
			sCPU := b.StartCPU
			eCPU := b.StartCPU + c.Cpus - 1
			s.AvailableCPUs[i].StartCPU = eCPU + 1
			s.Allocated[c.CID] = []ctlplaneapi.CPUBucket{
				{
					StartCPU: sCPU,
					EndCPU:   eCPU,
				},
			}

			var t string
			if sCPU == eCPU {
				t = strconv.Itoa(sCPU)
			} else {
				t = strconv.Itoa(sCPU) + "-" + strconv.Itoa(eCPU)
			}
			return d.ctrl.UpdateCPUSet(s.CGroupPath, s.CGroupSubPath, c, t, ResourceNotSet)
		}
	}
	return DaemonError{
		ErrorType:    CpusNotAvailable,
		ErrorMessage: "No available cpus for take request",
	}
}

func (d *DefaultAllocator) freeCpus(c Container, s *DaemonState) error {
	if c.QS != Guaranteed {
		return nil
	}

	v, ok := s.Allocated[c.CID]
	if !ok {
		return DaemonError{
			ErrorType:    ContainerNotFound,
			ErrorMessage: "Container " + c.CID + " not available for deletion",
		}
	}

	delete(s.Allocated, c.CID)
	for i := 0; i < len(s.AvailableCPUs); i++ {
		if v[0].EndCPU == s.AvailableCPUs[i].StartCPU-1 {
			s.AvailableCPUs[i].StartCPU = v[0].StartCPU
		}
	}
	return nil
}

func (d *DefaultAllocator) clearCpus(c Container, s *DaemonState) error {
	var allCpus []ctlplaneapi.CPUBucket
	allCpus = append(allCpus, s.AvailableCPUs...)
	for _, allocated := range s.Allocated {
		allCpus = append(allCpus, allocated...)
	}
	cpuSet := CPUSetFromBucketList(allCpus)
	return d.ctrl.UpdateCPUSet(s.CGroupPath, s.CGroupSubPath, c, cpuSet.ToCpuString(), ResourceNotSet)
}

// UpdateCPUSet updates the cpu set of a given child process.
func (cgc CgroupControllerImpl) UpdateCPUSet(pPath string, sPath string, c Container, cSet string, memSet string) error {
	runtimeURLPrefix := [2]string{"docker://", "containerd://"}
	if cgc.containerRuntime == Kind || cgc.containerRuntime != Kind &&
		strings.Contains(c.CID, runtimeURLPrefix[cgc.containerRuntime]) {
		slice := SliceName(c, cgc.containerRuntime, cgc.cgroupDriver)
		cgc.logger.V(2).Info("allocating cgroup", "cgroupPath", pPath, "slicePath", slice, "cpuSet", cSet, "memSet", memSet)

		if cgroups.Mode() == cgroups.Unified {
			return cgc.updateCgroupsV2(pPath, slice, cSet, memSet)
		}
		return cgc.updateCgroupsV1(pPath, sPath, slice, cSet, memSet)
	}

	return DaemonError{
		ErrorType:    ConfigurationError,
		ErrorMessage: "Control Plane configured runtime does not match pod runtime",
	}
}

func (cgc CgroupControllerImpl) updateCgroupsV1(pPath, sPath, slice, cSet, memSet string) error {
	var outputPath string
	if sPath != "" {
		outputPath = path.Join(pPath, "cpuset", sPath, slice)
	} else {
		outputPath = path.Join(pPath, "cpuset", slice)
	}

	if err := utils.ValidatePathInsideBase(outputPath, pPath); err != nil {
		return err
	}

	ctrl := cgroups.NewCpuset(pPath)
	err := ctrl.Update(slice, &specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Cpus: cSet,
			Mems: memSet,
		},
	})
	// if we set the memory pinning we should enable memory_migrate in cgroups v1
	if err == nil && memSet != "" {
		var migratePath string
		if sPath != "" {
			migratePath = path.Join(pPath, "cpuset", sPath, slice, "cpuset.memory_migrate")
		} else {
			migratePath = path.Join(pPath, "cpuset", slice, "cpuset.memory_migrate")
		}
		
		err = os.WriteFile(migratePath, []byte("1"), os.FileMode(0))
	}
	return err
}

func (cgc CgroupControllerImpl) updateCgroupsV2(pPath, slice, cSet, memSet string) error {
	outputPath := path.Join(pPath, slice)
	if err := utils.ValidatePathInsideBase(outputPath, pPath); err != nil {
		return err
	}

	res := cgroupsv2.Resources{CPU: &cgroupsv2.CPU{Cpus: cSet, Mems: memSet}}
	_, err := cgroupsv2.NewManager(pPath, slice, &res)
	// memory migration in cgroups v2 is always enabled, no need to set it as in cgroupsv1
	return err
}
