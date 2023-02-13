package cpudaemon

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/numautils"
)

var ErrNamespaceNotEmpty = errors.New("namespace")
var ErrNotEnoughSpaceInBucket = errors.New("not enough free cpus in namespace bucket")
var ErrContainerNotFound = errors.New("cannot find container")
var ErrBucketNotFound = errors.New("namespace cpu bucket not found")

// NumaPerNamespaceAllocator allocates cpus in N isolated sub-pools, based on namespace. Sub-pools are
// created by splitting topology tree leafs into N buckets. Cpus in a bucket are later assigned
// sequentially to new containers. Only one guaranteed container can be pinned to each cpu, but each
// non-guaranteed container is pinned to all cpus in sub-pool.
type NumaPerNamespaceAllocator struct {
	ctrl                  CgroupController
	logger                logr.Logger
	memoryPinning         bool
	exclusive             bool
	NumBuckets            int
	NamespaceToBucket     map[string]int
	BucketToNumContainers map[int]int
	globalBucket          int
}

var _ Allocator = &NumaPerNamespaceAllocator{}

// NewNumaPerNamespaceAllocator initializes all fields of the allocator, uses default cgroup controller.
func NewNumaPerNamespaceAllocator(
	numNamespaces int,
	cgroupController CgroupController,
	exclusive bool,
	memoryPinning bool,
	logger logr.Logger,
) *NumaPerNamespaceAllocator {
	return &NumaPerNamespaceAllocator{
		ctrl:                  cgroupController,
		logger:                logger.WithName("numaPerNamespaceAllocator"),
		NumBuckets:            numNamespaces,
		NamespaceToBucket:     make(map[string]int),
		BucketToNumContainers: make(map[int]int),
		exclusive:             exclusive,
		memoryPinning:         memoryPinning,
		globalBucket:          0,
	}
}

// getBucket returns list of cpus associated with given namespace.
func (d *NumaPerNamespaceAllocator) getBucket(s *DaemonState, namespace string) ([]*numautils.TopologyNode, error) {
	leafs := s.Topology.Topology.GetLeafs()
	bucketSize := len(leafs) / d.NumBuckets

	namespaceBucket, ok := d.NamespaceToBucket[namespace]

	if !ok {
		return []*numautils.TopologyNode{}, ErrBucketNotFound
	}

	if namespaceBucket == d.NumBuckets-1 { // it is last bucket, might be larger
		return leafs[bucketSize*namespaceBucket:], nil
	}
	return leafs[bucketSize*namespaceBucket : bucketSize*(namespaceBucket+1)], nil
}

func (d *NumaPerNamespaceAllocator) takeCpus(c Container, s *DaemonState) error {
	if c.QS == Guaranteed && c.Cpus == 0 {
		return DaemonError{
			ErrorType:    NotImplemented,
			ErrorMessage: "number of guaranteed container cpus shall be greater than 0",
		}
	}

	podMetadata, ok := s.Pods[c.PID]
	if !ok {
		return DaemonError{
			ErrorType:    PodNotFound,
			ErrorMessage: fmt.Sprintf("cannot retrieve pod %s metadata", c.PID),
		}
	}

	if _, ok := d.NamespaceToBucket[podMetadata.Namespace]; !ok {
		if err := d.newNamespace(podMetadata.Namespace); err != nil {
			return DaemonError{
				ErrorType:    CpusNotAvailable,
				ErrorMessage: err.Error(),
			}
		}
	}

	bucket, err := d.getBucket(s, podMetadata.Namespace)
	if err != nil {
		return DaemonError{
			ErrorType:    CpusNotAvailable,
			ErrorMessage: err.Error(),
		}
	}

	namespaceBucket := d.NamespaceToBucket[podMetadata.Namespace]
	d.BucketToNumContainers[namespaceBucket]++

	var cpuIds []int
	if c.QS == Guaranteed {
		cpuIds, err = d.takeGuaranteedCpusFromBucket(bucket, c)
	} else {
		cpuIds, err = d.takeAllCpusFromBucket(bucket, c)
	}
	if err != nil {
		return DaemonError{
			ErrorType:    CpusNotAvailable,
			ErrorMessage: err.Error(),
		}
	}
	allocatedList := make([]ctlplaneapi.CPUBucket, 0, len(cpuIds))
	cpuSetList := make([]string, 0, len(cpuIds))
	for _, cpuID := range cpuIds {
		allocatedList = append(allocatedList, ctlplaneapi.CPUBucket{
			StartCPU: cpuID,
			EndCPU:   cpuID,
		})
		cpuSetList = append(cpuSetList, strconv.Itoa(cpuID))
	}

	s.Allocated[c.CID] = allocatedList
	if err = d.ctrl.UpdateCPUSet(s.CGroupPath, c, strings.Join(cpuSetList, ","), getMemoryPinningIfEnabled(d.memoryPinning, &s.Topology, cpuIds)); err != nil {
		return err
	}

	if d.exclusive && c.QS == Guaranteed {
		return d.removeCpusFromCommonPool(s, podMetadata.Namespace, CPUSetFromBucketList(allocatedList))
	}
	return nil
}

