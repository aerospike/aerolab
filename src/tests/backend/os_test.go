package backend_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

var osTestList = make(chan *osTestDef, 100)

/* supported OSes:
* AWS:
  - amazon: 2023, 2
  - ubuntu: 24.04, 22.04, 20.04, 18.04
  - rocky: 9, 8
  - centos: 10, 9
  - debian: 12, 11, 10
* GCP:
  - ubuntu: 24.04, 22.04, 20.04, 18.04
  - rocky: 9, 8
  - centos: 9, 8
  - debian: 12, 11, 10
*/

var osTestSequential = false

func fillOsTestList() {
	if cloud == "aws" {
		osTestList <- &osTestDef{
			name:    "amazon",
			version: "2023",
		}
		osTestList <- &osTestDef{
			name:    "amazon",
			version: "2",
		}
	}
	osTestList <- &osTestDef{
		name:    "ubuntu",
		version: "24.04",
	}
	osTestList <- &osTestDef{
		name:    "ubuntu",
		version: "22.04",
	}
	osTestList <- &osTestDef{
		name:    "ubuntu",
		version: "20.04",
	}
	if cloud != "docker" {
		osTestList <- &osTestDef{
			name:    "ubuntu",
			version: "18.04",
		}
	}
	osTestList <- &osTestDef{
		name:    "rocky",
		version: "9",
	}
	osTestList <- &osTestDef{
		name:    "rocky",
		version: "8",
	}
	osTestList <- &osTestDef{
		name:    "debian",
		version: "12",
	}
	osTestList <- &osTestDef{
		name:    "debian",
		version: "11",
	}
	if cloud != "docker" {
		osTestList <- &osTestDef{
			name:    "debian",
			version: "10",
		}
	}
	if cloud == "aws" {
		osTestList <- &osTestDef{
			name:    "centos",
			version: "10",
		}
	}
	osTestList <- &osTestDef{
		name:    "centos",
		version: "9",
	}
	if cloud != "aws" {
		osTestList <- &osTestDef{
			name:    "centos",
			version: "8",
		}
	}
	close(osTestList)
}

type osTestDef struct {
	name    string
	version string
}

// docker container list -a |awk '{print $1}' |grep -v CONTAINER |xargs docker rm -f
// docker image list |grep -- -image |awk '{print $3}' |xargs docker rmi
// docker image list |grep -- '^amd64-' |awk '{print $3}' |xargs docker rmi
func Test99_OS(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("test delete root images", testDeleteRootImages)
	t.Run("os", testOS)
	t.Run("remove firewalls", testOSRemoveFirewalls)
	t.Run("end inventory empty", testInventoryEmpty)
}

func testOSRemoveFirewalls(t *testing.T) {
	require.NoError(t, setup(false))
	if cloud == "docker" {
		t.Skip("docker does not support firewalls")
		return
	}
	require.NoError(t, testBackend.RefreshChangedInventory())
	err := testBackend.GetInventory().Firewalls.Delete(10 * time.Minute)
	require.NoError(t, err)
}

func testOS(t *testing.T) {
	fillOsTestList()
	if osTestSequential {
		for osTest := range osTestList {
			t.Logf("testing %s:%s", osTest.name, osTest.version)
			err := osTest.test(osTest)
			if err != nil {
				require.NoError(t, err)
			}
		}
	} else {
		errs := make(chan error, 100)
		wg := sync.WaitGroup{}
		for osTest := range osTestList {
			wg.Add(1)
			go func(osTest *osTestDef) {
				defer wg.Done()
				err := osTest.test(osTest)
				if err != nil {
					errs <- err
				}
			}(osTest)
		}
		wg.Wait()
		close(errs)
		isErr := false
		for err := range errs {
			if err != nil {
				t.Log(err)
				isErr = true
			}
		}
		require.False(t, isErr)
	}
}

