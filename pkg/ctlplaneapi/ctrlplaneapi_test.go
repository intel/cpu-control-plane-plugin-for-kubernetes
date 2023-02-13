package ctlplaneapi

import (
	context "context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"
)

type DaemonMock struct {
	mock.Mock
}

func (m *DaemonMock) CreatePod(req *CreatePodRequest) (*AllocatedPodResources, error) {
	args := m.Called(req)
	return createTestCPUAllocation(req.Containers), args.Error(0)
}

func (m *DaemonMock) DeletePod(req *DeletePodRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *DaemonMock) UpdatePod(req *UpdatePodRequest) (*AllocatedPodResources, error) {
	args := m.Called(req)
	return modifyCPUAllocation(req.Containers), args.Error(0)
}

// Creates a bufconn grpc server for testing.
func NewMockedServer(ctx context.Context) (ControlPlaneClient, func(), *DaemonMock) {
	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)
	s := grpc.NewServer()
	m := DaemonMock{}
	RegisterControlPlaneServer(s, NewServer(&m))
	go func() {
		if err := s.Serve(listener); err != nil {
			panic(err)
		}
	}()

	conn, _ := grpc.DialContext(ctx, "", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	closer := func() {
		listener.Close()
		s.Stop()
	}

	client := NewControlPlaneClient(conn)

	return client, closer, &m
}

func createTestDeletion(m *DaemonMock, pid string, err error) (*DeletePodRequest, *PodAllocationReply) {
	m.On("DeletePod", &DeletePodRequest{PodId: pid}).Return(err).Once()
	return &DeletePodRequest{
			PodId: pid,
		}, &PodAllocationReply{
			PodId:      pid,
			AllocState: AllocationState_DELETED,
		}
}

func modifyCPUAllocation(container []*ContainerInfo) *AllocatedPodResources {
	a := createTestCPUAllocation(container)
	a.CPUSet = []CPUBucket{
		{
			StartCPU: 0,
			EndCPU:   128,
		},
	}
	return a
}

func createTestCPUAllocation(container []*ContainerInfo) *AllocatedPodResources {
	defaultBuckets := []CPUBucket{
		{
			StartCPU: 0,
			EndCPU:   12,
		},
		{
			StartCPU: 13,
			EndCPU:   24,
		},
	}
	cResources := []AllocatedContainerResource{}
	for _, c := range container {
		cResources = append(cResources,
			AllocatedContainerResource{
				ContainerID: c.ContainerId,
				CPUSet:      defaultBuckets,
			},
		)
	}
	return &AllocatedPodResources{
		CPUSet:             defaultBuckets,
		ContainerResources: cResources,
	}
}

func toGRPCHelper4Containers(c []AllocatedContainerResource) []*ContainerAllocationInfo {
	res := []*ContainerAllocationInfo{}
	for _, it := range c {
		res = append(res,
			&ContainerAllocationInfo{
				ContainerId: it.ContainerID,
				CpuSet:      toGRPCHelper4CPUSet(it.CPUSet),
			})
	}
	return res
}

func validateAllocatedPodReply(t *testing.T, eReply *PodAllocationReply, reply *PodAllocationReply) {
	assert.Equal(t, eReply.PodId, reply.PodId)
	assert.Equal(t, len(eReply.CpuSet), len(reply.CpuSet))
	assert.Equal(t, eReply.AllocState, reply.AllocState)
	for i := 0; i < len(eReply.CpuSet); i++ {
		assert.Equal(t, eReply.CpuSet[i].StartCPU, reply.CpuSet[i].StartCPU)
		assert.Equal(t, eReply.CpuSet[i].EndCPU, reply.CpuSet[i].EndCPU)
	}
}

func newQuantityAsBytes(v int64) []byte {
	rm := resource.NewQuantity(v, resource.DecimalSI)
	r, _ := rm.Marshal()
	return r
}

func modifyContainers(c []*ContainerInfo) []*ContainerInfo {
	res := []*ContainerInfo{}
	for i := 0; i < len(c); i++ {
		modResource := ResourceInfo{
			RequestedCpus:   1,
			LimitCpus:       2,
			RequestedMemory: newQuantityAsBytes(3),
			LimitMemory:     newQuantityAsBytes(4),
			CpuAffinity:     Placement_DEFAULT,
		}
		res = append(res,
			&ContainerInfo{
				ContainerId: c[i].ContainerId,
				Resources:   &modResource,
			},
		)
	}
	return res
}

// helper function to create some containers and resources allocations.
func createContainers(n int, a []Placement) []*ContainerInfo {
	containers := []*ContainerInfo{}
	for i := 0; i < n; i++ {
		cid := fmt.Sprintf("testCid-%d", i)
		cRInfo := ResourceInfo{
			RequestedCpus:   2,
			LimitCpus:       4,
			RequestedMemory: newQuantityAsBytes(8),
			LimitMemory:     newQuantityAsBytes(16),
			CpuAffinity:     a[i%len(a)],
		}
		containers = append(containers,
			&ContainerInfo{
				ContainerId: cid,
				Resources:   &cRInfo,
			},
		)
	}
	return containers
}

