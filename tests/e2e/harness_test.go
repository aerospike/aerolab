//go:build integration_docker || integration_cloud || integration_aerospike_cloud

// Package e2e_test contains end-to-end integration tests that drive the real
// aerolab binary. They replace the legacy tests/cli/test.sh suite.
//
// There are three tiers, selected by build tag:
//
//   - integration_docker : Docker-backend tests, CI-runnable on any Linux/mac
//     host with Docker. Run with `make test-docker` or
//     `go test -tags=integration_docker ./tests/e2e/...`.
//   - integration_cloud  : AWS/GCP backend tests. These need real cloud
//     credentials and are opt-in/manual. Run with `make test-cloud` or
//     `go test -tags=integration_cloud ./tests/e2e/...`.
//   - integration_aerospike_cloud : the `aerolab cloud ...` commands, which
//     drive the Aerospike Cloud API (plus the AWS calls those commands make
//     for VPC peering and log access). Provisions real, billable managed
//     clusters. Run with `make test-aerospike-cloud` or
//     `go test -tags=integration_aerospike_cloud ./tests/e2e/...`.
//
// The Aerospike Cloud tier is deliberately its own tag rather than a switch
// inside the cloud tier: it costs money per run, takes tens of minutes, and
// targets a service the AWS/GCP backend tests never touch, so it has to be
// startable (and re-runnable) on its own.
//
// Requirements:
//   - The aerolab binary. By default the harness builds it once from source into
//     a temp dir; set AEROLAB_BIN=/path/to/aerolab to use a prebuilt binary.
//   - Docker tier: a running Docker daemon (tests skip if `docker` is missing or
//     the daemon is unreachable).
//   - Aerospike Cloud tier: AEROSPIKE_CLOUD_KEY / AEROSPIKE_CLOUD_SECRET (the
//     API credentials aerolab itself requires) and AWS credentials for the
//     region under test. Tests skip when the API credentials are absent.
//
// Optional environment overrides (no hardcoded machine-specific paths). Paths
// must be absolute: the harness runs aerolab from a per-test working directory
// so that nothing it writes to a relative path escapes into the source tree.
//   - AEROLAB_BIN                    prebuilt aerolab binary to use instead of building
//   - AEROLAB_FEATURES_FILE          path to an Aerospike features file (needed for
//     Enterprise images); wired via `config defaults -k '*.FeaturesFilePath'`
//   - AEROLAB_E2E_DISTRO             base distro (default: ubuntu)
//   - AEROLAB_E2E_DISTRO_VER         distro version (default: 24.04)
//   - AEROLAB_E2E_ASVER              Aerospike version selector (default: 8.*)
//   - AEROLAB_E2E_OS_MATRIX          set to run the multi-distro OS matrix test
//   - AEROLAB_E2E_EXTENDED           set to run the TLS/XDR/data/net/client tests
//   - AEROLAB_E2E_AWS_REGION         AWS region for the cloud tier (default: us-east-1)
//   - AEROLAB_E2E_AWS_PROFILE        AWS shared-credentials profile (default: AWS_PROFILE)
//   - AEROLAB_E2E_MIGRATE            set to run the inventory migrate dry-run test
//   - AEROLAB_E2E_SSH_KEY_PATH       ssh key dir passed to `inventory migrate`
//
// Aerospike Cloud tier only:
//   - AEROSPIKE_CLOUD_KEY            Aerospike Cloud API key (required)
//   - AEROSPIKE_CLOUD_SECRET         Aerospike Cloud API secret (required)
//   - AEROSPIKE_CLOUD_ENV            set to "dev" to target the dev control plane
//   - AEROLAB_ASCLOUD_REGION         region the managed cluster is created in
//     (default: us-west-2, deliberately not the AWS tier's region)
//   - AEROLAB_ASCLOUD_AWS_PROFILE    AWS profile for the VPC peering / log access
//     calls (default: AWS_PROFILE)
//   - AEROLAB_ASCLOUD_VPC_ID         VPC to peer the cluster into (default:
//     "default", which exercises aerolab's default-VPC resolution)
//   - AEROLAB_ASCLOUD_INSTANCE_TYPE  instance type to create with (default: m5d.large)
//   - AEROLAB_ASCLOUD_UPDATE_INSTANCE_TYPE instance type to scale up to
//     (default: m5d.xlarge)
package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// aerolabBinary returns the path to an aerolab binary, building it once per test
// run if AEROLAB_BIN is not provided. On build failure it returns the error so
// callers can skip.
func aerolabBinary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("AEROLAB_BIN"); b != "" {
		return b
	}
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "aerolab-e2e-bin-")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "aerolab")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		// Build the CLI main package. Inherit the caller's environment but force
		// vendored, workspace-off builds to match the Makefile.
		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "github.com/citrusleaf/aerolab/cli")
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=vendor")
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildFailure{output: string(b), err: err}
			return
		}
		builtBin = out
	})
	if buildErr != nil {
		t.Skipf("aerolab binary unavailable (set AEROLAB_BIN to skip building): %v", buildErr)
	}
	return builtBin
}

