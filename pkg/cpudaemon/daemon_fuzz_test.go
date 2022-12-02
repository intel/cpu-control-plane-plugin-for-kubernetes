package cpudaemon

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

func updatePodRequestFromCreate(base *ctlplaneapi.CreatePodRequest, numDeletes, numUpdates uint) *ctlplaneapi.UpdatePodRequest {
	req := ctlplaneapi.UpdatePodRequest{
		PodId:     base.PodId,
		Resources: &ctlplaneapi.ResourceInfo{},
	}
	for i, container := range base.Containers {
		c := ctlplaneapi.ContainerInfo{
			ContainerId:   container.ContainerId,
			ContainerName: container.ContainerName,
			Resources: &ctlplaneapi.ResourceInfo{
				RequestedCpus:   container.Resources.RequestedCpus,
				LimitCpus:       container.Resources.LimitCpus,
				RequestedMemory: container.Resources.RequestedMemory,
				LimitMemory:     container.Resources.LimitMemory,
			},
		}
		iu := uint(i)
		if iu < numDeletes {
			continue
		}
		if iu < numDeletes+numUpdates {
			c.Resources.RequestedCpus += 1
			c.Resources.LimitCpus += 1
		}
		req.Containers = append(req.Containers, &c)
		req.Resources.LimitCpus += c.Resources.LimitCpus
		req.Resources.LimitMemory += c.Resources.LimitMemory
		req.Resources.RequestedCpus += c.Resources.RequestedCpus
		req.Resources.RequestedMemory += c.Resources.RequestedMemory
	}
	return &req
}

func createPodRequestForFuzzing(
	pid, podName, namespace, cid, containerName string,
	numContainers uint,
	reqCpu, limCpu, reqMem, limMem int32,
) *ctlplaneapi.CreatePodRequest {
	numContainers32 := int32(numContainers)
	req := ctlplaneapi.CreatePodRequest{
		PodId:        pid,
		PodName:      podName,
		PodNamespace: namespace,
		Resources: &ctlplaneapi.ResourceInfo{
			RequestedCpus:   reqCpu * numContainers32,
			LimitCpus:       limCpu * numContainers32,
			RequestedMemory: reqMem * numContainers32,
			LimitMemory:     limMem * numContainers32,
		},
		Containers: []*ctlplaneapi.ContainerInfo{},
	}
	for i := uint(0); i < numContainers; i++ {
		req.Containers = append(req.Containers, &ctlplaneapi.ContainerInfo{
			ContainerId:   fmt.Sprintf("%s-%d", cid, i),
			ContainerName: fmt.Sprintf("%s-name-%d", containerName, i),
			Resources: &ctlplaneapi.ResourceInfo{
				RequestedCpus:   reqCpu,
				LimitCpus:       limCpu,
				RequestedMemory: reqMem,
				LimitMemory:     limMem,
			},
		})
	}
	return &req
}

func FuzzCreatePod(f *testing.F) {
	f.Fuzz(func(t *testing.T, pid, podName, namespace, cid, containerName string, numContainers uint, reqCpu, limCpu, reqMem, limMem int32) {
		numContainers %= 100
		dir := t.TempDir()
		daemonStateFile := path.Join(dir, "daemon.state")
		defer os.Remove(daemonStateFile)

		m := MockedPolicy{}
		d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
		require.Nil(t, err)

		m.On("AssignContainer", mock.Anything, &d.state).Return(nil).Run(func(args mock.Arguments) {
			c := args.Get(0).(Container)
			require.Equal(t, int(reqCpu), c.Cpus)
			if !strings.HasPrefix(c.CID, cid) {
				require.Fail(t, "CID does not have proper prefix", "cid", c.CID, "prefix", cid)
			}
			if !strings.HasPrefix(c.PID, pid) {
				require.Fail(t, "PID does not have proper prefix", "pid", c.PID, "prefix", pid)
			}
			if !strings.HasPrefix(c.Name, containerName) {
				require.Fail(t, "container name does not have proper prefix", "name", c.Name, "prefix", containerName)
			}
		})

		req := createPodRequestForFuzzing(pid, podName, namespace, cid, containerName, numContainers, reqCpu, limCpu, reqMem, limMem)

		resp, err := d.CreatePod(req)

		if err != nil {
			derr := DaemonError{}
			if !errors.As(err, &derr) {
				t.Fatal("Error is not of type DaemonError")
			}
			if derr.ErrorType != PodSpecError {
				t.Fatal("Error is of different type than PodSpecError")
			}
		} else {
			require.Equal(t, numContainers, uint(len(resp.ContainerResources)))
			m.AssertNumberOfCalls(t, "AssignContainer", int(numContainers))
		}
	})
}

