// Package numautils reads topology as seen in /sys/devices/system/node (documentation available at
// https://www.kernel.org/doc/Documentation/ABI/stable/sysfs-devices-node). It then represents it as
// tree whose leafs are cpus.
package numautils

import (
	"errors"
	"fmt"
)

// ErrNotAvailable is returned when it is impossible to allocate cpus.
var ErrNotAvailable = errors.New("not enough cpus available")

// ErrNotFound is returned when cpu information cannot be found.
var ErrNotFound = errors.New("cpu not found")

// ErrLoadError is returned when loading topology information from kernel failed.
var ErrLoadError = errors.New("cannot read topology information")

// NumaTopology holds topology information of the machine. User should invoke `Load` method to
// initialize topology information.
type NumaTopology struct {
	Topology       *TopologyNode
	CpuInformation map[int]CpuInfo
}

// Take finds n non-used cpu in topology tree. It find such allocation, that will minimize the topology
// distance between cpus. In our case the topology distance between n leafs is defined as maximal
// path length from any of those leafs to the nearest common predecessor.
func (t *NumaTopology) Take(n int) ([]int, error) {
	l, _ := t.Topology.findLowestNodeWithEnoughAvailability(n, 0)
	if l == nil {
		return []int{}, ErrNotAvailable
	}
	leaves, err := l.takeLeaves(n)
	// takeLeves updated NumAvailable from l down to leaves.
	// We must now update l predecessors
	if l != t.Topology {
		path := t.Topology.find(func(tl *TopologyNode) bool { return tl == l })
		for _, node := range path[1:] { // 1st is l itself
			node.NumAvailable -= n
		}
	}
	if err != nil {
		return []int{}, ErrNotAvailable
	}
	cpuIDs := make([]int, 0, n)
	for _, leaf := range leaves {
		cpuIDs = append(cpuIDs, leaf.Value)
	}
	return cpuIDs, nil
}

// FindCpu returns TopologyNode of given cpu. The node is guaranteed to be a leaf of the topology
// tree.
func (t *NumaTopology) FindCpu(cpuID int) (*TopologyNode, error) {
	path := t.Topology.find(func(tl *TopologyNode) bool { return tl.IsLeaf() && tl.Value == cpuID })
	if len(path) == 0 {
		return nil, ErrNotFound
	}
	return path[0], nil
}

// Return returns given cpu to pool of available cpus.
func (t *NumaTopology) Return(cpuID int) error {
	path := t.Topology.find(func(tl *TopologyNode) bool { return tl.IsLeaf() && tl.Value == cpuID })
	if len(path) == 0 {
		return ErrNotFound
	}
	if path[0].NumAvailable == 0 {
		for _, node := range path {
			node.NumAvailable++
		}
	}

	return nil
}

// Load loads topology information from given topology path (usually it should be `LinuxTopologyPath`).
func (t *NumaTopology) Load(topologyPath string) error {
	nodes, err := loadNodes(topologyPath)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrLoadError, err)
	}

	cpuInfos := []CpuInfo{}
	for _, node := range nodes {
		nodeCpus, err := listCpusFromNode(topologyPath, node)
		if err != nil {
			return fmt.Errorf("%w: cannot load cpus information for node %d, %v", ErrLoadError, node, err)
		}
		cpuInfos = append(cpuInfos, nodeCpus...)
	}

	return t.LoadFromCpuInfo(cpuInfos)
}

// LoadFromCpuInfo loads topology tree information given list of cpus.
func (t *NumaTopology) LoadFromCpuInfo(cpus []CpuInfo) error {
	t.cpuInfoToTopology(cpus)

	t.CpuInformation = make(map[int]CpuInfo)
	for _, cpuInfo := range cpus {
		t.CpuInformation[cpuInfo.Cpu] = cpuInfo
	}

	return nil
}

// Create node topology tree.
func (t *NumaTopology) cpuInfoToTopology(cpuInfos []CpuInfo) {
	t.Topology = &TopologyNode{
		nodeInfo: nodeInfo{Type: Machine},
	}

	topoTypes := getUsedTopoTypes(cpuInfos)

	for _, cpu := range cpuInfos {
		t.Topology.append(cpuInfoToNodeInfoList(cpu, topoTypes))
	}
}
