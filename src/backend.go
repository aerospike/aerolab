package main

import (
	"encoding/json"
	"io"
	"os/exec"
	"syscall"
	"time"
)

var backends = make(map[string]backend)

func addBackend(name string, back backend) {
	backends[name] = back
}

func getBackend() (backend, error) {
	return backends[a.opts.Config.Backend.Type], nil
}

type backendExtra struct {
	clientType          string    // all: ams|elasticsearch|rest-gateway|VSCode|...
	cpuLimit            string    // docker only
	ramLimit            string    // docker only
	swapLimit           string    // docker only
	privileged          bool      // docker only
	exposePorts         []string  // docker only
	switches            []string  // docker only
	dockerHostname      bool      // docker only
	network             string    // docker only
	autoExpose          bool      // docker only
	securityGroupID     string    // aws only
	subnetID            string    // aws only
	ebs                 string    // aws only
	terminateOnPoweroff bool      // aws only
	spotInstance        bool      // aws only
	useFleet            bool      // aws only
	instanceType        string    // aws/gcp only
	ami                 string    // aws/gcp only
	publicIP            bool      // aws/gcp only
	tags                []string  // aws/gcp only
	firewallNamePrefix  []string  // aws/gcp only
	expiresTime         time.Time // aws/gcp only
	disks               []string  // gcp only
	zone                string    // gcp only
	labels              []string  // gcp only
	gcpMeta             map[string]string
}

type backendVersion struct {
	distroName       string
	distroVersion    string
	aerospikeVersion string
	isArm            bool
}

type fileList struct {
	filePath     string
	fileContents string
	fileSize     int
}

type fileListReader struct {
	filePath     string
	fileContents io.ReadSeeker
	fileSize     int
}

type TypeArch int

var TypeArchUndef = TypeArch(0)
var TypeArchArm = TypeArch(1)
var TypeArchAmd = TypeArch(2)

type backend interface {
	// aws efs volumes
	CreateVolume(name string, zone string, tags []string) error
	DeleteVolume(name string) error
	CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error)
	MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error
	GetAZName(subnetId string) (string, error)
	// cause gcp
	EnableServices() error
	// expiries calls
	ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error
	ExpiriesSystemRemove() error
	ExpiriesSystemFrequency(intervalMinutes int) error
	ClusterExpiry(zone string, clusterName string, expiry time.Duration, nodes []int) error
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
	DeployTemplate(v backendVersion, script string, files []fileListReader, extra *backendExtra) error
	// destroy template for a given version
	TemplateDestroy(v backendVersion) error
	// deploy cluster from template, requires version, name of new cluster and node count to deploy
	DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error
	// copy files to cluster, requires cluster name, list of files to copy and list of nodes in cluster to copy to
	CopyFilesToCluster(name string, files []fileList, nodes []int) error
	CopyFilesToClusterReader(name string, files []fileListReader, nodes []int) error
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
	ClusterListFull(json bool, owner string, noPager bool) (string, error)
	// return formatted for printing template list
	TemplateListFull(json bool, noPager bool) (string, error)
	// upload files to node
	Upload(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error
	// download files from node
	Download(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error
	// delete dangling template creations
	VacuumTemplates() error
	VacuumTemplate(v backendVersion) error
	// may implement
	DeleteSecurityGroups(vpc string, namePrefix string, internal bool) error
	// may implement
	CreateSecurityGroups(vpc string, namePrefix string) error
	// may implement
	LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string) error
	AssignSecurityGroups(clusterName string, names []string, vpcOrZone string, remove bool) error
	// may implement
	ListSecurityGroups() error
	// may implement
	ListSubnets() error
	// may implement (docker related mostly)
	CreateNetwork(name string, driver string, subnet string, mtu string) error
	DeleteNetwork(name string) error
	PruneNetworks() error
	ListNetworks(csv bool, writer io.Writer) error
	Inventory(owner string, inventoryItems []int) (inventoryJson, error)
	GetInstanceTypes(minCpu int, maxCpu int, minRam float64, maxRam float64, minDisks int, maxDisks int, findArm bool, gcpZone string) ([]instanceType, error)
	// docker: label, aws: tag, gcp: metadata
	SetLabel(clusterName string, key string, value string, gcpZone string) error
	// aws, gcp
	GetKeyPath(clusterName string) (keyPath string, err error)
}

type inventoryJson struct {
	Clusters      []inventoryCluster
	Clients       []inventoryClient
	Templates     []inventoryTemplate
	FirewallRules []inventoryFirewallRule
	Subnets       []inventorySubnet
	ExpirySystem  []inventoryExpiry
	Volumes       []inventoryVolume
}

type inventoryVolume struct {
	AvailabilityZoneId   string
	AvailabilityZoneName string
	CreationTime         time.Time
	CreationToken        string
	Encrypted            bool
	FileSystemArn        string
	FileSystemId         string
	LifeCycleState       string
	Name                 string
	NumberOfMountTargets int
	AWSOwnerId           string
	PerformanceMode      string
	ThroughputMode       string
	SizeBytes            int
	Tags                 map[string]string
	MountTargets         []inventoryMountTarget
}

