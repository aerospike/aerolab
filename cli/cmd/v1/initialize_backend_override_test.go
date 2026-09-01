package cmd

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/backend/backendtest"
	"github.com/stretchr/testify/require"
)

// TestInitializeUsesBackendOverride verifies the test-only Init.BackendOverride
// seam: when set, Initialize wires the injected backend straight through without
// constructing a real cloud backend.
func TestInitializeUsesBackendOverride(t *testing.T) {
	t.Setenv("AEROLAB_HOME", t.TempDir())
	t.Setenv("AEROLAB_TELEMETRY_DISABLE", "1")

	fb := backendtest.NewFakeBackend(backendtest.NewInventory(backendtest.NewCluster("mydc", 2)))

	sys, err := Initialize(&Init{
		InitBackend:     true,
		SkipArgsParsing: true,
		BackendOverride: fb,
	}, []string{"test", "backend-override"}, &Commands{})
	require.NoError(t, err)
	require.NotNil(t, sys)
	require.Same(t, fb, sys.Backend)
	require.Equal(t, 2, sys.Backend.GetInventory().Instances.Count())
}
