package aerospike_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/software/aerospike"
	"github.com/lithammer/shortuuid"
	"github.com/stretchr/testify/require"
)

func toolsScript(t *testing.T, np string) aerospike.Files {
	products, err := aerospike.GetProducts(time.Second * 10)
	require.NoError(t, err)
	require.NotNil(t, products)
	require.NotEmpty(t, products)
	product := products.WithName("aerospike-tools")
	require.NotEmpty(t, product)

	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	require.NoError(t, err)
	require.NotNil(t, versions)
	require.NotEmpty(t, versions)
	if np != "" {
		versions = versions.WithNamePrefix(np)
	}
	version := versions.Latest()
	require.NotNil(t, version)

	files, err := aerospike.GetFiles(time.Second*10, *version)
	require.NoError(t, err)
	require.NotNil(t, files)
	require.NotEmpty(t, files)

	return files
}

func Test01_ToolsScriptDocker(t *testing.T) {
	files := toolsScript(t, "")
	d, e := files.GetInstallScript(aerospike.ArchitectureTypeX86_64, aerospike.OSName("centos"), "9", true, true, true, true)
	require.NoError(t, e)
	require.NotNil(t, d)
	require.NotEmpty(t, d)
	fmt.Println(string(d))
}

func Test01_ToolsScript(t *testing.T) {
	fmt.Println("Getting file list")
	files := toolsScript(t, "")
	img := []string{"quay.io/centos/amd64:stream8", "quay.io/centos/amd64:stream9", "amd64/rockylinux:8", "amd64/rockylinux:9", "amd64/ubuntu:20.04", "amd64/ubuntu:22.04", "amd64/ubuntu:24.04", "amd64/debian:11", "amd64/debian:12"}
	fos := []string{"centos", "centos", "centos", "centos", "ubuntu", "ubuntu", "ubuntu", "debian", "debian"}
	fver := []string{"8", "9", "8", "9", "20.04", "22.04", "24.04", "11", "12"}
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)
	for i, o := range img {
		fmt.Println("Running docker for ", o)
		script, err := files.GetInstallScript(aerospike.ArchitectureTypeX86_64, aerospike.OSName(fos[i]), fver[i], true, true, true, true)
		require.NoError(t, err)
		require.NotNil(t, script)
		require.NotEmpty(t, script)
		uuid := shortuuid.New()
		err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
		require.NoError(t, err)
		out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, o, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
		fmt.Println(string(out))
		require.NoError(t, err)
	}
}

func Test01_ToolsScriptOld(t *testing.T) {
	fmt.Println("Getting file list")
	files := toolsScript(t, "7.3.1")
	img := []string{"amd64/ubuntu:20.04"}
	fos := []string{"ubuntu"}
	fver := []string{"20.04"}
	os.RemoveAll("dockertest")
	defer os.RemoveAll("dockertest")
	os.MkdirAll("dockertest", 0755)
	for i, o := range img {
		fmt.Println("Running docker for ", o)
		script, err := files.GetInstallScript(aerospike.ArchitectureTypeX86_64, aerospike.OSName(fos[i]), fver[i], true, true, true, true)
		require.NoError(t, err)
		require.NotNil(t, script)
		require.NotEmpty(t, script)
		uuid := shortuuid.New()
		err = os.WriteFile(fmt.Sprintf("dockertest/%s.sh", uuid), script, 0755)
		require.NoError(t, err)
		out, err := exec.Command("docker", "run", "-v", "./dockertest:/mnt", "--rm", "-i", "--name", uuid, o, "/bin/bash", "-c", fmt.Sprintf("echo 'x' && ls /mnt && chmod +x /mnt/%s.sh && /mnt/%s.sh", uuid, uuid)).CombinedOutput()
		fmt.Println(string(out))
		require.NoError(t, err)
	}
}
