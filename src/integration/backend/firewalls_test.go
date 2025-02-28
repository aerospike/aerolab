package backend_test

import (
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/stretchr/testify/require"
)

func Test04_Firewalls(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	fw := &fwTest{}
	fw.findNetwork(t)
	t.Run("inventory empty", testInventoryEmpty)                        // ensure inventory is empty
	t.Run("create firewall", fw.testCreateFirewall)                     // create a new firewall
	t.Run("update firewall", fw.testUpdateFirewall)                     // update firewall settings
	t.Run("add tags", fw.testAddTagsFirewall)                           // add tags to the firewall
	t.Run("remove tags", fw.testRemoveTagsFirewall)                     // remove tags from the firewall
	t.Run("create test instance", fw.testCreateTestInstanceForFirewall) // create a test instance with the new firewall attached
	t.Run("remove firewall", fw.testRemoveFirewallFromInstance)         // remove the new firewall from the test instance
	t.Run("assign firewall", fw.testAssignFirewallToInstance)           // assign the new firewall to the test instance again
	t.Run("delete test instance", fw.testDeleteTestInstanceForFirewall) // delete the test instance
	t.Run("delete firewall", fw.testDeleteFirewall)                     // delete the firewall
	t.Run("end inventory empty", testInventoryEmpty)                    // ensure inventory is empty again
}

type fwTest struct {
	network *backends.Network
}

func (fw *fwTest) findNetwork(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	network := testBackend.GetInventory().Networks.WithDefault(true).WithAerolabManaged(false).Describe()
	require.NotEmpty(t, network)
	fw.network = network[0]
}

func (fw *fwTest) testCreateFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	_, err := testBackend.CreateFirewall(
		&backends.CreateFirewallInput{
			BackendType: backends.BackendTypeAWS,
			Name:        "test-firewall",
			Description: "test-firewall-description",
			Owner:       "test-owner",
			Ports: []*backends.Port{
				{
					FromPort:   80,
					ToPort:     80,
					SourceCidr: "0.0.0.0/0",
					Protocol:   backends.ProtocolTCP,
				},
			},
			Network: fw.network,
		},
		time.Minute*10,
	)
	require.NoError(t, err)
}

func (fw *fwTest) testUpdateFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	f := testBackend.GetInventory().Firewalls.WithName("test-firewall").Describe()
	require.NotEmpty(t, f)
	require.Equal(t, len(f), 1)
	err := f.Update(backends.PortsIn{
		{
			Port: backends.Port{
				FromPort:   80,
				ToPort:     80,
				SourceCidr: "0.0.0.0/0",
				Protocol:   backends.ProtocolTCP,
			},
			Action: backends.PortActionDelete,
		},
	}, time.Minute*10)
	require.NoError(t, err)

	require.NoError(t, testBackend.RefreshChangedInventory())

	f = testBackend.GetInventory().Firewalls.WithName("test-firewall").Describe()
	require.NotEmpty(t, f)
	require.Equal(t, len(f), 1)
	err = f.Update(backends.PortsIn{
		{
			Port: backends.Port{
				FromPort:   443,
				ToPort:     443,
				SourceCidr: "0.0.0.0/0",
				Protocol:   backends.ProtocolTCP,
			},
			Action: backends.PortActionAdd,
		},
	}, time.Minute*10)
	require.NoError(t, err)

	require.NoError(t, testBackend.RefreshChangedInventory())

	f = testBackend.GetInventory().Firewalls.WithName("test-firewall").Describe()
	require.NotEmpty(t, f)
	require.Equal(t, len(f), 1)
	require.Equal(t, len(f[0].Ports), 1)
	require.Equal(t, f[0].Ports[0].FromPort, 443)
	require.Equal(t, f[0].Ports[0].ToPort, 443)
	require.Equal(t, f[0].Ports[0].SourceCidr, "0.0.0.0/0")
	require.Equal(t, f[0].Ports[0].Protocol, backends.ProtocolTCP)
}

func (fw *fwTest) testAddTagsFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	f := testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)
	require.NoError(t, f.AddTags(map[string]string{"test-tag": "test-value"}, time.Minute*10))
	require.NoError(t, testBackend.RefreshChangedInventory())
	f = testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)
	require.Contains(t, f.Describe()[0].Tags, "test-tag")
	require.Equal(t, f.Describe()[0].Tags["test-tag"], "test-value")
}

func (fw *fwTest) testRemoveTagsFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	f := testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)
	require.NoError(t, f.RemoveTags([]string{"test-tag"}, time.Minute*10))
	require.NoError(t, testBackend.RefreshChangedInventory())
	f = testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)
	require.NotContains(t, f.Describe()[0].Tags, "test-tag")
}

func (fw *fwTest) testCreateTestInstanceForFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: fw.network.NetworkId,
		Firewalls:        []string{"test-firewall"},
		BackendType:      backends.BackendTypeAWS,
		InstanceType:     "r6a.large",
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            []string{"type=gp2,size=20,count=2"},
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 1)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	require.Len(t, inst.Describe()[0].Firewalls, 2)
}

func (fw *fwTest) testRemoveFirewallFromInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	// get firewall
	f := testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)

	// get instance
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)

	// remove firewall from instance
	require.NoError(t, inst.RemoveFirewalls(f.Describe()))

	// refresh inventory
	require.NoError(t, testBackend.RefreshChangedInventory())

	// get instance and confirm firewall removed
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	require.NotContains(t, inst.Describe()[0].Firewalls, "test-firewall")
}

func (fw *fwTest) testAssignFirewallToInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	// get firewall
	f := testBackend.GetInventory().Firewalls.WithName("test-firewall")
	require.Equal(t, f.Count(), 1)

	// get instance
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)

	// assign firewall to instance
	require.NoError(t, inst.AssignFirewalls(f.Describe()))

	// refresh inventory
	require.NoError(t, testBackend.RefreshChangedInventory())

	// get instance and confirm firewall added
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	require.Len(t, inst.Describe()[0].Firewalls, 2)
}

func (fw *fwTest) testDeleteTestInstanceForFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.NoError(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance").Terminate(time.Minute*10))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance").Count(), 0)
}

func (fw *fwTest) testDeleteFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.NoError(t, testBackend.GetInventory().Firewalls.WithName("test-firewall").Delete(time.Minute*10))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.Equal(t, testBackend.GetInventory().Firewalls.WithName("test-firewall").Count(), 0)
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.Equal(t, testBackend.GetInventory().Firewalls.Count(), 1)
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(2*time.Minute))
}