func updateTestPodRequest(t *testing.T, m *DaemonMock, cReq *CreatePodRequest, c []*ContainerInfo, err error) (*UpdatePodRequest, *PodAllocationReply) {
	ePodAllock := modifyCPUAllocation(c)
	modifiedRInfo := ResourceInfo{
		RequestedCpus:   2,
		LimitCpus:       1,
		RequestedMemory: newQuantityAsBytes(5),
		LimitMemory:     newQuantityAsBytes(32),
		CpuAffinity:     Placement_DEFAULT,
	}
	request := UpdatePodRequest{
		PodId:      cReq.PodId,
		Resources:  &modifiedRInfo,
		Containers: c,
	}
	m.On("UpdatePod",
		mock.MatchedBy(func(r *UpdatePodRequest) bool {
			return proto.Equal(r, &request)
		}),
	).Return(err)
	return &request, &PodAllocationReply{
		PodId:                 cReq.PodId,
		CpuSet:                toGRPCHelper4CPUSet(ePodAllock.CPUSet),
		ContainersAllocations: toGRPCHelper4Containers(ePodAllock.ContainerResources),
		AllocState:            AllocationState_UPDATED,
	}
}

func createTestPodRequest(t *testing.T, pName, pNamespace string, m *DaemonMock, a Placement,
	c []*ContainerInfo, err error) (*CreatePodRequest, *PodAllocationReply) {
	pid := "testPid"
	rInfo := ResourceInfo{
		RequestedCpus:   2,
		LimitCpus:       4,
		RequestedMemory: newQuantityAsBytes(8),
		LimitMemory:     newQuantityAsBytes(16),
		CpuAffinity:     a,
	}

	request := CreatePodRequest{
		PodId:        pid,
		PodName:      pName,
		PodNamespace: pNamespace,
		Resources:    &rInfo,
		Containers:   c,
	}
	m.On("CreatePod", mock.MatchedBy(func(r *CreatePodRequest) bool {
		return proto.Equal(r, &request)
	})).Return(err)
	ePodAllock := createTestCPUAllocation(c)
	return &request, &PodAllocationReply{
		PodId:                 pid,
		CpuSet:                toGRPCHelper4CPUSet(ePodAllock.CPUSet),
		ContainersAllocations: toGRPCHelper4Containers(ePodAllock.ContainerResources),
		AllocState:            AllocationState_CREATED,
	}
}

func TestCreateAndUpdatePodNoError(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	client, closer, mDaemon := NewMockedServer(ctx)
	defer closer()
	affTestCases := []Placement{Placement_DEFAULT, Placement_COMPACT, Placement_SCATTER, Placement_POOL}
	containers := createContainers(4, affTestCases)
	modifiedContainers := modifyContainers(containers)
	for _, a := range affTestCases {
		pReq, exReply := createTestPodRequest(t, "test1", "test2", mDaemon, a, containers, nil)
		reply, err := client.CreatePod(ctx, pReq)
		assert.Nil(err)
		assert.NotNil(reply)
		validateAllocatedPodReply(t, exReply, reply)
		uReq, exReply := updateTestPodRequest(t, mDaemon, pReq, modifiedContainers, nil)
		reply, err = client.UpdatePod(ctx, uReq)
		assert.Nil(err)
		assert.NotNil(reply)
		validateAllocatedPodReply(t, exReply, reply)
	}
}

func TestCreatePodError(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	client, closer, mDaemon := NewMockedServer(ctx)
	defer closer()
	affTestCases := []Placement{Placement_DEFAULT}
	containers := createContainers(1, affTestCases)
	for _, a := range affTestCases {
		pErr := status.Error(codes.Aborted, "error")
		pReq, _ := createTestPodRequest(t, "test1", "test2", mDaemon, a, containers, pErr)
		reply, err := client.CreatePod(ctx, pReq)
		assert.NotNil(err)
		assert.Contains(err.Error(), pErr.Error())
		assert.Nil(reply)
	}
}

func TestDeletePodNotFound(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	client, closer, mDaemon := NewMockedServer(ctx)
	defer closer()
	containers := createContainers(1, []Placement{Placement_DEFAULT})
	pErr := status.Error(codes.Aborted, "error")
	pReq, _ := createTestPodRequest(t, "test1", "test2", mDaemon, Placement_DEFAULT, containers, nil)
	req, _ := createTestDeletion(mDaemon, pReq.PodId, pErr)
	reply, err := client.DeletePod(ctx, req)
	assert.NotNil(err)
	assert.Contains(err.Error(), pErr.Error())
	assert.Nil(reply)
}

func TestDeletePod(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	client, closer, mDaemon := NewMockedServer(ctx)
	defer closer()
	containers := createContainers(1, []Placement{Placement_DEFAULT})
	pReq, _ := createTestPodRequest(t, "test1", "test2", mDaemon, Placement_DEFAULT, containers, nil)
	_, err := client.CreatePod(ctx, pReq)
	assert.Nil(err)
	req, eReply := createTestDeletion(mDaemon, pReq.PodId, nil)
	reply, err := client.DeletePod(ctx, req)
	validateAllocatedPodReply(t, eReply, reply)
	assert.Nil(err)
}
