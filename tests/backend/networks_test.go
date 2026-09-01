//go:build integration_cloud

package backend_test

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/stretchr/testify/require"
)

type networkTest struct{}

func Test10_Networks(t *testing.T) {
	t.Cleanup(cleanup)
	networkTest := &networkTest{}
	t.Run("setup", testSetup)
	t.Run("list networks", networkTest.testListNetworks)
}

func (n *networkTest) testListNetworks(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	// Only docker's networks are fixed (bridge/host/none, one subnet). A cloud
	// account holds whatever VPCs and subnets its owner put there -- how many
	// is not aerolab's behaviour to assert, so the cloud backends only have to
	// surface something deployable.
	nets := testBackend.GetInventory().Networks.WithAerolabManaged(false)
	subs := nets.Subnets()
	if backendType == backends.BackendTypeDocker && !podman {
		require.Equal(t, nets.Count(), 3)
		require.Equal(t, len(subs), 1)
	} else {
		require.GreaterOrEqual(t, nets.Count(), 1)
		require.GreaterOrEqual(t, len(subs), 1)
	}
	subs = subs.WithDefault(true)
	subs = subs.WithAerolabManaged(false)
	if backendType == backends.BackendTypeAWS {
		subs = subs.WithZoneID(Options.TestRegions[0] + "a")
	}
	require.GreaterOrEqual(t, len(subs), 1)
}
