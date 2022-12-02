package agent

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

type resourceSpec struct {
	reqCpu string
	reqMem string
	limCpu string
	limMem string
}

func genPodFromSpec(containersResources []resourceSpec) corev1.Pod {
	containers := make([]corev1.Container, 0, len(containersResources))
	statuses := make([]corev1.ContainerStatus, 0, len(containersResources))
	for i, container := range containersResources {
		containers = append(containers, corev1.Container{
			Name: fmt.Sprintf("test container %d", i+1),
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(container.reqCpu),
					corev1.ResourceMemory: resource.MustParse(container.reqMem),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(container.limCpu),
					corev1.ResourceMemory: resource.MustParse(container.limMem),
				},
			},
		})

		statuses = append(statuses, corev1.ContainerStatus{
			ContainerID: fmt.Sprintf("id test container %d", i+1),
			Name:        fmt.Sprintf("test container %d", i+1),
			Ready:       true,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{
					StartedAt: metav1.Time{Time: time.Now()},
				},
			},
		})
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "default",
			UID:       "123",
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: statuses,
		},
	}
	return pod
}

func genTestPod() corev1.Pod {
	return genPodFromSpec(
		[]resourceSpec{
			{
				reqCpu: "2000",
				reqMem: "32Mi",
				limCpu: "3000",
				limMem: "64Mi",
			},
			{
				reqCpu: "3000",
				reqMem: "24Mi",
				limCpu: "4000",
				limMem: "48Mi",
			},
		},
	)
}

func assertResourcesEqualWithTestPod(t *testing.T, ri *ctlplaneapi.ResourceInfo) {
	assert.Equal(t, int32(5000), ri.RequestedCpus)
	assert.Equal(t, int32(7000), ri.LimitCpus)
	assert.Equal(t, int32(56*1024*1024), ri.RequestedMemory)
	assert.Equal(t, int32(112*1024*1024), ri.LimitMemory)
}

func assertContainersEqualWithTestPod(t *testing.T, ci []*ctlplaneapi.ContainerInfo) {
	assert.Equal(t, 2, len(ci))
	assert.Equal(t, "id test container 1", ci[0].ContainerId)
	assert.Equal(t, int32(2000), ci[0].Resources.RequestedCpus)
	assert.Equal(t, int32(3000), ci[0].Resources.LimitCpus)
	assert.Equal(t, int32(32*1024*1024), ci[0].Resources.RequestedMemory)
	assert.Equal(t, int32(64*1024*1024), ci[0].Resources.LimitMemory)
	assert.Equal(t, int32(3000), ci[1].Resources.RequestedCpus)
	assert.Equal(t, int32(4000), ci[1].Resources.LimitCpus)
	assert.Equal(t, int32(24*1024*1024), ci[1].Resources.RequestedMemory)
	assert.Equal(t, int32(48*1024*1024), ci[1].Resources.LimitMemory)
}

func TestGetCreatePodRequest(t *testing.T) {
	pod := genTestPod()
	pR, err := GetCreatePodRequest(&pod)
	require.Nil(t, err)
	assert.Equal(t, "123", pR.PodId)
	assertResourcesEqualWithTestPod(t, pR.Resources)
	assertContainersEqualWithTestPod(t, pR.Containers)
}

func TestGetUpdatePodRequest(t *testing.T) {
	pod := genTestPod()
	pR, err := GetUpdatePodRequest(&pod)
	require.Nil(t, err)
	assert.Equal(t, "123", pR.PodId)
	assertResourcesEqualWithTestPod(t, pR.Resources)
	assertContainersEqualWithTestPod(t, pR.Containers)
}

func TestGetDeletePodRequest(t *testing.T) {
	pod := genTestPod()
	pR := GetDeletePodRequest(&pod)
	assert.Equal(t, string(pod.GetUID()), pR.PodId)
}

func TestResourceCountingOverflow(t *testing.T) {
	limits := [][]int{{1, 1, 1, 1}, {math.MaxInt32, 1, 1, 1}}

	for i := 0; i < 4; i++ { // for each shift of limit indicies
		specs := []resourceSpec{}
		for _, spec := range limits {
			specs = append(specs, resourceSpec{
				reqCpu: strconv.Itoa(spec[(i+0)%4]),
				reqMem: strconv.Itoa(spec[(i+1)%4]),
				limCpu: strconv.Itoa(spec[(i+2)%4]),
				limMem: strconv.Itoa(spec[(i+3)%4]),
			})
		}
		t.Run(fmt.Sprintf("Shift %d", i), func(t *testing.T) {
			pod := genPodFromSpec(specs)
			_, err := GetCreatePodRequest(&pod)
			assert.ErrorIs(t, err, ErrCountingOverflow)
		})
	}
}
