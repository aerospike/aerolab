package main

import (
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
		if reflect.DeepEqual(element, s.Index(i).Interface()) {
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
	if name == "docker" || name == "" {
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
	return nil, fmt.Errorf(ERR_BACKEND_UNSUPPORTED, name)
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
func checkExecRetcode(err error) int {
	if err != nil {
		exiterr, ok := err.(*exec.ExitError)
		if !ok {
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

func cutSuffix(line string, pos int, split string) string {
	p := 0
	ret := ""
	for _, v := range strings.Split(line, split) {
		if v != "" {
			p = p + 1
		}
		if p >= pos {
			ret = ret + " " + v
		}
	}
	return ret
}

func chDir(dir string) (int64, error) {
	if dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return 1, fmt.Errorf("Working directory '%s' does not exist", dir)
		}
		err := os.Chdir(dir)
		if err != nil {
			return 1, fmt.Errorf("Could not change to working directory '%s'", dir)
		}
	}
	return 0, nil
}
