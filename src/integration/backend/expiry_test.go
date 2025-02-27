package backend_test

import (
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/stretchr/testify/require"
)

// TODO: test expire-eksctl and cleanup-dns

type expiryTest struct{}

func TestExpiry(t *testing.T) {
	t.Cleanup(cleanup)
	expiryTest := &expiryTest{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("expiry install", expiryTest.testExpiryInstall)
	t.Run("expiry change frequency", expiryTest.testExpiryChangeFrequency)
	t.Run("expiry change configuration", expiryTest.testExpiryChangeConfiguration)
	t.Run("expiry upgrade", expiryTest.testExpiryUpgrade)
	t.Run("create instance", expiryTest.testCreateInstance)
	t.Run("create attached volume", expiryTest.testCreateAttachedVolume)
	t.Run("create shared volume", expiryTest.testCreateSharedVolume)
	t.Run("wait for expiry", expiryTest.testWaitForExpiry)
	t.Run("expiry remove", expiryTest.testExpiryRemove)
	t.Run("end inventory empty", testInventoryEmpty)
}

func (e *expiryTest) testExpiryInstall(t *testing.T) {
	require.NoError(t, setup(false))
	err := testBackend.ExpiryInstall(backends.BackendTypeAWS, 10, 6, true, true, true, true, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backends.BackendTypeAWS)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 10)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 6)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, true)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
	}
}

func (e *expiryTest) testExpiryChangeFrequency(t *testing.T) {
	require.NoError(t, setup(false))
	err := testBackend.ExpiryChangeFrequency(backends.BackendTypeAWS, 1, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backends.BackendTypeAWS)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 1)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 6)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, true)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
	}
}

func (e *expiryTest) testExpiryUpgrade(t *testing.T) {
	require.NoError(t, setup(false))
	err := testBackend.ExpiryInstall(backends.BackendTypeAWS, 10, 5, true, true, true, true, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backends.BackendTypeAWS)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 1)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 5)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, true)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
	}
}

func (e *expiryTest) testCreateInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: Options.TestRegions[0],
		Firewalls:        []string{},
		BackendType:      backends.BackendTypeAWS,
		InstanceType:     "r6a.large",
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            []string{"type=gp2,size=20,count=1"},
		Expires:          time.Now().Add(90 * time.Second),
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 1)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
}

func (e *expiryTest) testCreateAttachedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	_, err := testBackend.CreateVolume(&backends.CreateVolumeInput{
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
		Expires:           time.Now().Add(60 * time.Second),
		DiskType:          "gp2",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
}

func (e *expiryTest) testCreateSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	_, err := testBackend.CreateVolume(&backends.CreateVolumeInput{
		BackendType:       backends.BackendTypeAWS,
		VolumeType:        backends.VolumeTypeSharedDisk,
		Name:              "test-shared-volume",
		Description:       "test-description",
		SizeGiB:           100,
		Placement:         Options.TestRegions[0],
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Now().Add(110 * time.Second),
		DiskType:          "",
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 1)
}

func (e *expiryTest) testWaitForExpiry(t *testing.T) {
	require.NoError(t, setup(false))
	time.Sleep(5 * time.Minute)
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning).WithName("test-instance")
	require.Equal(t, inst.Count(), 0)
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 0)
	vol = testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
	require.Equal(t, vol.Count(), 0)
}

func (e *expiryTest) testExpiryChangeConfiguration(t *testing.T) {
	require.NoError(t, setup(false))
	err := testBackend.ExpiryChangeConfiguration(backends.BackendTypeAWS, 3, false, false, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 3)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, false)
		require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, false)
	}
}

func (e *expiryTest) testExpiryRemove(t *testing.T) {
	require.NoError(t, setup(false))
	err := testBackend.ExpiryRemove(backends.BackendTypeAWS, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), 0)
}
