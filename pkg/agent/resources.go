package agent

import (
	"errors"
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

var (
	ErrNotRepresentable = errors.New("value not representable as int64")
	ErrCountingOverflow = errors.New("values sum is not representable as int32")
)

// GetCreatePodRequest creates CreatePodRequest from pod spec.
func GetCreatePodRequest(pod *corev1.Pod) (*ctlplaneapi.CreatePodRequest, error) {
	podID := pod.GetUID()

	containerInfo, resourceInfo, err := createPodResources(pod)

	if err != nil {
		return nil, err
	}

	createPodRequest := &ctlplaneapi.CreatePodRequest{
		PodId:        string(podID),
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		Resources:    resourceInfo,
		Containers:   containerInfo,
	}

	return createPodRequest, nil
}

// GetUpdatePodRequest creates UpdatePodRequest from pod spec.
func GetUpdatePodRequest(pod *corev1.Pod) (*ctlplaneapi.UpdatePodRequest, error) {
	podID := pod.GetUID()

	containerInfo, resourceInfo, err := createPodResources(pod)

	if err != nil {
		return nil, err
	}

	updatePodRequest := &ctlplaneapi.UpdatePodRequest{
		PodId:      string(podID),
		Resources:  resourceInfo,
		Containers: containerInfo,
	}

	return updatePodRequest, nil
}

// GetDeletePodRequest creates DeletePodRequest from pod spec.
func GetDeletePodRequest(pod *corev1.Pod) *ctlplaneapi.DeletePodRequest {
	podID := pod.GetUID()

	deletePodRequest := &ctlplaneapi.DeletePodRequest{
		PodId: string(podID),
	}

	return deletePodRequest
}

func createPodResources(pod *corev1.Pod) ([]*ctlplaneapi.ContainerInfo, *ctlplaneapi.ResourceInfo, error) {
	var podRequestedCpus int32
	var podLimitCpus int32
	var podRequestedMemory int32
	var podLimitMemory int32

	containerInfo := make([]*ctlplaneapi.ContainerInfo, 0)

	for _, container := range pod.Spec.Containers {
		container := container // prevent implicit memory alignment of iterator
		cInfo, err := getContainerInfo(&container)
		if err != nil {
			return []*ctlplaneapi.ContainerInfo{}, nil, err
		}
		cID := getContainerID(container.Name, pod)
		cInfo.ContainerId = cID

		podRequestedCpus += cInfo.Resources.RequestedCpus
		if podRequestedCpus < 0 {
			return containerInfo, nil, fmt.Errorf("cpus request: %w", ErrCountingOverflow)
		}
		podLimitCpus += cInfo.Resources.LimitCpus
		if podLimitCpus < 0 {
			return containerInfo, nil, fmt.Errorf("cpus limit: %w", ErrCountingOverflow)
		}
		podRequestedMemory += cInfo.Resources.RequestedMemory
		if podRequestedMemory < 0 {
			return containerInfo, nil, fmt.Errorf("mem request: %w", ErrCountingOverflow)
		}
		podLimitMemory += cInfo.Resources.LimitMemory
		if podLimitMemory < 0 {
			return containerInfo, nil, fmt.Errorf("mem limit: %w", ErrCountingOverflow)
		}

		containerInfo = append(containerInfo, cInfo)
	}

	resourceInfo := &ctlplaneapi.ResourceInfo{
		RequestedCpus:   podRequestedCpus,
		LimitCpus:       podLimitCpus,
		RequestedMemory: podRequestedMemory,
		LimitMemory:     podLimitMemory,
	}

	return containerInfo, resourceInfo, nil
}

func getContainerInfo(container *corev1.Container) (*ctlplaneapi.ContainerInfo, error) {
	containerResuestedCpus, containerRequestedMemory, err := getContainerResources(container.Resources.Requests)
	if err != nil {
		return nil, fmt.Errorf("requested resources error: %w", err)
	}
	containerLimitCpus, containerLimitMemory, err := getContainerResources(container.Resources.Limits)
	if err != nil {
		return nil, fmt.Errorf("limit resources error: %w", err)
	}

	containerInfo := &ctlplaneapi.ContainerInfo{
		ContainerName: container.Name,
		Resources: &ctlplaneapi.ResourceInfo{
			RequestedCpus:   containerResuestedCpus,
			LimitCpus:       containerLimitCpus,
			RequestedMemory: containerRequestedMemory,
			LimitMemory:     containerLimitMemory,
		},
	}

	return containerInfo, nil
}

func getContainerResources(resourceList corev1.ResourceList) (int32, int32, error) {
	cpusQuantity := resourceList.Cpu()
	cpus, representable := cpusQuantity.AsInt64()

	if !representable || cpus > math.MaxInt32 || cpus < 0 {
		return 0, 0, fmt.Errorf("cpu quantity %v: %w", cpusQuantity, ErrNotRepresentable)
	}

	memoryQuantity := resourceList.Memory()
	memory, representable := memoryQuantity.AsInt64()

	if !representable || memory > math.MaxInt32 || memory < 0 {
		return 0, 0, fmt.Errorf("mem quantity %v: %w", memoryQuantity, ErrNotRepresentable)
	}

	return int32(cpus), int32(memory), nil
}

func getContainerID(name string, pod *corev1.Pod) string {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == name {
			return containerStatus.ContainerID
		}
	}

	return ""
}
