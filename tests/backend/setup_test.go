//go:build integration_cloud

package backend_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aerospike-community/aerolab/pkg/backend"
	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds/bgcp"
	"github.com/aerospike-community/aerolab/pkg/utils/callerip"
	"github.com/rglonek/logger"
	"github.com/stretchr/testify/require"
)

// setup
var (
	testProject    string = "aerolab-test"
	tempDir        string
	aerolabVersion string = "v0.0.0"
	Options        *BackendTestOptions
	cloud          string
	podman         bool
	testBackend    backends.Backend
	backendType    backends.BackendType
	// testCredentials are the credentials the backend under test was built
	// with. Tests that have to inspect a cloud resource the backends interface
	// does not expose (the DNS test reads the hosted zone directly) build their
	// own client from these, so they authenticate exactly as the backend does.
	testCredentials *clouds.Credentials
)

type BackendTestOptions struct {
	TestRegions []string
	// Put test options here
	SkipCleanup bool
	TempDir     string
	// GCPUseIAP routes all SSH/SFTP traffic through IAP TCP forwarding
	// (AEROLAB_GCP_USE_IAP). Set it when the target project only reaches
	// instances that way, so every GCP test exercises the IAP path.
	GCPUseIAP bool
	// GCPNoPublicIP creates every GCP test instance without a public IP
	// (AEROLAB_GCP_NO_PUBLIC_IP). The project needs Cloud NAT for egress.
	GCPNoPublicIP bool
	// GCPAuthMethod selects how the GCP backend authenticates
	// (AEROLAB_GCP_AUTH_METHOD: any|login|service-account). It defaults to
	// service-account, matching the CLI, so the suite runs off Application
	// Default Credentials. The login method is the interactive browser flow
	// and only works with an OAuth client id in AEROLAB_GCP_CLIENT_ID.
	GCPAuthMethod clouds.GCPAuthMethod
	// GCPLoginSecrets carries the OAuth client id/secret for the login auth
	// method (AEROLAB_GCP_CLIENT_ID, AEROLAB_GCP_CLIENT_SECRET); nil when
	// unset, which is what every other auth method wants.
	GCPLoginSecrets *clouds.LoginGCPSecrets
	// GCPAutoEnableServices lets the backend enable the Google Cloud services
	// it needs (compute, iap when GCPUseIAP is set, ...) rather than refusing
	// to run. It defaults to on because the suite is non-interactive and the
	// prompt-free alternative is a hard failure at setup; set
	// AEROLAB_GCP_AUTO_ENABLE_SERVICES=0 to enable them by hand instead.
	GCPAutoEnableServices bool
}

func (o *BackendTestOptions) Validate() error {
	var err error

	if os.Getenv("AEROLAB_CLOUD") == "" {
		return errors.New("AEROLAB_CLOUD environment variable not set")
	}
	cloud = os.Getenv("AEROLAB_CLOUD")
	if cloud == "podman" {
		cloud = "docker"
		podman = true
	}

	switch cloud {
	case "aws":
		if os.Getenv("AWS_PROFILE") == "" {
			return errors.New("AWS_PROFILE environment variable not set")
		}
		backendType = backends.BackendTypeAWS
	case "gcp":
		if os.Getenv("GCP_PROJECT") == "" {
			return errors.New("GCP_PROJECT environment variable not set")
		}
		backendType = backends.BackendTypeGCP
		if err := lookupBoolEnv("AEROLAB_GCP_USE_IAP", &o.GCPUseIAP); err != nil {
			return err
		}
		if err := lookupBoolEnv("AEROLAB_GCP_NO_PUBLIC_IP", &o.GCPNoPublicIP); err != nil {
			return err
		}
		o.GCPAutoEnableServices = true
		if err := lookupBoolEnv("AEROLAB_GCP_AUTO_ENABLE_SERVICES", &o.GCPAutoEnableServices); err != nil {
			return err
		}
		o.GCPAuthMethod = clouds.GCPAuthMethod(getenvDefault("AEROLAB_GCP_AUTH_METHOD", clouds.GCPAuthMethodServiceAccount))
		switch o.GCPAuthMethod {
		case clouds.GCPAuthMethodServiceAccount, clouds.GCPAuthMethodLogin, clouds.GCPAuthMethodAny:
		default:
			return errors.New("AEROLAB_GCP_AUTH_METHOD must be one of any|login|service-account, got: " + string(o.GCPAuthMethod))
		}
		if clientID := os.Getenv("AEROLAB_GCP_CLIENT_ID"); clientID != "" {
			o.GCPLoginSecrets = &clouds.LoginGCPSecrets{
				ClientID:     clientID,
				ClientSecret: os.Getenv("AEROLAB_GCP_CLIENT_SECRET"), // can be empty for the PKCE flow
			}
		} else if o.GCPAuthMethod == clouds.GCPAuthMethodLogin {
			return errors.New("AEROLAB_GCP_AUTH_METHOD=login requires an OAuth client id in AEROLAB_GCP_CLIENT_ID")
		}
	case "docker":
		backendType = backends.BackendTypeDocker
	}

	if value, isSet := os.LookupEnv("AEROLAB_SKIP_CLEANUP"); isSet {
		o.SkipCleanup, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	}

	if value, isSet := os.LookupEnv("AEROLAB_" + strings.ToUpper(cloud) + "_TEST_REGIONS"); isSet && value != "" {
		o.TestRegions = strings.Split(value, ",")
	} else {
		return errors.New("AEROLAB_" + strings.ToUpper(cloud) + "_TEST_REGIONS environment variable not set")
	}

	if value := os.Getenv("AEROLAB_TEST_CUSTOM_TMPDIR"); value != "" {
		o.TempDir = value
	}

	return nil
}

