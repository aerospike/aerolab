package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/bestmethod/inslice"
	"golang.org/x/crypto/ssh"
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
	gcpServerTagUsedBy           = "used_by"
	gcpServerTagUsedByValue      = "aerolab4"
	gcpServerTagClusterName      = "aerolab4cluster_name"
	gcpServerTagNodeNumber       = "aerolab4node_number"
	gcpServerTagOperatingSystem  = "aerolab4operating_system"
	gcpServerTagOSVersion        = "aerolab4operating_system_version"
	gcpServerTagAerospikeVersion = "aerolab4aerospike_version"
	gcpServerFirewallTag         = "aerolab-server"

	// client
	gcpClientTagUsedBy           = "used_by"
	gcpClientTagUsedByValue      = "aerolab4client"
	gcpClientTagClusterName      = "aerolab4client_name"
	gcpClientTagNodeNumber       = "aerolab4client_node_number"
	gcpClientTagOperatingSystem  = "aerolab4client_operating_system"
	gcpClientTagOSVersion        = "aerolab4client_operating_system_version"
	gcpClientTagAerospikeVersion = "aerolab4client_aerospike_version"
	gcpClientFirewallTag         = "aerolab-client"
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

func gcpTagEnclose(str string) string {
	return "\"" + str + "\""
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
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
							return nil, fmt.Errorf("found aerolab instance without valid tag format: %v", *instance.Name)
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
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
						if len(instance.NetworkInterfaces) > 0 && instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
							nlist = append(nlist, *instance.NetworkInterfaces[0].NetworkIP)
						}
					}
				}
			}
		}
	}
	return nlist, nil
}

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
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
							return nil, fmt.Errorf("found aerolab instance with incorrect labels: %v", *instance.Name)
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
	clist := []gcpClusterListFull{}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					sysArch := "x86_64"
					if arm, _ := d.IsSystemArm(*instance.MachineType); arm {
						sysArch = "aarch64"
					}
					pubIp := "N/A"
					privIp := "N/A"
					if len(instance.NetworkInterfaces) > 0 {
						if instance.NetworkInterfaces[0].NetworkIP != nil && *instance.NetworkInterfaces[0].NetworkIP != "" {
							privIp = *instance.NetworkInterfaces[0].NetworkIP
						}
						if len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
							if instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != nil && *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP != "" {
								pubIp = *instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
							}
						}
					}
					clist = append(clist, gcpClusterListFull{
						ClusterName: instance.Labels[gcpTagClusterName],
						NodeNumber:  instance.Labels[gcpTagNodeNumber],
						IpAddress:   privIp,
						PublicIp:    pubIp,
						InstanceId:  *instance.Name,
						State:       *instance.Status,
						Arch:        sysArch,
					})
				}
			}
		}
	}
	clusterName := 20
	nodeNumber := 6
	ipAddress := 15
	publicIp := 15
	instanceId := 20
	state := 20
	arch := 10
	for _, item := range clist {
		if len(item.ClusterName) > clusterName {
			clusterName = len(item.ClusterName)
		}
		if len(item.NodeNumber) > nodeNumber {
			nodeNumber = len(item.NodeNumber)
		}
		if len(item.IpAddress) > ipAddress {
			ipAddress = len(item.IpAddress)
		}
		if len(item.PublicIp) > publicIp {
			publicIp = len(item.PublicIp)
		}
		if len(item.InstanceId) > instanceId {
			instanceId = len(item.InstanceId)
		}
		if len(item.State) > state {
			state = len(item.State)
		}
		if len(item.Arch) > arch {
			arch = len(item.Arch)
		}
	}
	sort.Slice(clist, func(i, j int) bool {
		if clist[i].ClusterName < clist[j].ClusterName {
			return true
		}
		if clist[i].ClusterName > clist[j].ClusterName {
			return false
		}
		ino, _ := strconv.Atoi(clist[i].NodeNumber)
		jno, _ := strconv.Atoi(clist[j].NodeNumber)
		return ino < jno
	})
	if !isJson {
		sprintf := "%-" + strconv.Itoa(clusterName) + "s %-" + strconv.Itoa(nodeNumber) + "s %-" + strconv.Itoa(ipAddress) + "s %-" + strconv.Itoa(publicIp) + "s %-" + strconv.Itoa(instanceId) + "s %-" + strconv.Itoa(state) + "s %-" + strconv.Itoa(arch) + "s\n"
		result := fmt.Sprintf(sprintf, "ClusterName", "NodeNo", "PrivateIp", "PublicIp", "InstanceId", "State", "Arch")
		result = result + fmt.Sprintf(sprintf, "--------------------", "------", "---------------", "---------------", "--------------------", "--------------------", "----------")
		for _, item := range clist {
			result = result + fmt.Sprintf(sprintf, item.ClusterName, item.NodeNumber, item.IpAddress, item.PublicIp, item.InstanceId, item.State, item.Arch)
		}
		return result, nil
	}
	out, err := json.MarshalIndent(clist, "", "    ")
	return string(out), err
}

