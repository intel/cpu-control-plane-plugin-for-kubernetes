package numautils

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

type ChildList []*TopologyNode

var testTree = &TopologyNode{
	nodeInfo:     nodeInfo{Node, 0},
	NumAvailable: 4,
	Children: ChildList{
		&TopologyNode{
			nodeInfo:     nodeInfo{Die, 0},
			NumAvailable: 3,
			Children: ChildList{
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 1},
					NumAvailable: 1,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 0},
							NumAvailable: 1,
						},
					},
				},
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 0},
					NumAvailable: 2,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 0},
							NumAvailable: 1,
						},
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 1},
							NumAvailable: 1,
						},
					},
				},
			},
		},
		&TopologyNode{
			nodeInfo:     nodeInfo{Die, 1},
			NumAvailable: 1,
			Children: ChildList{
				&TopologyNode{
					nodeInfo:     nodeInfo{Core, 1},
					NumAvailable: 1,
					Children: ChildList{
						&TopologyNode{
							nodeInfo:     nodeInfo{Cpu, 44},
							NumAvailable: 1,
						},
					},
				},
			},
		},
	},
}

var testTreeExpectedString = `    node 0 (4)
       die 0 (3)
          core 1 (1)
             cpu 0 (1)
          core 0 (2)
             cpu 0 (1)
             cpu 1 (1)
       die 1 (1)
          core 1 (1)
             cpu 44 (1)
`

func sortChildren(tree *TopologyNode) {
	if len(tree.Children) > 0 {
		sort.Slice(tree.Children, func(i, j int) bool {
			return tree.Children[i].Value < tree.Children[j].Value
		})
		for _, child := range tree.Children {
			sortChildren(child)
		}
	}
}

func cloneTree(tree *TopologyNode) (*TopologyNode, error) {
	jsonData, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}
	clonedTree := &TopologyNode{}
	err = json.Unmarshal(jsonData, clonedTree)
	if err != nil {
		return nil, err
	}
	return clonedTree, nil
}

func assertEqualTrees(t *testing.T, expected *TopologyNode, actual *TopologyNode) {
	treeToStandarizedForm := func(tree *TopologyNode) *TopologyNode {
		cloned, err := cloneTree(tree)
		assert.Nil(t, err)
		sortChildren(cloned)
		return cloned
	}

	expected = treeToStandarizedForm(expected)
	actual = treeToStandarizedForm(actual)

	assert.Equal(t, expected, actual)
}

func verifyNumAvailable(node *TopologyNode) bool {
	if node.IsLeaf() {
		return node.NumAvailable == 0 || node.NumAvailable == 1
	}
	numAvailable := 0
	for _, child := range node.Children {
		verified := verifyNumAvailable(child)
		if !verified {
			return false
		}
		numAvailable += child.NumAvailable
	}
	return numAvailable == node.NumAvailable
}

func TestAppend(t *testing.T) {
	appendList := [][]nodeInfo{
		{
			{Node, 0},
			{Die, 0},
			{Core, 1},
			{Cpu, 0},
		},
		{
			{Node, 0},
			{Die, 0},
			{Core, 0},
			{Cpu, 0},
		},
		{
			{Node, 0},
			{Die, 0},
			{Core, 0},
			{Cpu, 1},
		},
		{
			{Node, 0},
			{Die, 1},
			{Core, 1},
			{Cpu, 44},
		},
	}

	root := TopologyNode{}
	for _, infoPath := range appendList {
		root.append(infoPath)
	}

	assert.Equal(t, len(root.Children), 1)
	assertEqualTrees(t, root.Children[0], testTree)
}

func TestIsLeaf(t *testing.T) {
	testCases := []struct {
		name   string
		tree   *TopologyNode
		IsLeaf bool
	}{
		{"height 4", testTree, false},
		{"height 2", testTree.Children[0].Children[0], false},
		{"leaf", testTree.Children[0].Children[0].Children[0], true},
	}

	for _, testCase := range testCases {
		t.Run(
			testCase.name, func(t *testing.T) {
				assert.Equal(t, testCase.IsLeaf, testCase.tree.IsLeaf())
			},
		)
	}
}

func TestFindLowestNodeWithEnoughAvailability(t *testing.T) {
	testCases := []struct {
		name         string
		n            int
		expectedNode *TopologyNode
	}{
		{"one cpu", 1, testTree.Children[0].Children[0].Children[0]},
		{"two cpus", 2, testTree.Children[0].Children[1]},
		{"three cpus", 3, testTree.Children[0]},
		{"maximum number", 4, testTree},
		{"more than maximum", 5, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			node, _ := testTree.findLowestNodeWithEnoughAvailability(testCase.n, 0)
			assert.Same(t, testCase.expectedNode, node)
		})
	}
}

func TestTakeLeaves(t *testing.T) {
	tree, err := cloneTree(testTree)
	assert.Nil(t, err)
	allLeafs := []*TopologyNode{
		tree.Children[0].Children[0].Children[0],
		tree.Children[0].Children[1].Children[0],
		tree.Children[0].Children[1].Children[1],
		tree.Children[1].Children[0].Children[0],
	}
	for _, chld := range allLeafs {
		chld.NumAvailable = 0
	}

	for numLeaves := 1; numLeaves <= len(allLeafs); numLeaves++ {
		t.Run(
			fmt.Sprintf("take %d leaves", numLeaves),
			func(t *testing.T) {
				tree, err := cloneTree(testTree)
				assert.Nil(t, err)

				leaves, err := tree.takeLeaves(numLeaves)
				assert.Nil(t, err)

				assert.Equal(t, leaves, allLeafs[:numLeaves])
				assert.True(t, verifyNumAvailable(tree))
			},
		)
	}
}

func TestTakeMoreLeavesThanAvailable(t *testing.T) {
	numAvailable := testTree.NumAvailable
	trees, err := testTree.takeLeaves(numAvailable + 1)
	assert.Empty(t, trees)
	assert.NotNil(t, err)

	// check if we did set any child as taken
	assert.Equal(t, testTree.NumAvailable, numAvailable)
	assert.True(t, verifyNumAvailable(testTree))
}

func TestGetLeavesTestTree(t *testing.T) {
	leafs := testTree.GetLeafs()
	expectedLeafs := []*TopologyNode{
		{
			nodeInfo:     nodeInfo{Cpu, 0},
			NumAvailable: 1,
		},
		{
			nodeInfo:     nodeInfo{Cpu, 0},
			NumAvailable: 1,
		},
		{
			nodeInfo:     nodeInfo{Cpu, 1},
			NumAvailable: 1,
		},
		{
			nodeInfo:     nodeInfo{Cpu, 44},
			NumAvailable: 1,
		},
	}

	assert.Equal(t, expectedLeafs, leafs)
}

func TestToString(t *testing.T) {
	s := testTree.String()
	assert.Equal(t, testTreeExpectedString, s)
}
