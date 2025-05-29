package backend_test

import (
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/stretchr/testify/require"
)

type archTest struct{}

func Test45_Arch(t *testing.T) {
	t.Cleanup(cleanup)
	archTest := &archTest{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("t1: delete root images", testDeleteRootImages)
	t.Run("t1: deployAmd64", archTest.testDeployAmd64)
	t.Run("t1: deployArm64", archTest.testDeployArm64)
	t.Run("t1: cleanup instances and images", archTest.testCleanupInstancesAndImages)
	t.Run("t2: delete root images", testDeleteRootImages)
	t.Run("t2: deployArm64", archTest.testDeployArm64)
	t.Run("t2: deployAmd64", archTest.testDeployAmd64)
	t.Run("t2: cleanup instances and images", archTest.testCleanupInstancesAndImages)
	t.Run("delete firewalls", archTest.testDeleteFirewalls) // no docker
	t.Run("end inventory empty", testInventoryEmpty)        // all
}

func getArchImage(t *testing.T, arch backends.Architecture) *backends.Image {
	imgs := testBackend.GetInventory().Images
	require.NotEmpty(t, imgs.Describe())
	img1 := imgs.WithInAccount(false)
	require.NotEmpty(t, img1.Describe())
	img2 := img1.WithOSName("ubuntu")
	require.NotEmpty(t, img2.Describe())
	img3 := img2.WithOSVersion("24.04")
	require.NotEmpty(t, img3.Describe())
	img4 := img3.WithArchitecture(arch)
	require.NotEmpty(t, img4.Describe())
	image := img4.Describe()[0]
	require.NotNil(t, image)
	return image
}

func (at *archTest) testDeployAmd64(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getArchImage(t, backends.ArchitectureX8664)
	placement := Options.TestRegions[0] + "a"
	itype := "r6a.large"
	disks := []string{"type=gp2,size=20,count=1"}
	if cloud == "gcp" {
		placement = Options.TestRegions[0] + "-a"
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=20,count=1"}
	} else if cloud == "docker" {
		placement = "default,default"
		itype = ""
		disks = []string{}
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-amd64",
		Name:             "test-amd64",
		Nodes:            1,
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
	require.Equal(t, insts.Instances.Count(), 1)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	outs := insts.Instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "uname -a"},
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
	require.Equal(t, len(outs), 1)
	require.NoError(t, outs[0].Output.Err)
	require.NotContains(t, string(outs[0].Output.Stdout), " aarch64 ")
}

func (at *archTest) testDeployArm64(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := getArchImage(t, backends.ArchitectureARM64)
	placement := Options.TestRegions[0] + "a"
	itype := "r6g.large"
	disks := []string{"type=gp2,size=20,count=1"}
	if cloud == "gcp" {
		placement = Options.TestRegions[0] + "-a"
		itype = "t2a-standard-2"
		disks = []string{"type=pd-ssd,size=20,count=1"}
	} else if cloud == "docker" {
		placement = "default,default"
		itype = ""
		disks = []string{}
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-arm64",
		Name:             "test-arm64",
		Nodes:            1,
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
	require.Equal(t, insts.Instances.Count(), 1)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	outs := insts.Instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "uname -a"},
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
	require.Equal(t, len(outs), 1)
	require.NoError(t, outs[0].Output.Err)
	require.Contains(t, string(outs[0].Output.Stdout), " aarch64 ")
}

func (at *archTest) testCleanupInstancesAndImages(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.ForceRefreshInventory())
	inv := testBackend.GetInventory()
	require.NoError(t, inv.Instances.WithNotState(backends.LifeCycleStateTerminated).Terminate(10*time.Minute))
	require.NoError(t, inv.Images.WithInAccount(true).DeleteImages(10*time.Minute))
	if cloud == "docker" {
		require.NoError(t, inv.Images.WithInAccount(false).DeleteImages(10*time.Minute))
	}
}

func (at *archTest) testDeleteFirewalls(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support firewalls")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())

	fw := testBackend.GetInventory().Firewalls
	fwCount := 1
	if cloud == "gcp" {
		fwCount = 2
	}
	require.Equal(t, fw.Count(), fwCount)
	err := fw.Delete(10 * time.Minute)
	require.NoError(t, err)
	require.NoError(t, testBackend.RefreshChangedInventory())
	fw = testBackend.GetInventory().Firewalls
	require.Equal(t, fw.Count(), 0)
}
