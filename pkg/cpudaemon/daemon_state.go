package cpudaemon

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/containerd/cgroups"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/numautils"
	"resourcemanagement.controlplane/pkg/utils"
)

const daemonFilePermission = 0600

// DaemonState struct holding the current daemon state.
type DaemonState struct {
	AvailableCPUs []ctlplaneapi.CPUBucket            // Used ony with default allocator
	Allocated     map[string][]ctlplaneapi.CPUBucket // Maps container id to allocated cpus
	Pods          map[string]PodMetadata             // Maps pod id to its metadata
	Topology      numautils.NumaTopology             // Used with numa and numa-namespace allocators
	CGroupPath    string                             // Path to cgroup main folder (usually /sys/fs/cgroup)
	StatePath     string                             // Path to state file where DaemonState is marshalled/unmarshalled
}

func newState(cgroupPath string, numaPath string, statePath string) (*DaemonState, error) {
	s := DaemonState{
		CGroupPath: cgroupPath,
		Allocated:  make(map[string][]ctlplaneapi.CPUBucket),
		Pods:       make(map[string]PodMetadata),
		StatePath:  statePath,
	}

	var (
		gCgroupPath     string
		gCpusetFilePath string
	)
	if cgroups.Mode() != cgroups.Unified {
		gCgroupPath = cgroupPath + "/cpuset"
		gCpusetFilePath = "cpuset.cpus"
	} else {
		gCgroupPath = cgroupPath
		gCpusetFilePath = "cpuset.cpus.effective"
	}
	c, err := getValues(gCgroupPath, gCpusetFilePath)

	if err == nil {
		s.AvailableCPUs = c
	} else {
		return nil, DaemonError{
			ErrorType:    MissingCgroup,
			ErrorMessage: err.Error(),
		}
	}

	err = s.Topology.Load(numaPath)

	if err != nil {
		return nil, DaemonError{
			ErrorType:    NotImplemented,
			ErrorMessage: err.Error(),
		}
	}
	_, errSt := os.Stat(statePath)
	if errSt != nil && errors.Is(errSt, os.ErrNotExist) {
		err = s.SaveState()
	} else {
		err = s.LoadState()
	}
	_ = errSt
	if err != nil {
		return nil, err
	}
	return &s, err
}

// SaveState saves state to file given in StatePath.
func (d *DaemonState) SaveState() error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	err = os.WriteFile(d.StatePath, b, daemonFilePermission)
	return err
}

// LoadState loads state from StatePath. StatePath value is always preserved.
func (d *DaemonState) LoadState() error {
	statePath := d.StatePath
	if err := utils.ErrorIfSymlink(statePath); err != nil {
		return err
	}
	b, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, d)
	d.StatePath = statePath // do not modify statePath, even if different (eg. state file was copied)
	return err
}

// DaemonStateFromReader loads the state of the daemon from a stream.
func DaemonStateFromReader(reader io.Reader) (DaemonState, error) {
	d := DaemonState{}
	b, err := io.ReadAll(reader)
	if err != nil {
		return DaemonState{}, err
	}
	err = json.Unmarshal(b, &d)
	return d, err
}