type instanceDetail struct {
	instanceZone string
	instanceName string
}

func (d *backendGcp) getInstanceDetails(name string, nodes []int) (zones map[int]instanceDetail, err error) {
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return nil, fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones = make(map[int]instanceDetail)
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
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
						for _, node := range nodes {
							if strconv.Itoa(node) == instance.Labels[gcpTagNodeNumber] {
								zone := strings.Split(*instance.Zone, "/")
								zones[node] = instanceDetail{
									instanceZone: zone[len(zone)-1],
									instanceName: *instance.Name,
								}
							}
						}
					}
				}
			}
		}
	}
	return zones, nil
}

func (d *backendGcp) ClusterStart(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		req := &computepb.StartInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zones[node].instanceZone,
			Instance: zones[node].instanceName,
		}
		op, err := instancesClient.Start(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to start instance: %w", err)
		}
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ClusterStop(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		req := &computepb.StopInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zones[node].instanceZone,
			Instance: zones[node].instanceName,
		}
		op, err := instancesClient.Stop(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to stop instance: %w", err)
		}
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ClusterDestroy(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	zones, err := d.getInstanceDetails(name, nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, ok := zones[node]; !ok {
			return fmt.Errorf("node %v not found", node)
		}
	}
	for _, node := range nodes {
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()
		req := &computepb.DeleteInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zones[node].instanceZone,
			Instance: zones[node].instanceName,
		}
		op, err := instancesClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete instance: %w", err)
		}
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ListTemplates() ([]backendVersion, error) {
	ctx := context.Background()
	bv := []backendVersion{}
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()
	req := computepb.ListImagesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}

	it := imagesClient.List(ctx, &req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
			isArm := false
			if strings.Contains(*image.Architecture, "arm") || strings.Contains(*image.Architecture, "aarch") {
				isArm = true
			}
			bv = append(bv, backendVersion{
				distroName:       image.Labels[gcpServerTagOperatingSystem],
				distroVersion:    gcpResourceNameBack(image.Labels[gcpServerTagOSVersion]),
				aerospikeVersion: gcpResourceNameBack(image.Labels[gcpServerTagAerospikeVersion]),
				isArm:            isArm,
			})
		}
	}
	return bv, nil
}

func (d *backendGcp) deleteImage(name string) error {
	ctx := context.Background()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()

	req := computepb.DeleteImageRequest{
		Image:   name,
		Project: a.opts.Config.Backend.Project,
	}
	op, err := imagesClient.Delete(ctx, &req)
	if err != nil {
		return err
	}
	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) TemplateDestroy(v backendVersion) error {
	ctx := context.Background()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()
	req := computepb.ListImagesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}

	it := imagesClient.List(ctx, &req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
			isArm := false
			if strings.Contains(*image.Architecture, "arm") || strings.Contains(*image.Architecture, "aarch") {
				isArm = true
			}
			if (image.Labels[gcpServerTagOperatingSystem] == v.distroName || v.distroName == "all") && (image.Labels[gcpServerTagOSVersion] == gcpResourceName(v.distroVersion) || v.distroVersion == "all") && (image.Labels[gcpServerTagAerospikeVersion] == gcpResourceName(v.aerospikeVersion) || v.aerospikeVersion == "all") && isArm == v.isArm {
				err = d.deleteImage(*image.Name)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *backendGcp) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	nodeIps, err := d.GetNodeIpMap(name, false)
	if err != nil {
		return fmt.Errorf("could not get node ip map for cluster: %s", err)
	}
	_, keypath, err := d.getKey(name)
	if err != nil {
		return fmt.Errorf("could not get key: %s", err)
	}
	for _, node := range nodes {
		err = scp("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, files)
		if err != nil {
			return fmt.Errorf("scp failed: %s", err)
		}
	}
	return err
}

func (d *backendGcp) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
	var fout [][]byte
	var err error
	if nodes == nil {
		nodes, err = d.NodeListInCluster(clusterName)
		if err != nil {
			return nil, err
		}
	}
	_, keypath, err := d.getKey(clusterName)
	if err != nil {
		return nil, fmt.Errorf("could not get key:%s", err)
	}
	nodeIps, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return nil, fmt.Errorf("could not get node ip map:%s", err)
	}
	for _, node := range nodes {
		var out []byte
		var err error
		for _, command := range commands {
			comm := command[0]
			for _, c := range command[1:] {
				if strings.Contains(c, " ") {
					comm = comm + " \"" + c + "\""
				} else {
					comm = comm + " " + c
				}
			}
			out, err = remoteRun("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, comm, node)
			fout = append(fout, out)
			if checkExecRetcode(err) != 0 {
				return fout, fmt.Errorf("error running `%s`: %s", comm, err)
			}
		}
	}
	return fout, nil
}