type buildFailure struct {
	output string
	err    error
}

func (b *buildFailure) Error() string { return b.err.Error() + "\n" + b.output }

// requireDocker skips the test unless a reachable Docker daemon is available.
func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker binary not found in PATH; skipping Docker e2e test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not reachable; skipping Docker e2e test: %v\n%s", err, string(out))
	}
}

// cli is a configured aerolab command runner bound to an isolated AEROLAB_HOME
// and the docker backend.
type cli struct {
	t   *testing.T
	bin string
	env []string
	dir string
	// timeout bounds a single aerolab invocation. Zero means
	// defaultCommandTimeout. Exceeding it kills the subprocess, which surfaces
	// as "signal: killed" with no error from aerolab itself, so it has to be
	// set from what the slowest command in the tier legitimately needs rather
	// than from what the fast ones usually take.
	timeout time.Duration
}

// defaultCommandTimeout is the per-invocation budget for the docker and cloud
// tiers, where the longest command is a cluster create that provisions its own
// instances.
const defaultCommandTimeout = 30 * time.Minute

// aerospikeCloudCommandTimeout is the per-invocation budget for the Aerospike
// Cloud tier. `cloud clusters create` provisions a managed cluster and then
// peers its VPC, and its own progress message puts that at "up to an hour";
// `clusters delete` blocks until the cluster is decommissioned. Both routinely
// outlive defaultCommandTimeout, and being killed mid-create leaks a billable
// cluster plus the blackhole route create had installed to reserve its CIDR.
// Override with AEROLAB_ASCLOUD_CMD_TIMEOUT (any time.ParseDuration value).
const aerospikeCloudCommandTimeout = 120 * time.Minute

// newDockerCLI builds a runner: isolates AEROLAB_HOME to a temp dir, disables
// telemetry, sets the docker backend, and wires an optional features file.
func newDockerCLI(t *testing.T) *cli {
	t.Helper()
	requireDocker(t)
	bin := aerolabBinary(t)

	home := t.TempDir()
	env := append(os.Environ(),
		"AEROLAB_HOME="+home,
		"AEROLAB_TEST=1",
		"AEROLAB_TELEMETRY_DISABLE=1",
	)
	c := &cli{t: t, bin: bin, env: env, dir: t.TempDir()}

	c.run("config", "backend", "-t", "docker")
	if ff := os.Getenv("AEROLAB_FEATURES_FILE"); ff != "" {
		c.run("config", "defaults", "-k", "*.FeaturesFilePath", "-v", ff)
	}
	return c
}

// newAWSCLI builds a runner configured for the AWS backend: isolated
// AEROLAB_HOME, telemetry off, aws backend + region, optional features file.
//
// It applies no gate of its own. Every test that uses it is already opt-in
// through both a build tag and its own environment variable, and adding a
// second, tier-wide switch on top of those is how the migrate test ended up
// silently skipping in a run that had explicitly asked for it.
func newAWSCLI(t *testing.T) *cli {
	t.Helper()
	return newBackendCLI(t,
		getenvDefault("AEROLAB_E2E_AWS_REGION", "us-east-1"),
		getenvDefault("AEROLAB_E2E_AWS_PROFILE", os.Getenv("AWS_PROFILE")))
}

// newAerospikeCloudCLI builds a runner for the Aerospike Cloud tier. The
// `aerolab cloud ...` commands authenticate to the Aerospike Cloud API through
// AEROSPIKE_CLOUD_KEY / AEROSPIKE_CLOUD_SECRET, and separately use the AWS
// backend for the VPC peering and S3 log access they set up, so both sets of
// credentials have to be present. The region defaults away from the AWS tier's
// so the two can run against one account without meeting.
func newAerospikeCloudCLI(t *testing.T) *cli {
	t.Helper()
	if os.Getenv("AEROSPIKE_CLOUD_KEY") == "" || os.Getenv("AEROSPIKE_CLOUD_SECRET") == "" {
		t.Skip("set AEROSPIKE_CLOUD_KEY and AEROSPIKE_CLOUD_SECRET (with valid AWS credentials) to run the Aerospike Cloud tier")
	}
	c := newBackendCLI(t,
		aerospikeCloudRegion(),
		getenvDefault("AEROLAB_ASCLOUD_AWS_PROFILE", os.Getenv("AWS_PROFILE")))
	c.timeout = aerospikeCloudCommandTimeout
	if v := os.Getenv("AEROLAB_ASCLOUD_CMD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("AEROLAB_ASCLOUD_CMD_TIMEOUT=%q is not a duration: %v", v, err)
		}
		c.timeout = d
	}
	return c
}

