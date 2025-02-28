package backend_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/stretchr/testify/require"
)

type testVolume struct{}

func TestVolumes(t *testing.T) {
	t.Cleanup(cleanup)
	tv := &testVolume{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("create attached volume get price", tv.testCreateAttachedVolumeGetPrice)
	t.Run("create shared volume get price", tv.testCreateSharedVolumeGetPrice)
	t.Run("create test instance", tv.testCreateTestInstance)
	t.Run("create attached volume", tv.testCreateAttachedVolume)
	t.Run("add tags to attached volume", tv.testAddTagsToAttachedVolume)
	t.Run("remove tags from attached volume", tv.testRemoveTagsFromAttachedVolume)
	t.Run("attach attached volume to instance", tv.testAttachAttachedVolumeToInstance)
	t.Run("resize attached volume", tv.testResizeAttachedVolume)
	t.Run("detach attached volume from instance", tv.testDetachAttachedVolumeFromInstance)
	t.Run("create shared volume", tv.testCreateSharedVolume)
	t.Run("add tags to shared volume", tv.testAddTagsToSharedVolume)
	t.Run("remove tags from shared volume", tv.testRemoveTagsFromSharedVolume)
	t.Run("attach shared volume to instance", tv.testAttachSharedVolumeToInstance)
	t.Run("detach shared volume from instance", tv.testDetachSharedVolumeFromInstance)
	t.Run("delete test instance", tv.testDeleteTestInstance)
	t.Run("delete attached volume", tv.testDeleteAttachedVolume)
	t.Run("delete shared volume", tv.testDeleteSharedVolume)
	t.Run("delete firewalls", tv.testDeleteFirewalls)
	t.Run("print inventory", tv.testPrintInventory)
	t.Run("end inventory empty", testInventoryEmpty)
}

func (tv *testVolume) testPrintInventory(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inv := testBackend.GetInventory()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err := enc.Encode(inv)
	require.NoError(t, err)
}
func (tv *testVolume) testDeleteFirewalls(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	fw := testBackend.GetInventory().Firewalls
	require.Equal(t, fw.Count(), 1)
	err := fw.Delete(10 * time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	fw = testBackend.GetInventory().Firewalls
	require.Equal(t, fw.Count(), 0)
}

func (tv *testVolume) testCreateAttachedVolumeGetPrice(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	price, err := testBackend.CreateVolumeGetPrice(&backends.CreateVolumeInput{
		BackendType:       backends.BackendTypeAWS,
		VolumeType:        backends.VolumeTypeAttachedDisk,
		Name:              "test-attached-volume",
		Description:       "test-description",
		SizeGiB:           10,
		Placement:         Options.TestRegions[0],
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Time{},
		DiskType:          "gp2",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.NotEqual(t, price, 0)
}

func (tv *testVolume) testCreateSharedVolumeGetPrice(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	price, err := testBackend.CreateVolumeGetPrice(&backends.CreateVolumeInput{
		BackendType:       backends.BackendTypeAWS,
		VolumeType:        backends.VolumeTypeSharedDisk,
		Name:              "test-shared-volume",
		Description:       "test-description",
		SizeGiB:           0,
		Placement:         Options.TestRegions[0],
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Time{},
		DiskType:          "",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.NotEqual(t, price, 0)
}

func (tv *testVolume) testCreateAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol, err := testBackend.CreateVolume(&backends.CreateVolumeInput{
		BackendType:       backends.BackendTypeAWS,
		VolumeType:        backends.VolumeTypeAttachedDisk,
		Name:              "test-attached-volume",
		Description:       "test-description",
		SizeGiB:           10,
		Placement:         Options.TestRegions[0],
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Time{},
		DiskType:          "gp2",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.Equal(t, vol.Volume.VolumeType, backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Volume.Size, 10*backends.StorageGiB)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vols := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vols.Count(), 1)
}

func (tv *testVolume) testCreateSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol, err := testBackend.CreateVolume(&backends.CreateVolumeInput{
		BackendType:       backends.BackendTypeAWS,
		VolumeType:        backends.VolumeTypeSharedDisk,
		Name:              "test-shared-volume",
		Description:       "test-description",
		SizeGiB:           0,
		Placement:         Options.TestRegions[0],
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Time{},
		DiskType:          "gp2",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.Equal(t, vol.Volume.VolumeType, backends.VolumeTypeSharedDisk)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vols := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vols.Count(), 1)
}

func (tv *testVolume) testAddTagsToAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.AddTags(map[string]string{"test-key": "test-value"}, 10*time.Second)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	require.Contains(t, vol.Describe()[0].Tags, "test-key")
	require.Equal(t, vol.Describe()[0].Tags["test-key"], "test-value")
}

func (tv *testVolume) testAddTagsToSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.AddTags(map[string]string{"test-key": "test-value"}, 10*time.Second)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	require.Contains(t, vol.Describe()[0].Tags, "test-key")
	require.Equal(t, vol.Describe()[0].Tags["test-key"], "test-value")
}

func (tv *testVolume) testRemoveTagsFromAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.RemoveTags([]string{"test-key"}, 10*time.Second)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	require.NotContains(t, vol.Describe()[0].Tags, "test-key")
}

func (tv *testVolume) testRemoveTagsFromSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.RemoveTags([]string{"test-key"}, 10*time.Second)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	require.NotContains(t, vol.Describe()[0].Tags, "test-key")
}

func (tv *testVolume) testCreateTestInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: Options.TestRegions[0] + "a",
		Firewalls:        []string{},
		BackendType:      backends.BackendTypeAWS,
		InstanceType:     "r6a.large",
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            []string{"type=gp2,size=20,count=1"},
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 1)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 1)
}

func (tv *testVolume) testAttachAttachedVolumeToInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.Attach(inst.Describe()[0], nil, 10*time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	require.Contains(t, vol.Describe()[0].AttachedTo, inst.Describe()[0].InstanceID)
}

func (tv *testVolume) testResizeAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.Resize(16, 1*time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	require.Equal(t, vol.Describe()[0].Size, 16*backends.StorageGiB)
}

func (tv *testVolume) testDetachAttachedVolumeFromInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.Detach(inst.Describe()[0], 10*time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	require.Empty(t, vol.Describe()[0].AttachedTo)
}

func (tv *testVolume) testAttachSharedVolumeToInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.Attach(inst.Describe()[0], &backends.VolumeAttachShared{
		MountTargetDirectory: "/mnt/shared",
		FIPS:                 false,
	}, 10*time.Minute)
	require.NoError(t, err)
}

func (tv *testVolume) testDetachSharedVolumeFromInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.Detach(inst.Describe()[0], 10*time.Minute)
	require.NoError(t, err)
}

func (tv *testVolume) testDeleteTestInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	err := inst.Terminate(10 * time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 0)
}

func (tv *testVolume) testDeleteAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.DeleteVolumes(testBackend.GetInventory().Firewalls.Describe(), 10*time.Minute)
	require.NoError(t, err)

	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 0)
}

func (tv *testVolume) testDeleteSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
	err := vol.DeleteVolumes(testBackend.GetInventory().Firewalls.Describe(), 10*time.Minute)
	require.NoError(t, err)

	require.NoError(t, testBackend.RefreshChangedInventory())
	vol = testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 0)
}
