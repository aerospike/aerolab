package backend_test

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rglonek/logger"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

var (
	testProject    string = "aerolab-test"
	tempDir        string
	aerolabVersion string = "v0.0.0"
	Options        BackendTestOptions

	testBackend backend.Backend
)

type BackendTestOptions struct {
	TestRegions []string
	// Put test options here
	SkipCleanup bool
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

	return nil
}

var _ = BeforeSuite(func() {
	options := &BackendTestOptions{}
	err := options.Validate()
	Expect(err).NotTo(HaveOccurred())

	credentials := &clouds.Credentials{
		AWS: clouds.AWS{
			AuthMethod: clouds.AWSAuthMethodShared,
		},
	}

	tempDir, err = os.MkdirTemp("", testProject)
	Expect(err).NotTo(HaveOccurred())

	// Put setup boilerplate here
	testBackend, err = backend.Init(testProject,
		&backend.Config{
			RootDir:         tempDir,
			Cache:           false,
			Credentials:     credentials,
			LogLevel:        logger.DETAIL,
			AerolabVersion:  aerolabVersion,
			ListAllProjects: false,
		},
		false)
	Expect(err).NotTo(HaveOccurred())

	err = testBackend.AddRegion(backend.BackendTypeAWS, options.TestRegions...)
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

	inv, err := testBackend.GetInventory()
	Expect(err).NotTo(HaveOccurred())

	err = inv.Instances.Terminate(time.Minute * 10)
	Expect(err).NotTo(HaveOccurred())
}
