package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"github.com/stretchr/testify/require"
)

func Test20_InstancesDNS(t *testing.T) {
	t.Cleanup(cleanup)
	test := &testInstancesDNS{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("create instance", test.testCreateInstance)
	t.Run("test dns", test.testInstancesDNS)
	t.Run("cleanup dns", test.testCleanupDNS)
	t.Run("terminate instance", test.testInstancesTerminate)
	t.Run("cleanup dns", test.testCleanupDNS)
	t.Run("end inventory empty", testInventoryEmpty)
}

func (d *testInstancesDNS) testCreateInstance(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	placement := Options.TestRegions[0] + "a"
	if strings.Count(Options.TestRegions[0], "-") == 1 {
		placement = Options.TestRegions[0] + "-a"
	}
	params := map[backends.BackendType]interface{}{
		backends.BackendTypeAWS: &baws.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: Options.TestRegions[0] + "a",
			InstanceType:     "r6a.large",
			Disks:            []string{"type=gp2,size=20,count=1"},
			Firewalls:        []string{},
			CustomDNS: &backends.InstanceDNS{
				DomainID:   "Z08885863MUP8ENZ1K1Z7",
				DomainName: "aerospike.me",
				Region:     "us-east-1",
			},
		},
		backends.BackendTypeGCP: &bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
			CustomDNS: &backends.InstanceDNS{
				DomainID:   "aerospikeme",
				DomainName: "aerospike.me",
				Region:     "global",
			},
		},
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:           "test-cluster",
		Nodes:                 3,
		BackendType:           backendType,
		Owner:                 "test-owner",
		Description:           "test-description",
		BackendSpecificParams: params,
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 3)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 3)
}

func (d *testInstancesDNS) testInstancesDNS(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	domainId := "Z08885863MUP8ENZ1K1Z7"
	if cloud == "gcp" {
		domainId = "aerospikeme"
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, inst.Count(), 3)
	for _, i := range inst.Describe() {
		require.Equal(t, i.CustomDNS.DomainID, domainId)
		require.Equal(t, i.CustomDNS.DomainName, "aerospike.me")
		require.Equal(t, i.CustomDNS.Region, "us-east-1")
		require.Equal(t, i.CustomDNS.Name, i.InstanceID)
	}
}

func (d *testInstancesDNS) testInstancesTerminate(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	err := inst.Terminate(2 * time.Minute)
	require.NoError(t, err)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 0)
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(10*time.Minute))
}

func (d *testInstancesDNS) testCleanupDNS(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support dns")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	err := testBackend.CleanupDNS()
	require.NoError(t, err)
}
