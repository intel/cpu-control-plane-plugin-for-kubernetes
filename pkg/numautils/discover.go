package numautils

import (
	"path"
	"strconv"
	"strings"

	"resourcemanagement.controlplane/pkg/utils"
)

// LinuxTopologyPath is a path where kernels exposes machine topology information.
const LinuxTopologyPath = "/sys/devices/system/node"

const (
	nodePrefix  = "node"
	cpuPrefix   = "cpu"
	topologyDir = "topology"
	packageFile = "package_id"
	dieFile     = "die_id"
	coreFile    = "core_id"
)

// CpuInfo stores topology information about single CPU.
type CpuInfo struct {
	Node    int
	Package int
	Die     int
	Core    int
	Cpu     int
}

func loadNodes(topologyPath string) ([]int, error) {
	return getEntriesWithPrefixAndNumber(topologyPath, nodePrefix)
}

func listCpusFromNode(topologyPath string, node int) ([]CpuInfo, error) {
	cpuIDs, err := getEntriesWithPrefixAndNumber(getNodeDirPath(topologyPath, node), cpuPrefix)
	if err != nil {
		return []CpuInfo{}, err
	}
	cpus := []CpuInfo{}
	for _, cpu := range cpuIDs {
		cpuTopologyBase := path.Join(getCPUDirPath(topologyPath, node, cpu), topologyDir)
		readOrDefault := func(fileName string) int {
			data, err := readIntFromFile(cpuTopologyBase, fileName)
			if err != nil {
				return 0
			}
			return data
		}
		cpu := CpuInfo{
			Cpu:     cpu,
			Node:    node,
			Package: readOrDefault(packageFile),
			Die:     readOrDefault(dieFile),
			Core:    readOrDefault(coreFile),
		}
		cpus = append(cpus, cpu)
	}

	return cpus, nil
}

func getNodeDirPath(topologyPath string, node int) string {
	return path.Join(topologyPath, nodePrefix+strconv.Itoa(node))
}

func getCPUDirPath(topologyPath string, node int, cpu int) string {
	return path.Join(getNodeDirPath(topologyPath, node), cpuPrefix+strconv.Itoa(cpu))
}

func readIntFromFile(basePath, filename string) (int, error) {
	data, err := utils.ReadFileAt(basePath, filename)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
