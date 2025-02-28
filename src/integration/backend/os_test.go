package backend_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

var osTestList = make(chan *osTestDef, 100)

func fillOsTestList() {
	osTestList <- &osTestDef{
		name:    "amazon",
		version: "2023",
	}
	osTestList <- &osTestDef{
		name:    "amazon",
		version: "2",
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
	osTestList <- &osTestDef{
		name:    "ubuntu",
		version: "18.04",
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
	osTestList <- &osTestDef{
		name:    "debian",
		version: "10",
	}
	osTestList <- &osTestDef{
		name:    "centos",
		version: "10",
	}
	osTestList <- &osTestDef{
		name:    "centos",
		version: "9",
	}
	close(osTestList)
}

type osTestDef struct {
	name    string
	version string
}

func Test99_OS(t *testing.T) {
	t.Cleanup(cleanup)
	t.Run("setup", testSetup)
	t.Run("inventory empty", testInventoryEmpty)
	t.Run("os", testOS)
	t.Run("remove firewalls", testOSRemoveFirewalls)
	t.Run("end inventory empty", testInventoryEmpty)
}

func testOSRemoveFirewalls(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	err := testBackend.GetInventory().Firewalls.Delete(10 * time.Minute)
	require.NoError(t, err)
}

func testOS(t *testing.T) {
	fillOsTestList()
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

func (o *osTestDef) test(os *osTestDef) error {
	instanceName := shortuuid.New()
	// create new instance
	err := testBackend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("1: image %s:%s %w", os.name, os.version, err)
	}
	image := testBackend.GetInventory().Images.WithInAccount(false).WithOSName(os.name).WithOSVersion(os.version).WithArchitecture(backends.ArchitectureX8664)
	if image.Count() == 0 {
		return fmt.Errorf("2: image %s:%s not found", os.name, os.version)
	}
	if image.Count() > 1 {
		return fmt.Errorf("3: multiple images found for %s:%s", os.name, os.version)
	}
	insts, err := testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      instanceName,
		Name:             instanceName,
		Nodes:            1,
		Image:            image.Describe()[0],
		NetworkPlacement: Options.TestRegions[0],
		Firewalls:        []string{},
		BackendType:      backends.BackendTypeAWS,
		InstanceType:     "r6a.large",
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            []string{"type=gp2,size=20,count=1"},
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
	// create image from instance
	_, err = testBackend.CreateImage(&backends.CreateImageInput{
		BackendType: backends.BackendTypeAWS,
		Instance:    inst.Describe()[0],
		Name:        instanceName,
		Description: "test-description",
		SizeGiB:     12,
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
	image = testBackend.GetInventory().Images.WithInAccount(true).WithName(instanceName)
	if image.Count() != 1 {
		return fmt.Errorf("10: image %s:%s expected 1 image, got %d", os.name, os.version, image.Count())
	}
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
	insts, err = testBackend.CreateInstances(&backends.CreateInstanceInput{
		ClusterName:      instanceName,
		Name:             instanceName,
		Nodes:            1,
		Image:            image.Describe()[0],
		NetworkPlacement: Options.TestRegions[0],
		Firewalls:        []string{},
		BackendType:      backends.BackendTypeAWS,
		InstanceType:     "r6a.large",
		Owner:            "test-owner",
		Description:      "test-description",
		Disks:            []string{"type=gp2,size=20,count=1"},
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
	err = testBackend.GetInventory().Images.WithName(instanceName).DeleteImages(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("18: image %s:%s %w", os.name, os.version, err)
	}
	return nil
}
