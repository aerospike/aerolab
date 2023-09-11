package ingest

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTest() error {
	os.Remove("cpu.pprof")
	os.RemoveAll("ingest")
	os.Setenv("LOGINGEST_CPUPROFILE_FILE", "cpu.pprof")
	os.Unsetenv("DOCKER_HOST")
	out, err := exec.Command("aerolab", "config", "backend", "-t", "docker").CombinedOutput()
	if err != nil {
		return fmt.Errorf("err: %s out: %s", err, string(out))
	}
	out, err = exec.Command("aerolab", "cluster", "create", "--name=ingest", "--expose-ports=3100:3100", "--aerospike-version=6.3.0.5", "--start=no", "--no-autoexpose").CombinedOutput()
	if err != nil {
		return fmt.Errorf("err: %s out: %s", err, string(out))
	}
	out, err = exec.Command("aerolab", "conf", "adjust", "--name=ingest", "set", "network.service.port", "3100").CombinedOutput()
	if err != nil {
		return fmt.Errorf("err: %s out: %s", err, string(out))
	}
	out, err = exec.Command("aerolab", "aerospike", "start", "--name=ingest").CombinedOutput()
	if err != nil {
		return fmt.Errorf("err: %s out: %s", err, string(out))
	}
	return nil
}

func teardownTest() error {
	out, err := exec.Command("aerolab", "cluster", "destroy", "-f", "--name=ingest").CombinedOutput()
	if err != nil {
		return fmt.Errorf("err: %s out: %s", err, string(out))
	}
	return nil
}

// TODO: convert this into a Run() function as a wrapper for do-it-all
func TestAll(t *testing.T) {
	t.Log("Tearing down")
	teardownTest()
	t.Log("Sleep 5 sec")
	time.Sleep(5 * time.Second)
	t.Log("Setting up")
	if err := setupTest(); err != nil {
		t.Fatal(err)
	}
	t.Log("Setting up config")
	os.Setenv("LOGINGEST_LOGLEVEL", "6")
	os.Setenv("LOGINGEST_S3SOURCE_ENABLED", "true")
	os.Setenv("LOGINGEST_SFTPSOURCE_ENABLED", "true")
	os.Setenv("LOGINGEST_S3SOURCE_REGION", "ca-central-1")
	//os.Setenv("LOGINGEST_S3SOURCE_BUCKET", "") // set outside
	//os.Setenv("LOGINGEST_S3SOURCE_KEYID", "") // set outside
	//os.Setenv("LOGINGEST_S3SOURCE_SECRET", "") // set outside
	os.Setenv("LOGINGEST_S3SOURCE_PATH", "logs/")
	os.Setenv("LOGINGEST_S3SOURCE_REGEX", "^.*\\.tgz")
	//os.Setenv("LOGINGEST_SFTPSOURCE_HOST", "") // set outside
	os.Setenv("LOGINGEST_SFTPSOURCE_PORT", "22")
	//os.Setenv("LOGINGEST_SFTPSOURCE_USER", "") // set outside
	//os.Setenv("LOGINGEST_SFTPSOURCE_PASSWORD", "") // set outside
	os.Setenv("LOGINGEST_SFTPSOURCE_PATH", "new.tgz")
	//os.Setenv("LOGINGEST_SFTPSOURCE_REGEX", "^.*\\.tgz")
	t.Log("Creating a config")
	yamlConfig := "aerospike:\n  namespace: \"test\""
	config, err := MakeConfigReader(true, strings.NewReader(yamlConfig), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Init ingest system")
	i, err := Init(config)
	if err != nil {
		t.Fatal(err)
	}
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
		err := i.ProcessLogs()
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
