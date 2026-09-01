package backendtest_test

import (
	"testing"
	"time"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/backendtest"
)

// Every command a user runs against a node funnels through one of these, so
// each has to give the caller a way in before it tries to connect. Losing one
// of them means a user whose address moved, or who is working on somebody
// else's cluster, hangs on a connection that will never be accepted.
func TestNodeTouchingActionsEnsureCallerAccess(t *testing.T) {
	bt := backends.BackendType("faketest-calleraccess")
	cases := []struct {
		action string
		run    func(backends.InstanceList)
	}{
		{"Exec", func(l backends.InstanceList) { l.Exec(&backends.ExecInput{}) }},
		{"Start", func(l backends.InstanceList) { l.Start(time.Minute) }},            //nolint:errcheck
		{"GetSftpConfig", func(l backends.InstanceList) { l.GetSftpConfig("root") }}, //nolint:errcheck
		{"GetSSHKeyPath", func(l backends.InstanceList) { l.GetSSHKeyPath() }},
	}
	for _, c := range cases {
		t.Run(c.action, func(t *testing.T) {
			fc := &backendtest.FakeCloud{}
			backendtest.RegisterFakeCloud(t, bt, fc)
			cluster := backendtest.NewCluster("mydc", 2, backendtest.WithBackendType(bt))

			c.run(cluster)

			if fc.CallCount("EnsureCallerAccess") != 1 {
				t.Fatalf("%s asked for caller access %d times, want once", c.action, fc.CallCount("EnsureCallerAccess"))
			}
			if len(fc.CallerAccessInstances) != 2 {
				t.Fatalf("%s asked for caller access to %d instances, want all 2", c.action, len(fc.CallerAccessInstances))
			}
		})
	}
}

// Actions which never open a connection have no reason to touch firewalls, and
// paying for an address lookup and a firewall read on every inventory listing
// would be felt.
func TestNonConnectingActionsLeaveFirewallsAlone(t *testing.T) {
	bt := backends.BackendType("faketest-nocalleraccess")
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, bt, fc)
	cluster := backendtest.NewCluster("mydc", 2, backendtest.WithBackendType(bt))

	if err := cluster.Stop(false, time.Minute); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := cluster.Terminate(time.Minute); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if fc.CallCount("EnsureCallerAccess") != 0 {
		t.Fatalf("stopping and terminating asked for caller access %d times, want none", fc.CallCount("EnsureCallerAccess"))
	}
}

// An empty selection must not reach the cloud at all: 'aerolab attach shell'
// against a cluster which does not exist should fail on the missing cluster,
// not create a firewall as a side effect.
func TestEmptyInstanceListSkipsCallerAccess(t *testing.T) {
	bt := backends.BackendType("faketest-emptycalleraccess")
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, bt, fc)

	backends.InstanceList{}.Exec(&backends.ExecInput{})

	if fc.CallCount("EnsureCallerAccess") != 0 {
		t.Fatalf("an empty instance list asked for caller access %d times, want none", fc.CallCount("EnsureCallerAccess"))
	}
}
