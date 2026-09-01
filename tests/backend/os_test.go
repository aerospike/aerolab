//go:build integration_cloud

package backend_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/bgcp"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

var osTestList = make(chan *osTestDef, 100)

/* supported OSes:
* AWS: (ubuntu 18.04 and debian 10 are EOL and no longer published as AMIs)
  - amazon: 2023, 2
  - ubuntu: 26.04, 24.04, 22.04, 20.04
  - rocky: 10, 9, 8
  - centos: 10, 9
  - debian: 13, 12, 11
* GCP:
  - ubuntu: 26.04, 24.04, 22.04, 20.04, 18.04
  - rocky: 10, 9, 8
  - centos: 10, 9, 8
  - debian: 13, 12, 11, 10
*/

var osTestSequential = false

// errImageUnavailable marks an entry the target region publishes no image for.
// The lists above are per-cloud, but image availability is also per-region
// (rocky 8 exists in us-east-1 but not ca-central-1, for example), so those
// entries are reported and skipped rather than failed.
var errImageUnavailable = errors.New("no image published in this region")

// imagesInTestRegion narrows public images to the ones the region under test can
// actually launch.
//
// The backends do not describe an image's location the same way, so this cannot
// be an equality test on ZoneName. AWS publishes an image per region and reports
// that one region. GCP images are global, and the backend reports their storage
// locations joined with commas -- a fifty-element string that never equals a
// single region, which meant an equality filter here skipped every entry in the
// list and let the OS test pass having deployed nothing at all.
//
// Membership is therefore tested against the split value, and a location is also
// accepted when it is the multi-region containing the region ("us" covers
// us-central1), which is how GCP describes an image stored multi-regionally. An
// image that reports no location is treated as unrestricted.
func imagesInTestRegion(images backends.Images) backends.Images {
	region := Options.TestRegions[0]
	ret := backends.ImageList{}
	for _, img := range images.Describe() {
		if img.ZoneName == "" {
			ret = append(ret, img)
			continue
		}
		for _, loc := range strings.Split(img.ZoneName, ",") {
			loc = strings.TrimSpace(loc)
			if loc == region || strings.HasPrefix(region, loc+"-") {
				ret = append(ret, img)
				break
			}
		}
	}
	return ret
}

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
		version: "26.04",
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
	if cloud == "gcp" {
		osTestList <- &osTestDef{
			name:    "ubuntu",
			version: "18.04",
		}
	}
	osTestList <- &osTestDef{
		name:    "rocky",
		version: "10",
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
	if cloud == "gcp" {
		osTestList <- &osTestDef{
			name:    "debian",
			version: "10",
		}
	}
	osTestList <- &osTestDef{
		name:    "centos",
		version: "10",
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
	entries := 0
	skipped := 0
	if osTestSequential {
		for osTest := range osTestList {
			entries++
			t.Logf("testing %s:%s", osTest.name, osTest.version)
			err := osTest.test(osTest)
			if errors.Is(err, errImageUnavailable) {
				skipped++
				t.Logf("skipping: %s", err)
				continue
			}
			if err != nil {
				require.NoError(t, err)
			}
		}
	} else {
		errs := make(chan error, 100)
		wg := sync.WaitGroup{}
		for osTest := range osTestList {
			entries++
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
			if errors.Is(err, errImageUnavailable) {
				skipped++
				t.Logf("skipping: %s", err)
				continue
			}
			if err != nil {
				t.Log(err)
				isErr = true
			}
		}
		require.False(t, isErr, "one or more OS images failed, see the logged errors above")
	}

	// Skipping an entry the region genuinely has no image for is expected;
	// skipping every one of them is not, and it used to pass as a green test
	// that had deployed nothing. Whatever the cause -- an image lookup that
	// cannot match, a region with no images, an empty list -- a run that
	// exercised no OS at all has not tested anything and must say so.
	require.NotZero(t, entries, "the OS list is empty for cloud %q", cloud)
	require.NotEqual(t, entries, skipped,
		"all %d OS entries were skipped as unavailable in %s: the image lookup or the list is wrong, not the images",
		entries, Options.TestRegions[0])
}

// osTestCleanup removes, best effort, whatever the run for one OS created.
func osTestCleanup(instanceName string, imageName string) {
	if err := testBackend.RefreshChangedInventory(); err != nil {
		return
	}
	inventory := testBackend.GetInventory()
	inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).WithName(instanceName).Terminate(10 * time.Minute) //nolint:errcheck
	inventory.Images.WithInAccount(true).WithName(imageName).DeleteImages(10 * time.Minute)                                //nolint:errcheck
}

func (o *osTestDef) test(os *osTestDef) (err error) {
	instanceName := "z" + strings.ToLower(shortuuid.New())
	// AWS rejects ':' in an AMI name, and docker tags the committed image
	// itself, so ':latest' belongs only in the name we later look the image up
	// by, and only on docker.
	imageName := instanceName + "-image"
	lookupName := imageName
	if cloud == "docker" {
		lookupName = imageName + ":latest"
	}
	// create new instance
	err = testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("1: image %s:%s %w", os.name, os.version, err)
	}
	// Scoped to the region under test, because on AWS the same OS/version is
	// published as a separate image per region and one region's image cannot be
	// launched into another.
	images := imagesInTestRegion(
		testBackend.GetInventory().Images.WithInAccount(false).WithOSName(os.name).WithOSVersion(os.version).WithArchitecture(backends.ArchitectureX8664))
	if images.Count() == 0 {
		return fmt.Errorf("2: image %s:%s: %w", os.name, os.version, errImageUnavailable)
	}
	if images.Count() > 1 {
		return fmt.Errorf("3: multiple images found for %s:%s", os.name, os.version)
	}
	image := images.Describe()[0]
	placement := Options.TestRegions[0] + "a"
	if strings.Count(Options.TestRegions[0], "-") == 1 {
		placement = Options.TestRegions[0] + "-a"
	}
	params := map[backends.BackendType]any{
		backends.BackendTypeAWS: &baws.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: Options.TestRegions[0] + "a",
			InstanceType:     "r6a.large",
			Disks:            []string{"type=gp2,size=20,count=1"},
			Firewalls:        []string{},
		},
		backends.BackendTypeGCP: gcpParams(&bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
		}),
		backends.BackendTypeDocker: &bdocker.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: "default,default",
			Disks:            []string{},
			Firewalls:        []string{},
		},
	}
	// Failures are collected and reported once every OS has been tried, so a
	// run that gives up half way must not strand its instance: the firewall
	// removal that follows would fail on the dependency it still holds.
	defer func() {
		if err != nil {
			osTestCleanup(instanceName, lookupName)
		}
	}()
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
	images = testBackend.GetInventory().Images.WithInAccount(true).WithName(lookupName)
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
	params = map[backends.BackendType]any{
		backends.BackendTypeAWS: &baws.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: Options.TestRegions[0] + "a",
			InstanceType:     "r6a.large",
			Disks:            []string{"type=gp2,size=20,count=1"},
			Firewalls:        []string{},
		},
		backends.BackendTypeGCP: gcpParams(&bgcp.CreateInstanceParams{
			Image:            image,
			NetworkPlacement: placement,
			InstanceType:     "e2-standard-4",
			Disks:            []string{"type=pd-ssd,size=20,count=1"},
			Firewalls:        []string{},
		}),
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
	err = testBackend.GetInventory().Images.WithName(lookupName).DeleteImages(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("18: image %s:%s %w", os.name, os.version, err)
	}
	return nil
}
