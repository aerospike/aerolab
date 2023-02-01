package main

import (
	"io"
	"os/exec"
	"syscall"
)

var backends = make(map[string]backend)

func addBackend(name string, back backend) {
	backends[name] = back
}

func getBackend() (backend, error) {
	return backends[a.opts.Config.Backend.Type], nil
}

type backendExtra struct {
	cpuLimit        string   // docker only
	ramLimit        string   // docker only
	swapLimit       string   // docker only
	privileged      bool     // docker only
	exposePorts     []string // docker only
	switches        string   // docker only
	dockerHostname  bool     // docker only
	ami             string   // aws only
	instanceType    string   // aws only
	ebs             string   // aws only
	securityGroupID string   // aws only
	subnetID        string   // aws only
	publicIP        bool     // aws only
	tags            []string // aws only
}

type backendVersion struct {
	distroName       string
	distroVersion    string
	aerospikeVersion string
	isArm            bool
}

type fileList struct {
	filePath     string
	fileContents io.ReadSeeker
	fileSize     int
}

type TypeArch int

var TypeArchUndef = TypeArch(0)
var TypeArchArm = TypeArch(1)
var TypeArchAmd = TypeArch(2)

type backend interface {
	// returns whether the given system is arm (using instanceType)
	IsSystemArm(systemType string) (bool, error)
	// check if given node is ARM or not
	IsNodeArm(clusterName string, nodeNumber int) (bool, error)
	// output which architecture MUST be used, or otherwise, Undef if both Arch are supported
	Arch() TypeArch
	// select to work on clients or servers
	WorkOnClients()
	WorkOnServers()
	// return slice of strings holding cluster names, or error
	ClusterList() ([]string, error)
	// accept cluster name, return slice of int holding node numbers or error
	NodeListInCluster(name string) ([]int, error)
	// return a slice of 'version' structs containing versions of templates available
	ListTemplates() ([]backendVersion, error)
	// deploy a template, naming it with version, running 'script' inside for installation and copying 'files' into it
	DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error
	// destroy template for a given version
	TemplateDestroy(v backendVersion) error
	// deploy cluster from template, requires version, name of new cluster and node count to deploy
	DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error
	// copy files to cluster, requires cluster name, list of files to copy and list of nodes in cluster to copy to
	CopyFilesToCluster(name string, files []fileList, nodes []int) error
	// run command(s) inside node(s) in cluster. Requires cluster name, commands as slice of command slices, and nodes list slice
	// returns a slice of byte slices containing each node/command output and error
	RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error)
	// returns a string slice containing IPs of given cluster name
	GetClusterNodeIps(name string) ([]string, error)
	// used to initialize the backend, for example check if docker is installed and install it if not on linux (error on mac)
	Init() error
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterStart(name string, nodes []int) error
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterStop(name string, nodes []int) error
	// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
	ClusterDestroy(name string, nodes []int) error
	// attach to a node in cluster and run a single command. does not return output of command.
	AttachAndRun(clusterName string, node int, command []string) (err error)
	// like AttachAndRun, but provide custom stdin, stdout and stderr the command should pipe to
	RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error)
	// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
	GetNodeIpMap(name string, internalIPs bool) (map[int]string, error)
	// return formatted for printing cluster list
	ClusterListFull(json bool) (string, error)
	// return formatted for printing template list
	TemplateListFull(json bool) (string, error)
	// upload files to node
	Upload(clusterName string, node int, source string, destination string, verbose bool) error
	// download files from node
	Download(clusterName string, node int, source string, destination string, verbose bool) error
	// delete dangling template creations
	VacuumTemplates() error
	VacuumTemplate(v backendVersion) error
	// may implement
	DeleteSecurityGroups(vpc string) error
	// may implement
	CreateSecurityGroups(vpc string) error
	// may implement
	LockSecurityGroups(ip string, lockSSH bool, vpc string) error
	// may implement
	ListSecurityGroups() error
	// may implement
	ListSubnets() error
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