func (d *backendGcp) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendGcp) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
	clusters, err := d.ClusterList()
	if err != nil {
		return fmt.Errorf("could not get cluster list: %s", err)
	}
	if !inslice.HasString(clusters, clusterName) {
		return fmt.Errorf("cluster not found")
	}
	nodes, err := d.NodeListInCluster(clusterName)
	if err != nil {
		return fmt.Errorf("could not get node list: %s", err)
	}
	if !inslice.HasInt(nodes, node) {
		return fmt.Errorf("node not found")
	}
	nodeIp, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return fmt.Errorf("could not get node ip map: %s", err)
	}
	_, keypath, err := d.getKey(clusterName)
	if err != nil {
		return fmt.Errorf("could not get key path: %s", err)
	}
	var comm string
	if len(command) > 0 {
		comm = command[0]
		for _, c := range command[1:] {
			if strings.Contains(c, " ") {
				comm = comm + " \"" + strings.ReplaceAll(c, "\"", "\\\"") + "\""
			} else {
				comm = comm + " " + c
			}
		}
	} else {
		comm = "bash"
	}
	err = remoteAttachAndRun("root", fmt.Sprintf("%s:22", nodeIp[node]), keypath, comm, stdin, stdout, stderr, node)
	return err
}

func (d *backendGcp) Upload(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	nodes, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return errors.New("found no nodes in cluster")
	}
	nodeIp, ok := nodes[node]
	if !ok {
		return errors.New("node not found in cluster")
	}
	_, key, err := d.getKey(clusterName)
	if err != nil {
		return err
	}
	return scpExecUpload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose, legacy)
}

func (d *backendGcp) Download(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
	nodes, err := d.GetNodeIpMap(clusterName, false)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return errors.New("found no nodes in cluster")
	}
	nodeIp, ok := nodes[node]
	if !ok {
		return errors.New("node not found in cluster")
	}
	_, key, err := d.getKey(clusterName)
	if err != nil {
		return err
	}
	return scpExecDownload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose, legacy)
}

func (d *backendGcp) vacuum(v *backendVersion) error {
	isArm := "amd"
	if v != nil && v.isArm {
		isArm = "arm"
	}
	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("newInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// Use the `MaxResults` parameter to limit the number of results that the API returns per response page.
	req := &computepb.AggregatedListInstancesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}
	it := instancesClient.AggregatedList(ctx, req)
	delList := []*computepb.Instance{}
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("listing instances: %s", err)
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
					if (v != nil && *instance.Name == fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)) || (v == nil && strings.HasPrefix(*instance.Name, "aerolab4-template-")) {
						instance := instance
						delList = append(delList, instance)
					}
				}
			}
		}
	}
	return d.deleteInstances(delList)
}

func gcpResourceName(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

func gcpResourceNameBack(name string) string {
	return strings.ReplaceAll(name, "-", ".")
}

func (d *backendGcp) deleteInstances(list []*computepb.Instance) error {
	delOps := make(map[context.Context]*compute.Operation)
	for _, instance := range list {
		ctx := context.Background()
		deleteClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer deleteClient.Close()
		zone := strings.Split(*instance.Zone, "/")
		log.Printf("Deleting project=%s zone=%s name=%s", a.opts.Config.Backend.Project, zone[len(zone)-1], *instance.Name)
		req := &computepb.DeleteInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     zone[len(zone)-1],
			Instance: *instance.Name,
		}
		op, err := deleteClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete instance: %w", err)
		}
		delOps[ctx] = op
	}
	for ctx, op := range delOps {
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) VacuumTemplate(v backendVersion) error {
	return d.vacuum(&v)
}

func (d *backendGcp) VacuumTemplates() error {
	return d.vacuum(nil)
}

type gcpTemplateListFull struct {
	OsName           string
	OsVersion        string
	AerospikeVersion string
	ImageId          string
	Arch             string
}

