package cpudaemon

import (
	"fmt"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type PodMetaData struct {
	pid                 string
	name                string
	namespace           string
	resources           *ctlplaneapi.ResourceInfo
	containers          []Container
	deletedContainers   []Container
	containersResources []*ctlplaneapi.ContainerInfo
	expectations        ctlplaneapi.AllocatedPodResources
}

func newQuantityAsBytes(v int64) []byte {
	rm := resource.NewQuantity(v, resource.DecimalSI)
	r, _ := rm.Marshal()
	return r
}

type MockedPolicy struct {
	mock.Mock
}

func (m *MockedPolicy) AssignContainer(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func (m *MockedPolicy) DeleteContainer(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func (m *MockedPolicy) ClearContainer(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func setupTest() (string, func(tb testing.TB)) {
	return "daemon.state", func(tb testing.TB) {
		os.Remove("daemon.state")
	}
}

func createTestPod(n int) PodMetaData {
	r := ctlplaneapi.ResourceInfo{
		RequestedCpus:   2,
		LimitCpus:       2,
		RequestedMemory: newQuantityAsBytes(8),
		LimitMemory:     newQuantityAsBytes(8),
		CpuAffinity:     ctlplaneapi.Placement_COMPACT,
	}
	pid := "testPid"
	p := PodMetaData{
		pid:                 pid,
		name:                pid,
		namespace:           pid,
		resources:           &r,
		containers:          []Container{},
		containersResources: []*ctlplaneapi.ContainerInfo{},
	}

	for i := 0; i < n; i++ {
		cid := fmt.Sprintf("testCid-%d", i)
		cr := ctlplaneapi.ResourceInfo{
			RequestedCpus:   int32(i + 1),
			LimitCpus:       int32(i + 1),
			RequestedMemory: newQuantityAsBytes(8),
			LimitMemory:     newQuantityAsBytes(8),
			CpuAffinity:     ctlplaneapi.Placement_COMPACT,
		}
		p.containers = append(p.containers,
			Container{
				CID:  cid,
				PID:  pid,
				Name: cid,
				Cpus: i + 1,
				QS:   Guaranteed,
			},
		)
		p.containersResources = append(p.containersResources,
			&ctlplaneapi.ContainerInfo{
				ContainerId:   cid,
				ContainerName: cid,
				Resources:     &cr,
			},
		)
		p.expectations.ContainerResources = append(p.expectations.ContainerResources,
			ctlplaneapi.AllocatedContainerResource{
				ContainerID: cid,
				CPUSet: []ctlplaneapi.CPUBucket{
					{
						StartCPU: 0,
						EndCPU:   i + 1,
					},
				},
			},
		)
	}
	return p
}

func modifyTestPod(p PodMetaData, d int, u int) PodMetaData {
	mp := PodMetaData{}
	r := ctlplaneapi.ResourceInfo{
		RequestedCpus:   4,
		LimitCpus:       4,
		RequestedMemory: newQuantityAsBytes(16),
		LimitMemory:     newQuantityAsBytes(16),
		CpuAffinity:     ctlplaneapi.Placement_COMPACT,
	}
	mp.pid = p.pid
	mp.resources = &r
	// delete last d containers
	for i := len(p.containers) - d - 1; i < len(p.containers); i++ {
		mp.deletedContainers = append(mp.deletedContainers, p.containers[i])
	}
	// modify the rest
	for i := 0; i < len(p.containers)-d; i++ {
		cpus := p.containers[i].Cpus
		if i < u {
			cpus = p.containers[i].Cpus + 1
		}
		cr := ctlplaneapi.ResourceInfo{
			RequestedCpus:   int32(cpus),
			LimitCpus:       int32(cpus),
			RequestedMemory: newQuantityAsBytes(8),
			LimitMemory:     newQuantityAsBytes(8),
			CpuAffinity:     ctlplaneapi.Placement_COMPACT,
		}
		mp.containers = append(mp.containers,
			Container{
				CID:  p.containers[i].CID,
				PID:  p.containers[i].PID,
				Name: p.containers[i].Name,
				Cpus: cpus,
				QS:   Guaranteed,
			},
		)
		mp.containersResources = append(mp.containersResources,
			&ctlplaneapi.ContainerInfo{
				ContainerId:   p.containers[i].CID,
				ContainerName: p.containers[i].Name,
				Resources:     &cr,
			},
		)
		mp.expectations.ContainerResources = append(mp.expectations.ContainerResources,
			ctlplaneapi.AllocatedContainerResource{
				ContainerID: p.containers[i].CID,
				CPUSet: []ctlplaneapi.CPUBucket{
					{
						StartCPU: 0,
						EndCPU:   i + 2,
					},
				},
			},
		)
	}

	return mp
}

func TestNewDaemonNoState(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &MockedPolicy{}, logr.Discard())
	require.Nil(t, err)
	assert.NotNil(t, d)
	expectedState := DaemonState{
		CGroupPath: "testdata/no_state",
		Pods:       make(map[string]PodMetadata),
		StatePath:  daemonStateFile,
	}
	expectedState.AvailableCPUs = append(expectedState.AvailableCPUs,
		ctlplaneapi.CPUBucket{
			StartCPU: 0,
			EndCPU:   127,
		})
	expectedState.Allocated = make(map[string][]ctlplaneapi.CPUBucket, 0)
	assert.Nil(t, expectedState.Topology.Load("testdata/node_info"))
	assert.Equal(t, expectedState, d.state)
}

func TestCreateDaemonWithState(t *testing.T) {
	d, err := New("testdata/with_state/", "testdata/node_info", "testdata/with_state/daemon.state", &MockedPolicy{}, logr.Discard())
	require.Nil(t, err)
	assert.NotNil(t, d)

	expectedState := DaemonState{
		CGroupPath: "testdata/with_state/",
		Pods:       make(map[string]PodMetadata),
		StatePath:  "testdata/with_state/daemon.state",
	}
	expectedState.AvailableCPUs = append(expectedState.AvailableCPUs,
		ctlplaneapi.CPUBucket{
			StartCPU: 0,
			EndCPU:   55,
		},
		ctlplaneapi.CPUBucket{
			StartCPU: 76,
			EndCPU:   78,
		},
		ctlplaneapi.CPUBucket{
			StartCPU: 99,
			EndCPU:   99,
		},
	)
	expectedState.Allocated = make(map[string][]ctlplaneapi.CPUBucket)
	assert.Nil(t, expectedState.Topology.Load("testdata/node_info"))
	assert.Equal(t, expectedState, d.state)
}

func TestCreateAndModifyPodDefaultPolity(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	p := createTestPod(3)

	// set the container cpu state
	for i, c := range p.containers {
		expectecCPUSet := []ctlplaneapi.CPUBucket{
			{
				StartCPU: 0,
				EndCPU:   i + 1,
			},
		}
		d.state.Allocated[c.CID] = expectecCPUSet
		m.On("AssignContainer", c, &d.state).Return(nil).Once()
	}
	allocCPUs, err := d.CreatePod(
		&ctlplaneapi.CreatePodRequest{
			PodId:        p.pid,
			PodName:      p.name,
			PodNamespace: p.namespace,
			Resources:    p.resources,
			Containers:   p.containersResources,
		},
	)

	assert.Nil(t, err)
	if err == nil {
		assert.Equal(t, p.expectations, *allocCPUs)
	}
	del := 2
	mod := 1
	mp := modifyTestPod(p, del, mod)

	// delete removed containers
	for _, c := range mp.deletedContainers {
		m.On("DeleteContainer", c, &d.state).Return(nil).Once()
	}
	// assign modified cpus and set the container cpu state
	for i, c := range mp.containers {
		if i < mod {
			expectecCPUSet := []ctlplaneapi.CPUBucket{
				{
					StartCPU: 0,
					EndCPU:   i + 2,
				},
			}
			d.state.Allocated[c.CID] = expectecCPUSet
			m.On("AssignContainer", c, &d.state).Return(nil).Once()
		}
	}

	allocCPUs, err = d.UpdatePod(
		&ctlplaneapi.UpdatePodRequest{
			PodId:      p.pid,
			Resources:  mp.resources,
			Containers: mp.containersResources,
		},
	)
	assert.Nil(t, err)
	if err == nil {
		assert.Equal(t, 1, len(allocCPUs.ContainerResources))
		assert.Equal(t, mp.expectations, *allocCPUs)
	}
}

func TestCreatePodDefaultPolicyNoSuffcientCPUsError(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	p := createTestPod(3)

	// set the container cpu state
	for _, c := range p.containers {
		m.On("AssignContainer", c, &d.state).Return(
			DaemonError{ErrorType: CpusNotAvailable, ErrorMessage: " No Cpus avaialbe!"},
		).Once()
	}
	allocCPUs, err := d.CreatePod(
		&ctlplaneapi.CreatePodRequest{
			PodId:        p.pid,
			PodName:      p.name,
			PodNamespace: p.namespace,
			Resources:    p.resources,
			Containers:   p.containersResources,
		},
	)
	expErr := DaemonError{ErrorType: CpusNotAvailable, ErrorMessage: " No Cpus avaialbe!"}
	assert.Equal(t, expErr, err)
	assert.Nil(t, allocCPUs)
}

func TestDeletePodDefaultPolicy(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	p := createTestPod(2)
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	meta := d.state.Pods[p.pid]
	meta.Containers = p.containers
	d.state.Pods[p.pid] = meta
	m.On("DeleteContainer", p.containers[0], &d.state).Return(nil).Once()
	m.On("DeleteContainer", p.containers[1], &d.state).Return(nil).Once()
	err = d.DeletePod(&ctlplaneapi.DeletePodRequest{PodId: p.pid})
	assert.Nil(t, err)
}

func TestDeletePodDefaultPolicyError(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	p := createTestPod(1)
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	err = d.DeletePod(&ctlplaneapi.DeletePodRequest{PodId: p.pid})
	expErr := DaemonError{ErrorType: PodNotFound, ErrorMessage: "Pod not found in CPU State"}
	assert.Equal(t, expErr, err)
}

func TestDaemonCreatePodRollbacks(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	p := createTestPod(2)

	// set the container cpu state
	m.On("AssignContainer", p.containers[0], &d.state).Return(nil).Once()
	m.On("AssignContainer", p.containers[1], &d.state).Return(
		DaemonError{ErrorType: CpusNotAvailable, ErrorMessage: " No Cpus avaialbe!"},
	).Once()
	m.On("ClearContainer", p.containers[0], &d.state).Return(nil).Once()

	allocCPUs, err := d.CreatePod(
		&ctlplaneapi.CreatePodRequest{
			PodId:        p.pid,
			PodName:      p.name,
			PodNamespace: p.namespace,
			Resources:    p.resources,
			Containers:   p.containersResources,
		},
	)
	expErr := DaemonError{ErrorType: CpusNotAvailable, ErrorMessage: " No Cpus avaialbe!"}
	assert.Equal(t, expErr, err)
	assert.Nil(t, allocCPUs)
	assert.NotContains(t, d.state.Pods, p.pid)
}

func TestDeletePodContinuesDeletionAfterError(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	p := createTestPod(2)
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	meta := d.state.Pods[p.pid]
	meta.Containers = p.containers
	d.state.Pods[p.pid] = meta
	expectedError := DaemonError{ErrorMessage: "test"}
	m.On("DeleteContainer", p.containers[0], &d.state).Return(expectedError).Once()
	m.On("DeleteContainer", p.containers[1], &d.state).Return(nil).Once()

	err = d.DeletePod(&ctlplaneapi.DeletePodRequest{PodId: p.pid})

	assert.Equal(t, failedContainersErrors{failedContainer{p.containers[0].CID, expectedError}}, err)
	m.AssertExpectations(t)
}

func TestUpdatePodContinuesAfterError(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	m := MockedPolicy{}
	d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
	require.Nil(t, err)
	p := createTestPod(3)

	// set the container cpu state
	for i, c := range p.containers {
		expectecCPUSet := []ctlplaneapi.CPUBucket{
			{
				StartCPU: 0,
				EndCPU:   i + 1,
			},
		}
		d.state.Allocated[c.CID] = expectecCPUSet
		m.On("AssignContainer", c, &d.state).Return(nil).Once()
	}
	allocCPUs, err := d.CreatePod(
		&ctlplaneapi.CreatePodRequest{
			PodId:        p.pid,
			PodName:      p.name,
			PodNamespace: p.namespace,
			Resources:    p.resources,
			Containers:   p.containersResources,
		},
	)

	assert.Nil(t, err)
	if err == nil {
		assert.Equal(t, p.expectations, *allocCPUs)
	}
	del := 2
	mod := 1
	mp := modifyTestPod(p, del, mod)

	deleteError := fmt.Errorf("delete error") //nolint
	// delete removed containers, first error, then pass
	for i, c := range mp.deletedContainers {
		if i == 2 {
			m.On("DeleteContainer", c, &d.state).Return(deleteError).Once()
		} else {
			m.On("DeleteContainer", c, &d.state).Return(nil).Once()
		}
	}

	updateError := fmt.Errorf("update error") //nolint
	// assign modified cpus and set the container cpu state
	for i, c := range mp.containers {
		if i < mod {
			expectecCPUSet := []ctlplaneapi.CPUBucket{
				{
					StartCPU: 0,
					EndCPU:   i + 2,
				},
			}
			d.state.Allocated[c.CID] = expectecCPUSet
			m.On("AssignContainer", c, &d.state).Return(updateError).Once()
		}
	}
	expectedErr := DaemonError{
		ErrorType: RuntimeError,
		ErrorMessage: fmt.Sprintf("Delete errors: %s, Add errors: nil, Update errors: %s",
			failedContainersErrors{failedContainer{mp.deletedContainers[2].CID, deleteError}},
			failedContainersErrors{failedContainer{mp.containers[0].CID, updateError}},
		),
	}

	_, err = d.UpdatePod(
		&ctlplaneapi.UpdatePodRequest{
			PodId:      p.pid,
			Resources:  mp.resources,
			Containers: mp.containersResources,
		},
	)
	assert.Equal(t, expectedErr, err)
	assert.Empty(t, d.state.Pods[p.pid].Containers) // because update pod failed
}
