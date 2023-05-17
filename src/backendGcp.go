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
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
	clist := []clusterListFull{}
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
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
					clist = append(clist, clusterListFull{
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
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
					if instance.Labels[gcpTagClusterName] == name {
						for _, node := range nodes {
							if strconv.Itoa(node) == instance.Labels[gcpTagNodeNumber] {
								zones[node] = instanceDetail{
									instanceZone: *instance.Zone,
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
			return fmt.Errorf("unable to start instance: %w", err)
		}
		if err = op.Wait(ctx); err != nil {
			return fmt.Errorf("unable to wait for the operation: %w", err)
		}
	}
	return nil
}

func (d *backendGcp) ClusterDestroy(name string, nodes []int) error {
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
			return fmt.Errorf("unable to start instance: %w", err)
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
				distroVersion:    image.Labels[gcpServerTagOSVersion],
				aerospikeVersion: image.Labels[gcpServerTagAerospikeVersion],
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
			if image.Labels[gcpServerTagOperatingSystem] == v.distroName && image.Labels[gcpServerTagOSVersion] == v.distroVersion && image.Labels[gcpServerTagAerospikeVersion] == v.aerospikeVersion && isArm == v.isArm {
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

func (d *backendGcp) Upload(clusterName string, node int, source string, destination string, verbose bool) error {
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
	return scpExecUpload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose)
}

func (d *backendGcp) Download(clusterName string, node int, source string, destination string, verbose bool) error {
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
	return scpExecDownload("root", nodeIp, "22", key, source, destination, os.Stdout, 30*time.Second, verbose)
}

func (d *backendGcp) vacuum(v *backendVersion) error {
	isArm := "amd"
	if v.isArm {
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
			return err
		}
		instances := pair.Value.Instances
		if len(instances) > 0 {
			for _, instance := range instances {
				if instance.Labels[gcpTagUsedBy] == gcpTagUsedByValue && *instance.Status != "TERMINATED" {
					if (v != nil && *instance.Name == fmt.Sprintf("aerolab4-template-%s_%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion, isArm)) || (v == nil && strings.HasPrefix(*instance.Name, "aerolab4-template-")) {
						instance := instance
						delList = append(delList, instance)
					}
				}
			}
		}
	}
	return d.deleteInstances(delList)
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
		req := &computepb.DeleteInstanceRequest{
			Project:  a.opts.Config.Backend.Project,
			Zone:     *instance.Zone,
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
	result := "Templates:\n\nOS_NAME\t\tOS_VER\t\tAEROSPIKE_VERSION\t\tARCH\n-----------------------------------------\n"
	resList := []templateListFull{}

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
			result = fmt.Sprintf("%s%s\t\t%s\t\t%s\t\t%s\n", result, image.Labels[gcpTagOperatingSystem], image.Labels[gcpTagOSVersion], image.Labels[gcpTagAerospikeVersion], *image.Architecture)
			resList = append(resList, templateListFull{
				OsName:           image.Labels[gcpTagOperatingSystem],
				OsVersion:        image.Labels[gcpTagOSVersion],
				AerospikeVersion: image.Labels[gcpTagAerospikeVersion],
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
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
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
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
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
	privateKeyFile, err := os.OpenFile(keyPath+".private", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer privateKeyFile.Close()
	if err != nil {
		return
	}
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return keyName, keyPath, err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(keyPath, ssh.MarshalAuthorizedKey(pub), 0600)
	if err != nil {
		return
	}

	return
}

// get KeyPair
func (d *backendGcp) killKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	os.Remove(keyPath)
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
			imName = *image.Name
		}
	}
	return imName, nil
}

func (d *backendGcp) DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error {
	addShutdownHandler("deployGcpTemplate", func(os.Signal) {
		deployGcpTemplateShutdownMaking <- 1
		d.VacuumTemplate(v)
	})
	defer delShutdownHandler("deployGcpTemplate")
	labels := make(map[string]string)
	badNames := []string{gcpTagOperatingSystem, gcpTagOSVersion, gcpTagAerospikeVersion, gcpTagUsedBy, gcpTagClusterName, gcpTagNodeNumber, "Arch", "Name"}
	for _, extraTag := range extra.tags {
		kv := strings.Split(extraTag, "=")
		if len(kv) < 2 {
			return errors.New("tags must follow key=value format")
		}
		key := kv[0]
		if inslice.HasString(badNames, key) {
			return fmt.Errorf("key %s is used internally by aerolab, cannot be used", key)
		}
		val := strings.Join(kv[1:], "=")
		for _, char := range val {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return fmt.Errorf("invalid tag value for `%s`, only the following are allowed: [a-zA-Z0-9_-]")
			}
		}
		for _, char := range key {
			if (char < 48 && char != 45) || char > 122 || (char > 57 && char < 65) || (char > 90 && char < 97 && char != 95) {
				return fmt.Errorf("invalid tag name for `%s`, only the following are allowed: [a-zA-Z0-9_-]")
			}
		}
		labels[key] = val
	}
	genKeyName := "template" + strconv.Itoa(int(time.Now().Unix()))
	keyName, keyPath, err := d.getKey(genKeyName)
	if err != nil {
		d.killKey(genKeyName)
		keyName, keyPath, err = d.makeKey(genKeyName)
		if err != nil {
			return fmt.Errorf("could not obtain or make 'template' key:%s", err)
		}
	}
	defer d.killKey(genKeyName)
	// start VM
	imageName, err := d.getImage(v)
	if err != nil {
		return err
	}

	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	labels[gcpTagOperatingSystem] = v.distroName
	labels[gcpTagOSVersion] = strings.ReplaceAll(v.distroVersion, ".", "-")
	labels[gcpTagAerospikeVersion] = strings.ReplaceAll(v.aerospikeVersion, ".", "-")
	labels[gcpTagUsedBy] = gcpTagUsedByValue
	labels["Arch"] = isArm
	name := fmt.Sprintf("aerolab4-template-%s_%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion, isArm)

	err = d.createSecurityGroupsIfNotExist()
	if err != nil {
		return fmt.Errorf("firewall: %s", err)
	}

	// TODO: below
	input := ec2.RunInstancesInput{}
	input.DryRun = aws.Bool(false)
	input.ImageId = aws.String(templateId)
	if !v.isArm {
		input.InstanceType = aws.String("t3.medium")
	} else {
		input.InstanceType = aws.String("t4g.medium")
	}
	keyname := keyName
	input.KeyName = &keyname
	// number of EBS volumes of the right size
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("image-id"),
				Values: []*string{aws.String(templateId)},
			},
		},
	}
	images, err := d.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return fmt.Errorf("could not run DescribeImages\n%s", err)
	}
	if images.Images == nil || len(images.Images) == 0 || images.Images[0] == nil {
		return errors.New("image not found")
	}
	myImage := images.Images[0]
	if myImage.Architecture != nil && v.isArm && !strings.Contains(*myImage.Architecture, "arm") {
		return fmt.Errorf("requested arm system with non-arm AMI")
	}
	if myImage.Architecture != nil && !v.isArm && strings.Contains(*myImage.Architecture, "arm") {
		return fmt.Errorf("requested non-arm system with arm AMI")
	}

	bdms := []*ec2.BlockDeviceMapping{}
	if myImage.RootDeviceType != nil && myImage.RootDeviceName != nil && *myImage.RootDeviceType == ec2.RootDeviceTypeEbs {
		bdms = []*ec2.BlockDeviceMapping{
			{
				DeviceName: myImage.RootDeviceName,
				Ebs: &ec2.EbsBlockDevice{
					DeleteOnTermination: aws.Bool(true),
					VolumeSize:          aws.Int64(12),
					VolumeType:          aws.String("gp2"),
				},
			},
		}
	}
	input.BlockDeviceMappings = bdms
	//end ebs part
	one := int64(1)
	input.MaxCount = &one
	input.MinCount = &one
	input.TagSpecifications = tsa
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}
	reservationsX, err := d.ec2svc.RunInstances(&input)
	if err != nil {
		return fmt.Errorf("could not run RunInstances\n%s", err)
	}
	reservations = append(reservations, reservationsX)
	// wait for instances to be made available via SSH
	instanceCount := 0
	for _, reservation := range reservations {
		instanceCount = instanceCount + len(reservation.Instances)
	}
	instanceReady := 0
	timeStart := time.Now()
	var nout *ec2.DescribeInstancesOutput
	for instanceReady < instanceCount {
		for _, reservation := range reservations {
			for _, instance := range reservation.Instances {
				err := d.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
					InstanceIds: []*string{instance.InstanceId},
				})
				if err != nil {
					return fmt.Errorf("failed on waiting for instance to be running: %s", err)
				}
				// get PublicIpAddress as it's not assigned at first so the field is empty
				for ntry := 1; ntry <= 3; ntry += 1 {
					nout, err = d.ec2svc.DescribeInstances(&ec2.DescribeInstancesInput{
						InstanceIds: []*string{instance.InstanceId},
					})
					if err != nil {
						if strings.Contains(err.Error(), "Request limit exceeded") && ntry < 3 {
							time.Sleep(time.Duration(ntry*10) * time.Second)
						} else {
							return fmt.Errorf("failed on describe instance which is running: %s", err)
						}
					} else {
						break
					}
				}

				if len(nout.Reservations) == 0 {
					fmt.Println("Got zero reservations back")
					time.Sleep(time.Second)
					continue
				}

				if len(nout.Reservations[0].Instances) == 0 {
					fmt.Println("AWS returned 0 instances running")
					time.Sleep(time.Second)
					continue
				}

				if nout.Reservations[0].Instances[0].PublicIpAddress == nil {
					fmt.Println("Have not received Public IP Address from AWS - just slow, or subnet in AWS is misconfigured - must be default provide public IP address")
					time.Sleep(time.Second)
					continue
				}
				if len(deployAwsTemplateShutdownMaking) > 0 {
					for {
						time.Sleep(time.Second)
					}
				}

				_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls", 0)
				if err == nil {
					instanceReady = instanceReady + 1
				} else {
					fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", *nout.Reservations[0].Instances[0].PublicIpAddress, keyPath, err)
					time.Sleep(time.Second)
				}
			}
		}
		td := time.Since(timeStart)
		if td.Seconds() > 600 {
			return errors.New("waited for 600 seconds for SSH to come up, but it hasn't. Giving up")
		}
	}

	instance := nout.Reservations[0].Instances[0]
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	fmt.Println("Connection succeeded, continuing deployment...")

	// sort out root/ubuntu issues
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh", 0)
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh", 0)
		if err != nil {
			return fmt.Errorf("mkdir .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh", 0)
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh", 0)
		if err != nil {
			return fmt.Errorf("chown .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh", 0)
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh", 0)
		if err != nil {
			return fmt.Errorf("chmod .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/", 0)
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/", 0)
		if err != nil {
			return fmt.Errorf("cp .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys", 0)
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys", 0)
		if err != nil {
			return fmt.Errorf("chmod auth_keys failed: %s\n%s", string(out), err)
		}
	}
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// copy files as required to VM
	files = append(files, fileList{"/root/installer.sh", strings.NewReader(script), len(script)})
	err = scp("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, files)
	if err != nil {
		return fmt.Errorf("scp failed: %s", err)
	}
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// run script as required
	_, err = remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "chmod 755 /root/installer.sh", 0)
	if err != nil {
		out, err := remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "chmod 755 /root/installer.sh", 0)
		if err != nil {
			return fmt.Errorf("chmod failed: %s\n%s", string(out), err)
		}
	}
	out, err := remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "/bin/bash -c /root/installer.sh", 0)
	if err != nil {
		return fmt.Errorf("/root/installer.sh failed: %s\n%s", string(out), err)
	}
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// shutdown VM
	input1 := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			aws.String(*instance.InstanceId),
		},
		DryRun: aws.Bool(false),
	}
	result, err := d.ec2svc.StopInstances(input1)
	if err != nil {
		return fmt.Errorf("error stopping instance %s\n%s\n%s", *instance.InstanceId, result, err)
	}
	d.ec2svc.WaitUntilInstanceStopped(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{instance.InstanceId},
	})
	if len(deployAwsTemplateShutdownMaking) > 0 {
		for {
			time.Sleep(time.Second)
		}
	}

	// create ami from VM
	cii := ec2.CreateImageInput{}
	cii.InstanceId = instance.InstanceId
	cii.DryRun = aws.Bool(false)
	cii.Name = aws.String(fmt.Sprintf("aerolab4-template-%s_%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion, isArm))
	cii.NoReboot = aws.Bool(true)
	cio, err := d.ec2svc.CreateImage(&cii)
	if err != nil {
		return fmt.Errorf("error creating AMI from instance %s", err)
	}
	imageId := cio.ImageId
	d.ec2svc.WaitUntilImageAvailable(&ec2.DescribeImagesInput{
		DryRun:   aws.Bool(false),
		ImageIds: []*string{imageId},
	})
	// add tags to the image
	imageTags := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgNodeNumber1, &tgUsedBy, &tgName, &tgArch}
	imageTags = append(imageTags, extraTags...)
	cti := ec2.CreateTagsInput{
		DryRun:    aws.Bool(false),
		Resources: []*string{imageId},
		Tags:      imageTags,
	}
	d.ec2svc.CreateTags(&cti)

	// erase VM
	input2 := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(*instance.InstanceId),
		},
		DryRun: aws.Bool(false),
	}
	result1, err := d.ec2svc.TerminateInstances(input2)
	if err != nil {
		return fmt.Errorf("error terminating template instance %s\n%s\n%s", *instance.InstanceId, result1, err)
	}
	return nil
}

func (d *backendGcp) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
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

func (d *backendGcp) createSecurityGroupsIfNotExist() error {
}