func (d *backendGcp) TemplateListFull(isJson bool) (string, error) {
	result := "Templates:\n\nOS_NAME\t\tOS_VER\t\tAEROSPIKE_VERSION\t\tARCH\n--------------------------------------------------------------------------\n"
	resList := []gcpTemplateListFull{}

	ctx := context.Background()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()
	req := computepb.ListImagesRequest{
		Project: a.opts.Config.Backend.Project,
		Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
	}

	it := imagesClient.List(ctx, &req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
			result = fmt.Sprintf("%s%s\t\t%s\t\t%s\t\t%s\n", result, image.Labels[gcpTagOperatingSystem], gcpResourceNameBack(image.Labels[gcpTagOSVersion]), gcpResourceNameBack(image.Labels[gcpTagAerospikeVersion]), *image.Architecture)
			resList = append(resList, gcpTemplateListFull{
				OsName:           image.Labels[gcpTagOperatingSystem],
				OsVersion:        gcpResourceNameBack(image.Labels[gcpTagOSVersion]),
				AerospikeVersion: gcpResourceNameBack(image.Labels[gcpTagAerospikeVersion]),
				ImageId:          *image.Name,
				Arch:             *image.Architecture,
			})
		}
	}

	if !isJson {
		return result, nil
	}
	out, err := json.MarshalIndent(resList, "", "    ")
	return string(out), err
}

// get KeyPair
func (d *backendGcp) getKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	// check keypath exists, if not, error
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		err = fmt.Errorf("key does not exist in given location: %s", keyPath)
		return keyName, keyPath, err
	}
	return
}

// get KeyPair
func (d *backendGcp) makeKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	_, _, err = d.getKey(clusterName)
	if err == nil {
		return
	}
	// check keypath exists, if not, make
	if _, err := os.Stat(string(a.opts.Config.Backend.SshKeyPath)); os.IsNotExist(err) {
		os.MkdirAll(string(a.opts.Config.Backend.SshKeyPath), 0755)
	}

	// generate keypair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}

	// generate and write private key as PEM
	privateKeyFile, err := os.OpenFile(keyPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer privateKeyFile.Close()
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return keyName, keyPath, err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return
	}
	err = os.WriteFile(keyPath+".pub", ssh.MarshalAuthorizedKey(pub), 0600)
	if err != nil {
		return
	}

	return
}

// get KeyPair
func (d *backendGcp) killKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-gcp-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	os.Remove(keyPath)
	os.Remove(keyPath + ".pub")
	return
}

var deployGcpTemplateShutdownMaking = make(chan int, 1)

func (d *backendGcp) getImage(v backendVersion) (string, error) {
	imName := ""
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer client.Close()
	filter := ""
	arch := "X86_64"
	if v.isArm {
		arch = "ARM64"
	}
	project := ""
	switch v.distroName {
	case "debian":
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("debian-"+v.distroVersion+"*"), gcpTagEnclose(arch))
		project = "debian-cloud"
	case "centos":
		dv := v.distroVersion
		if dv == "8" {
			dv = "stream-8"
		}
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("centos-"+dv+"*"), gcpTagEnclose(arch))
		project = "centos-cloud"
	case "ubuntu":
		filter = fmt.Sprintf("(family:%s) AND (architecture:%s)", gcpTagEnclose("ubuntu-"+strings.ReplaceAll(v.distroVersion, ".", "")+"-lts*"), gcpTagEnclose(arch))
		project = "ubuntu-os-cloud"
	}
	req := &computepb.ListImagesRequest{
		Filter:  &filter,
		Project: project,
	}
	it := client.List(ctx, req)
	for {
		image, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		if *image.Name > imName {
			imName = *image.SelfLink
		}
	}
	return imName, nil
}

func (d *backendGcp) makeLabels(extra []string, isArm string, v backendVersion) (map[string]string, error) {
	labels := make(map[string]string)
	badNames := []string{gcpTagOperatingSystem, gcpTagOSVersion, gcpTagAerospikeVersion, gcpTagUsedBy, gcpTagClusterName, gcpTagNodeNumber, "Arch", "Name"}
	for _, extraTag := range extra {
		kv := strings.Split(extraTag, "=")
		if len(kv) < 2 {
			return nil, errors.New("tags must follow key=value format")
		}
		key := kv[0]
		if inslice.HasString(badNames, key) {
			return nil, fmt.Errorf("key %s is used internally by aerolab, cannot be used", key)
		}
		val := strings.Join(kv[1:], "=")
		for _, char := range val {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return nil, fmt.Errorf("invalid tag value for `%s`, only the following are allowed: [a-zA-Z0-9_-]", val)
			}
		}
		for _, char := range key {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return nil, fmt.Errorf("invalid tag name for `%s`, only the following are allowed: [a-zA-Z0-9_-]", val)
			}
		}
		labels[key] = val
	}
	labels[gcpTagOperatingSystem] = v.distroName
	labels[gcpTagOSVersion] = strings.ReplaceAll(v.distroVersion, ".", "-")
	labels[gcpTagAerospikeVersion] = strings.ReplaceAll(v.aerospikeVersion, ".", "-")
	labels[gcpTagUsedBy] = gcpTagUsedByValue
	labels["arch"] = isArm
	return labels, nil
}

