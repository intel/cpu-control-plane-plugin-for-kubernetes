package cpudaemon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockedNumaAllocator() *NumaAwareAllocator {
	cgroupMock := CgroupsMock{}
	allocator := &NumaAwareAllocator{
		ctrl:          &cgroupMock,
		memoryPinning: true,
	}
	return allocator
}

func TestNumaTakeCpuWithoutMemoryPinning(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(2)

	allocator := newMockedNumaAllocator()
	allocator.memoryPinning = false
	container := baseContainer(1)
	container.Cpus = 2

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0,1", "").Return(nil)

	assert.Nil(t, allocator.takeCpus(container, s))

	assertCpuState(t, s, &container, "0,1")
	mock.AssertExpectations(t)
}

func TestNumaTakeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(2)

	allocator := newMockedNumaAllocator()
	container := baseContainer(1)
	container.Cpus = 2

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0,1", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(container, s))

	assertCpuState(t, s, &container, "0,1")
	mock.AssertExpectations(t)
}

func TestNumaTakeCpuFailsIfTooMuchCpus(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(2)

	allocator := newMockedNumaAllocator()
	container := baseContainer(1)
	container.Cpus = 3

	assert.NotNil(t, allocator.takeCpus(container, s))
}

func TestNumaFreeCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(2)

	allocator := newMockedNumaAllocator()

	container := baseContainer(1)

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0", "0").Return(nil)

	assert.Nil(t, allocator.takeCpus(container, s))
	assert.Contains(t, s.Allocated, container.CID)

	assert.Nil(t, allocator.freeCpus(container, s))
	assert.NotContains(t, s.Allocated, container.CID)
	mock.AssertExpectations(t)
}

func TestNumaClearCpu(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_cpu")
	require.Nil(t, err)
	defer os.RemoveAll(dir)

	s := getTestDaemonState(dir, 2)
	s.Topology = oneLevelTopology(2)

	allocator := newMockedNumaAllocator()
	container := baseContainer(1)
	container.Cpus = 2

	mock := allocator.ctrl.(*CgroupsMock)
	mock.On("UpdateCPUSet", s.CGroupPath, s.CGroupSubPath, container, "0,1", "0").Return(nil)

	assert.Nil(t, allocator.clearCpus(container, s))

	mock.AssertExpectations(t)
}
