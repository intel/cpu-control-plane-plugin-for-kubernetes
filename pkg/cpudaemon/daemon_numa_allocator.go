package cpudaemon

import (
	"strconv"
	"strings"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/numautils"
)

// NumaAwareAllocator allocates cpus based on node numa topology. The topology is represented as tree
// whose leafs are cpus and nodes are next levels of topology organization. For each request alllocator
// find such allocation, that will minimize the topology distance between cpus. In our case the topology
// distance between n leafs is defined as maximal path length from any of those leafs to the nearest
// common predecessor.
type NumaAwareAllocator struct {
	ctrl          CgroupController
	memoryPinning bool
}

var _ Allocator = &NumaAwareAllocator{}

// NewNumaAwareAllocator Creates new numa-aware allocator with default cgroup controller.
func NewNumaAwareAllocator(cgroupController CgroupController, memoryPinning bool) *NumaAwareAllocator {
	return &NumaAwareAllocator{
		ctrl:          cgroupController,
		memoryPinning: memoryPinning,
	}
}

func getMemoryPinningIfEnabledFromCpuSet(memoryPinning bool, topology *numautils.NumaTopology, cpus CPUSet) string {
	if !memoryPinning {
		return ""
	}

	return getMemoryPinning(topology, cpus.Sorted())
}

func getMemoryPinningIfEnabled(memoryPinning bool, topology *numautils.NumaTopology, cpuIds []int) string {
	if !memoryPinning {
		return ""
	}

	return getMemoryPinning(topology, cpuIds)
}

func getMemoryPinning(topology *numautils.NumaTopology, cpuIds []int) string {
	nodesSet := map[int]struct{}{}

	for _, cpu := range cpuIds {
		nodesSet[topology.CpuInformation[cpu].Node] = struct{}{}
	}

	nodesList := make([]string, 0, len(nodesSet))
	for k := range nodesSet {
		nodesList = append(nodesList, strconv.Itoa(k))
	}
	return strings.Join(nodesList, ",")
}

func (d *NumaAwareAllocator) takeCpus(c Container, s *DaemonState) error {
	if c.QS != Guaranteed {
		return nil
	}

	cpuIds, err := s.Topology.Take(c.Cpus)
	if err != nil {
		return DaemonError{
			ErrorType:    CpusNotAvailable,
			ErrorMessage: err.Error(),
		}
	}

	allocatedList := s.Allocated[c.CID]
	cpuSetList := make([]string, 0, c.Cpus)
	for _, cpuID := range cpuIds {
		allocatedList = append(allocatedList, ctlplaneapi.CPUBucket{
			StartCPU: cpuID,
			EndCPU:   cpuID,
		})
		cpuSetList = append(cpuSetList, strconv.Itoa(cpuID))
	}
	s.Allocated[c.CID] = allocatedList

	return d.ctrl.UpdateCPUSet(
		s.CGroupPath,
		c,
		strings.Join(cpuSetList, ","),
		getMemoryPinningIfEnabled(d.memoryPinning, &s.Topology, cpuIds),
	)
}

func (d *NumaAwareAllocator) freeCpus(c Container, s *DaemonState) error {
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
	return nil
}

func (d *NumaAwareAllocator) clearCpus(c Container, s *DaemonState) error {
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
