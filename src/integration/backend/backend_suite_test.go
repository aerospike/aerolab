package backend_test

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rglonek/logger"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

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
		Expect(err).NotTo(HaveOccurred())
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

var _ = BeforeSuite(func() {
	Options = &BackendTestOptions{}
	err := Options.Validate()
	Expect(err).NotTo(HaveOccurred())

	credentials := &clouds.Credentials{
		AWS: clouds.AWS{
			AuthMethod: clouds.AWSAuthMethodShared,
		},
	}

	if Options.TempDir == "" {
		tempDir, err = os.MkdirTemp("", testProject)
		Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())

	err = testBackend.AddRegion(backends.BackendTypeAWS, Options.TestRegions...)
	Expect(err).NotTo(HaveOccurred())

	err = testBackend.ForceRefreshInventory()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if !Options.SkipCleanup {
		cleanupBackend()

		// Clean up everything (anything we may have deployed) here
		os.RemoveAll(tempDir)
	}
})

func TestBackend(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backend Integration Suite")
}

func cleanupBackend() {
	err := testBackend.ForceRefreshInventory()
	Expect(err).NotTo(HaveOccurred())

	inv := testBackend.GetInventory()

	err = inv.Instances.Terminate(time.Minute * 10)
	Expect(err).NotTo(HaveOccurred())

	err = inv.Firewalls.Delete(time.Minute * 10)
	Expect(err).NotTo(HaveOccurred())
}
