package numautils

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

var ErrNotALeaf = errors.New("node is not a leaf")

// TopologyEntryType holds information about level of given topological information (eg. Node/Package/Die).
type TopologyEntryType int

// TopologyEntryType enum.
const (
	Machine TopologyEntryType = iota
	Node
	Package
	Die
	Core
	Cpu
)

var topoTypeByImportance = []TopologyEntryType{Node, Package, Die, Core, Cpu}

func (t TopologyEntryType) String() string {
	switch t {
	case Machine:
		return "machine"
	case Node:
		return "node"
	case Package:
		return "package"
	case Die:
		return "die"
	case Core:
		return "core"
	case Cpu:
		return "cpu"
	default:
		return "UNKNOWN"
	}
}

type nodeInfo struct {
	Type  TopologyEntryType
	Value int
}

// TopologyNode struct holds information about single node in topology tree. It holds number of
// available leafs, defined as follows: leaf has NumAvailable = 1, for each non-leaf node, its
// NumAvailabe = sum of childrens NumAvailable. If leaf is flagged as not-available it's value becomes
// 0 and all other node values are updated.
type TopologyNode struct {
	nodeInfo
	NumAvailable int
	Children     []*TopologyNode
}

func (t *TopologyNode) String() string {
	return t.toString(1)
}

// IsLeaf returns true if node is a leaf node i.e. it has no children.
func (t *TopologyNode) IsLeaf() bool {
	return len(t.Children) == 0
}

// GetLeafs returns list of tree leafs, ordered by child precedence.
func (t *TopologyNode) GetLeafs() []*TopologyNode {
	leafs := []*TopologyNode{}
	queue := []*TopologyNode{t}
	var node *TopologyNode
	for len(queue) > 0 {
		node = queue[0]
		if node.IsLeaf() {
			leafs = append(leafs, node)
		} else {
			queue = append(queue, node.Children...)
		}
		queue = queue[1:]
	}
	return leafs
}

// Available returns true if node has available leafs.
func (t *TopologyNode) Available() bool {
	return t.NumAvailable > 0
}

// Take marks leaf as non-available. Returns error if node is not a leaf.
func (t *TopologyNode) Take() error {
	if !t.IsLeaf() {
		return ErrNotALeaf
	}
	t.NumAvailable--
	return nil
}

// Return marks leaf as available. Returns error if node is not a leaf.
func (t *TopologyNode) Return() error {
	if !t.IsLeaf() {
		return ErrNotALeaf
	}
	t.NumAvailable++
	return nil
}

func (t TopologyEntryType) valueFromCpuInfo(c CpuInfo) int {
	switch t {
	case Node:
		return c.Node
	case Package:
		return c.Package
	case Die:
		return c.Die
	case Core:
		return c.Core
	case Cpu:
		return c.Cpu
	default:
		klog.Fatalf("dont know how to get topology type %v", t)
	}
	return -1
}

func (t *TopologyNode) toString(level int) string {
	var builder strings.Builder
	builder.WriteString(
		fmt.Sprintf("%s %s %d (%d)\n", strings.Repeat("   ", level), t.Type, t.Value, t.NumAvailable),
	)
	nextLevel := level + 1
	for _, child := range t.Children {
		builder.WriteString(child.toString(nextLevel))
	}
	return builder.String()
}

func (t *TopologyNode) append(nodeInfoPath []nodeInfo) {
	if len(nodeInfoPath) == 0 { // leaf
		t.NumAvailable = 1
		return
	}
	var nextChild *TopologyNode
	for _, child := range t.Children {
		if child.Value == nodeInfoPath[0].Value {
			nextChild = child
			break
		}
	}
	if nextChild == nil {
		nextChild = &TopologyNode{
			NumAvailable: 0,
			nodeInfo:     nodeInfoPath[0],
		}
		t.Children = append(t.Children, nextChild)
	}
	t.NumAvailable++
	nextChild.append(nodeInfoPath[1:])
}

func (t *TopologyNode) findLowestNodeWithEnoughAvailability(n int, currentLevel int) (*TopologyNode, int) {
	if t.NumAvailable < n {
		return nil, -1
	}
	var (
		bestLevel    *TopologyNode
		bestLevelNum int
	)

	for _, child := range t.Children {
		level, levelNum := child.findLowestNodeWithEnoughAvailability(n, currentLevel+1)
		if level != nil && levelNum > bestLevelNum {
			bestLevel, bestLevelNum = level, levelNum
		}
	}

	if bestLevel == nil {
		return t, currentLevel
	}
	return bestLevel, bestLevelNum
}

func (t *TopologyNode) takeLeaves(n int) ([]*TopologyNode, error) {
	if n > t.NumAvailable {
		return []*TopologyNode{}, ErrNotAvailable
	}
	if t.IsLeaf() {
		t.NumAvailable = 0
		return []*TopologyNode{t}, nil
	}

	leaves := make([]*TopologyNode, 0, n)
	for _, child := range t.Children {
		if child.NumAvailable == 0 {
			continue
		}
		leavesToTake := n - len(leaves)
		if child.NumAvailable < leavesToTake {
			leavesToTake = child.NumAvailable
		}

		takenLeaves, err := child.takeLeaves(leavesToTake)
		if err != nil {
			return []*TopologyNode{t}, err
		}
		leaves = append(leaves, takenLeaves...)

		if len(leaves) == n {
			break
		}
	}
	t.NumAvailable -= n
	return leaves, nil
}

type nodeComparator func(*TopologyNode) bool

func (t *TopologyNode) find(comparator nodeComparator) []*TopologyNode {
	if comparator(t) {
		return []*TopologyNode{t}
	}
	for _, child := range t.Children {
		path := child.find(comparator)
		if len(path) > 0 {
			path = append(path, t)
			return path
		}
	}
	return []*TopologyNode{}
}

func cpuInfoToNodeInfoList(c CpuInfo, topoTypes []TopologyEntryType) []nodeInfo {
	info := make([]nodeInfo, 0, len(topoTypes))
	for _, topoType := range topoTypes {
		info = append(info, nodeInfo{topoType, topoType.valueFromCpuInfo(c)})
	}
	return info
}

// If all cpus have the same value for given topology level (node, die, etc.) let's ignore it.
func getUsedTopoTypes(cpus []CpuInfo) []TopologyEntryType {
	if len(cpus) == 0 {
		return []TopologyEntryType{}
	}

	areValuesTheSame := func(topoType TopologyEntryType) bool {
		value := topoType.valueFromCpuInfo(cpus[0])
		for _, cpu := range cpus[1:] {
			if topoType.valueFromCpuInfo(cpu) != value {
				return false
			}
		}
		return true
	}

	result := []TopologyEntryType{}
	for _, topoType := range topoTypeByImportance {
		if !areValuesTheSame(topoType) {
			result = append(result, topoType)
		}
	}
	return result
}
