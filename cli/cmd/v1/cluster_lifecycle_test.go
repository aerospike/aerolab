package cmd

import (
	"strings"
	"testing"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/backendtest"
	"github.com/stretchr/testify/require"
)

// runningServerCluster builds an inventory of `count` running, server-typed
// Docker instances for the given cluster name.
func runningServerCluster(name string, count int) backends.InstanceList {
	return backendtest.NewCluster(name, count,
		backendtest.WithBackendType(backends.BackendTypeDocker),
		backendtest.WithAerolabType("server"),
		backendtest.WithState(backends.LifeCycleStateRunning),
	)
}

func stoppedServerCluster(name string, count int) backends.InstanceList {
	return backendtest.NewCluster(name, count,
		backendtest.WithBackendType(backends.BackendTypeDocker),
		backendtest.WithAerolabType("server"),
		backendtest.WithState(backends.LifeCycleStateStopped),
	)
}

func newStopHarness(t *testing.T, instances backends.InstanceList) (*System, *backends.Inventory, *backendtest.FakeCloud) {
	t.Helper()
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, backends.BackendTypeDocker, fc)
	inv := backendtest.NewInventory(instances)
	fb := backendtest.NewFakeBackend(inv)
	sys := &System{Logger: backendtest.QuietLogger(), Backend: fb}
	return sys, inv, fc
}

func TestStopClusterAllNodes(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 3))
	c := &ClusterStopCmd{ClusterName: "mydc"}

	got, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.NoError(t, err)
	require.Equal(t, 3, got.Count())
	require.Equal(t, 1, fc.CallCount("InstancesStop"))
	require.Len(t, fc.StoppedInstances, 3)
}

func TestStopClusterNodeFilter(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 3))
	c := &ClusterStopCmd{ClusterName: "mydc", Nodes: "1,2"}

	got, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.NoError(t, err)
	require.Equal(t, 2, got.Count())
	require.Len(t, fc.StoppedInstances, 2)
}

func TestStopClusterUnknownNodeErrors(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 3))
	c := &ClusterStopCmd{ClusterName: "mydc", Nodes: "1,5"}

	_, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.Equal(t, 0, fc.CallCount("InstancesStop"))
}

func TestStopClusterEmptyNameErrors(t *testing.T) {
	sys, inv, _ := newStopHarness(t, runningServerCluster("mydc", 1))
	c := &ClusterStopCmd{ClusterName: ""}

	_, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cluster name is required")
}

func TestStopClusterMissingClusterErrors(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 2))
	c := &ClusterStopCmd{ClusterName: "does-not-exist"}

	_, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.Error(t, err)
	require.Equal(t, 0, fc.CallCount("InstancesStop"))
}

func TestStopClusterMultiClusterFanOut(t *testing.T) {
	instances := append(runningServerCluster("mydc", 2), runningServerCluster("other", 3)...)
	sys, inv, fc := newStopHarness(t, instances)
	c := &ClusterStopCmd{ClusterName: "mydc,other"}

	got, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.NoError(t, err)
	require.Equal(t, 5, got.Count())
	// One dispatch per cluster in the fan-out.
	require.Equal(t, 2, fc.CallCount("InstancesStop"))
	require.Len(t, fc.StoppedInstances, 5)
}

func TestStopClusterMultiClusterValidatesBeforeActing(t *testing.T) {
	// "other" does not exist: the up-front validation loop must reject the whole
	// request before stopping any node of "mydc".
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 2))
	c := &ClusterStopCmd{ClusterName: "mydc,other"}

	_, err := c.StopCluster(sys, inv, sys.Logger, nil, "stop")
	require.Error(t, err)
	require.Equal(t, 0, fc.CallCount("InstancesStop"), "must not stop any node when one cluster is invalid")
}

func TestStartClusterAllNodes(t *testing.T) {
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, backends.BackendTypeDocker, fc)
	inv := backendtest.NewInventory(stoppedServerCluster("mydc", 3))
	fb := backendtest.NewFakeBackend(inv)
	sys := &System{Logger: backendtest.QuietLogger(), Backend: fb}

	// NoFixMesh + NoStart avoid the SSH/systemctl steps so the test stays hermetic.
	c := &ClusterStartCmd{ClusterName: "mydc", NoFixMesh: true, NoStart: true, Threads: 1}
	got, err := c.StartCluster(sys, inv, sys.Logger, nil, "start")
	require.NoError(t, err)
	require.Equal(t, 3, got.Count())
	require.Equal(t, 1, fc.CallCount("InstancesStart"))
	require.Len(t, fc.StartedInstances, 3)
	require.Equal(t, 1, fb.CallCount("RefreshChangedInventory"))
}

func TestDestroyClusterForce(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 3))
	c := &ClusterDestroyCmd{ClusterName: "mydc", Force: true}

	got, err := c.DestroyCluster(sys, inv, sys.Logger, nil, "destroy")
	require.NoError(t, err)
	require.Equal(t, 3, got.Count())
	require.Equal(t, 1, fc.CallCount("InstancesTerminate"))
	require.Len(t, fc.TerminatedInstances, 3)
}

func TestDestroyClusterNodeFilterForce(t *testing.T) {
	sys, inv, fc := newStopHarness(t, runningServerCluster("mydc", 3))
	c := &ClusterDestroyCmd{ClusterName: "mydc", Nodes: "2", Force: true}

	got, err := c.DestroyCluster(sys, inv, sys.Logger, nil, "destroy")
	require.NoError(t, err)
	require.Equal(t, 1, got.Count())
	require.Len(t, fc.TerminatedInstances, 1)
	require.Equal(t, 2, fc.TerminatedInstances[0].NodeNo)
}

func TestStartClusterNoStoppedNodesErrors(t *testing.T) {
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, backends.BackendTypeDocker, fc)
	// All nodes already running -> nothing to start.
	inv := backendtest.NewInventory(runningServerCluster("mydc", 2))
	fb := backendtest.NewFakeBackend(inv)
	sys := &System{Logger: backendtest.QuietLogger(), Backend: fb}

	c := &ClusterStartCmd{ClusterName: "mydc", NoFixMesh: true, NoStart: true, Threads: 1}
	_, err := c.StartCluster(sys, inv, sys.Logger, nil, "start")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "no nodes to start") || strings.Contains(err.Error(), "not found"),
		"unexpected error: %v", err)
	require.Equal(t, 0, fc.CallCount("InstancesStart"))
}