func (d *backendGcp) DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error {
	if extra.zone == "" {
		return errors.New("zone must be specified")
	}
	addShutdownHandler("deployGcpTemplate", func(os.Signal) {
		deployGcpTemplateShutdownMaking <- 1
		d.VacuumTemplate(v)
	})
	defer delShutdownHandler("deployGcpTemplate")
	genKeyName := "template" + strconv.Itoa(int(time.Now().Unix()))
	_, keyPath, err := d.getKey(genKeyName)
	if err != nil {
		d.killKey(genKeyName)
		_, keyPath, err = d.makeKey(genKeyName)
		if err != nil {
			return fmt.Errorf("could not obtain or make 'template' key:%s", err)
		}
	}
	defer d.killKey(genKeyName)
	sshKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return err
	}
	// start VM
	imageName, err := d.getImage(v)
	if err != nil {
		return err
	}

	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	labels, err := d.makeLabels(extra.labels, isArm, v)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)

	err = d.createSecurityGroupsIfNotExist()
	if err != nil {
		return fmt.Errorf("firewall: %s", err)
	}

	ctx := context.Background()
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	instanceType := "e2-medium"
	if v.isArm {
		instanceType = "t2a-standard-1"
	}
	tags := append(extra.tags, "aerolab-server")
	req := &computepb.InsertInstanceRequest{
		Project: a.opts.Config.Backend.Project,
		Zone:    extra.zone,
		InstanceResource: &computepb.Instance{
			Metadata: &computepb.Metadata{
				Items: []*computepb.Items{
					{
						Key:   proto.String("ssh-keys"),
						Value: proto.String("root:" + string(sshKey)),
					},
				},
			},
			Labels:      labels,
			Name:        proto.String(name),
			MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", extra.zone, instanceType)),
			Tags: &computepb.Tags{
				Items: tags,
			},
			Scheduling: &computepb.Scheduling{
				AutomaticRestart:  proto.Bool(true),
				OnHostMaintenance: proto.String("MIGRATE"),
				ProvisioningModel: proto.String("STANDARD"),
			},
			Disks: []*computepb.AttachedDisk{
				{
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						DiskSizeGb:  proto.Int64(20),
						SourceImage: proto.String(imageName),
					},
					AutoDelete: proto.Bool(true),
					Boot:       proto.Bool(true),
					Type:       proto.String(computepb.AttachedDisk_PERSISTENT.String()),
				},
			},
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					StackType: proto.String("IPV4_ONLY"),
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name:        proto.String("External NAT"),
							NetworkTier: proto.String("PREMIUM"),
						},
					},
				},
			},
		},
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	op, err := instancesClient.Insert(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to create instance: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	inst, err := instancesClient.Get(ctx, &computepb.GetInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: name,
	})
	if err != nil {
		return fmt.Errorf("failed to poll new instance: %s", err)
	}
	if len(inst.NetworkInterfaces) < 1 || len(inst.NetworkInterfaces[0].AccessConfigs) < 1 || inst.NetworkInterfaces[0].AccessConfigs[0].NatIP == nil {
		return errors.New("instance failed to obtain public IP")
	}
	instIp := *inst.NetworkInterfaces[0].AccessConfigs[0].NatIP
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}
	for {
		// wait for instances to be made available via SSH
		_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "ls", 0)
		if err == nil {
			break
		} else {
			fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", instIp, keyPath, err)
			time.Sleep(time.Second)
		}
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	fmt.Println("Connection succeeded, continuing deployment...")

	// copy files as required to VM
	files = append(files, fileList{"/root/installer.sh", strings.NewReader(script), len(script)})
	err = scp("root", fmt.Sprintf("%s:22", instIp), keyPath, files)
	if err != nil {
		return fmt.Errorf("scp failed: %s", err)
	}
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// run script as required
	_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "chmod 755 /root/installer.sh", 0)
	if err != nil {
		out, err := remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "chmod 755 /root/installer.sh", 0)
		if err != nil {
			return fmt.Errorf("chmod failed: %s\n%s", string(out), err)
		}
	}
	out, err := remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "/bin/bash -c /root/installer.sh", 0)
	if err != nil {
		return fmt.Errorf("/root/installer.sh failed: %s\n%s", string(out), err)
	}
	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// shutdown VM
	reqStop := &computepb.StopInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: *inst.Name,
	}
	op, err = instancesClient.Stop(ctx, reqStop)
	if err != nil {
		return fmt.Errorf("unable to stop instance: %w", err)
	}
	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	if len(deployGcpTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// create ami from VM
	imgName := fmt.Sprintf("aerolab4-template-%s-%s-%s-%s", v.distroName, gcpResourceName(v.distroVersion), gcpResourceName(v.aerospikeVersion), isArm)
	err = d.makeImageFromDisk(extra.zone, *inst.Name, imgName, labels)
	if err != nil {
		return fmt.Errorf("makeImageFromDisk: %s", err)
	}

	// destroy VM
	reqd := &computepb.DeleteInstanceRequest{
		Project:  a.opts.Config.Backend.Project,
		Zone:     extra.zone,
		Instance: *inst.Name,
	}
	_, err = instancesClient.Delete(ctx, reqd)
	if err != nil {
		return fmt.Errorf("unable to destroy instance: %w", err)
	}
	//if err = op.Wait(ctx); err != nil {
	//	return fmt.Errorf("unable to wait for the operation: %w", err)
	//}
	return nil
}

