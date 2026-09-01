package backendtest

import (
	"testing"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
)

// RegisterFakeCloud registers c as the Cloud implementation for backend type bt
// in the process-global backend registry, and arranges (via t.Cleanup) to
// restore the previous registration when the test finishes.
//
// Because the backend registry is a process-global map, tests that call this
// helper MUST NOT use t.Parallel(): concurrent registration/deregistration
// would race and cross-contaminate. Inventory list actions such as
// InstanceList.Exec / Terminate dispatch through this global registry (keyed by
// each instance's BackendType), so a fake must be registered here for those
// actions to reach it.
func RegisterFakeCloud(t *testing.T, bt backends.BackendType, c backends.Cloud) {
	t.Helper()
	prev, had := backends.LookupBackend(bt)
	backends.RegisterBackend(bt, c)
	t.Cleanup(func() {
		if had {
			backends.RegisterBackend(bt, prev)
		} else {
			backends.UnregisterBackend(bt)
		}
	})
}
