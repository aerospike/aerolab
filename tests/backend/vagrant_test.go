//go:build integration_vagrant

package backend_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/rglonek/logger"
	"github.com/stretchr/testify/require"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bvagrant"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

// Test10_Vagrant exercises the vagrant backend end to end against a real, local
// vagrant + VirtualBox install: create 2 instances, confirm running, exec, stop,
// start, terminate, and confirm no leftover cluster state on disk.
//
// This is gated behind three independent checks, all of which must pass before
// anything destructive (vagrant up, VM creation) runs:
//  1. AEROLAB_TEST_VAGRANT=1 is set (opt-in; this test is slow and mutates local
//     VirtualBox/vagrant state).
//  2. a "vagrant" binary is on PATH.
//  3. a usable provider is detected (VBoxManage on PATH is treated as sufficient
//     evidence of a working VirtualBox provider).
//
// Run with: AEROLAB_TEST_VAGRANT=1 go test -tags integration_vagrant ./tests/backend/ -run Vagrant -v
func Test10_Vagrant(t *testing.T) {
	if os.Getenv("AEROLAB_TEST_VAGRANT") != "1" {
		t.Skip("set AEROLAB_TEST_VAGRANT=1 to run vagrant backend integration tests")
	}
	if _, err := exec.LookPath("vagrant"); err != nil {
		t.Skip("vagrant binary not found on PATH")
	}
	if _, err := exec.LookPath("VBoxManage"); err != nil {
		t.Skip("VBoxManage not found on PATH (no usable vagrant provider detected)")
	}

	tempDir, err := os.MkdirTemp("", "aerolab-test-vagrant")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tempDir) //nolint:errcheck
	})

	credentials := &clouds.Credentials{
		VAGRANT: clouds.VAGRANT{
			DefaultProvider: "virtualbox",
		},
	}

	vagrantBackend, err := backend.New("aerolab-test-vagrant",
		&backend.Config{
			RootDir:         tempDir,
			Cache:           false,
			Credentials:     credentials,
			LogLevel:        logger.DETAIL,
			LogMillisecond:  true,
			AerolabVersion:  "v0.0.0",
			ListAllProjects: false,
		},
		false, []backends.BackendType{backends.BackendTypeVagrant}, nil)
	require.NoError(t, err)

	require.NoError(t, vagrantBackend.AddRegion(backends.BackendTypeVagrant, "local"))
	require.NoError(t, vagrantBackend.ForceRefreshInventory())

	clusterName := "test-cluster"
	t.Cleanup(func() {
		vagrantBackend.RefreshChangedInventory() //nolint:errcheck
		insts := vagrantBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
		if insts.Count() > 0 {
			insts.Terminate(10 * time.Minute) //nolint:errcheck
		}
	})

	t.Run("inventory empty", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		inv := vagrantBackend.GetInventory()
		require.Equal(t, 0, inv.Instances.WithNotState(backends.LifeCycleStateTerminated).Count())
	})

	var image *backends.Image
	t.Run("resolve image", func(t *testing.T) {
		imgs := vagrantBackend.GetInventory().Images
		candidates := imgs.WithInAccount(false).WithOSName("ubuntu").WithOSVersion("24.04").WithArchitecture(backends.ArchitectureX8664)
		require.NotEmpty(t, candidates.Describe())
		image = candidates.Describe()[0]
		require.NotNil(t, image)
	})

	t.Run("create 2 instances", func(t *testing.T) {
		params := map[backends.BackendType]any{
			backends.BackendTypeVagrant: &bvagrant.CreateInstanceParams{
				Image: image,
			},
		}
		insts, err := vagrantBackend.CreateInstances(&backends.CreateInstanceInput{
			ClusterName:           clusterName,
			Nodes:                 2,
			BackendType:           backends.BackendTypeVagrant,
			Owner:                 "test-owner",
			Description:           "test-description",
			BackendSpecificParams: params,
		}, 10*time.Minute)
		require.NoError(t, err)
		require.Equal(t, 2, insts.Instances.Count())
	})

	t.Run("instances running", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		insts := vagrantBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
		require.Equal(t, 2, insts.Count())
	})

	t.Run("exec ls /", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		insts := vagrantBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
		require.Equal(t, 2, insts.Count())
		outs := insts.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"ls", "/"},
				SessionTimeout: 30 * time.Second,
				Terminal:       true,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 2,
		})
		require.Equal(t, 2, len(outs))
		for _, out := range outs {
			require.NotNil(t, out.Output)
			require.NoError(t, out.Output.Err)
		}
	})

	t.Run("stop", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		insts := vagrantBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
		require.Equal(t, 2, insts.Count())
		require.NoError(t, insts.Stop(false, 5*time.Minute))
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		require.Equal(t, 0, vagrantBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning).Count())
	})

	t.Run("start", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		insts := vagrantBackend.GetInventory().Instances.WithState(backends.LifeCycleStateStopped)
		require.Equal(t, 2, insts.Count())
		require.NoError(t, insts.Start(5*time.Minute))
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		require.Equal(t, 2, vagrantBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning).Count())
	})

	t.Run("terminate", func(t *testing.T) {
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		insts := vagrantBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
		require.Equal(t, 2, insts.Count())
		require.NoError(t, insts.Terminate(10*time.Minute))
		require.NoError(t, vagrantBackend.RefreshChangedInventory())
		require.Equal(t, 0, vagrantBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count())
	})

	t.Run("no leftover cluster dir", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(tempDir, "clusters", clusterName))
		require.True(t, os.IsNotExist(err), "expected cluster dir to be removed after terminate, got err=%v", err)
	})
}