func (d *backendGcp) makeImageFromDisk(zone string, sourceDiskName string, imageName string, imageLabels map[string]string) error {
	ctx := context.Background()
	disksClient, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewDisksRESTClient: %w", err)
	}
	defer disksClient.Close()
	imagesClient, err := compute.NewImagesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewImagesRESTClient: %w", err)
	}
	defer imagesClient.Close()

	// Get the source disk
	source_req := &computepb.GetDiskRequest{
		Disk:    sourceDiskName,
		Project: a.opts.Config.Backend.Project,
		Zone:    zone,
	}

	disk, err := disksClient.Get(ctx, source_req)
	if err != nil {
		return fmt.Errorf("unable to get source disk: %w", err)
	}

	// Create the image
	req := computepb.InsertImageRequest{
		ForceCreate: proto.Bool(true),
		ImageResource: &computepb.Image{
			Name:             &imageName,
			SourceDisk:       disk.SelfLink,
			StorageLocations: nil,
			Labels:           imageLabels,
		},
		Project: a.opts.Config.Backend.Project,
	}

	op, err := imagesClient.Insert(ctx, &req)
	if err != nil {
		return err
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	return nil
}

func (d *backendGcp) ListSubnets() error {
	return errors.New("not implemented")
}

func (d *backendGcp) CreateSecurityGroups(vpc string) error {
	return d.createSecurityGroupsIfNotExist()
}

func (d *backendGcp) createSecurityGroupInternal() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("tcp"),
				Ports:      []string{"22", "3000", "8080", "8888", "9200"},
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String("aerolab-managed-external"),
		TargetTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		Description: proto.String("Allowing external access to aerolab-managed instance services"),
		SourceRanges: []string{
			"0.0.0.0/0",
		},
	}

	req := &computepb.InsertFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		FirewallResource: firewallRule,
	}

	op, err := firewallsClient.Insert(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to create firewall rule: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) createSecurityGroupExternal() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule2 := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("all"),
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String("aerolab-managed-internal"),
		TargetTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		SourceTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		Description: proto.String("Allowing internal access to aerolab-managed instance services"),
	}

	req2 := &computepb.InsertFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		FirewallResource: firewallRule2,
	}

	op2, err := firewallsClient.Insert(ctx, req2)
	if err != nil {
		return fmt.Errorf("unable to create firewall rule: %w", err)
	}

	if err = op2.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) LockSecurityGroups(ip string, lockSSH bool, vpc string) error {
	if ip == "discover-caller-ip" {
		ip = getip2()
	}
	if !strings.Contains(ip, "/") {
		ip = ip + "/32"
	}
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	firewallRule := &computepb.Firewall{
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: proto.String("tcp"),
				Ports:      []string{"22", "3000", "8080", "8888", "9200"},
			},
		},
		Direction: proto.String(computepb.Firewall_INGRESS.String()),
		Name:      proto.String("aerolab-managed-external"),
		TargetTags: []string{
			"aerolab-server",
			"aerolab-client",
		},
		Description: proto.String("Allowing external access to aerolab-managed instance services"),
		SourceRanges: []string{
			ip,
		},
	}

	req := &computepb.UpdateFirewallRequest{
		Project:          a.opts.Config.Backend.Project,
		Firewall:         "aerolab-managed-external",
		FirewallResource: firewallRule,
	}

	op, err := firewallsClient.Update(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to update firewall rule: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}
	return nil
}