type inventoryMountTarget struct {
	AvailabilityZoneId   string
	AvailabilityZoneName string
	FileSystemId         string
	IpAddress            string
	LifeCycleState       string
	MountTargetId        string
	NetworkInterfaceId   string
	AWSOwnerId           string
	SubnetId             string
	VpcId                string
	SecurityGroups       []string
}

type inventoryExpiry struct {
	IAMScheduler string
	IAMFunction  string
	Scheduler    string
	Function     string
	Schedule     string
	SourceBucket string
}

type inventorySubnet struct {
	AWS inventorySubnetAWS
}

type inventorySubnetAWS struct {
	VpcId            string
	VpcName          string
	VpcCidr          string
	AvailabilityZone string
	SubnetId         string
	SubnetCidr       string
	IsAzDefault      bool
	SubnetName       string
	AutoPublicIP     bool
}

type inventoryCluster struct {
	ClusterName            string
	NodeNo                 string
	PrivateIp              string
	PublicIp               string
	InstanceId             string
	ImageId                string
	State                  string
	Arch                   string
	Distribution           string
	OSVersion              string
	AerospikeVersion       string
	Firewalls              []string
	Zone                   string
	InstanceRunningCost    float64
	Owner                  string
	DockerExposePorts      string
	DockerInternalPort     string
	Expires                string
	AccessUrl              string
	Features               FeatureSystem
	AGILabel               string
	gcpLabelFingerprint    string
	gcpLabels              map[string]string
	gcpMetadataFingerprint string
	gcpMeta                map[string]string
	awsTags                map[string]string
	dockerLabels           map[string]string
	awsSubnet              string
	awsSecGroups           []string
}

type FeatureSystem int64

func (f *FeatureSystem) MarshalJSON() ([]byte, error) {
	resp := []string{}
	if *f&ClusterFeatureAerospike > 0 {
		resp = append(resp, "Aerospike")
	}
	if *f&ClusterFeatureAerospikeTools > 0 {
		resp = append(resp, "AerospikeTools")
	}
	if *f&ClusterFeatureAGI > 0 {
		resp = append(resp, "AGI")
	}
	return json.Marshal(resp)
}

const (
	ClusterFeatureAerospike      = FeatureSystem(1)
	ClusterFeatureAerospikeTools = FeatureSystem(2)
	ClusterFeatureAGI            = FeatureSystem(4)
)

type inventoryClient struct {
	ClientName             string
	NodeNo                 string
	PrivateIp              string
	PublicIp               string
	InstanceId             string
	ImageId                string
	State                  string
	Arch                   string
	Distribution           string
	OSVersion              string
	AerospikeVersion       string
	ClientType             string
	AccessUrl              string
	AccessPort             string
	Firewalls              []string
	Zone                   string
	InstanceRunningCost    float64
	Owner                  string
	DockerExposePorts      string
	DockerInternalPort     string
	Expires                string
	gcpLabelFingerprint    string
	gcpLabels              map[string]string
	gcpMeta                map[string]string
	gcpMetadataFingerprint string
	awsTags                map[string]string
	dockerLabels           map[string]string
	awsSubnet              string
	awsSecGroups           []string
}

type inventoryTemplate struct {
	Distribution     string
	OSVersion        string
	AerospikeVersion string
	Arch             string
	Region           string
}

type inventoryFirewallRule struct {
	GCP    *inventoryFirewallRuleGCP
	AWS    *inventoryFirewallRuleAWS
	Docker *inventoryFirewallRuleDocker
}

type inventoryFirewallRuleGCP struct {
	FirewallName string
	TargetTags   []string
	SourceTags   []string
	SourceRanges []string
	AllowPorts   []string
	DenyPorts    []string
}

type inventoryFirewallRuleAWS struct {
	VPC               string
	SecurityGroupName string
	SecurityGroupID   string
	IPs               []string
	Region            string
}

type inventoryFirewallRuleDocker struct {
	NetworkName   string
	NetworkDriver string
	Subnets       string
	MTU           string
}

type instanceTypesCache struct {
	Expires       time.Time
	InstanceTypes []instanceType
}

type instanceType struct {
	InstanceName             string
	CPUs                     int
	RamGB                    float64
	EphemeralDisks           int
	EphemeralDiskTotalSizeGB float64
	PriceUSD                 float64
	IsArm                    bool
	IsX86                    bool
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

var InventoryItemClusters = 1
var InventoryItemClients = 2
var InventoryItemFirewalls = 3
var InventoryItemTemplates = 4
var InventoryItemExpirySystem = 5
var InventoryItemAGI = 6
var InventoryItemAWSAllRegions = 7
var InventoryItemVolumes = 8