func (d *NumaPerNamespaceAllocator) takeGuaranteedCpusFromBucket(
	bucket []*numautils.TopologyNode,
	c Container,
) ([]int, error) {
	// we firstly check if we are able to allocate daemon
	numAvailable := 0
	for _, cpu := range bucket {
		if cpu.Available() {
			numAvailable++
			if numAvailable == c.Cpus {
				break // no need to count all
			}
		}
	}

	if numAvailable < c.Cpus {
		return []int{},
			fmt.Errorf(
				"%w: cannot allocate %d cpus, only %d processors available in bucket",
				ErrNotEnoughSpaceInBucket,
				c.Cpus,
				numAvailable,
			)
	}

	// now we can take cpus without having to return them in case if we are unable to allocate them
	var cpuIds = make([]int, 0, c.Cpus)
	for _, cpu := range bucket {
		if cpu.Available() {
			cpuIds = append(cpuIds, cpu.Value)
			if err := cpu.Take(); err != nil {
				return cpuIds, err
			}
			if len(cpuIds) == c.Cpus {
				break
			}
		}
	}
	return cpuIds, nil
}

func (d *NumaPerNamespaceAllocator) takeAllCpusFromBucket(
	bucket []*numautils.TopologyNode,
	c Container,
) ([]int, error) {
	var cpuIds = make([]int, 0, c.Cpus)
	for _, cpu := range bucket {
		if !d.exclusive || cpu.Available() { // for exlusive assignment take only cpus not taken exclusively
			cpuIds = append(cpuIds, cpu.Value)
		}
	}
	return cpuIds, nil
}

func (d *NumaPerNamespaceAllocator) freeCpus(c Container, s *DaemonState) error {
	v, ok := s.Allocated[c.CID]
	if !ok {
		return DaemonError{
			ErrorType:    ContainerNotFound,
			ErrorMessage: "Container " + c.CID + " not available for deletion",
		}
	}
	delete(s.Allocated, c.CID)

	podMetadata, ok := s.Pods[c.PID]
	if !ok {
		return DaemonError{
			ErrorType:    PodNotFound,
			ErrorMessage: fmt.Sprintf("cannot retrieve pod %s metadata", c.PID),
		}
	}

	namespaceBucket := d.NamespaceToBucket[podMetadata.Namespace]
	d.BucketToNumContainers[namespaceBucket]--
	if d.BucketToNumContainers[namespaceBucket] == 0 {
		if err := d.freeNamespace(podMetadata.Namespace); err != nil {
			return DaemonError{RuntimeError, err.Error()}
		}
	}

	for _, cpuBucket := range v {
		for cpu := cpuBucket.StartCPU; cpu <= cpuBucket.EndCPU; cpu++ {
			err := s.Topology.Return(cpu)
			if err != nil {
				return DaemonError{
					ErrorType:    CpusNotAvailable,
					ErrorMessage: err.Error(),
				}
			}
		}
	}
	if d.exclusive && c.QS == Guaranteed {
		return d.addCpusToCommonPool(s, podMetadata.Namespace, CPUSetFromBucketList(v))
	}
	return nil
}