func (d *backendGcp) DeleteSecurityGroups(vpc string) error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}
	it := firewallsClient.List(ctx, req)
	existInternal := false
	existExternal := false
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if *firewallRule.Name == "aerolab-managed-internal" {
			existInternal = true
		}
		if *firewallRule.Name == "aerolab-managed-external" {
			existExternal = true
		}
	}

	if existExternal {
		req := &computepb.DeleteFirewallRequest{
			Project:  a.opts.Config.Backend.Project,
			Firewall: "aerolab-managed-external",
		}

		op, err := firewallsClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete firewall rule: %w", err)
		}

		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}

	if existInternal {
		req := &computepb.DeleteFirewallRequest{
			Project:  a.opts.Config.Backend.Project,
			Firewall: "aerolab-managed-internal",
		}

		op, err := firewallsClient.Delete(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to delete firewall rule: %w", err)
		}

		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ListSecurityGroups() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}

	it := firewallsClient.List(ctx, req)
	output := []string{}
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		allow := []string{}
		for _, all := range firewallRule.Allowed {
			allow = append(allow, all.Ports...)
		}
		deny := []string{}
		for _, all := range firewallRule.Denied {
			deny = append(deny, all.Ports...)
		}
		output = append(output, fmt.Sprintf("%s\t%v\t%v\t%v\t%v\t%v", *firewallRule.Name, firewallRule.TargetTags, firewallRule.SourceTags, firewallRule.SourceRanges, allow, deny))
	}
	sort.Strings(output)
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 4, ' ', 0)
	fmt.Fprintln(w, "FirewallName\tTargetTags\tSourceTags\tSourceRanges\tAllowPorts\tDenyPorts")
	fmt.Fprintln(w, "------------\t----------\t----------\t------------\t----------\t---------")
	for _, line := range output {
		fmt.Fprint(w, line+"\n")
	}
	w.Flush()
	return nil
}

func (d *backendGcp) createSecurityGroupsIfNotExist() error {
	ctx := context.Background()
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("NewInstancesRESTClient: %w", err)
	}
	defer firewallsClient.Close()

	req := &computepb.ListFirewallsRequest{
		Project: a.opts.Config.Backend.Project,
	}

	it := firewallsClient.List(ctx, req)
	existInternal := false
	existExternal := false
	for {
		firewallRule, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if *firewallRule.Name == "aerolab-managed-internal" {
			existInternal = true
		}
		if *firewallRule.Name == "aerolab-managed-external" {
			existExternal = true
		}
	}
	if !existInternal {
		err = d.createSecurityGroupInternal()
		if err != nil {
			return err
		}
	}
	if !existExternal {
		err = d.createSecurityGroupExternal()
		if err != nil {
			return err
		}
	}
	return nil
}

type gcpMakeOps struct {
	ctx context.Context
	op  *compute.Operation
}

type gcpDisk struct {
	diskType string
	diskSize int
}

