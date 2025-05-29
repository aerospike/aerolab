package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/stretchr/testify/require"
)

type imageTest struct{}

func Test25_Images(t *testing.T) {
	t.Cleanup(cleanup)
	imageTest := &imageTest{}
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("delete root images", testDeleteRootImages)
	t.Run("create vanilla instance", imageTest.testCreateVanillaInstance)
	t.Run("add file to instance", imageTest.testAddFileToInstance)
	t.Run("image create", imageTest.testImageCreate)
	t.Run("delete vanilla instance", imageTest.testDeleteVanillaInstance)
	t.Run("create test instance from image", imageTest.testCreateInstanceFromImage)
	t.Run("read file from instance", imageTest.testReadFileFromInstance)
	t.Run("delete test instance from image", imageTest.testDeleteInstanceFromImage)
	t.Run("image delete", imageTest.testImageDelete)
	t.Run("delete firewall", imageTest.testDeleteFirewall)
	t.Run("delete root images", testDeleteRootImages)
	t.Run("end inventory empty", testInventoryEmpty)
}

func testDeleteRootImages(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud != "docker" {
		t.Skip("only docker supports removing root public images")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	image := testBackend.GetInventory().Images.WithInAccount(false)
	err := image.DeleteImages(5 * time.Minute)
	require.NoError(t, err)
}

func (i *imageTest) testCreateVanillaInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())

	image := getBasicImage(t)
	placement := Options.TestRegions[0]
	itype := "r6a.large"
	disks := []string{"type=gp2,size=20,count=1,encrypted=true"}
	if cloud == "gcp" {
		if strings.Count(Options.TestRegions[0], "-") == 1 {
			placement = Options.TestRegions[0] + "-a"
		}
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=20,count=1"}
	} else if cloud == "docker" {
		itype = ""
		disks = []string{}
		placement = ""
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image,
		NetworkPlacement: placement,
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 1)
}

func (i *imageTest) testAddFileToInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	out := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"touch", "/root/test-image-file"},
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: 30 * time.Second,
			Env:            []*sshexec.Env{},
			Terminal:       true,
		},
		Username:        "root",
		ConnectTimeout:  10 * time.Second,
		ParallelThreads: 1,
	})
	require.Len(t, out, 1)
	require.NoError(t, out[0].Output.Err)
}

func (i *imageTest) testImageCreate(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	err := inst.Describe()[0].Stop(false, 10*time.Minute)
	require.NoError(t, err)
	out, err := testBackend.CreateImage(&backends.CreateImageInput{
		BackendType: backendType,
		Instance:    inst.Describe()[0],
		Name:        "test-image",
		Description: "test-description",
		SizeGiB:     30,
		Owner:       "test-owner",
		Tags:        map[string]string{},
		Encrypted:   true,
		OSName:      "test-os",
		OSVersion:   "test-os-version",
	}, 20*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, out.Image.Name, "test-image")
	require.Equal(t, out.Image.Description, "test-description")
	require.Equal(t, out.Image.Size, 30*backends.StorageGiB)
	require.Equal(t, out.Image.Owner, "test-owner")
	require.Equal(t, out.Image.Encrypted, true)
	require.Equal(t, out.Image.OSName, "test-os")
	require.Equal(t, out.Image.OSVersion, "test-os-version")
	require.NoError(t, testBackend.RefreshChangedInventory())
	wn := "test-image"
	if cloud == "docker" {
		wn = "test-image:latest"
	}
	image := testBackend.GetInventory().Images.WithName(wn).WithOSName("test-os").WithOSVersion("test-os-version")
	require.Equal(t, image.Count(), 1)
	if cloud == "aws" {
		require.Equal(t, image.Describe()[0].Size, 30*backends.StorageGiB)
	}
}

func (i *imageTest) testDeleteVanillaInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	err := inst.Describe()[0].Terminate(5 * time.Minute)
	require.NoError(t, err)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 0)
}

func (i *imageTest) testCreateInstanceFromImage(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	wn := "test-image"
	if cloud == "docker" {
		wn = "test-image:latest"
	}
	image := testBackend.GetInventory().Images.WithName(wn).WithOSName("test-os").WithOSVersion("test-os-version")
	require.Equal(t, image.Count(), 1)
	placement := Options.TestRegions[0]
	itype := "r6a.large"
	disks := []string{"type=gp2,size=30,count=1,encrypted=true"}
	if cloud == "gcp" {
		if strings.Count(Options.TestRegions[0], "-") == 1 {
			placement = Options.TestRegions[0] + "-a"
		}
		itype = "e2-standard-4"
		disks = []string{"type=pd-ssd,size=30,count=1"}
	} else if cloud == "docker" {
		itype = ""
		disks = []string{}
		placement = ""
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      "test-cluster",
		Name:             "test-instance",
		Nodes:            1,
		Image:            image.Describe()[0],
		NetworkPlacement: placement,
		BackendType:      backendType,
		InstanceType:     itype,
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            disks,
	}, 2*time.Minute)
	require.NoError(t, err)
	require.Equal(t, insts.Instances.Count(), 1)
}

func (i *imageTest) testReadFileFromInstance(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	out := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/root/test-image-file"},
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: 30 * time.Second,
			Env:            []*sshexec.Env{},
			Terminal:       true,
		},
		Username:        "root",
		ConnectTimeout:  10 * time.Second,
		ParallelThreads: 1,
	})
	require.Len(t, out, 1)
	require.NoError(t, out[0].Output.Err)
}

func (i *imageTest) testImageDelete(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	wn := "test-image"
	if cloud == "docker" {
		wn = "test-image:latest"
	}
	image := testBackend.GetInventory().Images.WithName(wn)
	require.Equal(t, image.Count(), 1)
	err := image.DeleteImages(5 * time.Minute)
	require.NoError(t, err)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	image = testBackend.GetInventory().Images.WithName(wn)
	require.Equal(t, image.Count(), 0)
}

func (i *imageTest) testDeleteInstanceFromImage(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 1)
	err := inst.Describe()[0].Terminate(5 * time.Minute)
	require.NoError(t, err)
	err = testBackend.RefreshChangedInventory()
	require.NoError(t, err)
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName("test-instance")
	require.Equal(t, inst.Count(), 0)
}

func (i *imageTest) testDeleteFirewall(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support firewalls")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	require.NoError(t, testBackend.GetInventory().Firewalls.Delete(5*time.Minute))
}
