package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

type ControlPlaneClientMock struct {
	mock.Mock
}

func (c *ControlPlaneClientMock) CreatePod(
	ctx context.Context,
	in *ctlplaneapi.CreatePodRequest,
	opts ...grpc.CallOption,
) (*ctlplaneapi.PodAllocationReply, error) {
	args := c.Called(ctx, in)
	return args.Get(0).(*ctlplaneapi.PodAllocationReply), args.Error(1)
}

func (c *ControlPlaneClientMock) UpdatePod(
	ctx context.Context,
	in *ctlplaneapi.UpdatePodRequest,
	opts ...grpc.CallOption,
) (*ctlplaneapi.PodAllocationReply, error) {
	args := c.Called(ctx, in)
	return args.Get(0).(*ctlplaneapi.PodAllocationReply), args.Error(1)
}

func (c *ControlPlaneClientMock) DeletePod(
	ctx context.Context,
	in *ctlplaneapi.DeletePodRequest,
	opts ...grpc.CallOption,
) (*ctlplaneapi.PodAllocationReply, error) {
	args := c.Called(ctx, in)
	return args.Get(0).(*ctlplaneapi.PodAllocationReply), args.Error(1)
}

var _ ctlplaneapi.ControlPlaneClient = &ControlPlaneClientMock{}
var testCtx = logr.NewContext(context.TODO(), logr.Discard())

func TestCreatePodPasses(t *testing.T) {
	cpMock := ControlPlaneClientMock{}
	pod := genTestPods()
	podRequest, err := GetCreatePodRequest(&pod)
	require.Nil(t, err)
	cpMock.On("CreatePod", mock.Anything, podRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent := NewAgent(testCtx, &cpMock, "")

	agent.update(struct{}{}, &pod)

	cpMock.AssertExpectations(t)
}

func TestUpdateIgnoresDeletingPods(t *testing.T) {
	mock := ControlPlaneClientMock{}
	pod := genTestPods()
	pod.DeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
	agent := NewAgent(testCtx, &mock, "")

	agent.update(struct{}{}, &pod)

	mock.AssertExpectations(t)
}

func TestUpdateIgnoresNamespaceWithWrongPrefix(t *testing.T) {
	mock := ControlPlaneClientMock{}
	pod := genTestPods()
	agent := NewAgent(testCtx, &mock, "test")

	agent.update(struct{}{}, &pod)

	mock.AssertExpectations(t)
}

func TestUpdateIgnoresInitializingPods(t *testing.T) {
	mock := ControlPlaneClientMock{}
	pod := genTestPods()
	pod.Status.ContainerStatuses[0].Ready = false
	agent := NewAgent(testCtx, &mock, "")

	agent.update(struct{}{}, &pod)

	mock.AssertExpectations(t)
}

func TestUpdatePodPasses(t *testing.T) {
	cpMock := ControlPlaneClientMock{}
	pod := genTestPods()
	podCreateRequest, err := GetCreatePodRequest(&pod)
	require.Nil(t, err)
	podUpdateRequest, err := GetUpdatePodRequest(&pod)
	require.Nil(t, err)
	agent := NewAgent(testCtx, &cpMock, "")

	cpMock.On("CreatePod", mock.Anything, podCreateRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent.update(struct{}{}, &pod)
	cpMock.On("UpdatePod", mock.Anything, podUpdateRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent.update(struct{}{}, &pod)

	cpMock.AssertExpectations(t)
}

func TestUpdatePodPassesWithError(t *testing.T) {
	cpMock := ControlPlaneClientMock{}
	pod := genTestPods()
	podCreateRequest, err := GetCreatePodRequest(&pod)
	require.Nil(t, err)
	podUpdateRequest, err := GetUpdatePodRequest(&pod)
	require.Nil(t, err)
	agent := NewAgent(testCtx, &cpMock, "")

	cpMock.On("CreatePod", mock.Anything, podCreateRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent.update(struct{}{}, &pod)
	err = errors.New("some update error") //nolint
	cpMock.On("UpdatePod", mock.Anything, podUpdateRequest).Return(&ctlplaneapi.PodAllocationReply{}, err)
	agent.update(struct{}{}, &pod)
	assert.Equal(t, agent.numConsecutiveUnsuccessfulAttempts, uint(1))
}

func TestDeletePodPasses(t *testing.T) {
	cpMock := ControlPlaneClientMock{}
	pod := genTestPods()
	podCreateRequest, err := GetCreatePodRequest(&pod)
	require.Nil(t, err)
	podDeleteRequest := GetDeletePodRequest(&pod)
	agent := NewAgent(testCtx, &cpMock, "")

	cpMock.On("CreatePod", mock.Anything, podCreateRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent.update(struct{}{}, &pod)
	cpMock.On("DeletePod", mock.Anything, podDeleteRequest).Return(&ctlplaneapi.PodAllocationReply{}, nil)
	agent.delete(&pod)

	cpMock.AssertExpectations(t)
}

func TestDeletePodIfNotAddedPreviously(t *testing.T) {
	cpMock := ControlPlaneClientMock{}
	pod := genTestPods()
	podDeleteRequest := GetDeletePodRequest(&pod)
	agent := NewAgent(testCtx, &cpMock, "")
	err := errors.New("unsuccessful deletion") //nolint
	cpMock.On("DeletePod", mock.Anything, podDeleteRequest).Return(&ctlplaneapi.PodAllocationReply{}, err)
	agent.delete(&pod)
	assert.Equal(t, agent.numConsecutiveUnsuccessfulAttempts, uint(1))
	cpMock.AssertExpectations(t)
}

func TestDeleteIgnoresNamespaceWithWrongPrefix(t *testing.T) {
	mock := ControlPlaneClientMock{}
	pod := genTestPods()
	agent := NewAgent(testCtx, &mock, "test")

	agent.delete(&pod)

	mock.AssertExpectations(t)
}
