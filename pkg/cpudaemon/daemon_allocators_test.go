package cpudaemon

import (
	"strconv"
	"testing"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type CgroupsMock struct {
	mock.Mock
}

func (m *CgroupsMock) UpdateCPUSet(pP string, sP string, c Container, cpu string, mem string) error {
	args := m.Called(pP, sP, c, cpu, mem)
	return args.Error(0)
}

func newMockedPolicy(m CgroupController) *DefaultAllocator {
	return newAllocator(m)
}

func takeCPUs(t *testing.T, d *DefaultAllocator, ctrl *CgroupsMock, st *DaemonState, c Container, s int, e int) {
	ctrl.On("UpdateCPUSet", st.CGroupPath, st.CGroupSubPath, c, strconv.Itoa(s)+"-"+strconv.Itoa(e), ResourceNotSet).Return(nil)
	// check no error
	assert.Nil(t, d.takeCpus(c, st))
	// check list of allocated containers
	v, ok := st.Allocated[c.CID]
	assert.True(t, ok)
	assert.Equal(t, []ctlplaneapi.CPUBucket{
		{
			StartCPU: s,
			EndCPU:   e,
		},
	}, v, "TakeCPU returned unexpected cpu bucket!")
	// check list of available cpus
	assert.Equal(t,
		[]ctlplaneapi.CPUBucket{
			{
				StartCPU: e + 1,
				EndCPU:   127,
			},
		}, st.AvailableCPUs)
	// check stored state
}

func deleteContainer(t *testing.T, d *DefaultAllocator, st *DaemonState, c Container, nS int) {
	assert.Nil(t, d.freeCpus(c, st))
	_, ok := st.Allocated[c.CID]
	assert.False(t, ok)
	assert.Equal(t,
		[]ctlplaneapi.CPUBucket{
			{
				StartCPU: nS,
				EndCPU:   127,
			},
		}, st.AvailableCPUs)
}

func TestDefaultAllocatorTakeCPU(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	mockCtrl := CgroupsMock{}
	st, err := newState("testdata/no_state", "", "testdata/node_info", daemonStateFile)
	assert.Nil(t, err)
	d := newMockedPolicy(&mockCtrl)
	c := Container{
		PID:  "test_pod_id1",
		CID:  "test_container_iud1",
		Cpus: 10,
		QS:   Guaranteed,
	}
	takeCPUs(t, d, &mockCtrl, st, c, 0, 9)
	c = Container{
		PID:  "test_pod_id2",
		CID:  "test_container_iud2",
		Cpus: 10,
		QS:   Guaranteed,
	}
	takeCPUs(t, d, &mockCtrl, st, c, 10, 19)
}

func TestErrorNoCPUsAvailableOnTake(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	s, err := newState("testdata/no_state", "", "testdata/node_info", daemonStateFile)
	assert.Nil(t, err)

	d := NewDefaultAllocator(NewCgroupController(Docker, DriverSystemd, logr.Discard()))
	assert.NotNil(t, d)
	c := Container{
		PID:  "test_pod_id",
		CID:  "test_container_id",
		Cpus: 129,
		QS:   Guaranteed,
	}
	err = d.takeCpus(c, s)
	assert.Equal(t, DaemonError{
		ErrorType:    CpusNotAvailable,
		ErrorMessage: "No available cpus for take request",
	}, err)
}

func TestErrorWrongRuntimeConfiguration(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	st, err := newState("testdata/no_state", "", "testdata/node_info", daemonStateFile)
	assert.Nil(t, err)
	d := NewDefaultAllocator(NewCgroupController(Docker, DriverSystemd, logr.Discard()))
	assert.NotNil(t, d)
	c := Container{
		PID:  "test_pod_id1",
		CID:  "containerd://test_container_iud1",
		Cpus: 10,
		QS:   Guaranteed,
	}
	err = d.takeCpus(c, st)
	assert.Equal(t, DaemonError{
		ErrorType:    ConfigurationError,
		ErrorMessage: "Control Plane configured runtime does not match pod runtime",
	}, err)
}
func TestTakeAndDeleteContainer(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	mockCtrl := CgroupsMock{}
	st, err := newState("testdata/no_state", "", "testdata/node_info", daemonStateFile)
	assert.Nil(t, err)

	d := newMockedPolicy(&mockCtrl)
	assert.NotNil(t, d)
	c := Container{
		PID:  "test_pod_id1",
		CID:  "test_container_iud1",
		Cpus: 10,
		QS:   Guaranteed,
	}
	takeCPUs(t, d, &mockCtrl, st, c, 0, 9)
	c = Container{
		PID:  "test_pod_id2",
		CID:  "test_container_iud2",
		Cpus: 10,
		QS:   Guaranteed,
	}
	takeCPUs(t, d, &mockCtrl, st, c, 10, 19)
	deleteContainer(t, d, st, c, 10)
}

func TestDefaultAllocatorClearCPU(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	mockCtrl := CgroupsMock{}
	st, err := newState("testdata/no_state", "", "testdata/node_info", daemonStateFile)
	assert.Nil(t, err)
	d := newMockedPolicy(&mockCtrl)
	c := Container{
		PID:  "test_pod_id1",
		CID:  "test_container_iud1",
		Cpus: 10,
		QS:   Guaranteed,
	}
	expectedCpuSet, err := CPUSetFromString("0-127")
	require.Nil(t, err)

	mockCtrl.On("UpdateCPUSet", st.CGroupPath, st.CGroupSubPath, c, expectedCpuSet.ToCpuString(), ResourceNotSet).Return(nil)
	assert.Nil(t, d.clearCpus(c, st))

	mockCtrl.AssertExpectations(t)
}

func TestSliceNameKind(t *testing.T) {
	container := Container{CID: "containerd://cid", PID: "pid-01", QS: Burstable}
	expectedSlice := "kubelet/kubepods/burstable/podpid-01/cid"
	assert.Equal(t, expectedSlice, SliceName(container, Kind, DriverCgroupfs))
}

func TestSliceNameSystemd(t *testing.T) {
	container := Container{CID: "containerd://cid", PID: "pid-01", QS: Burstable}
	expectedSlice := "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podpid_01.slice/cri-containerd-cid.scope"
	assert.Equal(t, expectedSlice, SliceName(container, ContainerdRunc, DriverSystemd))
}

func TestSliceNameCgroupfs(t *testing.T) {
	container := Container{CID: "docker://cid", PID: "pid-01", QS: Burstable}
	expectedSlice := "/kubepods/burstable/podpid-01/cid"
	assert.Equal(t, expectedSlice, SliceName(container, Docker, DriverCgroupfs))
}