// aerospikeCloudRegion is the region the Aerospike Cloud tier provisions in.
// The default is deliberately not the AWS tier's default: the two tiers share
// an AWS account, and keeping them in separate regions means the AWS suite's
// destructive cleanup (it terminates instances and deletes volumes, firewalls
// and images in its configured regions) cannot reach the VPC peering this tier
// sets up.
func aerospikeCloudRegion() string {
	return getenvDefault("AEROLAB_ASCLOUD_REGION", "us-west-2")
}

// newBackendCLI builds a runner bound to the aws backend in region, with an
// isolated AEROLAB_HOME and telemetry disabled.
func newBackendCLI(t *testing.T, region string, profile string) *cli {
	t.Helper()
	bin := aerolabBinary(t)

	home := t.TempDir()
	env := append(os.Environ(),
		"AEROLAB_HOME="+home,
		"AEROLAB_TEST=1",
		"AEROLAB_TELEMETRY_DISABLE=1",
	)
	c := &cli{t: t, bin: bin, env: env, dir: t.TempDir()}

	backendArgs := []string{"config", "backend", "-t", "aws", "-r", region}
	if profile != "" {
		backendArgs = append(backendArgs, "--aws.profile", profile)
	}
	c.run(backendArgs...)
	if ff := os.Getenv("AEROLAB_FEATURES_FILE"); ff != "" {
		c.run("config", "defaults", "-k", "*.FeaturesFilePath", "-v", ff)
	}
	return c
}

// withT returns a copy of the runner bound to t.
//
// A runner reports failures with t.Fatalf, which calls FailNow on whichever
// *testing.T it holds. A runner built in a parent test therefore has to be
// rebound before it is used inside a subtest: calling FailNow on the parent from
// the subtest's goroutine aborts the whole parent test (reported as "test
// executed panic(nil) or runtime.Goexit") and every remaining subtest never
// runs.
func (c *cli) withT(t *testing.T) *cli {
	t.Helper()
	clone := *c
	clone.t = t
	return &clone
}

// run executes aerolab with the given args, failing the test on non-zero exit.
// It returns the combined stdout+stderr.
//
// The output is not repeated in the failure message: runErr has already logged
// it, along with every other invocation's.
func (c *cli) run(args ...string) string {
	c.t.Helper()
	out, err := c.runErr(args...)
	if err != nil {
		c.t.Fatalf("aerolab %s failed: %v", strings.Join(args, " "), err)
	}
	return out
}

// runErr executes aerolab and returns the combined output and any error without
// failing the test, for cases where a non-zero exit is acceptable/expected.
//
// Every invocation is logged with its output and how long it took, whether it
// succeeded or not. Logging only failures reads as economical until something
// fails: the command that explains it is usually an earlier one that succeeded
// -- a cluster create reporting which CIDR it picked and which peering stages
// it ran, say -- and by then that output is gone. These runs take an hour and
// provision billable infrastructure, so reproducing them to get the log back is
// not a reasonable ask. The tiers all run `go test -v`, so this reaches the
// tier log; it does make the docker tier's log substantially larger, which is
// the intended trade.
func (c *cli) runErr(args ...string) (string, error) {
	c.t.Helper()
	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Env = c.env
	// Anything aerolab writes to a relative path lands here rather than in the
	// package source directory. `tls generate` is the one that matters: its
	// work dir defaults to the process working directory and it creates a CA
	// (private key included) there, reusing whatever CA it already finds. The
	// docker and cloud tiers are separate processes running the same package,
	// so with a shared cwd they would generate over each other's CA when run
	// concurrently, on top of leaving the key in the tree.
	cmd.Dir = c.dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	start := time.Now()
	err := cmd.Run()
	out := buf.String()

	status := "ok"
	if err != nil {
		status = "FAILED: " + err.Error()
	}
	c.t.Logf("$ aerolab %s [%s, %s]\n%s",
		strings.Join(args, " "), time.Since(start).Round(time.Second), status, out)

	return out, err
}

// getenvDefault returns the environment value for key or def if unset/empty.
func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
