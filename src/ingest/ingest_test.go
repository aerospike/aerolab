package ingest

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func setupTest() error {
	os.Remove("cpu.pprof")
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

func TestAll(t *testing.T) {
	t.Log("Setting up")
	if err := setupTest(); err != nil {
		t.Log("FAIL: tearing down")
		teardownTest()
		t.Fatal(err)
	}
	defer func() {
		t.Log("Tearing down")
		if err := teardownTest(); err != nil {
			t.Fatal(err)
		}
	}()
	t.Log("Creating a config")
	config, err := MakeConfig(true, "", true)
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
	t.Log("Cleanup")
	i.Close()
}
