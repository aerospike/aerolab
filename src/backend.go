package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/term"
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
	limitNoFile         int       // docker only
	customDockerImage   string    // docker only
	securityGroupID     string    // aws only
	subnetID            string    // aws only
	ebs                 string    // aws only
	terminateOnPoweroff bool      // aws only
	spotInstance        bool      // aws only
	instanceRole        string    // aws only
	instanceType        string    // aws/gcp only
	ami                 string    // aws/gcp only
	publicIP            bool      // aws/gcp only
	tags                []string  // aws/gcp only
	firewallNamePrefix  []string  // aws/gcp only
	expiresTime         time.Time // aws/gcp only
	isAgiFirewall       bool      // aws/gcp only
	disks               []string  // gcp only
	zone                string    // gcp only
	labels              []string  // gcp only
	onHostMaintenance   string    // gcp only
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
	// gcp/aws volumes
	DisablePricingAPI()
	DisableExpiryInstall()
	CreateVolume(name string, zone string, tags []string, expires time.Duration, size int64, desc string) error
	TagVolume(fsId string, tagName string, tagValue string, zone string) error
	DeleteVolume(name string, zone string) error
	// volumes: efs only
	CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error)
	MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error
	GetAZName(subnetId string) (string, error)
	// volumes: gcp only
	AttachVolume(name string, zone string, clusterName string, node int) error
	ResizeVolume(name string, zone string, newSize int64) error
	DetachVolume(name string, clusterName string, node int, zone string) error
	// cause gcp
	EnableServices() error
	// expiries calls
	ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error
	ExpiriesSystemRemove(region string) error
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
	AttachAndRun(clusterName string, node int, command []string, isInteractive bool) (err error)
	// like AttachAndRun, but provide custom stdin, stdout and stderr the command should pipe to
	RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, isInteractive bool, dockerForceUser *string) (err error)
	// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
	GetNodeIpMap(name string, internalIPs bool) (map[int]string, error)
	// return formatted for printing cluster list
	ClusterListFull(json bool, owner string, noPager bool, isPretty bool, sort []string, renderer string) (string, error)
	// return formatted for printing template list
	TemplateListFull(json bool, noPager bool, isPretty bool, sort []string, renderer string) (string, error)
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
	CreateSecurityGroups(vpc string, namePrefix string, isAgi bool, extraPorts []string, noDefaults bool) error
	// may implement
	LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string, isAgi bool, extraPorts []string, noDefaults bool) error
	AssignSecurityGroups(clusterName string, names []string, vpcOrZone string, remove bool, performLocking bool, extraPorts []string, noDefaults bool) error
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
	AGI           []inventoryWebAGI
}

type inventoryWebAGI struct {
	Name           string
	State          string
	Status         string
	Expires        string `backends:"gcp,aws"`
	VolOwner       string `backends:"gcp,aws"`
	Owner          string
	AccessURL      string `hidden:"true"`
	AGILabel       string
	VolSize        string  `backends:"gcp,aws"`
	VolExpires     string  `backends:"gcp,aws"`
	RunningCost    float64 `backends:"gcp,aws"`
	PublicIP       string
	PrivateIP      string
	Firewalls      []string `backends:"gcp,aws"`
	Zone           string   `backends:"gcp,aws"`
	VolID          string   `backends:"aws"`
	InstanceID     string
	ImageID        string    `backends:"docker"`
	InstanceType   string    `backends:"gcp,aws"`
	CreationTime   time.Time `hidden:"true"`
	SourceLocal    string    `backends:"gcp,aws"`
	SourceSftp     string    `backends:"gcp,aws"`
	SourceS3       string    `backends:"gcp,aws"`
	IsRunning      bool      `row:"Action"`
	AccessProtocol string    `hidden:"true"` // http:// https://
}

type inventoryVolume struct {
	Name                 string
	FileSystemId         string    `row:"FsID"`
	AvailabilityZoneName string    `row:"Zone"`
	CreationTime         time.Time `row:"Created"`
	SizeBytes            int       `hidden:"true"`
	SizeString           string    `row:"Size"`       // only used by webform
	ExpiresIn            string    `row:"Expires In"` // only used by webform
	AWS                  inventoryVolumeAws
	GCP                  inventoryVolumeGcp
	Owner                string
	AGIVolume            bool              // only used by webform
	AgiLabel             string            // only used by webform
	AvailabilityZoneId   string            `hidden:"true"`
	LifeCycleState       string            `hidden:"true"`
	Tags                 map[string]string `hidden:"true"`
}

type inventoryVolumeAws struct {
	NumberOfMountTargets int                    `row:"MountTargets"`
	CreationToken        string                 `hidden:"true"`
	Encrypted            bool                   `hidden:"true"`
	FileSystemArn        string                 `hidden:"true"`
	AWSOwnerId           string                 `hidden:"true"`
	PerformanceMode      string                 `hidden:"true"`
	ThroughputMode       string                 `hidden:"true"`
	MountTargets         []inventoryMountTarget `hidden:"true"`
}

