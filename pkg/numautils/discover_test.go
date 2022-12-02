package numautils

import (
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dirMode = 0700
const fileMode = 0600

type optionalCpuInfo struct {
	packageID int
	dieID     int
	coreID    int
}

type testNode struct {
	nodeNum int
	cpus    map[int]optionalCpuInfo
}

func createNodeFiles(dir string, node testNode) error {
	nodePath := path.Join(dir, nodePrefix+strconv.Itoa(node.nodeNum))
	if err := os.Mkdir(nodePath, 0750); err != nil {
		return err
	}

	for cpuID, cpuData := range node.cpus {
		cpuPath := path.Join(nodePath, cpuPrefix+strconv.Itoa(cpuID))

		if err := os.Mkdir(cpuPath, dirMode); err != nil {
			return err
		}

		topologyPath := path.Join(cpuPath, topologyDir)
		if err := os.Mkdir(topologyPath, dirMode); err != nil {
			return err
		}

		createFileIfValueSet := func(fname string, value int) error {
			if value < 0 {
				return nil
			}
			filePath := path.Join(topologyPath, fname)
			valueString := strconv.Itoa(value)
			return os.WriteFile(filePath, []byte(valueString), fileMode)
		}

		if err := createFileIfValueSet(packageFile, cpuData.packageID); err != nil {
			return err
		}

		if err := createFileIfValueSet(dieFile, cpuData.dieID); err != nil {
			return err
		}

		if err := createFileIfValueSet(coreFile, cpuData.coreID); err != nil {
			return err
		}
	}
	return nil
}

func TestLoadNodes(t *testing.T) {
	testDir, err := os.MkdirTemp("", "test")
	assert.Nil(t, err)
	defer os.RemoveAll(testDir)

	err = createNodeFiles(testDir, testNode{
		nodeNum: 41,
	})
	require.Nil(t, err)
	err = createNodeFiles(testDir, testNode{
		nodeNum: 5,
	})
	require.Nil(t, err)

	nodes, err := loadNodes(testDir)
	assert.Nil(t, err)
	assert.ElementsMatch(t, []int{41, 5}, nodes)
}

func TestReadIntFromFiles(t *testing.T) {
	testCases := []struct {
		content string
		result  int
		isError bool
	}{
		{"123", 123, false},
		{"123\n", 123, false},
		{"test", 0, true},
		{"", 0, true},
		{"-1", -1, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.content, func(t *testing.T) {
			file, err := os.CreateTemp("", "test")
			assert.Nil(t, err)
			defer os.Remove(file.Name())

			_, err = file.WriteString(testCase.content)
			assert.Nil(t, err)

			value, err := readIntFromFile(file.Name(), "")
			if testCase.isError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}

			assert.Equal(t, testCase.result, value)
		})
	}
}

func TestListCpusFromNodeTestpath(t *testing.T) {
	testDir, err := os.MkdirTemp("", "test")
	assert.Nil(t, err)
	defer os.RemoveAll(testDir)

	err = createNodeFiles(testDir, testNode{
		nodeNum: 41,
		cpus: map[int]optionalCpuInfo{
			1: {
				packageID: -1,
				dieID:     1,
				coreID:    0,
			},
			3: {
				packageID: -1,
				dieID:     1,
				coreID:    0,
			},
			5: {
				packageID: -1,
				dieID:     1,
				coreID:    1,
			},
			8: {
				packageID: -1,
				dieID:     2,
				coreID:    1,
			},
		},
	})
	require.Nil(t, err)
	expectedCpus := []CpuInfo{
		{
			Cpu:     1,
			Node:    41,
			Package: 0,
			Die:     1,
			Core:    0,
		},
		{
			Cpu:     3,
			Node:    41,
			Package: 0,
			Die:     1,
			Core:    0,
		},
		{
			Cpu:     5,
			Node:    41,
			Package: 0,
			Die:     1,
			Core:    1,
		},
		{
			Cpu:     8,
			Node:    41,
			Package: 0,
			Die:     2,
			Core:    1,
		},
	}

	cpuInfos, err := listCpusFromNode(testDir, 41)
	assert.Nil(t, err)

	assert.ElementsMatch(t, expectedCpus, cpuInfos)
}
