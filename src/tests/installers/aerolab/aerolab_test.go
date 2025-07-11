package aerolab_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

func TestAerolabLatestUbuntu24(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(nil, nil)
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

func TestAerolabLatestCentos8(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(nil, nil)
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

func TestAerolabLatestStable(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(nil, installers.Bool(false))
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

func TestAerolabLatestPrelease(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(nil, installers.Bool(true))
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

func TestAerolabVersioned(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(installers.String("7.7.0"), installers.Bool(false))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	require.Contains(t, string(script), "7.7.0")

	img := "amd64/ubuntu:24.04"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}

func TestAerolabVersionPrefixed(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := aerolab.GetLinuxInstallScript(installers.String("7.7.*"), installers.Bool(false))
	require.NoError(t, err)
	require.NotNil(t, script)
	require.NotEmpty(t, script)
	require.Contains(t, string(script), "7.7.1")

	img := "amd64/ubuntu:24.04"
	uuid := shortuuid.New()
	err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
	require.NoError(t, err)
	out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, img, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
	_ = out
	//fmt.Println(string(out))
	require.NoError(t, err)
}