func (d *backendGcp) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
	if extra.zone == "" {
		return errors.New("zone must be specified")
	}
	if extra.instanceType == "" {
		return errors.New("instance type must be specified")
	}

	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	labels, err := d.makeLabels(extra.labels, isArm, v)
	if err != nil {
		return err
	}
	labels[gcpTagClusterName] = name

	disksInt := []gcpDisk{}
	if len(extra.disks) == 0 {
		extra.disks = []string{"balanced:20"}
	}
	for _, disk := range extra.disks {
		diska := strings.Split(disk, ":")
		if len(diska) != 2 {
			return errors.New("disk specification incorrect, must be type:sizeGB; example: balanced:20")
		}
		dint, err := strconv.Atoi(diska[1])
		if err != nil {
			return fmt.Errorf("incorrect format for disk mapping: size must be an integer: %s", err)
		}
		disksInt = append(disksInt, gcpDisk{
			diskType: diska[0],
			diskSize: dint,
		})
	}

	var imageName string
	if !d.client {
		imageName, err = d.getImage(v)
		if err != nil {
			return err
		}
	} else {
		ctx := context.Background()
		imagesClient, err := compute.NewImagesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewImagesRESTClient: %w", err)
		}
		defer imagesClient.Close()
		req := computepb.ListImagesRequest{
			Project: a.opts.Config.Backend.Project,
			Filter:  proto.String("labels." + gcpTagUsedBy + "=" + gcpTagEnclose(gcpTagUsedByValue)),
		}

		it := imagesClient.List(ctx, &req)
		for {
			image, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			if image.Labels[gcpTagUsedBy] == gcpTagUsedByValue {
				arch := "amd"
				if v.isArm {
					arch = "arm"
				}
				if image.Labels[gcpTagOperatingSystem] == gcpResourceName(v.distroName) && image.Labels[gcpTagOSVersion] == gcpResourceName(v.distroVersion) && image.Labels[gcpTagAerospikeVersion] == gcpResourceName(v.aerospikeVersion) && image.Labels["arch"] == arch {
					imageName = *image.Name
				}
			}
		}
	}

	nodes, err := d.NodeListInCluster(name)
	if err != nil {
		return fmt.Errorf("could not run NodeListInCluster\n%s", err)
	}
	start := 1
	for _, node := range nodes {
		if node >= start {
			start = node + 1
		}
	}

	err = d.createSecurityGroupsIfNotExist()
	if err != nil {
		return fmt.Errorf("firewall: %s", err)
	}
	var keyPath string
	ops := []gcpMakeOps{}
	for i := start; i < (nodeCount + start); i++ {
		labels[gcpTagNodeNumber] = strconv.Itoa(i)
		_, keyPath, err = d.getKey(name)
		if err != nil {
			d.killKey(name)
			_, keyPath, err = d.makeKey(name)
			if err != nil {
				return fmt.Errorf("could not run getKey\n%s", err)
			}
		}
		sshKey, err := os.ReadFile(keyPath + ".pub")
		if err != nil {
			return err
		}
		// disk mapping
		// zones/zone/diskTypes/diskType
		disksList := []*computepb.AttachedDisk{}
		for nI, nDisk := range disksInt {
			var simage *string
			boot := false
			if nI == 0 {
				simage = proto.String(imageName)
				boot = true
			}
			disksList = append(disksList, &computepb.AttachedDisk{
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:  proto.Int64(int64(nDisk.diskSize)),
					SourceImage: simage,
					DiskType:    proto.String(fmt.Sprintf("zones/%s/diskTypes/pd-%s", extra.zone, nDisk.diskType)),
				},
				AutoDelete: proto.Bool(true),
				Boot:       proto.Bool(boot),
				Type:       proto.String(computepb.AttachedDisk_PERSISTENT.String()),
			})
		}
		// create instance (remember name and labels and tags and extra tags)
		ctx := context.Background()
		instancesClient, err := compute.NewInstancesRESTClient(ctx)
		if err != nil {
			return fmt.Errorf("NewInstancesRESTClient: %w", err)
		}
		defer instancesClient.Close()

		instanceType := extra.instanceType
		tags := append(extra.tags, "aerolab-server")
		req := &computepb.InsertInstanceRequest{
			Project: a.opts.Config.Backend.Project,
			Zone:    extra.zone,
			InstanceResource: &computepb.Instance{
				Metadata: &computepb.Metadata{
					Items: []*computepb.Items{
						{
							Key:   proto.String("ssh-keys"),
							Value: proto.String("root:" + string(sshKey)),
						},
					},
				},
				Labels:      labels,
				Name:        proto.String(fmt.Sprintf("aerolab4-%s-%d", name, i)),
				MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", extra.zone, instanceType)),
				Tags: &computepb.Tags{
					Items: tags,
				},
				Scheduling: &computepb.Scheduling{
					AutomaticRestart:  proto.Bool(true),
					OnHostMaintenance: proto.String("MIGRATE"),
					ProvisioningModel: proto.String("STANDARD"),
				},
				Disks: disksList,
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						StackType: proto.String("IPV4_ONLY"),
						AccessConfigs: []*computepb.AccessConfig{
							{
								Name:        proto.String("External NAT"),
								NetworkTier: proto.String("PREMIUM"),
							},
						},
					},
				},
			},
		}

		op, err := instancesClient.Insert(ctx, req)
		if err != nil {
			return fmt.Errorf("unable to create instance: %w", err)
		}
		ops = append(ops, gcpMakeOps{
			ctx: ctx,
			op:  op,
		})
	}

	for _, o := range ops {
		if err = o.op.Wait(o.ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}

	nodeIps, err := d.GetNodeIpMap(name, false)
	if err != nil {
		return err
	}
	newNodeCount := len(nodes)

	for {
		working := 0
		for _, instIp := range nodeIps {
			_, err = remoteRun("root", fmt.Sprintf("%s:22", instIp), keyPath, "ls", 0)
			if err == nil {
				working++
			} else {
				fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", instIp, keyPath, err)
				time.Sleep(time.Second)
			}
		}
		if working >= newNodeCount {
			break
		}
	}
	return nil
}
