package backend_test

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
	"github.com/stretchr/testify/require"
)

// setup
var (
	testProject    string = "aerolab-test"
	tempDir        string
	aerolabVersion string = "v0.0.0"
	Options        *BackendTestOptions

	testBackend backends.Backend
)

type BackendTestOptions struct {
	TestRegions []string
	// Put test options here
	SkipCleanup bool
	TempDir     string
}

func (o *BackendTestOptions) Validate() error {
	var err error

	if os.Getenv("AWS_PROFILE") == "" {
		return errors.New("AWS_PROFILE environment variable not set")
	}

	if value, isSet := os.LookupEnv("AEROLAB_SKIP_CLEANUP"); isSet {
		o.SkipCleanup, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	}

	if value, isSet := os.LookupEnv("AEROLAB_AWS_TEST_REGIONS"); isSet && value != "" {
		o.TestRegions = strings.Split(value, ",")
	} else {
		return errors.New("AEROLAB_AWS_TEST_REGIONS environment variable not set")
	}

	if value := os.Getenv("AEROLAB_TEST_CUSTOM_TMPDIR"); value != "" {
		o.TempDir = value
	}

	return nil
}

func setup(fresh bool) error {
	if Options != nil {
		return nil // already setup
	}
	Options = &BackendTestOptions{}
	err := Options.Validate()
	if err != nil {
		return err
	}

	credentials := &clouds.Credentials{
		AWS: clouds.AWS{
			AuthMethod: clouds.AWSAuthMethodShared,
		},
	}

	if Options.TempDir == "" {
		tempDir, err = os.MkdirTemp("", testProject)
		if err != nil {
			return err
		}
	} else {
		tempDir = Options.TempDir
		os.MkdirAll(tempDir, 0755)
	}
	if Options.SkipCleanup {
		fmt.Printf("Skipping cleanup, tempDir=%s\n", tempDir)
	}

	// Put setup boilerplate here
	testBackend, err = backend.New(testProject,
		&backend.Config{
			RootDir:         tempDir,
			Cache:           false,
			Credentials:     credentials,
			LogLevel:        logger.DETAIL,
			LogMillisecond:  true,
			AerolabVersion:  aerolabVersion,
			ListAllProjects: false,
		},
		false)
	if err != nil {
		return err
	}

	err = testBackend.AddRegion(backends.BackendTypeAWS, Options.TestRegions...)
	if err != nil {
		return err
	}

	if fresh {
		err = cleanupBackend()
		if err != nil {
			return err
		}
	}

	err = testBackend.ForceRefreshInventory()
	if err != nil {
		return err
	}

	return nil
}

func cleanupBackend() error {
	log.Print("CLEANING UP BACKEND")
	err := testBackend.ForceRefreshInventory()
	if err != nil {
		return err
	}

	inv := testBackend.GetInventory()

	err = inv.Instances.Terminate(time.Minute * 10)
	if err != nil {
		return err
	}

	err = inv.Volumes.WithDeleteOnTermination(false).DeleteVolumes(inv.Firewalls.Describe(), time.Minute*10)
	if err != nil {
		return err
	}

	err = inv.Firewalls.Delete(time.Minute * 10)
	if err != nil {
		return err
	}

	err = inv.Images.WithInAccount(true).DeleteImages(time.Minute * 10)
	if err != nil {
		return err
	}

	expiries, err := testBackend.ExpiryList()
	if err != nil {
		return err
	}

	expiryRegions := []string{}
	for _, expiry := range expiries.ExpirySystems {
		expiryRegions = append(expiryRegions, expiry.Zone)
	}

	err = testBackend.ExpiryRemove(backends.BackendTypeAWS, expiryRegions...)
	if err != nil {
		return err
	}

	log.Print("CLEANED UP BACKEND")
	return nil
}

func cleanup() {
	if !Options.SkipCleanup {
		Options.SkipCleanup = true
		cleanupBackend()
		os.RemoveAll(tempDir)
	}
}

func testSetup(t *testing.T) {
	require.NoError(t, setup(true))
}

func testInventoryEmpty(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inventory := testBackend.GetInventory()
	require.Equal(t, inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Count(), 0)
	require.Equal(t, inventory.Volumes.Count(), 0)
	require.Equal(t, inventory.Networks.WithAerolabManaged(true).Count(), 0)
	require.Equal(t, inventory.Networks.WithAerolabManaged(false).Count(), 1)
	require.Equal(t, inventory.Firewalls.Count(), 0)
	require.Equal(t, inventory.Images.WithInAccount(true).Count(), 0)
	require.Equal(t, inventory.Images.WithInAccount(false).Count(), 34)
	expiries, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiries.ExpirySystems), 0)
}
