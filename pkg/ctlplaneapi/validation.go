package ctlplaneapi

import (
	"errors"
	"fmt"
)

var (
	ErrEmptyString             = errors.New("string is empty")
	ErrLessThanZero            = errors.New("value cannot be less than 0")
	ErrLimitSmallerThanRequest = errors.New("limit cannot be smaller than request")
	ErrNoContainers            = errors.New("pod spec does not include any containers")
)

// ValidateResourceInfo checks if resource info fulfills following requirements:
//   - request and limit cpu/memory cannot be less than zero
//   - requested cpu/memory cannot be larger than their limit
func ValidateResourceInfo(info *ResourceInfo) error {
	if err := returnErrorIfLessThanZero([]lessThanZeroValidatorEntry{
		{info.RequestedCpus, "request CPU"},
		{info.LimitCpus, "limit CPU"},
		{info.RequestedMemory, "request memory"},
		{info.LimitMemory, "limit memory"},
	}); err != nil {
		return err
	}

	if info.LimitCpus < info.RequestedCpus {
		return fmt.Errorf("CPU: %w. %d vs %d", ErrLimitSmallerThanRequest, info.LimitCpus, info.RequestedCpus)
	}

	if info.LimitMemory < info.RequestedMemory {
		return fmt.Errorf("memory: %w", ErrLimitSmallerThanRequest)
	}

	return nil
}

// ValidateContainers checks if slice of container infos fulfills following requirements:
//   - container id and name cannot be empty
//   - container resources fullfil requirements of ValidateResourceInfo
func ValidateContainers(containers []*ContainerInfo) error {
	for _, container := range containers {
		if err := returnErrorIfEmptyString([]emptyStringValidatorEntry{
			{container.ContainerId, "container id cannot be nil"},
			{container.ContainerName, "container name cannot be nil"},
		}); err != nil {
			return err
		}

		if err := ValidateResourceInfo(container.Resources); err != nil {
			return err
		}
	}
	return nil
}

// ValidateCreatePodRequest checks if CreatePodRequest fulfills following requirements:
//   - number of containers must be greater than 0
//   - pod id, name, namespace cannot be empty
//   - pod resources fullfil requirements of ValidateResourceInfo
//   - all containers must fullfil requirements of ValidateContainers
func ValidateCreatePodRequest(req *CreatePodRequest) error {
	if len(req.Containers) == 0 {
		return ErrNoContainers
	}

	if err := returnErrorIfEmptyString([]emptyStringValidatorEntry{
		{req.PodId, "pod id cannot be nil"},
		{req.PodName, "pod name cannot be nil"},
		{req.PodNamespace, "pod namespace cannot be nil"},
	}); err != nil {
		return err
	}

	if err := ValidateResourceInfo(req.Resources); err != nil {
		return err
	}

	if err := ValidateContainers(req.Containers); err != nil {
		return err
	}

	return nil
}

// ValidateDeletePodRequest checks if DeletePodRequest fulfills following requirements:
//   - PodId cannot be empty string
func ValidateDeletePodRequest(req *DeletePodRequest) error {
	if req.PodId == "" {
		return fmt.Errorf("pod id error: %w", ErrEmptyString)
	}
	return nil
}

// ValidateUpdatePodRequest checks if UpdatePodRequest fulfills following requirements:
//   - number of containers must be greater than 0
//   - pod id cannot be empty
//   - pod resources fullfil requirements of ValidateResourceInfo
//   - all containers must fullfil requirements of ValidateContainers
func ValidateUpdatePodRequest(req *UpdatePodRequest) error {
	if len(req.Containers) == 0 {
		return ErrNoContainers
	}

	if req.PodId == "" {
		return fmt.Errorf("pod id error: %w", ErrEmptyString)
	}

	if err := ValidateResourceInfo(req.Resources); err != nil {
		return err
	}

	if err := ValidateContainers(req.Containers); err != nil {
		return err
	}

	return nil
}

type emptyStringValidatorEntry struct {
	s   string
	err string
}

func returnErrorIfEmptyString(entries []emptyStringValidatorEntry) error {
	for _, entry := range entries {
		if entry.s == "" {
			return fmt.Errorf("%w: %s", ErrEmptyString, entry.err)
		}
	}
	return nil
}

type lessThanZeroValidatorEntry struct {
	i   int32
	err string
}

func returnErrorIfLessThanZero(entries []lessThanZeroValidatorEntry) error {
	for _, entry := range entries {
		if entry.i < 0 {
			return fmt.Errorf("%w: %s", ErrLessThanZero, entry.err)
		}
	}
	return nil
}