type inventoryVolumeGcp struct {
	AttachedToString string   `row:"Attached To"` // only used by webform
	AttachedTo       []string `hidden:"true"`
	Description      string   `hidden:"true"`
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
	Schedule     string
	IAMScheduler string
	IAMFunction  string
	Scheduler    string
	Function     string
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
	SubnetName       string
	SubnetCidr       string
	IsAzDefault      bool
	AutoPublicIP     bool
}

type inventoryCluster struct {
	ClusterName            string
	NodeNo                 string
	Expires                string `backends:"aws,gcp"`
	State                  string
	PublicIp               string
	PrivateIp              string
	DockerExposePorts      string `row:"ExposedPort" backends:"docker"`
	DockerInternalPort     string `hidden:"true"`
	Owner                  string
	AerospikeVersion       string   `row:"AsdVer"`
	InstanceRunningCost    float64  `row:"RunningCost" backends:"aws,gcp"`
	Firewalls              []string `backends:"aws,gcp"`
	Arch                   string
	Distribution           string `row:"Distro"`
	OSVersion              string `row:"DistroVer"`
	Zone                   string `backends:"aws,gcp"`
	InstanceId             string
	ImageId                string        `backends:"docker"`
	InstanceType           string        `backends:"aws,gcp"`
	AwsIsSpot              bool          `row:"Spot" backends:"aws"`
	GcpIsSpot              bool          `row:"Spot" backends:"gcp"`
	AccessUrl              string        `hidden:"true"`
	Features               FeatureSystem `hidden:"true"`
	AGILabel               string        `hidden:"true"`
	IsRunning              bool          `row:"Action"`
	gcpLabelFingerprint    string
	gcpLabels              map[string]string
	gcpMetadataFingerprint string
	gcpMeta                map[string]string
	awsTags                map[string]string
	dockerLabels           map[string]string
	awsSubnet              string
	awsSecGroups           []string
	AccessProtocol         string `hidden:"true"`
}

type FeatureSystem int64

func (f *FeatureSystem) UnmarshalJSON(data []byte) error {
	d := []string{}
	err := json.Unmarshal(data, &d)
	if err != nil {
		return err
	}
	for _, i := range d {
		switch i {
		case "Aerospike":
			*f = *f + ClusterFeatureAerospike
		case "AerospikeTools":
			*f = *f + ClusterFeatureAerospikeTools
		case "AGI":
			*f = *f + ClusterFeatureAGI
		default:
			*f = *f + ClusterFeatureUnknown
		}
	}
	return nil
}

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
	if *f&ClusterFeatureUnknown > 0 {
		resp = append(resp, "Unknown")
	}
	return json.Marshal(resp)
}

const (
	ClusterFeatureAerospike      = FeatureSystem(1)
	ClusterFeatureAerospikeTools = FeatureSystem(2)
	ClusterFeatureAGI            = FeatureSystem(4)
	ClusterFeatureUnknown        = FeatureSystem(2147483648)
)

type inventoryClient struct {
	ClientName             string
	NodeNo                 string
	Expires                string `backends:"aws,gcp"`
	State                  string
	PublicIp               string
	PrivateIp              string
	ClientType             string
	AccessUrl              string
	AccessPort             string
	Owner                  string
	AerospikeVersion       string   `row:"AsdVer"`
	InstanceRunningCost    float64  `row:"RunningCost" backends:"aws,gcp"`
	Firewalls              []string `backends:"aws,gcp"`
	Arch                   string
	Distribution           string `row:"Distro"`
	OSVersion              string `row:"DistroVer"`
	Zone                   string `backends:"aws,gcp"`
	InstanceId             string
	ImageId                string `backends:"docker"`
	InstanceType           string `backends:"aws,gcp"`
	DockerExposePorts      string `backends:"docker" row:"ExposePorts"`
	DockerInternalPort     string `hidden:"true"`
	AwsIsSpot              bool   `backends:"aws" row:"Spot"`
	GcpIsSpot              bool   `backends:"gcp" row:"Spot"`
	IsRunning              bool   `row:"Action"`
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
	AerospikeVersion string `row:"AsdVersion"`
	Distribution     string
	OSVersion        string `row:"DistroVersion"`
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
	Ports             []string
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
	SpotPriceUSD             float64
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

func termSize(fd uintptr) []byte {
	size := make([]byte, 16)

	w, h, err := term.GetSize(int(fd))
	if err != nil {
		binary.BigEndian.PutUint32(size, uint32(80))
		binary.BigEndian.PutUint32(size[4:], uint32(24))
		return size
	}

	binary.BigEndian.PutUint32(size, uint32(w))
	binary.BigEndian.PutUint32(size[4:], uint32(h))

	return size
}
