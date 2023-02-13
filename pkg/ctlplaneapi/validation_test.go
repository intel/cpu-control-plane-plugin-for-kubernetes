package ctlplaneapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func properResourceInfo() *ResourceInfo {
	return &ResourceInfo{
		RequestedCpus:   1,
		LimitCpus:       1,
		RequestedMemory: newQuantityAsBytes(1),
		LimitMemory:     newQuantityAsBytes(1),
	}
}

func properContainers() []*ContainerInfo {
	return []*ContainerInfo{
		{
			ContainerId:   "ci",
			ContainerName: "cn",
			Resources: &ResourceInfo{
				RequestedCpus:   1,
				LimitCpus:       1,
				RequestedMemory: newQuantityAsBytes(1),
				LimitMemory:     newQuantityAsBytes(1),
			},
		},
	}
}

func TestValidateResourceInfo(t *testing.T) {
	require.Nil(t, ValidateResourceInfo(properResourceInfo()))

	testCases := []struct {
		modifier    func(*ResourceInfo)
		expectedErr error
	}{
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitCpus = 2 },
			expectedErr: nil,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitMemory = newQuantityAsBytes(2) },
			expectedErr: nil,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.RequestedCpus = -1 },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitCpus = -1 },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.RequestedMemory = newQuantityAsBytes(-1) },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitMemory = newQuantityAsBytes(-1) },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitMemory = newQuantityAsBytes(0) },
			expectedErr: ErrLimitSmallerThanRequest,
		},
		{
			modifier:    func(ri *ResourceInfo) { ri.LimitCpus = 0 },
			expectedErr: ErrLimitSmallerThanRequest,
		},
	}

	for _, testCase := range testCases {
		req := properResourceInfo()
		testCase.modifier(req)

		err := ValidateResourceInfo(req)
		assert.ErrorIs(t, err, testCase.expectedErr)
	}
}

func TestValidateContainers(t *testing.T) {
	require.Nil(t, ValidateContainers(properContainers()))

	testCases := []struct {
		modifier    func([]*ContainerInfo)
		expectedErr error
	}{
		{
			modifier:    func(ci []*ContainerInfo) { ci[0].ContainerId = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(ci []*ContainerInfo) { ci[0].ContainerName = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(ci []*ContainerInfo) { ci[0].Resources.LimitCpus = -1 },
			expectedErr: ErrLessThanZero,
		},
	}

	for _, testCase := range testCases {
		req := properContainers()
		testCase.modifier(req)

		err := ValidateContainers(req)
		assert.ErrorIs(t, err, testCase.expectedErr)
	}
}

func TestValidateCreatePodRequest(t *testing.T) {
	properPodRequest := func() *CreatePodRequest {
		return &CreatePodRequest{
			PodId:        "i",
			PodName:      "n",
			PodNamespace: "ns",
			Resources:    properResourceInfo(),
			Containers:   properContainers(),
		}
	}

	require.Nil(t, ValidateCreatePodRequest(properPodRequest()))

	testCases := []struct {
		modifier    func(*CreatePodRequest)
		expectedErr error
	}{
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.Containers = []*ContainerInfo{} },
			expectedErr: ErrNoContainers,
		},
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.PodId = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.PodName = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.PodNamespace = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.Resources.LimitCpus = -1 },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(cpr *CreatePodRequest) { cpr.Containers[0].ContainerId = "" },
			expectedErr: ErrEmptyString,
		},
	}

	for _, testCase := range testCases {
		req := properPodRequest()
		testCase.modifier(req)

		err := ValidateCreatePodRequest(req)
		assert.ErrorIs(t, err, testCase.expectedErr)
	}
}

func TestValidateDeletePodRequest(t *testing.T) {
	assert.Nil(t, ValidateDeletePodRequest(&DeletePodRequest{PodId: "i"}))
	assert.ErrorIs(t, ValidateDeletePodRequest(&DeletePodRequest{}), ErrEmptyString)
}

func TestValidateUpdatePodRequest(t *testing.T) {
	properPodRequest := func() *UpdatePodRequest {
		return &UpdatePodRequest{
			PodId:      "i",
			Resources:  properResourceInfo(),
			Containers: properContainers(),
		}
	}

	require.Nil(t, ValidateUpdatePodRequest(properPodRequest()))

	testCases := []struct {
		modifier    func(*UpdatePodRequest)
		expectedErr error
	}{
		{
			modifier:    func(cpr *UpdatePodRequest) { cpr.Containers = []*ContainerInfo{} },
			expectedErr: ErrNoContainers,
		},
		{
			modifier:    func(cpr *UpdatePodRequest) { cpr.PodId = "" },
			expectedErr: ErrEmptyString,
		},
		{
			modifier:    func(cpr *UpdatePodRequest) { cpr.Resources.LimitCpus = -1 },
			expectedErr: ErrLessThanZero,
		},
		{
			modifier:    func(cpr *UpdatePodRequest) { cpr.Containers[0].ContainerName = "" },
			expectedErr: ErrEmptyString,
		},
	}

	for _, testCase := range testCases {
		req := properPodRequest()
		testCase.modifier(req)

		err := ValidateUpdatePodRequest(req)
		assert.ErrorIs(t, err, testCase.expectedErr)
	}
}
