package cpudaemon

import (
	"os"
	"strconv"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/numautils"
)

func oneLevelTopology(numCpus int) numautils.NumaTopology {
	topology := numautils.NumaTopology{
		CpuInformation: make(map[int]numautils.CpuInfo),
	}

	cpus := []numautils.CpuInfo{}
	for i := 0; i < numCpus; i++ {
		cpus = append(cpus, numautils.CpuInfo{
			Cpu: i,
		})
	}

	if err := topology.LoadFromCpuInfo(cpus); err != nil {
		panic(err)
	}
	return topology
}

func getTestDaemonState(tempDir string, numCpus int) *DaemonState {
	s := DaemonState{
		Allocated: map[string][]ctlplaneapi.CPUBucket{},
		Pods: map[string]PodMetadata{
			"pod1": {
				PID:       "pod1",
				Name:      "pod1_name",
				Namespace: "pod1_namespace",
			},
			"pod2": {
				PID:       "pod2",
				Name:      "pod2_name",
				Namespace: "pod2_namespace",
			},
			"pod3": {
				PID:       "pod3",
				Name:      "pod3_name",
				Namespace: "pod3_namespace",
			},
		},
		Topology:   numautils.NumaTopology{},
		CGroupPath: tempDir,
	}
	s.Topology = oneLevelTopology(numCpus)

	return &s
}

func newMockedNumaPerNamespaceAllocator(numBuckets int, exclusive bool) *NumaPerNamespaceAllocator {
	cgroupMock := CgroupsMock{}
	allocator := &NumaPerNamespaceAllocator{
		ctrl:                  &cgroupMock,
		logger:                logr.Discard(),
		exclusive:             exclusive,
		NumBuckets:            numBuckets,
		NamespaceToBucket:     map[string]int{},
		BucketToNumContainers: map[int]int{},
		memoryPinning:         true,
	}
	return allocator
}

func baseContainer(num int) Container {
	numStr := strconv.Itoa(num)
	return Container{
		CID:  "cid" + numStr,
		PID:  "pod" + numStr,
		Name: "cid" + numStr + "_name",
		Cpus: 1,
		QS:   Guaranteed,
	}
}

func getGuaranteedAndBurstableContainers() (Container, Container) {
	guaranteed := baseContainer(1)
	burstable := baseContainer(2)
	burstable.PID = "pod1"
	burstable.QS = Burstable
	return guaranteed, burstable
}

func addContainerToState(s *DaemonState, c Container) {
	podMeta := s.Pods[c.PID]
	podMeta.Containers = append(podMeta.Containers, c)
	s.Pods[c.PID] = podMeta
}

func assertCpuState(t *testing.T, s *DaemonState, container *Container, cpuString string) {
	expectedCpus, err := CPUSetFromString(cpuString)
	require.Nil(t, err)
	assert.Equal(t, expectedCpus, CPUSetFromBucketList(s.Allocated[container.CID]))
}

func TestNumaNamespaceTakeCpuWithoutMemoryPinning(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	allocator.memoryPinning = false
	containerNs1 := baseContainer(1)
	containerNs2 := baseContainer(2)

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs1, "0", "").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs2, "1", "").Return(nil)

	assert.Nil(t, allocator.takeCpus(containerNs1, s))
	assert.Nil(t, allocator.takeCpus(containerNs2, s))

	assertCpuState(t, s, &containerNs1, "0")
	assertCpuState(t, s, &containerNs2, "1")
}

func TestNumaNamespaceTakeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	containerNs1 := baseContainer(1)
	containerNs2 := baseContainer(2)

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs1, "0", "0").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs2, "1", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(containerNs1, s))
	assert.Nil(t, allocator.takeCpus(containerNs2, s))

	assertCpuState(t, s, &containerNs1, "0")
	assertCpuState(t, s, &containerNs2, "1")
}

func TestNumaNamespaceOversubscribedTakeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 4)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	containerNs1 := baseContainer(1)
	containerNs2 := baseContainer(2)
	containerNs3 := baseContainer(3)

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs1, "0", "0").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs2, "2", "0").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerNs3, "1", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(containerNs1, s))
	assert.Nil(t, allocator.takeCpus(containerNs2, s))
	assert.Nil(t, allocator.takeCpus(containerNs3, s))

	assertCpuState(t, s, &containerNs1, "0")
	assertCpuState(t, s, &containerNs2, "2")
	assertCpuState(t, s, &containerNs3, "1")
}

func TestNumaNamespaceExclusiveTakeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 8)

	allocator := newMockedNumaPerNamespaceAllocator(2, true)
	containerGuaranteed, containerBurstable := getGuaranteedAndBurstableContainers()
	containerBurstable2 := containerBurstable
	containerBurstable2.CID = "pod3"

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerGuaranteed, "0", "0").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable, "1,2,3", "0").Return(nil)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable2, "1,2,3", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(containerGuaranteed, s))
	assert.Nil(t, allocator.takeCpus(containerBurstable, s))
	assert.Nil(t, allocator.takeCpus(containerBurstable2, s))
	mock.AssertExpectations(t)

	assertCpuState(t, s, &containerGuaranteed, "0")
	assertCpuState(t, s, &containerBurstable, "1,2,3")
	assertCpuState(t, s, &containerBurstable2, "1,2,3")
}

func TestNumaNamespaceExclusiveTakeCpuWithReallocation(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 4)

	allocator := newMockedNumaPerNamespaceAllocator(2, true)
	containerGuaranteed, containerBurstable := getGuaranteedAndBurstableContainers()

	mock := allocator.ctrl.(*CgroupsMock)

	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable, "0,1", "0").Return(nil) // 1st allocation of burstable
	assert.Nil(t, allocator.takeCpus(containerBurstable, s))
	assertCpuState(t, s, &containerBurstable, "0,1")
	addContainerToState(s, containerBurstable)

	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerGuaranteed, "0", "0").Return(nil) // allocation of guaranteed
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable, "1", "0").Return(nil)  // reallocation of burstable
	assert.Nil(t, allocator.takeCpus(containerGuaranteed, s))
	mock.AssertExpectations(t)

	assertCpuState(t, s, &containerBurstable, "1")
	assertCpuState(t, s, &containerGuaranteed, "0")
}

func TestNumaNamespaceTakeCpuNonGuaranteed(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(4)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	container := baseContainer(1)
	container.QS = Burstable

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0,1", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(container, s))
	mock.AssertExpectations(t)

	assertCpuState(t, s, &container, "0,1")
}

func TestNumaNamespaceFreeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)

	container := baseContainer(1)

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(container, s))
	assert.Contains(t, s.Allocated, container.CID)

	assert.Nil(t, allocator.freeCpus(container, s))
	assert.NotContains(t, s.Allocated, container.CID)
	mock.AssertExpectations(t)
}

func TestNumaNamespaceExclusiveFreeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 4)

	allocator := newMockedNumaPerNamespaceAllocator(1, true)
	containerGuaranteed, containerBurstable := getGuaranteedAndBurstableContainers()

	mock := allocator.ctrl.(*CgroupsMock)

	// add guaranteed container for cpu 0
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerGuaranteed, "0", "0").Return(nil)
	assert.Nil(t, allocator.takeCpus(containerGuaranteed, s))
	addContainerToState(s, containerGuaranteed)

	// add burstable container for cpu 1,2,3
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable, "1,2,3", "0").Return(nil)
	assert.Nil(t, allocator.takeCpus(containerBurstable, s))
	addContainerToState(s, containerBurstable)

	assert.Contains(t, s.Allocated, containerGuaranteed.CID)

	// remove guaranteed container, the burstable container shall now be reassigned to cpus 0,1,2,3
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, containerBurstable, "0,1,2,3", "0").Return(nil)
	assert.Nil(t, allocator.freeCpus(containerGuaranteed, s))

	assert.NotContains(t, s.Allocated, containerGuaranteed.CID)

	mock.AssertExpectations(t)
}

func TestNumaNamespaceTakeCpuFailsIfNotEnoughSpace(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)

	assert.Error(t, allocator.takeCpus(Container{
		CID:  "cid1",
		PID:  "pod1",
		Name: "cid1_name",
		Cpus: 2,
		QS:   Guaranteed,
	}, s))
}

func TestNumaNamespaceTakeCpuFailsIfAllBucketsTaken(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	cmock := allocator.ctrl.(*CgroupsMock)
	cmock.On("UpdateCPUSet", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	assert.Nil(t, allocator.takeCpus(baseContainer(1), s))
	assert.Nil(t, allocator.takeCpus(baseContainer(2), s))
	assert.Error(t, allocator.takeCpus(baseContainer(3), s))
	cmock.AssertExpectations(t)
}

func TestNumaNamespaceClearCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(4)

	allocator := newMockedNumaPerNamespaceAllocator(2, false)
	container := baseContainer(1)
	container.QS = Burstable

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0,1,2,3", "0").Return(nil)

	assert.Nil(t, allocator.clearCpus(container, s))
	mock.AssertExpectations(t)
}
