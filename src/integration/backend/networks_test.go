package backend_test

import (
	"testing"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/stretchr/testify/require"
)

type networkTest struct{}

func Test00_Networks(t *testing.T) {
	t.Cleanup(cleanup)
	networkTest := &networkTest{}
	t.Run("setup", testSetup)
	t.Run("list networks", networkTest.testListNetworks)
}

func (n *networkTest) testListNetworks(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	nets := testBackend.GetInventory().Networks.WithAerolabManaged(false)
	require.Equal(t, nets.Count(), 1)
	subs := nets.Subnets()
	subCount := 1
	if backendType == backends.BackendTypeAWS {
		subCount = 3
	}
	require.Equal(t, len(subs), subCount)
	subs = subs.WithDefault(true)
	subs = subs.WithAerolabManaged(false)
	if backendType == backends.BackendTypeAWS {
		subs = subs.WithZoneID("ca-central-1a")
	}
	require.Equal(t, len(subs), 1)
}