func (d *NumaPerNamespaceAllocator) clearCpus(c Container, s *DaemonState) error {
	allCpus := s.Topology.Topology.GetLeafs()
	cpuSet := CPUSet{}
	for _, leaf := range allCpus {
		cpuSet.Add(leaf.Value)
	}
	return d.ctrl.UpdateCPUSet(
		s.CGroupPath,
		c,
		cpuSet.ToCpuString(),
		getMemoryPinningIfEnabledFromCpuSet(d.memoryPinning, &s.Topology, cpuSet),
	)
}

func (d *NumaPerNamespaceAllocator) newNamespace(namespace string) error {
	d.NamespaceToBucket[namespace] = d.globalBucket % d.NumBuckets
	d.globalBucket++
	d.logger.Info("created namespace bucket", "name", namespace)
	return nil
}

func (d *NumaPerNamespaceAllocator) freeNamespace(namespace string) error {
	namespaceBucket := d.NamespaceToBucket[namespace]
	if d.BucketToNumContainers[namespaceBucket] > 0 {
		return ErrNamespaceNotEmpty
	}

	delete(d.BucketToNumContainers, namespaceBucket)
	delete(d.NamespaceToBucket, namespace)
	d.logger.Info("deleted namespace bucket", "name", namespace)
	return nil
}

func (d *NumaPerNamespaceAllocator) removeCpusFromCommonPool(s *DaemonState, namespace string, cpus CPUSet) error {
	for cid, allocatedList := range s.Allocated {
		c, err := findContainer(s, cid)
		if err != nil {
			d.logger.Error(err, "cannot find container")
			continue
		}
		if s.Pods[c.PID].Namespace != namespace || c.QS == Guaranteed {
			continue
		}

		originalCPUs := CPUSetFromBucketList(allocatedList)
		newCPUs := originalCPUs.Clone().RemoveAll(cpus)
		d.logger.Info(
			"reallocating container",
			"reason",
			"remove",
			"cid",
			cid,
			"originalBuckets",
			originalCPUs,
			"newBucket",
			newCPUs,
		)
		err = d.ctrl.UpdateCPUSet(
			s.CGroupPath,
			c,
			newCPUs.ToCpuString(),
			getMemoryPinningIfEnabledFromCpuSet(d.memoryPinning, &s.Topology, newCPUs),
		)
		if err != nil {
			d.logger.Error(err, "could not remove cpus from common pool", "cid", cid)
			return err
		}
		s.Allocated[cid] = newCPUs.ToBucketList()
	}
	return nil
}

func (d *NumaPerNamespaceAllocator) addCpusToCommonPool(s *DaemonState, namespace string, cpus CPUSet) error {
	for cid, allocatedList := range s.Allocated {
		c, err := findContainer(s, cid)
		if err != nil {
			d.logger.Error(err, "cannot find container")
			continue
		}
		if s.Pods[c.PID].Namespace != namespace || c.QS == Guaranteed {
			continue
		}

		originalCPUs := CPUSetFromBucketList(allocatedList)
		newCPUs := originalCPUs.Clone().Merge(cpus)
		d.logger.Info(
			"reallocating container",
			"reason",
			"add",
			"cid",
			cid,
			"originalBuckets",
			originalCPUs,
			"newBucket",
			newCPUs,
		)
		err = d.ctrl.UpdateCPUSet(
			s.CGroupPath,
			c,
			newCPUs.ToCpuString(),
			getMemoryPinningIfEnabledFromCpuSet(d.memoryPinning, &s.Topology, newCPUs),
		)
		if err != nil {
			return err
		}
		s.Allocated[cid] = newCPUs.ToBucketList()
	}
	return nil
}

func findContainer(s *DaemonState, cid string) (Container, error) {
	for _, podMeta := range s.Pods {
		for _, container := range podMeta.Containers {
			if container.CID == cid {
				return container, nil
			}
		}
	}
	return Container{}, fmt.Errorf("%w %s", ErrContainerNotFound, cid)
}
