package backend_test

import (
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"github.com/stretchr/testify/require"
)

type expiryTest struct{}

func Test40_Expiry(t *testing.T) {
	t.Cleanup(cleanup)
	expiryTest := &expiryTest{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("expiry install", expiryTest.testExpiryInstall)
	t.Run("expiry change frequency", expiryTest.testExpiryChangeFrequency)
	t.Run("expiry upgrade", expiryTest.testExpiryUpgrade)
	t.Run("create instance", expiryTest.testCreateInstance)
	t.Run("create attached volume", expiryTest.testCreateAttachedVolume)
	t.Run("create shared volume", expiryTest.testCreateSharedVolume)
	t.Run("wait for expiry", expiryTest.testWaitForExpiry)
	t.Run("expiry change configuration", expiryTest.testExpiryChangeConfiguration)
	t.Run("expiry remove", expiryTest.testExpiryRemove)
	t.Run("cleanup firewalls", expiryTest.testCleanupFirewalls)
	t.Run("end inventory empty", testInventoryEmpty)
}

func (e *expiryTest) testCleanupFirewalls(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(10*time.Minute))
}

func (e *expiryTest) testExpiryInstall(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	eksCtl := false
	if backendType == backends.BackendTypeAWS {
		eksCtl = true
	}
	err := testBackend.ExpiryInstall(backendType, 2, 6, eksCtl, true, true, true, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backendType)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 2)
		if backendType == backends.BackendTypeAWS {
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, eksCtl)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
		} else if backendType == backends.BackendTypeGCP {
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).CleanupDNS, true)
		}
	}
}

func (e *expiryTest) testExpiryChangeFrequency(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	err := testBackend.ExpiryChangeFrequency(backendType, 1, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backendType)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 1)
		if backendType == backends.BackendTypeAWS {
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, true)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
		} else if backendType == backends.BackendTypeGCP {
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).CleanupDNS, true)
		}
	}
}

func (e *expiryTest) testExpiryUpgrade(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	eksCtl := false
	if backendType == backends.BackendTypeAWS {
		eksCtl = true
	}
	err := testBackend.ExpiryInstall(backendType, 10, 5, eksCtl, true, true, true, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		require.Equal(t, expiry.BackendType, backendType)
		require.Contains(t, Options.TestRegions, expiry.Zone)
		require.Equal(t, expiry.FrequencyMinutes, 1)
		if backendType == backends.BackendTypeAWS {
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, true)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, true)
		} else if backendType == backends.BackendTypeGCP {
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).LogLevel, 6)
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).CleanupDNS, true)
		}
	}
}

func (e *expiryTest) testCreateInstance(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	placement := Options.TestRegions[0] + "a"
	itype := "r6a.large"
	disks := []string{"type=gp2,size=20,count=1"}
	if cloud == "gcp" {
		placement = Options.TestRegions[0] + "-a"
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=20,count=1"}
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: placement,
		Firewalls:        []string{},
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
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
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	diskType := "gp2"
	placement := Options.TestRegions[0]
	if cloud == "gcp" {
		diskType = "pd-ssd"
		placement = placement + "-a"
	}
	_, err := testBackend.CreateVolume(&backends.CreateVolumeInput{
		BackendType:       backendType,
		VolumeType:        backends.VolumeTypeAttachedDisk,
		Name:              "test-attached-volume",
		Description:       "test-description",
		SizeGiB:           10,
		Placement:         placement,
		Iops:              0,
		Throughput:        0,
		Owner:             "test-owner",
		Tags:              map[string]string{},
		Encrypted:         false,
		Expires:           time.Now().Add(60 * time.Second),
		DiskType:          diskType,
		SharedDiskOneZone: false,
	})
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 1)
}

func (e *expiryTest) testCreateSharedVolume(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	if backendType == backends.BackendTypeGCP {
		t.Skip("GCP does not support shared volumes")
		return
	}
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
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	t.Log("Sleeping for 5 minutes")
	time.Sleep(5 * time.Minute)
	t.Log("Checking inventory")
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning).WithName("test-instance")
	require.Equal(t, inst.Count(), 0)
	vol := testBackend.GetInventory().Volumes.WithName("test-attached-volume").WithType(backends.VolumeTypeAttachedDisk)
	require.Equal(t, vol.Count(), 0)
	if backendType == backends.BackendTypeAWS {
		vol = testBackend.GetInventory().Volumes.WithName("test-shared-volume").WithType(backends.VolumeTypeSharedDisk)
		require.Equal(t, vol.Count(), 0)
	}
}

func (e *expiryTest) testExpiryChangeConfiguration(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	err := testBackend.ExpiryChangeConfiguration(backendType, 3, false, false, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), len(Options.TestRegions))
	for _, expiry := range expiryList.ExpirySystems {
		require.Equal(t, expiry.InstallationSuccess, true)
		if backendType == backends.BackendTypeAWS {
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).LogLevel, 3)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).ExpireEksctl, false)
			require.Equal(t, expiry.BackendSpecific.(*baws.ExpiryDetail).CleanupDNS, false)
		} else if backendType == backends.BackendTypeGCP {
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).LogLevel, 3)
			require.Equal(t, expiry.BackendSpecific.(*bgcp.ExpirySystemDetail).CleanupDNS, false)
		}
	}
}

func (e *expiryTest) testExpiryRemove(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support expiry")
		return
	}
	err := testBackend.ExpiryRemove(backendType, Options.TestRegions...)
	require.NoError(t, err)
	expiryList, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiryList.ExpirySystems), 0)
}
