package cpudaemon

import (
	"os"
	"path"
	"testing"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewState(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	s, err := newState("testdata/no_state", "", "testdata/node_info", string(daemonStateFile))
	assert.Nil(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.Allocated)
	assert.Equal(t, len(s.Allocated), 0)
	assert.Equal(t, len(s.AvailableCPUs), 1)
}

func TestThrowLoadState(t *testing.T) {
	d := DaemonState{
		StatePath: "daemon_cpuset_not_exist.state",
	}
	err := d.LoadState()
	assert.Equal(t, err != nil, true)
}

func TestMissingCGroup(t *testing.T) {
	daemonStateFile, tearDown := setupTest()
	defer tearDown(t)
	s, err := newState("testdata/no_cgroup", "", "testdata/node_info", daemonStateFile)
	assert.NotNil(t, err)
	assert.Nil(t, s)
	assert.IsType(t, DaemonError{}, err)
	assert.Equal(t, MissingCgroup, err.(DaemonError).ErrorType) //nolint: errorlint
}

func TestSaveAndLoadDaemonState(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test")
	require.Nil(t, err)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	expectedState := DaemonState{
		StatePath: tempFile.Name(),
	}
	expectedState.AvailableCPUs = []ctlplaneapi.CPUBucket{
		{
			StartCPU: 0,
			EndCPU:   127,
		},
	}

	savedState := DaemonState{
		StatePath: tempFile.Name(),
	}
	savedState.AvailableCPUs = expectedState.AvailableCPUs
	require.Nil(t, savedState.SaveState())

	loadedState := DaemonState{
		StatePath: tempFile.Name(),
	}
	require.Nil(t, loadedState.LoadState())

	assert.Equal(t, expectedState, loadedState)
}

func TestDoNotLoadDaemonStateIfSymlink(t *testing.T) {
	dir := t.TempDir()

	sourcePath := path.Join(dir, "data.json")
	symlinkPath := path.Join(dir, "symlink.json")

	require.Nil(t, os.Symlink(sourcePath, symlinkPath))

	state := DaemonState{
		StatePath: symlinkPath,
	}

	require.ErrorIs(t, state.LoadState(), utils.ErrFileIsSymlink)
}
