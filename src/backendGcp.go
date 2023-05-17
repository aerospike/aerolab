package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
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
	return nil
}

func (d *backendGcp) IsSystemArm(systemType string) (bool, error) {
	return strings.HasPrefix(systemType, "t2"), nil
}

func (d *backendGcp) ClusterList() ([]string, error) {
	clist := []string{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagUsedByValue),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					clist = append(clist, instance.Labels[gcpTagClusterName])
				}
			}
		}
	}
	return clist, nil
}

func (d *backendGcp) IsNodeArm(clusterName string, nodeNumber int) (bool, error) {
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return false, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagUsedByValue),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == clusterName && instance.Labels[gcpTagNodeNumber] == strconv.Itoa(nodeNumber) {
						return d.IsSystemArm(*instance.MachineType)
					}
				}
			}
		}
	}
	return false, fmt.Errorf("cluster %s node %v not found", clusterName, nodeNumber)
}

func (d *backendGcp) NodeListInCluster(name string) ([]int, error) {
	nlist := []int{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagUsedByValue),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						nodeNo, err := strconv.Atoi(instance.Labels[gcpTagNodeNumber])
						if err != nil {
							return nil, fmt.Errorf("found aerolab instance without valid tag format: %v", *instance.Id)
						}
						nlist = append(nlist, nodeNo)
					}
				}
			}
		}
	}
	return nlist, nil
}

func (d *backendGcp) GetClusterNodeIps(name string) ([]string, error) {
	nlist := []string{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagUsedByValue),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						if len(instance.NetworkInterfaces) > 0 && instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" && *instance.Status != "TERMINATED" {
							nlist = append(nlist, *instance.NetworkInterfaces[0].NetworkIP)
						}
					}
				}
			}
		}
	}
	return nlist, nil
}

// TODO: test internal vs external IP
func (d *backendGcp) GetNodeIpMap(name string, internalIPs bool) (map[int]string, error) {
	nlist := make(map[int]string)
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagUsedByValue),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if instance.Labels[gcpTagClusterName] == name {
						nodeNo, err := strconv.Atoi(instance.Labels[gcpTagNodeNumber])
						if err != nil {
							return nil, fmt.Errorf("found aerolab instance with incorrect labels: %v", *instance.Id)
						}
						ip := "N/A"
						if len(instance.NetworkInterfaces) > 0 {
							if internalIPs {
								if instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
									ip = *instance.NetworkInterfaces[0].NetworkIP
								}
							} else {
								if len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
									if instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != nil && *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != "" {
										ip = *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
									}
								}
							}
						}
						nlist[nodeNo] = ip
					}
				}
			}
		}
	}
	return nlist, nil
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

func (d *backendGcp) ClusterStart(name string, nodes []int) error {
}

func (d *backendGcp) ClusterStop(name string, nodes []int) error {
}

func (d *backendGcp) ClusterDestroy(name string, nodes []int) error {
}

func (d *backendGcp) ListTemplates() ([]backendVersion, error) {
}

func (d *backendGcp) TemplateDestroy(v backendVersion) error {
}

func (d *backendGcp) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
}

func (d *backendGcp) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
}

func (d *backendGcp) VacuumTemplate(v backendVersion) error {
}

func (d *backendGcp) VacuumTemplates() error {
}

func (d *backendGcp) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendGcp) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
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
