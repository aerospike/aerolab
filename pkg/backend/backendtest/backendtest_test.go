package backendtest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/backendtest"
)

func TestFakeBackendServesSeededInventory(t *testing.T) {
	inv := backendtest.NewInventory(backendtest.NewCluster("mydc", 3))
	fb := backendtest.NewFakeBackend(inv)

	got := fb.GetInventory()
	if got.Instances.Count() != 3 {
		t.Fatalf("expected 3 instances, got %d", got.Instances.Count())
	}
	if got.Instances.WithClusterName("mydc").Count() != 3 {
		t.Fatalf("expected 3 in cluster mydc, got %d", got.Instances.WithClusterName("mydc").Count())
	}
	if got.Instances.WithNodeNo(1, 2).Count() != 2 {
		t.Fatalf("expected 2 for nodes 1,2, got %d", got.Instances.WithNodeNo(1, 2).Count())
	}
	if fb.CallCount("GetInventory") != 1 {
		t.Fatalf("expected GetInventory called once, got %d", fb.CallCount("GetInventory"))
	}
}

func TestFakeBackendReturnsConfiguredError(t *testing.T) {
	fb := backendtest.NewFakeBackend(nil)
	sentinel := errors.New("boom")
	fb.Errs = map[string]error{"ForceRefreshInventory": sentinel}
	if err := fb.ForceRefreshInventory(); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if err := fb.RefreshChangedInventory(); err != nil {
		t.Fatalf("expected nil for unset error, got %v", err)
	}
}

func TestFakeCloudExecAndListActions(t *testing.T) {
	fc := &backendtest.FakeCloud{}
	backendtest.RegisterFakeCloud(t, backends.BackendType("faketest"), fc)

	cluster := backendtest.NewCluster("mydc", 2, backendtest.WithBackendType(backends.BackendType("faketest")))

	// InstanceList.Exec dispatches through the global registry to our fake.
	outs := cluster.Exec(&backends.ExecInput{})
	if len(outs) != 2 {
		t.Fatalf("expected 2 exec outputs, got %d", len(outs))
	}
	if fc.CallCount("InstancesExec") != 1 {
		t.Fatalf("expected InstancesExec called once, got %d", fc.CallCount("InstancesExec"))
	}

	// Terminate dispatches to the fake and is captured.
	if err := cluster.Terminate(time.Minute); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if len(fc.TerminatedInstances) != 2 {
		t.Fatalf("expected 2 terminated instances captured, got %d", len(fc.TerminatedInstances))
	}
}

func TestRegisterFakeCloudRestoresRegistry(t *testing.T) {
	bt := backends.BackendType("faketest-restore")
	if _, ok := backends.LookupBackend(bt); ok {
		t.Fatalf("precondition: %s should not be registered", bt)
	}
	t.Run("inner", func(t *testing.T) {
		backendtest.RegisterFakeCloud(t, bt, &backendtest.FakeCloud{})
		if _, ok := backends.LookupBackend(bt); !ok {
			t.Fatalf("expected %s registered inside subtest", bt)
		}
	})
	if _, ok := backends.LookupBackend(bt); ok {
		t.Fatalf("expected %s unregistered after subtest cleanup", bt)
	}
}