func (o *osTestDef) test(os *osTestDef) error {
	instanceName := "z" + strings.ToLower(shortuuid.New())
	// create new instance
	err := testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("1: image %s:%s %w", os.name, os.version, err)
	}
	images := testBackend.GetInventory().Images.WithInAccount(false).WithOSName(os.name).WithOSVersion(os.version).WithArchitecture(backends.ArchitectureX8664)
	if images.Count() == 0 {
		return fmt.Errorf("2: image %s:%s not found", os.name, os.version)
	}
	if images.Count() > 1 {
		return fmt.Errorf("3: multiple images found for %s:%s", os.name, os.version)
	}
	image := images.Describe()[0]
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
		},
		backends.BackendTypeGCP: &bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
		},
		backends.BackendTypeDocker: &bdocker.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: "default,default",
			Disks:            []string{},
			Firewalls:        []string{},
		},
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:           instanceName,
		Name:                  instanceName,
		Nodes:                 1,
		BackendType:           backendType,
		Owner:                 "test-owner",
		Description:           "test-description",
		BackendSpecificParams: params,
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("4: image %s:%s %w", os.name, os.version, err)
	}
	if insts.Instances.Count() != 1 {
		return fmt.Errorf("5: image %s:%s expected 1 instance, got %d", os.name, os.version, insts.Instances.Count())
	}
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("6: image %s:%s %w", os.name, os.version, err)
	}
	inst := testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName(instanceName)
	if inst.Count() != 1 {
		return fmt.Errorf("7: image %s:%s expected 1 instance, got %d", os.name, os.version, inst.Count())
	}
	err = inst.Describe()[0].Stop(false, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("7.1: image %s:%s %w", os.name, os.version, err)
	}
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("7.2: image %s:%s %w", os.name, os.version, err)
	}
	// create image from instance
	imageName := instanceName + "-image:latest"
	_, err = testBackend.CreateImage(&backends.CreateImageInput{
		BackendType: backendType,
		Instance:    inst.Describe()[0],
		Name:        imageName,
		Description: "test-description",
		SizeGiB:     20,
		Owner:       "test-owner",
		Tags:        map[string]string{},
		Encrypted:   false,
		OSName:      os.name,
		OSVersion:   os.version,
	}, 20*time.Minute)
	if err != nil {
		return fmt.Errorf("8: image %s:%s %w", os.name, os.version, err)
	}
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("9: image %s:%s %w", os.name, os.version, err)
	}
	images = testBackend.GetInventory().Images.WithInAccount(true).WithName(imageName)
	if images.Count() != 1 {
		return fmt.Errorf("10: image %s:%s expected 1 image, got %d", os.name, os.version, images.Count())
	}
	image = images.Describe()[0]
	// destroy original instance
	err = testBackend.GetInventory().Instances.WithName(instanceName).Terminate(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("11: image %s:%s %w", os.name, os.version, err)
	}
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("12: image %s:%s %w", os.name, os.version, err)
	}
	// create new instance from image
	placement = Options.TestRegions[0] + "a"
	if strings.Count(Options.TestRegions[0], "-") == 1 {
		placement = Options.TestRegions[0] + "-a"
	}
	params = map[backends.BackendType]interface{}{
		backends.BackendTypeAWS: &baws.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: Options.TestRegions[0] + "a",
			InstanceType:     "r6a.large",
			Disks:            []string{"type=gp2,size=20,count=1"},
			Firewalls:        []string{},
		},
		backends.BackendTypeGCP: &bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
		},
		backends.BackendTypeDocker: &bdocker.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: "default,default",
			Disks:            []string{},
			Firewalls:        []string{},
		},
	}
	insts, err = testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:           instanceName,
		Name:                  instanceName,
		Nodes:                 1,
		BackendType:           backendType,
		Owner:                 "test-owner",
		Description:           "test-description",
		BackendSpecificParams: params,
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("13: image %s:%s %w", os.name, os.version, err)
	}
	if insts.Instances.Count() != 1 {
		return fmt.Errorf("14: image %s:%s expected 1 instance, got %d", os.name, os.version, insts.Instances.Count())
	}
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("15: image %s:%s %w", os.name, os.version, err)
	}
	inst = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName(instanceName)
	if inst.Count() != 1 {
		return fmt.Errorf("16: image %s:%s expected 1 instance, got %d", os.name, os.version, inst.Count())
	}
	// destroy new instance
	err = testBackend.GetInventory().Instances.WithNotState(backends.LifeCycleStateTerminated).WithName(instanceName).Terminate(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("17: image %s:%s %w", os.name, os.version, err)
	}
	// destroy image
	err = testBackend.GetInventory().Images.WithName(imageName).DeleteImages(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("18: image %s:%s %w", os.name, os.version, err)
	}
	return nil
}
