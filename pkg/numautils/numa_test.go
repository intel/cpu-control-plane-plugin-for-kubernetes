package numautils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedTree = &TopologyNode{
	nodeInfo:     nodeInfo{Machine, 0},
	NumAvailable: 8,
	Children: ChildList{
		&TopologyNode{
			nodeInfo:     nodeInfo{Node, 0},
			NumAvailable: 4,
			Children: ChildList{
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 0},
					NumAvailable: 2,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 1},
							NumAvailable: 1,
						},
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 3},
							NumAvailable: 1,
						},
					},
				},
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 1},
					NumAvailable: 2,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 5},
							NumAvailable: 1,
						},
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 7},
							NumAvailable: 1,
						},
					},
				},
			},
		},
		&TopologyNode{
			nodeInfo:     nodeInfo{Node, 1},
			NumAvailable: 4,
			Children: ChildList{
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 0},
					NumAvailable: 2,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 2},
							NumAvailable: 1,
						},
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 4},
							NumAvailable: 1,
						},
					},
				},
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 1},
					NumAvailable: 2,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 6},
							NumAvailable: 1,
						},
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 8},
							NumAvailable: 1,
						},
					},
				},
			},
		},
	},
}

func setupNumaTest(t *testing.T) (string, func()) {
	testDir, err := os.MkdirTemp("", "test")
	assert.Nil(t, err)
	teardownFunc := func() { os.RemoveAll(testDir) }

	err = createNodeFiles(testDir, testNode{
		nodeNum: 0,
		cpus: map[int]optionalCpuInfo{
			1: {
				coreID: 0,
			},
			3: {
				coreID: 0,
			},
			5: {
				coreID: 1,
			},
			7: {
				coreID: 1,
			},
		},
	})
	require.Nil(t, err)
	err = createNodeFiles(testDir, testNode{
		nodeNum: 1,
		cpus: map[int]optionalCpuInfo{
			2: {
				coreID: 0,
			},
			4: {
				coreID: 0,
			},
			6: {
				coreID: 1,
			},
			8: {
				coreID: 1,
			},
		},
	})
	require.Nil(t, err)

	return testDir, teardownFunc
}

func newNuma(t *testing.T) NumaTopology {
	tree, err := cloneTree(expectedTree)
	assert.Nil(t, err)
	return NumaTopology{
		Topology: tree,
	}
}

func TestLoad(t *testing.T) {
	testDir, teardownFunc := setupNumaTest(t)
	defer teardownFunc()

	numa := NumaTopology{}
	err := numa.Load(testDir)
	require.Nil(t, err)

	assertEqualTrees(t, expectedTree, numa.Topology)
}

func TestTake(t *testing.T) {
	type takeCase struct {
		n               int
		expectedisError bool
		expectedCpus    []int
	}

	testCases := []struct {
		name  string
		takes []takeCase
	}{
		{"1", []takeCase{{1, false, []int{1}}}},
		{"1,2", []takeCase{
			{1, false, []int{1}},
			{2, false, []int{5, 7}},
		}},
		{"1,5", []takeCase{
			{1, false, []int{1}},
			{5, false, []int{3, 5, 7, 2, 4}},
		}},
		{"2,1,2", []takeCase{
			{2, false, []int{1, 3}},
			{1, false, []int{5}},
			{2, false, []int{2, 4}},
		}},
		{"1, 8", []takeCase{
			{1, false, []int{1}},
			{8, true, []int{}},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			numa := newNuma(t)
			for _, takeCase := range testCase.takes {
				cpus, err := numa.Take(takeCase.n)
				if takeCase.expectedisError {
					assert.NotNil(t, err)
				} else {
					assert.Nil(t, err)
					assert.Equal(t, takeCase.expectedCpus, cpus)
				}
				assert.True(t, verifyNumAvailable(numa.Topology))
			}
		})
	}
}

func TestReturnCorrect(t *testing.T) {
	numa := newNuma(t)
	ids, err := numa.Take(2)
	assert.Nil(t, err)

	for _, id := range ids {
		assert.Nil(t, numa.Return(id))
		assert.True(t, verifyNumAvailable(numa.Topology))
	}
}

func TestReturnIncorrect(t *testing.T) {
	numa := newNuma(t)
	assert.Nil(t, numa.Return(1))
	assert.True(t, verifyNumAvailable(numa.Topology))
}