// lookupBoolEnv sets dst from the named environment variable, leaving it
// untouched when the variable is unset or empty.
func lookupBoolEnv(name string, dst *bool) error {
	value, isSet := os.LookupEnv(name)
	if !isSet || value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = parsed
	return nil
}

// getenvDefault returns the environment value for key, or def if unset/empty.
func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// awsArchInstanceType and gcpArchInstanceType return an instance type matching
// the CPU architecture under test. ARM images do not boot on the x86 types the
// rest of the suite uses, so the arch tests must pick per-architecture types.
func awsArchInstanceType(arch backends.Architecture) string {
	if arch == backends.ArchitectureARM64 {
		return getenvDefault("AEROLAB_TEST_AWS_ARM_INSTANCE_TYPE", "r6g.large")
	}
	return getenvDefault("AEROLAB_TEST_AWS_INSTANCE_TYPE", "r6a.large")
}

func gcpArchInstanceType(arch backends.Architecture) string {
	if arch == backends.ArchitectureARM64 {
		return getenvDefault("AEROLAB_TEST_GCP_ARM_INSTANCE_TYPE", "t2a-standard-4")
	}
	return getenvDefault("AEROLAB_TEST_GCP_INSTANCE_TYPE", "e2-standard-4")
}

// instanceReadyWait is how long a create waits for its instances to become
// ssh-reachable. Docker containers are up immediately, but a cloud VM boots on
// its own schedule: a GCP instance with no public IP, reached over an IAP
// tunnel, regularly needs more than two minutes from create to sshd accepting
// connections. The CLI allows itself ten minutes and up for the same wait, so
// the suite is not testing anything real by insisting on a tighter budget.
func instanceReadyWait() time.Duration {
	if cloud == "docker" {
		return 2 * time.Minute
	}
	return 5 * time.Minute
}

// execConnectTimeout bounds a single exec's connect: the dial plus the SSH
// handshake. Docker answers on loopback, but a cloud instance reached over an
// IAP tunnel occasionally needs more than ten seconds to finish the handshake,
// so the cloud budget matches the 30s the CLI gives itself everywhere rather
// than a tighter number that only usually works.
func execConnectTimeout() time.Duration {
	if cloud == "docker" {
		return 10 * time.Second
	}
	return 30 * time.Second
}

// execMaxRetries and execRetrySleep give the suite's execs the same tolerance
// for a transient SSH failure that the CLI gives itself: every aerolab exec
// path defaults to --max-retries 1 / --retry-sleep 5s, while these tests set
// neither and so got a single attempt.
//
// That made the suite stricter than the product it tests. A freshly created
// instance is exactly where a redial pays off -- one blip on the first
// connection after create is normal, sshd having just come up (and on GCP,
// with an IAP tunnel in front of it) -- and aerolab's own readiness loop
// reaches the same instance by retrying once a second. A test that takes the
// single-attempt reading and fails is reporting the blip, not a defect.
func execMaxRetries() int { return 1 }

func execRetrySleep() time.Duration { return 5 * time.Second }

// gcpParams applies the suite-wide GCP options to a set of create-instance
// params. Every GCP instance the suite creates goes through here so a project
// that requires private-only instances is tested the way it is actually used.
func gcpParams(p *bgcp.CreateInstanceParams) *bgcp.CreateInstanceParams {
	if Options != nil {
		p.DisablePublicIP = Options.GCPNoPublicIP
	}
	return p
}

