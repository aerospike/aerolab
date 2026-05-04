package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// tempIngestDirs returns a YAML fragment that points every ingest
// directory at a subdirectory of root. Without this the tests
// inherit the defaults ("ingest/files/logs" etc.) which are relative
// to the process CWD and therefore (a) fail fast with "lstat
// ingest/files/logs: no such file or directory" when the test is
// run from a clean checkout and (b) leak state into the working
// directory when they don't fail.
func tempIngestDirs(t *testing.T, root string) string {
	t.Helper()
	for _, sub := range []string{"logs", "logs-cut", "collectinfo", "input", "other", "progress"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %s", sub, err)
		}
	}
	return fmt.Sprintf(
		"directories:\n  logs: %q\n  noStatOut: %q\n  collectInfo: %q\n  dirtyTemp: %q\n  otherFiles: %q\nprogressFile:\n  outputFilePath: %q\n",
		filepath.Join(root, "logs"),
		filepath.Join(root, "logs-cut"),
		filepath.Join(root, "collectinfo"),
		filepath.Join(root, "input"),
		filepath.Join(root, "other"),
		filepath.Join(root, "progress")+string(os.PathSeparator),
	)
}

func TestAll(t *testing.T) {
	os.Remove("cpu.pprof")
	os.RemoveAll("ingest")
	// t.Setenv is auto-restored at end of test; previous Setenv
	// (without restore) leaked LOGINGEST_CPUPROFILE_FILE into
	// TestPart, which then failed at "cpu profiling already in use"
	// because TestAll's pprof was never stopped (its t.Fatal short-
	// circuited Close).
	t.Setenv("LOGINGEST_CPUPROFILE_FILE", "cpu.pprof")
	t.Log("Setting up config")
	t.Setenv("LOGINGEST_LOGLEVEL", "6")
	t.Setenv("LOGINGEST_S3SOURCE_ENABLED", "false")
	t.Setenv("LOGINGEST_SFTPSOURCE_ENABLED", "true")
	t.Setenv("LOGINGEST_S3SOURCE_REGION", "ca-central-1")
	t.Setenv("LOGINGEST_S3SOURCE_PATH", "logs/")
	t.Setenv("LOGINGEST_S3SOURCE_REGEX", "^.*\\.tgz")
	t.Setenv("LOGINGEST_SFTPSOURCE_PORT", "22")
	t.Setenv("LOGINGEST_SFTPSOURCE_PATH", "withhist.tgz")
	//os.Setenv("LOGINGEST_SFTPSOURCE_REGEX", "^.*\\.tgz")
	t.Log("Creating a config")
	root := t.TempDir()
	yamlConfig := fmt.Sprintf("db:\n  path: %q\n%s", filepath.Join(root, "db"), tempIngestDirs(t, root))
	config, err := MakeConfigReader(true, strings.NewReader(yamlConfig), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Init ingest system")
	i, err := Init(config)
	if err != nil {
		t.Fatal(err)
	}
	// Always close so the CPU profiler is released, even on early
	// failure — otherwise pprof remains "in use" and any subsequent
	// test that turns CPU profiling on will fail.
	defer i.Close()
	t.Log("Download logs")
	err = i.Download()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Unpacking")
	err = i.Unpack()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("PreProcess")
	err = i.PreProcess()
	if err != nil {
		t.Fatal(err)
	}
	nerr := []error{}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		foundLogs, meta, err := i.ProcessLogsPrep()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessLogsPrep: %s", err))
			return
		}
		err = i.ProcessLogs(foundLogs, meta)
		if err != nil {
			nerr = append(nerr, fmt.Errorf("processLogs: %s", err))
		}
	}()
	go func() {
		defer wg.Done()
		err := i.ProcessCollectInfo()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("processCollectInfo: %s", err))
		}
	}()
	wg.Wait()
	t.Log("Cleanup")
	i.Close()
	if len(nerr) > 0 {
		for _, e := range nerr {
			t.Log(e)
		}
		t.Fatal("Errors Encountered")
	}
}

func TestPart(t *testing.T) {
	t.Log("Setting up config")
	// Ensure CPU profiling is off for this test even if a previous
	// test leaked the env var (TestAll uses t.Setenv now, but be
	// defensive against `go test -run TestPart` ordering).
	os.Unsetenv("LOGINGEST_CPUPROFILE_FILE")
	t.Setenv("LOGINGEST_LOGLEVEL", "6")
	t.Setenv("LOGINGEST_S3SOURCE_ENABLED", "true")
	t.Setenv("LOGINGEST_SFTPSOURCE_ENABLED", "true")
	t.Setenv("LOGINGEST_S3SOURCE_REGION", "ca-central-1")
	t.Setenv("LOGINGEST_S3SOURCE_PATH", "logs/")
	t.Setenv("LOGINGEST_S3SOURCE_REGEX", "^.*\\.tgz")
	t.Setenv("LOGINGEST_SFTPSOURCE_PORT", "22")
	t.Setenv("LOGINGEST_SFTPSOURCE_PATH", "withhist.tgz")
	t.Log("Creating a config")
	root := t.TempDir()
	yamlConfig := fmt.Sprintf("db:\n  path: %q\n%s", filepath.Join(root, "db"), tempIngestDirs(t, root))
	config, err := MakeConfigReader(true, strings.NewReader(yamlConfig), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Init ingest system")
	i, err := Init(config)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	nerr := []error{}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		foundLogs, meta, err := i.ProcessLogsPrep()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessLogsPrep: %s", err))
			return
		}
		err = i.ProcessLogs(foundLogs, meta)
		if err != nil {
			nerr = append(nerr, fmt.Errorf("processLogs: %s", err))
		}
	}()
	go func() {
		defer wg.Done()
		err := i.ProcessCollectInfo()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("processCollectInfo: %s", err))
		}
	}()
	wg.Wait()
	t.Log("Cleanup")
	i.Close()
	if len(nerr) > 0 {
		for _, e := range nerr {
			t.Log(e)
		}
		t.Fatal("Errors Encountered")
	}
}
