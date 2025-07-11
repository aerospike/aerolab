package grafana_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/aerospike/aerolab/pkg/utils/installers/grafana"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

func TestGrafanaLatestUbuntu24(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := grafana.GetInstallScript("", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	img := "amd64/ubuntu:24.04"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}

func TestGrafanaLatestCentos8(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := grafana.GetInstallScript("", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)

	img := "quay.io/centos/amd64:stream8"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}

func TestGrafanaVersioned(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := grafana.GetInstallScript("10.4.19", false, false)
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	require.Contains(t, string(script), "10.4.19")
	img := "amd64/ubuntu:24.04"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}