func setup(fresh bool) (err error) {
	if Options != nil {
		return nil // already setup
	}
	Options = &BackendTestOptions{}
	// A half-finished setup must not look like a completed one: the "already
	// setup" short-circuit above would hand the next subtest a nil testBackend,
	// which then panics and takes the whole test binary down with it, hiding
	// the real error.
	defer func() {
		if err != nil {
			Options = nil
			testBackend = nil
		}
	}()
	err = Options.Validate()
	if err != nil {
		return err
	}

	if Options.TempDir == "" {
		tempDir, err = os.MkdirTemp("", testProject)
		if err != nil {
			return err
		}
	} else {
		tempDir = Options.TempDir
		os.MkdirAll(tempDir, 0755) //nolint:errcheck
	}
	if Options.SkipCleanup {
		fmt.Printf("Skipping cleanup, tempDir=%s\n", tempDir)
	}

	credentials := &clouds.Credentials{
		AWS: clouds.AWS{
			AuthMethod: clouds.AWSAuthMethodShared,
		},
		GCP: clouds.GCP{
			Project:            os.Getenv("GCP_PROJECT"),
			AuthMethod:         Options.GCPAuthMethod,
			UseIAP:             Options.GCPUseIAP,
			AutoEnableServices: Options.GCPAutoEnableServices,
			Login: clouds.LoginGCPConfig{
				Secrets:            Options.GCPLoginSecrets,
				Browser:            true,
				TokenCacheFilePath: filepath.Join(tempDir, "gcp_token.json"),
			},
		},
		DOCKER: clouds.DOCKER{
			EnableDefaultFromEnv: true,
		},
	}
	testCredentials = credentials

	var btype backends.BackendType
	switch cloud {
	case "aws":
		btype = backends.BackendTypeAWS
	case "gcp":
		btype = backends.BackendTypeGCP
	case "docker":
		btype = backends.BackendTypeDocker
	default:
		return errors.New("invalid cloud: " + cloud)
	}
	_ = btype // reserved for backend selection if needed

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
			// Behave like the CLI: instances get a per-user firewall locked to
			// the address these tests run from, so the tests can SSH in.
			Identity: &backends.Identity{
				Owner:       "test-owner",
				CallerCidrs: callerip.Resolve,
				Autolock:    true,
			},
		},
		false, []backends.BackendType{btype}, nil)
	if err != nil {
		return err
	}
	err = testBackend.AddRegion(btype, Options.TestRegions...)
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
	if testBackend == nil {
		// setup() never got as far as building a backend; there is nothing to
		// clean up, and dereferencing it here would panic inside t.Cleanup and
		// take the run down instead of reporting the setup failure.
		log.Print("BACKEND NOT INITIALIZED, NOTHING TO CLEAN UP")
		return nil
	}
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

	err = testBackend.ExpiryRemove(backendType, expiryRegions...)
	if err != nil {
		return err
	}

	log.Print("CLEANED UP BACKEND")
	return nil
}

func cleanup() {
	var skipCleanup bool
	var err error
	if value, isSet := os.LookupEnv("AEROLAB_SKIP_CLEANUP"); isSet {
		skipCleanup, err = strconv.ParseBool(value)
		if err != nil {
			panic(err)
		}
	}

	if !skipCleanup && (Options == nil || !Options.SkipCleanup) {
		cleanupBackend() //nolint:errcheck
		os.RemoveAll(tempDir)
		// setup() short-circuits while Options is set, so without this the next
		// top-level Test function would keep using a backend whose root dir
		// (and GCP token cache) has just been deleted.
		Options = nil
		testBackend = nil
		tempDir = ""
		return
	}
	if Options != nil {
		Options.SkipCleanup = skipCleanup
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
	// Networks aerolab did not create belong to the operator. Docker's are
	// fixed (bridge/host/none), but a cloud account can hold any number of
	// pre-existing VPCs, so all this can honestly assert is that there is at
	// least one to deploy into; the aerolab-managed count above is the part
	// that says the inventory is clean.
	if cloud == "docker" && !podman {
		require.Equal(t, inventory.Networks.WithAerolabManaged(false).Count(), 3)
	} else {
		require.GreaterOrEqual(t, inventory.Networks.WithAerolabManaged(false).Count(), 1)
	}
	require.Equal(t, inventory.Firewalls.Count(), 0)
	require.Equal(t, inventory.Images.WithInAccount(true).Count(), 0)
	require.GreaterOrEqual(t, inventory.Images.WithInAccount(false).Count(), 20)
	expiries, err := testBackend.ExpiryList()
	require.NoError(t, err)
	require.Equal(t, len(expiries.ExpirySystems), 0)
}

func testInventoryPrint(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	inv := testBackend.GetInventory()
	j, err := json.MarshalIndent(inv, "", "  ")
	require.NoError(t, err)
	fmt.Printf("%s\n", string(j))
}

func testExpiriesPrint(t *testing.T) {
	require.NoError(t, setup(false))
	require.NoError(t, testBackend.RefreshChangedInventory())
	expiries, err := testBackend.ExpiryList()
	require.NoError(t, err)
	j, err := json.MarshalIndent(expiries, "", "  ")
	require.NoError(t, err)
	fmt.Printf("%s\n", string(j))
}
