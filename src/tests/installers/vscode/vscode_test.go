package vscode_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/vscode"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

func TestVscodeLatestUbuntu24(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := vscode.GetLinuxInstallScript(false, false, nil, nil, nil, nil, false, nil, "/root", "root")
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

func TestVscodeLatestCentos8(t *testing.T) {
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)

	script, err := vscode.GetLinuxInstallScript(false, false, installers.String("testpw"), installers.String("0.0.0.0:8080"), []string{"golang.go"}, []string{"some-does-not-exist"}, true, installers.String("/opt"), "/root", "root")
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
