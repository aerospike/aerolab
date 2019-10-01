package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
)

// what it says
func inArray(array interface{}, element interface{}) (index int) {
	index = -1
	s := reflect.ValueOf(array)
	for i := 0; i < s.Len(); i++ {
		if reflect.DeepEqual(element, s.Index(i).Interface()) == true {
			index = i
			return
		}
	}
	return
}

// versions struct for backend interface ListTemplates
type version struct {
	distroName       string
	distroVersion    string
	aerospikeVersion string
}

// list of files and contents - for putting in the cluster nodes
type fileList struct {
	filePath     string
	fileContents []byte
}

// get backend interface
func getBackend(name string, remote string, pubkey string) (backend, error) {
	// TODO: should remove this whole 'if' and just set the name to docker if name is ""
	if name == "" {
		// select backend based on uname of target system. Darwin = docker, Linux = lxc
		var out []byte
		var err error
		if remote == "" {
			out, err = exec.Command("/bin/bash", "-c", "uname").CombinedOutput()
		} else {
			out, err = remoteRun(strings.Split(remote, "@")[0], strings.Split(remote, "@")[1], pubkey, "uname")
		}
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Could not determine platform with uname: %s, %s", err, string(out)))
		}
		if strings.Contains(string(out), "Darwin") {
			name = "docker"
		} else {
			name = "docker"
		}
	}
	if name == "lxc" {
		var g backend
		if remote != "" {
			var r b_lxc
			g = r.ConfigRemote(remote, pubkey)
		} else {
			g = b_lxc{}
		}
		g, err := g.Init()
		return g, err
	} else if name == "docker" {
		var g backend
		if remote != "" {
			var r b_docker
			g = r.ConfigRemote(remote, pubkey)
		} else {
			g = b_docker{}
		}
		g, err := g.Init()
		return g, err
	} else if name == "aws" {
		var g backend
		var r b_aws
		g = r.ConfigRemote(remote, pubkey)
		g, err := g.Init()
		return g, err
	}
	return nil, errors.New(fmt.Sprintf(ERR_BACKEND_UNSUPPORTED, name))
}

// define backend interface
type backend interface {
	// return slice of strings holding cluster names, or error
	ClusterList() ([]string, error)
	// accept cluster name, return slice of int holding node numbers or error
	NodeListInCluster(name string) ([]int, error)
	// return a slice of 'version' structs containing versions of templates available
	ListTemplates() ([]version, error)
	// deploy a template, naming it with version, running 'script' inside for installation and copying 'files' into it
	DeployTemplate(v version, script string, files []fileList) error
	// destroy template for a given version
	TemplateDestroy(v version) error
	// deploy cluster from template, requires version, name of new cluster and node count to deploy
	DeployCluster(v version, name string, nodeCount int, exposePorts []string) error
	// deploy cluster from template, requires version, name of new cluster and node count to deploy. accept and use limits
	DeployClusterWithLimits(v version, name string, nodeCount int, exposePorts []string, cpuLimit string, ramLimit string, swapLimit string, privileged bool) error
	// copy files to cluster, requires cluster name, list of files to copy and list of nodes in cluster to copy to
	CopyFilesToCluster(name string, files []fileList, nodes []int) error
	// run command(s) inside node(s) in cluster. Requires cluster name, commands as slice of command slices, and nodes list slice
	// returns a slice of byte slices containing each node/command output and error
	RunCommand(clusterName string, commands [][]string, nodes []int) ([][]byte, error)
	// returns a string slice containing IPs of given cluster name
	GetClusterNodeIps(name string) ([]string, error)
	// used by backend to configure itself for remote access. e.g. store b.user, b.host, b.pubkey from given parameters
	ConfigRemote(host string, pubkey string) backend
	// used to initialize the backend, for example check if docker is installed and install it if not on linux (error on mac)
	Init() (backend, error)
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterStart(name string, nodes []int) error
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterStop(name string, nodes []int) error
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterDestroy(name string, nodes []int) error
	// returns an unformatted string with list of clusters, to be printed to user
	ClusterListFull() (string, error)
	// attach to a node in cluster and run a single command. does not return output of command.
	AttachAndRun(clusterName string, node int, command []string) (err error)
	// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
	GetNodeIpMap(name string) (map[int]string, error)
	// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
	GetNodeIpMapInternal(name string) (map[int]string, error)
	// get Backend type
	GetBackendName() string
}

// check return code from exec function
func check_exec_retcode(err error) int {
	if err != nil {
		exiterr, ok := err.(*exec.ExitError)
		if ok == false {
			return 666
		}
		return exiterr.Sys().(syscall.WaitStatus).ExitStatus()
	}
	return 0
}

func cut(line string, pos int, split string) string {
	p := 0
	for _, v := range strings.Split(line, split) {
		if v != "" {
			p = p + 1
		}
		if p == pos {
			return v
		}
	}
	return ""
}

func chDir(dir string) (error, int64) {
	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return errors.New(fmt.Sprintf("Working directory '%s' does not exist", dir)), 1
		}
		err := os.Chdir(dir)
		if err != nil {
			return errors.New(fmt.Sprintf("Could not change to working directory '%s'", dir)), 1
		}
	}
	return nil, 0
}

func makeError(format string, args ...interface{}) error {
	return errors.New(fmt.Sprintf(format, args...))
}
