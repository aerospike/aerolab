package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

// test cases
func Test01_Instances(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	t.Run("inventory print", testInventoryPrint)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("create instance get price", testCreateInstanceGetPrice)
	t.Run("create 3 instances", testCreateInstance)
	t.Run("instance create tags", testInstanceCreateTags)
	t.Run("instance remove tags", testInstanceRemoveTags)
	t.Run("instance change expiry", testInstanceChangeExpiry)
	t.Run("instances stop", testInstancesStop)
	t.Run("instances start", testInstancesStart)
	t.Run("instances exec", testInstancesExec)
	t.Run("instances sftp", testInstancesSftp)
	t.Run("update hosts file", testInstancesUpdateHostsFile)
	t.Run("instances terminate", testInstancesTerminate)
	t.Run("end inventory empty", testInventoryEmpty)
}

type testInstancesDNS struct{}

func testInstancesUpdateHostsFile(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
	require.Equal(t, insts.Count(), 3)
	require.NoError(t, insts.UpdateHostsFile(insts.Describe(), 2))
}

func getBasicImage(t *testing.T) *backends.Image {
	imgs := testBackend.GetInventory().Images
	require.NotEmpty(t, imgs.Describe())
	img1 := imgs.WithInAccount(false)
	require.NotEmpty(t, img1.Describe())
	img2 := img1.WithOSName("ubuntu")
	require.NotEmpty(t, img2.Describe())
	img3 := img2.WithOSVersion("24.04")
	require.NotEmpty(t, img3.Describe())
	img4 := img3.WithArchitecture(backends.ArchitectureX8664)
	require.NotEmpty(t, img4.Describe())
	image := img4.Describe()[0]
	require.NotNil(t, image)
	return image
}

func testCreateInstanceGetPrice(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	placement := Options.TestRegions[0] + "a"
	itype := "r6a.large"
	disks := []string{"type=gp2,size=20,count=2"}
	if cloud == "gcp" {
		if strings.Count(Options.TestRegions[0], "-") == 1 {
			placement = Options.TestRegions[0] + "-a"
		}
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=20,count=2"}
	}
	costPPH, costGB, err := testBackend.CreateInstancesGetPrice(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Nodes:            3,
		Image:            image,
		NetworkPlacement: placement,
		Firewalls:        []string{"test-aerolab-fw"},
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
	})
	require.NoError(t, err)
	require.NotZero(t, costPPH)
	require.NotZero(t, costGB)
	costPPH1, costGB1, err := testBackend.CreateInstancesGetPrice(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: placement,
		Firewalls:        []string{"test-aerolab-fw"},
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
	})
	require.NoError(t, err)
	require.Equal(t, costPPH1*3, costPPH)
	require.Equal(t, costGB1*3, costGB)
}

func testCreateInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getBasicImage(t)
	placement := Options.TestRegions[0] + "a"
	itype := "r6a.large"
	disks := []string{"type=gp2,size=20,count=2"}
	if cloud == "gcp" {
		placement = Options.TestRegions[0] + "-a"
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=20,count=2"}
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Nodes:            3,
		Image:            image,
		NetworkPlacement: placement,
		Firewalls:        []string{},
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 3)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	require.Equal(t, testBackend.GetInventory().Firewalls.Count(), 1)
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 3)
	require.Equal(t, testBackend.GetInventory().Volumes.Count(), 6)
	for _, vol := range testBackend.GetInventory().Volumes.Describe() {
		require.Equal(t, vol.Size, 20*backends.StorageGiB)
	}
}

func testInstanceCreateTags(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	require.NoError(t, insts.AddTags(map[string]string{"test-tag": "test-value"}))
	require.NoError(t, testBackend.RefreshChangedInventory())

	insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	for _, inst := range insts.Describe() {
		require.Contains(t, inst.Tags, "test-tag")
		require.Equal(t, inst.Tags["test-tag"], "test-value")
	}
}

func testInstanceRemoveTags(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.NoError(t, insts.RemoveTags([]string{"test-tag"}))
	require.NoError(t, testBackend.RefreshChangedInventory())

	insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	for _, inst := range insts.Describe() {
		require.NotContains(t, inst.Tags, "test-tag")
	}
}

func testInstanceChangeExpiry(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	newExp := time.Now().Add(time.Hour * 24 * 30)
	require.NoError(t, insts.ChangeExpiry(newExp))
	require.NoError(t, testBackend.RefreshChangedInventory())

	insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	for _, inst := range insts.Describe() {
		require.WithinDuration(t, inst.Expires, newExp, time.Second*10)
	}
	require.NoError(t, insts.ChangeExpiry(time.Time{}))
	require.NoError(t, testBackend.RefreshChangedInventory())

	insts = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	for _, inst := range insts.Describe() {
		require.Zero(t, inst.Expires)
	}
}

func testInstancesStop(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
	require.Equal(t, insts.Count(), 3)
	require.NoError(t, insts.Stop(false, 2*time.Minute))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts = testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
	require.Equal(t, insts.Count(), 0)
	for _, inst := range insts.Describe() {
		require.Equal(t, inst.InstanceState, backends.LifeCycleStateStopped)
	}
}

func testInstancesStart(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateStopped)
	require.Equal(t, insts.Count(), 3)
	require.NoError(t, insts.Start(2*time.Minute))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts = testBackend.GetInventory().Instances.WithState(backends.LifeCycleStateRunning)
	require.Equal(t, insts.Count(), 3)
	for _, inst := range insts.Describe() {
		require.Equal(t, inst.InstanceState, backends.LifeCycleStateRunning)
	}
}

func testInstancesExec(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	outs := insts.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"ls", "-la"},
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: 10 * time.Second,
			Env:            []*sshexec.Env{},
			Terminal:       true,
		},
		Username:        "root",
		ConnectTimeout:  10 * time.Second,
		ParallelThreads: 2,
	})
	require.Equal(t, len(outs), 3)
	for _, out := range outs {
		require.NotNil(t, out.Output)
		require.NoError(t, out.Output.Err)
	}
}

func testInstancesSftp(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	confs, err := insts.GetSftpConfig("root")
	require.NoError(t, err)
	require.Equal(t, len(confs), 3)
	for _, conf := range confs {
		require.NotEmpty(t, conf.Host)
		require.NotZero(t, conf.Port)
		require.Equal(t, conf.Username, "root")
		sftpClient, err := sshexec.NewSftp(conf)
		require.NoError(t, err)
		require.NotNil(t, sftpClient)
		remote := sftpClient.GetRemoteClient()
		require.NotNil(t, remote)
		files, err := remote.ReadDir("/tmp")
		require.NoError(t, err)
		require.NotEmpty(t, files)
		sftpClient.Close()
	}
}

func testInstancesTerminate(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	insts := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated)
	require.Equal(t, insts.Count(), 3)
	require.NoError(t, insts.Terminate(2*time.Minute))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.Equal(t, testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 0)
	require.Equal(t, testBackend.GetInventory().Volumes.Count(), 0)
	require.Equal(t, testBackend.GetInventory().Firewalls.Count(), 1)
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(2*time.Minute))
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.Equal(t, testBackend.GetInventory().Firewalls.Count(), 0)
}
