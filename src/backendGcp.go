package main

import (
	"io"
	"os"
)

func (d *backendGcp) Arch() TypeArch {
	return TypeArchUndef
}

type backendGcp struct {
	server bool
	client bool
}

func init() {
	addBackend("gcp", &backendGcp{})
}

const (
	// server
	gcpServerTagUsedBy           = "UsedBy"
	gcpServerTagUsedByValue      = "aerolab4"
	gcpServerTagClusterName      = "Aerolab4ClusterName"
	gcpServerTagNodeNumber       = "Aerolab4NodeNumber"
	gcpServerTagOperatingSystem  = "Aerolab4OperatingSystem"
	gcpServerTagOSVersion        = "Aerolab4OperatingSystemVersion"
	gcpServerTagAerospikeVersion = "Aerolab4AerospikeVersion"

	// client
	gcpClientTagUsedBy           = "UsedBy"
	gcpClientTagUsedByValue      = "aerolab4client"
	gcpClientTagClusterName      = "Aerolab4clientClusterName"
	gcpClientTagNodeNumber       = "Aerolab4clientNodeNumber"
	gcpClientTagOperatingSystem  = "Aerolab4clientOperatingSystem"
	gcpClientTagOSVersion        = "Aerolab4clientOperatingSystemVersion"
	gcpClientTagAerospikeVersion = "Aerolab4clientAerospikeVersion"
)

var (
	gcpTagUsedBy           = gcpServerTagUsedBy
	gcpTagUsedByValue      = gcpServerTagUsedByValue
	gcpTagClusterName      = gcpServerTagClusterName
	gcpTagNodeNumber       = gcpServerTagNodeNumber
	gcpTagOperatingSystem  = gcpServerTagOperatingSystem
	gcpTagOSVersion        = gcpServerTagOSVersion
	gcpTagAerospikeVersion = gcpServerTagAerospikeVersion
)

func (d *backendGcp) WorkOnClients() {
	d.server = false
	d.client = true
	gcpTagUsedBy = gcpClientTagUsedBy
	gcpTagUsedByValue = gcpClientTagUsedByValue
	gcpTagClusterName = gcpClientTagClusterName
	gcpTagNodeNumber = gcpClientTagNodeNumber
	gcpTagOperatingSystem = gcpClientTagOperatingSystem
	gcpTagOSVersion = gcpClientTagOSVersion
	gcpTagAerospikeVersion = gcpClientTagAerospikeVersion
}

func (d *backendGcp) WorkOnServers() {
	d.server = true
	d.client = false
	gcpTagUsedBy = gcpServerTagUsedBy
	gcpTagUsedByValue = gcpServerTagUsedByValue
	gcpTagClusterName = gcpServerTagClusterName
	gcpTagNodeNumber = gcpServerTagNodeNumber
	gcpTagOperatingSystem = gcpServerTagOperatingSystem
	gcpTagOSVersion = gcpServerTagOSVersion
	gcpTagAerospikeVersion = gcpServerTagAerospikeVersion
}

func (d *backendGcp) CreateNetwork(name string, driver string, subnet string, mtu string) error {
	return nil
}
func (d *backendGcp) DeleteNetwork(name string) error {
	return nil
}
func (d *backendGcp) PruneNetworks() error {
	return nil
}
func (d *backendGcp) ListNetworks(csv bool, writer io.Writer) error {
	return nil
}

func (d *backendGcp) Init() error {
}

func (d *backendGcp) IsSystemArm(systemType string) (bool, error) {
}

func (d *backendGcp) ClusterList() ([]string, error) {
}

func (d *backendGcp) IsNodeArm(clusterName string, nodeNumber int) (bool, error) {
}

func (d *backendGcp) NodeListInCluster(name string) ([]int, error) {
}

func (d *backendGcp) ListTemplates() ([]backendVersion, error) {
}

func (d *backendGcp) TemplateDestroy(v backendVersion) error {
}

func (d *backendGcp) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
}

func (d *backendGcp) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
}

func (d *backendGcp) GetClusterNodeIps(name string) ([]string, error) {
}

func (d *backendGcp) ClusterStart(name string, nodes []int) error {
}

func (d *backendGcp) ClusterStop(name string, nodes []int) error {
}

func (d *backendGcp) VacuumTemplate(v backendVersion) error {
}

func (d *backendGcp) VacuumTemplates() error {
}

func (d *backendGcp) ClusterDestroy(name string, nodes []int) error {
}

func (d *backendGcp) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendGcp) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
}

func (d *backendGcp) GetNodeIpMap(name string, internalIPs bool) (map[int]string, error) {
}

type gcpClusterListFull struct {
	ClusterName string
	NodeNumber  string
	IpAddress   string
	PublicIp    string
	InstanceId  string
	State       string
	Arch        string
}

func (d *backendGcp) ClusterListFull(isJson bool) (string, error) {
}

type gcpTemplateListFull struct {
	OsName           string
	OsVersion        string
	AerospikeVersion string
	ImageId          string
	Arch             string
}

func (d *backendGcp) TemplateListFull(isJson bool) (string, error) {
}

var deployGbpTemplateShutdownMaking = make(chan int, 1)

func (d *backendGcp) DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error {
}

func (d *backendGcp) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
}

func (d *backendGcp) Upload(clusterName string, node int, source string, destination string, verbose bool) error {
}

func (d *backendGcp) Download(clusterName string, node int, source string, destination string, verbose bool) error {
}

func (d *backendGcp) DeleteSecurityGroups(vpc string) error {
}

func (d *backendGcp) LockSecurityGroups(ip string, lockSSH bool, vpc string) error {
}

func (d *backendGcp) CreateSecurityGroups(vpc string) error {
}

func (d *backendGcp) ListSecurityGroups() error {
}

func (d *backendGcp) ListSubnets() error {
}