func FuzzDeletePod(f *testing.F) {
	f.Fuzz(func(t *testing.T, pid string, podInState bool) {
		dir := t.TempDir()
		daemonStateFile := path.Join(dir, "daemon.state")
		defer os.Remove(daemonStateFile)

		m := MockedPolicy{}
		d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
		require.Nil(t, err)

		if pid != "" && podInState {
			d.state.Pods[pid] = PodMetadata{
				PID:       "pid",
				Name:      "name",
				Namespace: "namespace",
				Containers: []Container{{
					CID:  "cid",
					PID:  "pid",
					Name: "name",
					Cpus: 3,
				}},
			}
			m.On("DeleteContainer", d.state.Pods[pid].Containers[0], &d.state).Return(nil).Once()
		}

		req := ctlplaneapi.DeletePodRequest{PodId: pid}
		err = d.DeletePod(&req)

		if err != nil {
			derr := DaemonError{}
			if !errors.As(err, &derr) {
				t.Fatal("Error is not of type DaemonError")
			}
			if derr.ErrorType == PodSpecError {
				return
			}
			if podInState {
				t.Fatal("Pod is in state and error is of different type than PodSpecError")
			}
		}
	})
}

func FuzzUpdatePod(f *testing.F) {
	f.Fuzz(func(
		t *testing.T, pid, podName, namespace, cid, containerName string, numContainers uint,
		reqCpu, limCpu, reqMem, limMem int32, numDel uint, numUpdate uint,
	) {
		numContainers %= 100
		numDel %= 10
		numUpdate %= 10

		if numDel+numUpdate > numContainers || numDel == numContainers {
			return
		}

		dir := t.TempDir()
		daemonStateFile := path.Join(dir, "daemon.state")
		defer os.Remove(daemonStateFile)

		m := MockedPolicy{}

		d, err := New("testdata/no_state", "testdata/node_info", daemonStateFile, &m, logr.Discard())
		require.Nil(t, err)

		m.On("DeleteContainer", mock.Anything, &d.state).Return(nil)
		m.On("AssignContainer", mock.Anything, &d.state).Return(nil).Run(func(args mock.Arguments) {
			c := args.Get(0).(Container)
			rc := int(reqCpu)
			if c.Cpus != rc && c.Cpus != rc+1 {
				require.Fail(t, "Wrong number of CPUs", "cpus", c.Cpus, "expected", []int{rc, rc + 1})
			}
			if !strings.HasPrefix(c.CID, cid) {
				require.Fail(t, "CID does not have proper prefix", "cid", c.CID, "prefix", cid)
			}
			if !strings.HasPrefix(c.PID, pid) {
				require.Fail(t, "PID does not have proper prefix", "pid", c.PID, "prefix", pid)
			}
			if !strings.HasPrefix(c.Name, containerName) {
				require.Fail(t, "container name does not have proper prefix", "name", c.Name, "prefix", containerName)
			}
		})

		req := createPodRequestForFuzzing(pid, podName, namespace, cid, containerName, numContainers, reqCpu, limCpu, reqMem, limMem)
		_, err = d.CreatePod(req)

		// We add pod and want to continue only if it was added successfully
		if err != nil {
			return
		}

		reqUpdate := updatePodRequestFromCreate(req, numDel, numUpdate)
		t.Log(reqUpdate)
		resp, err := d.UpdatePod(reqUpdate)

		require.Nil(t, err)
		require.Equal(t, numUpdate, uint(len(resp.ContainerResources)))
		m.AssertNumberOfCalls(t, "DeleteContainer", int(numDel+numUpdate))
		m.AssertNumberOfCalls(t, "AssignContainer", int(numContainers+numUpdate))
	})
}
