package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/bestmethod/inslice"
)

func (d *backendAws) Arch() TypeArch {
	return TypeArchUndef
}

type backendAws struct {
	sess   *session.Session
	ec2svc *ec2.EC2
	server bool
	client bool
}

func init() {
	addBackend("aws", &backendAws{})
}

const (
	// server
	awsServerTagUsedBy           = "UsedBy"
	awsServerTagUsedByValue      = "aerolab4"
	awsServerTagClusterName      = "Aerolab4ClusterName"
	awsServerTagNodeNumber       = "Aerolab4NodeNumber"
	awsServerTagOperatingSystem  = "Aerolab4OperatingSystem"
	awsServerTagOSVersion        = "Aerolab4OperatingSystemVersion"
	awsServerTagAerospikeVersion = "Aerolab4AerospikeVersion"

	// client
	awsClientTagUsedBy           = "UsedBy"
	awsClientTagUsedByValue      = "aerolab4client"
	awsClientTagClusterName      = "Aerolab4clientClusterName"
	awsClientTagNodeNumber       = "Aerolab4clientNodeNumber"
	awsClientTagOperatingSystem  = "Aerolab4clientOperatingSystem"
	awsClientTagOSVersion        = "Aerolab4clientOperatingSystemVersion"
	awsClientTagAerospikeVersion = "Aerolab4clientAerospikeVersion"
)

var (
	awsTagUsedBy           = awsServerTagUsedBy
	awsTagUsedByValue      = awsServerTagUsedByValue
	awsTagClusterName      = awsServerTagClusterName
	awsTagNodeNumber       = awsServerTagNodeNumber
	awsTagOperatingSystem  = awsServerTagOperatingSystem
	awsTagOSVersion        = awsServerTagOSVersion
	awsTagAerospikeVersion = awsServerTagAerospikeVersion
)

func (d *backendAws) WorkOnClients() {
	d.server = false
	d.client = true
	awsTagUsedBy = awsClientTagUsedBy
	awsTagUsedByValue = awsClientTagUsedByValue
	awsTagClusterName = awsClientTagClusterName
	awsTagNodeNumber = awsClientTagNodeNumber
	awsTagOperatingSystem = awsClientTagOperatingSystem
	awsTagOSVersion = awsClientTagOSVersion
	awsTagAerospikeVersion = awsClientTagAerospikeVersion
}

func (d *backendAws) WorkOnServers() {
	d.server = true
	d.client = false
	awsTagUsedBy = awsServerTagUsedBy
	awsTagUsedByValue = awsServerTagUsedByValue
	awsTagClusterName = awsServerTagClusterName
	awsTagNodeNumber = awsServerTagNodeNumber
	awsTagOperatingSystem = awsServerTagOperatingSystem
	awsTagOSVersion = awsServerTagOSVersion
	awsTagAerospikeVersion = awsServerTagAerospikeVersion
}

func (d *backendAws) CreateNetwork(name string, driver string, subnet string, mtu string) error {
	return nil
}
func (d *backendAws) DeleteNetwork(name string) error {
	return nil
}
func (d *backendAws) PruneNetworks() error {
	return nil
}
func (d *backendAws) ListNetworks(csv bool, writer io.Writer) error {
	return nil
}

func (d *backendAws) Init() error {
	var err error

	d.sess, err = session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		return fmt.Errorf("could not establish a session to aws, make sure your ~/.aws/credentials file is correct\n%s", err)
	}

	var svc *ec2.EC2
	if a.opts.Config.Backend.Region == "" {
		svc = ec2.New(d.sess)
	} else {
		svc = ec2.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	_, err = svc.DescribeRegions(nil)
	if err != nil {
		if strings.HasPrefix(err.Error(), "MissingRegion") {
			return fmt.Errorf("invalid aws config; region could not be resolved; add `region=YOUR-REGION-HERE` to your ~/.aws/config or specify region using `aerolab config backend -t aws -r YOUR-REGION-HERE`: %s", err)
		}
		return fmt.Errorf("could not establish a session to aws, make sure your ~/.aws/credentials file is correct\n%s", err)
	}

	d.ec2svc = svc
	d.WorkOnServers()
	return nil
}

func (d *backendAws) IsSystemArm(systemType string) (bool, error) {
	out, err := d.ec2svc.DescribeInstanceTypes(&ec2.DescribeInstanceTypesInput{
		InstanceTypes: aws.StringSlice([]string{systemType}),
	})
	if err != nil {
		return false, fmt.Errorf("DescribeInstanceTypes: %s", err)
	}
	if len(out.InstanceTypes) == 0 {
		return false, errors.New("instance type not found")
	}
	for _, i := range out.InstanceTypes {
		if i.InstanceType != nil && *i.InstanceType == systemType {
			if i.ProcessorInfo == nil {
				return false, nil
			}
			for _, narch := range i.ProcessorInfo.SupportedArchitectures {
				if narch != nil && *narch == ec2.ArchitectureTypeArm64 {
					return true, nil
				} else {
					return false, nil
				}
			}
		}
	}
	return false, errors.New("instance type could not be matched")
}

func (d *backendAws) ClusterList() ([]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	clusterList := []string{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagClusterName {
						clusterList = append(clusterList, *tag.Value)
					}
				}
			}
		}
	}
	return clusterList, nil
}

func (d *backendAws) IsNodeArm(clusterName string, nodeNumber int) (bool, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(clusterName)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return false, fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNo, err := strconv.Atoi(*tag.Value)
						if err != nil {
							return false, errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
						if nodeNo == nodeNumber {
							if instance.Architecture == nil {
								return false, nil
							}
							if strings.Contains(*instance.Architecture, "arm") {
								return true, nil
							}
							return false, nil
						}
					}
				}
			}
		}
	}
	return false, errors.New("node not found in cluster")
}

func (d *backendAws) NodeListInCluster(name string) ([]int, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	nodeList := []int{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err := strconv.Atoi(*tag.Value)
						if err != nil {
							return nil, errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
						nodeList = append(nodeList, nodeNumber)
					}
				}
			}
		}
	}
	return nodeList, nil
}

func (d *backendAws) ListTemplates() ([]backendVersion, error) {
	v := []backendVersion{}
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	images, err := d.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return v, fmt.Errorf("could not run DescribeImages\n%s", err)
	}
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == awsTagOperatingSystem {
				imOs = *tag.Value
			}
			if *tag.Key == awsTagOSVersion {
				imOsVer = *tag.Value
			}
			if *tag.Key == awsTagAerospikeVersion {
				imAerVer = *tag.Value
			}
		}
		isArm := false
		if image.Architecture != nil && strings.Contains(*image.Architecture, "arm") {
			isArm = true
		}
		v = append(v, backendVersion{imOs, imOsVer, imAerVer, isArm})
	}
	return v, nil
}

func (d *backendAws) TemplateDestroy(v backendVersion) error {
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	images, err := d.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return fmt.Errorf("could not run DescribeImages\n%s", err)
	}
	templateId := ""
	snapIds := []*string{}
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == awsTagOperatingSystem {
				imOs = *tag.Value
			}
			if *tag.Key == awsTagOSVersion {
				imOsVer = *tag.Value
			}
			if *tag.Key == awsTagAerospikeVersion {
				imAerVer = *tag.Value
			}
		}
		if image.Architecture != nil && strings.Contains(*image.Architecture, "arm") && !v.isArm {
			continue
		}
		if image.Architecture != nil && !strings.Contains(*image.Architecture, "arm") && v.isArm {
			continue
		}
		if v.distroName == imOs && v.distroVersion == imOsVer && v.aerospikeVersion == imAerVer {
			templateId = *image.ImageId
			for _, bdm := range image.BlockDeviceMappings {
				if bdm.Ebs != nil && bdm.Ebs.SnapshotId != nil {
					snapId := *bdm.Ebs.SnapshotId
					snapIds = append(snapIds, &snapId)
				}
			}
			break
		}
	}
	if templateId == "" {
		return errors.New("could not find chosen template")
	}
	if len(snapIds) == 0 {
		log.Print("WARNING: backing snapshot for AMI not found")
	}
	rem := ec2.DeregisterImageInput{}
	rem.ImageId = aws.String(templateId)
	rem.DryRun = aws.Bool(false)
	result, err := d.ec2svc.DeregisterImage(&rem)
	if err != nil {
		return fmt.Errorf("could not deregister image:%s\n%s", result, err)
	}
	for _, snapId := range snapIds {
		_, err = d.ec2svc.DeleteSnapshot(&ec2.DeleteSnapshotInput{
			DryRun:     aws.Bool(false),
			SnapshotId: snapId,
		})
		if err != nil {
			log.Printf("Failed to delete snapshot ID=%s %s", *snapId, err)
		}
	}
	return nil
}

func (d *backendAws) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
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

func (d *backendAws) RunCommands(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
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
			out, err = remoteRun("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, comm)
			fout = append(fout, out)
			if checkExecRetcode(err) != 0 {
				return fout, fmt.Errorf("error running `%s`: %s", comm, err)
			}
		}
	}
	return fout, nil
}

func (d *backendAws) GetClusterNodeIps(name string) ([]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	nodeList := []string{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				nodeList = append(nodeList, *instance.PrivateIpAddress)
			}
		}
	}
	return nodeList, nil
}

func (d *backendAws) ClusterStart(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	var instList []*string
	var stateCodes []int64
	stateCodes = append(stateCodes, int64(0), int64(16))
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					}
				}
				if inslice.HasInt(nodes, nodeNumber) {
					if instance.State.Code != &stateCodes[0] && instance.State.Code != &stateCodes[1] {
						input := &ec2.StartInstancesInput{
							InstanceIds: []*string{
								aws.String(*instance.InstanceId),
							},
							DryRun: aws.Bool(false),
						}
						result, err := d.ec2svc.StartInstances(input)
						if err != nil {
							return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
						}
					}
					instList = append(instList, instance.InstanceId)
				}
			}
		}
	}

	log.Print("Waiting for Instance to be UP")
	d.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instList,
	})

	start := time.Now()
	// wait for IPs
	log.Print("Waiting for Public IPs")
	var nip map[int]string
	for {
		nip, err = d.GetNodeIpMap(name, false)
		if err != nil {
			return err
		}
		allOk := true
		for _, node := range nodes {
			if mip, ok := nip[node]; !ok || mip == "" {
				fmt.Printf("Node %d does not have an IP yet, waiting...", node)
				time.Sleep(10 * time.Second)
				allOk = false
				break
			}
		}
		if allOk {
			break
		}
		if time.Since(start) > time.Second*600 {
			return errors.New("didn't get Public IP for 10 minutes, giving up")
		}
	}

	start = time.Now()
	// wait for SSH to be up
	log.Print("Waiting for SSH to be available")
	_, keyPath, err := d.getKey(name)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		err = errors.New("waiting")
		for err != nil {
			_, err = remoteRun("root", fmt.Sprintf("%s:22", nip[node]), keyPath, "ls")
			if err != nil {
				if time.Since(start) > time.Second*600 {
					return errors.New("didn't get SSH for 10 minutes, giving up")
				}
				fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", nip[node], keyPath, err)
				time.Sleep(time.Second)
			}
		}
	}
	return nil
}

func (d *backendAws) ClusterStop(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	var instList []*string
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					}
				}
				if inslice.HasInt(nodes, nodeNumber) {
					input := &ec2.StopInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					instList = append(instList, instance.InstanceId)
					result, err := d.ec2svc.StopInstances(input)
					if err != nil {
						return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
					}
				}
			}
		}
	}

	d.ec2svc.WaitUntilInstanceStopped(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instList,
	})
	return nil
}

func (d *backendAws) VacuumTemplate(v backendVersion) error {
	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}

	instances, err := d.ec2svc.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	var instanceIds []*string

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				found := false
				for _, tag := range instance.Tags {
					if *tag.Key == "Name" && *tag.Value == fmt.Sprintf("aerolab4-template-%s_%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion, isArm) {
						found = true
						break
					}
				}
				if found {
					instanceIds = append(instanceIds, instance.InstanceId)
					input := &ec2.TerminateInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					result, err := d.ec2svc.TerminateInstances(input)
					if err != nil {
						return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
					}
				}
			}
		}
	}
	if len(instanceIds) > 0 {
		d.ec2svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
			DryRun:      aws.Bool(false),
			InstanceIds: instanceIds,
		})
	}
	return nil
}

func (d *backendAws) VacuumTemplates() error {
	instances, err := d.ec2svc.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	var instanceIds []*string

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				found := false
				for _, tag := range instance.Tags {
					if *tag.Key == "Name" && strings.HasPrefix(*tag.Value, "aerolab4-template-") {
						found = true
						break
					}
				}
				if found {
					instanceIds = append(instanceIds, instance.InstanceId)
					input := &ec2.TerminateInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					result, err := d.ec2svc.TerminateInstances(input)
					if err != nil {
						return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
					}
				}
			}
		}
	}
	if len(instanceIds) > 0 {
		d.ec2svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
			DryRun:      aws.Bool(false),
			InstanceIds: instanceIds,
		})
	}
	return nil
}

func (d *backendAws) ClusterDestroy(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = d.NodeListInCluster(name)
		if err != nil {
			return fmt.Errorf("could not get IP list\n%s", err)
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	var instanceIds []*string

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					}
				}
				if inslice.HasInt(nodes, nodeNumber) {
					instanceIds = append(instanceIds, instance.InstanceId)
					input := &ec2.TerminateInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					result, err := d.ec2svc.TerminateInstances(input)
					if err != nil {
						return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
					}
				}
			}
		}
	}
	if len(instanceIds) > 0 {
		d.ec2svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
			DryRun:      aws.Bool(false),
			InstanceIds: instanceIds,
		})
	}
	cl, err := d.ClusterList()
	if err != nil {
		return fmt.Errorf("could not kill key as ClusterList failed: %s", err)
	}
	if !inslice.HasString(cl, name) {
		d.killKey(name)
	}

	return nil
}

func (d *backendAws) AttachAndRun(clusterName string, node int, command []string) (err error) {
	return d.RunCustomOut(clusterName, node, command, os.Stdin, os.Stdout, os.Stderr)
}

func (d *backendAws) RunCustomOut(clusterName string, node int, command []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (err error) {
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
	err = remoteAttachAndRun("root", fmt.Sprintf("%s:22", nodeIp[node]), keypath, comm, stdin, stdout, stderr)
	return err
}

func (d *backendAws) GetNodeIpMap(name string, internalIPs bool) (map[int]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagClusterName),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	nodeList := make(map[int]string)
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return nil, errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					}
				}
				nip := instance.PublicIpAddress
				if internalIPs {
					nip = instance.PrivateIpAddress
				}
				if nip != nil {
					nodeList[nodeNumber] = *nip
				} else {
					nodeList[nodeNumber] = "N/A"
				}
			}
		}
	}
	return nodeList, nil
}

type clusterListFull struct {
	ClusterName string
	NodeNumber  string
	IpAddress   string
	PublicIp    string
	InstanceId  string
	State       string
	Arch        string
}

func (d *backendAws) ClusterListFull(isJson bool) (string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return "", fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	clist := []clusterListFull{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code == int64(48) {
				continue
			}
			clist1 := clusterListFull{}
			for _, tag := range instance.Tags {
				if *tag.Key == awsTagClusterName {
					clist1.ClusterName = *tag.Value
				}
				if *tag.Key == awsTagNodeNumber {
					clist1.NodeNumber = *tag.Value
				}
			}
			if instance.PrivateIpAddress != nil {
				clist1.IpAddress = *instance.PrivateIpAddress
			}
			if instance.PublicIpAddress != nil {
				clist1.PublicIp = *instance.PublicIpAddress
			}
			if instance.InstanceId != nil {
				clist1.InstanceId = *instance.InstanceId
			}
			if instance.State != nil && instance.State.Name != nil {
				clist1.State = *instance.State.Name
			}
			if instance.Architecture != nil {
				clist1.Arch = *instance.Architecture
			}
			clist = append(clist, clist1)
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

type templateListFull struct {
	OsName           string
	OsVersion        string
	AerospikeVersion string
	ImageId          string
	Arch             string
}

func (d *backendAws) TemplateListFull(isJson bool) (string, error) {
	result := "Templates:\n\nOS_NAME\t\tOS_VER\t\tAEROSPIKE_VERSION\t\tARCH\n-----------------------------------------\n"
	resList := []templateListFull{}
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	images, err := d.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return "", fmt.Errorf("could not run DescribeImages\n%s", err)
	}
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == awsTagOperatingSystem {
				imOs = *tag.Value
			}
			if *tag.Key == awsTagOSVersion {
				imOsVer = *tag.Value
			}
			if *tag.Key == awsTagAerospikeVersion {
				imAerVer = *tag.Value
			}
		}
		mid := ""
		if image.ImageId != nil {
			mid = *image.ImageId
		}
		result = fmt.Sprintf("%s%s\t\t%s\t\t%s\t\t%s\n", result, imOs, imOsVer, imAerVer, *image.Architecture)
		resList = append(resList, templateListFull{
			OsName:           imOs,
			OsVersion:        imOsVer,
			AerospikeVersion: imAerVer,
			ImageId:          mid,
			Arch:             *image.Architecture,
		})
	}
	if !isJson {
		return result, nil
	}
	out, err := json.MarshalIndent(resList, "", "    ")
	return string(out), err
}

var deployAwsTemplateShutdownMaking = make(chan int, 1)

func (d *backendAws) DeployTemplate(v backendVersion, script string, files []fileList, extra *backendExtra) error {
	addShutdownHandler("deployAwsTemplate", func(os.Signal) {
		deployAwsTemplateShutdownMaking <- 1
		d.VacuumTemplate(v)
	})
	defer delShutdownHandler("deployAwsTemplate")
	var extraTags []*ec2.Tag
	badNames := []string{awsTagOperatingSystem, awsTagOSVersion, awsTagAerospikeVersion, awsTagUsedBy, awsTagClusterName, awsTagNodeNumber, "Arch", "Name"}
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
		extraTags = append(extraTags, &ec2.Tag{
			Key:   aws.String(key),
			Value: aws.String(val),
		})
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
	templateId, err := d.getAmi(a.opts.Config.Backend.Region, v)
	if err != nil {
		return err
	}
	var reservations []*ec2.Reservation
	// tag setup
	tgClusterName := ec2.Tag{}
	tgClusterName.Key = aws.String(awsTagOperatingSystem)
	tgClusterName.Value = aws.String(v.distroName)
	tgNodeNumber := ec2.Tag{}
	tgNodeNumber.Key = aws.String(awsTagOSVersion)
	tgNodeNumber.Value = aws.String(v.distroVersion)
	tgNodeNumber1 := ec2.Tag{}
	tgNodeNumber1.Key = aws.String(awsTagAerospikeVersion)
	tgNodeNumber1.Value = aws.String(v.aerospikeVersion)
	tgUsedBy := ec2.Tag{}
	tgUsedBy.Key = aws.String(awsTagUsedBy)
	tgUsedBy.Value = aws.String(awsTagUsedByValue)
	isArm := "amd"
	if v.isArm {
		isArm = "arm"
	}
	tgArch := ec2.Tag{}
	tgArch.Key = aws.String("Arch")
	tgArch.Value = aws.String(isArm)
	tgName := ec2.Tag{}
	tgName.Key = aws.String("Name")
	tgName.Value = aws.String(fmt.Sprintf("aerolab4-template-%s_%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion, isArm))
	tgs := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgNodeNumber1, &tgUsedBy, &tgName, &tgArch}
	tgs = append(tgs, extraTags...)
	ts := ec2.TagSpecification{}
	ts.Tags = tgs
	ntype := "instance"
	ts.ResourceType = &ntype
	ts1 := ec2.TagSpecification{}
	ts1.Tags = tgs
	ntype1 := "volume"
	ts1.ResourceType = &ntype1
	tsa := []*ec2.TagSpecification{&ts, &ts1}
	// end tag setup
	input := ec2.RunInstancesInput{}
	//this is needed - security group iD
	extra.securityGroupID, extra.subnetID, err = d.resolveSecGroupAndSubnet(extra.securityGroupID, extra.subnetID, true)
	if err != nil {
		return err
	}
	secgroupIds := strings.Split(extra.securityGroupID, ",")
	input.SecurityGroupIds = aws.StringSlice(secgroupIds)
	subnetId := extra.subnetID
	if subnetId != "" {
		input.SubnetId = &subnetId
	}
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
			&ec2.BlockDeviceMapping{
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

				_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls")
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
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
		if err != nil {
			return fmt.Errorf("mkdir .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
		if err != nil {
			return fmt.Errorf("chown .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
		if err != nil {
			return fmt.Errorf("chmod .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/")
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/")
		if err != nil {
			return fmt.Errorf("cp .ssh failed: %s\n%s", string(out), err)
		}
	}
	_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
	if err != nil {
		out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
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
	_, err = remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "chmod 755 /root/installer.sh")
	if err != nil {
		out, err := remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "chmod 755 /root/installer.sh")
		if err != nil {
			return fmt.Errorf("chmod failed: %s\n%s", string(out), err)
		}
	}
	out, err := remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "/bin/bash -c /root/installer.sh")
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

func (d *backendAws) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
	var extraTags []*ec2.Tag
	badNames := []string{awsTagOperatingSystem, awsTagOSVersion, awsTagAerospikeVersion, awsTagUsedBy, awsTagClusterName, awsTagNodeNumber, "Arch", "Name"}
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
		extraTags = append(extraTags, &ec2.Tag{
			Key:   aws.String(key),
			Value: aws.String(val),
		})
	}
	hostType := extra.instanceType
	if len(strings.Trim(extra.ebs, "\t\r\n ")) == 0 {
		return errors.New("root disk size must be specified")
	}
	disks := strings.Split(extra.ebs, ",")
	disksInt := []int64{}
	for _, disk := range disks {
		dint, err := strconv.Atoi(disk)
		if err != nil {
			return fmt.Errorf("incorrect format for disk mapping - must be comma-separated integers: %s", err)
		}
		disksInt = append(disksInt, int64(dint))
	}

	templateId := ""
	var myImage *ec2.Image
	if !d.client {
		filterA := ec2.DescribeImagesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("tag:" + awsTagUsedBy),
					Values: []*string{aws.String(awsTagUsedByValue)},
				},
			},
		}
		images, err := d.ec2svc.DescribeImages(&filterA)
		if err != nil {
			return fmt.Errorf("could not run DescribeImages\n%s", err)
		}
		for _, image := range images.Images {
			var imOs string
			var imOsVer string
			var imAerVer string
			var imArch string
			for _, tag := range image.Tags {
				if *tag.Key == awsTagOperatingSystem {
					imOs = *tag.Value
				}
				if *tag.Key == awsTagOSVersion {
					imOsVer = *tag.Value
				}
				if *tag.Key == awsTagAerospikeVersion {
					imAerVer = *tag.Value
				}
				if *tag.Key == "Arch" {
					imArch = *tag.Value
				}
			}
			if v.distroName == imOs && v.distroVersion == imOsVer && v.aerospikeVersion == imAerVer {
				if (v.isArm && imArch == "arm") || (!v.isArm && (imArch == "amd" || imArch == "")) {
					templateId = *image.ImageId
					myImage = image
					break
				}
			}
		}
		if templateId == "" {
			return errors.New("could not find chosen template")
		}
	} else {
		var err error
		templateId, err = d.getAmi(a.opts.Config.Backend.Region, v)
		if err != nil {
			return err
		}
		filterA := ec2.DescribeImagesInput{
			ImageIds: []*string{aws.String(templateId)},
		}
		images, err := d.ec2svc.DescribeImages(&filterA)
		if err != nil {
			return fmt.Errorf("could not run DescribeImages\n%s", err)
		}
		templateId = ""
		for _, image := range images.Images {
			templateId = *image.ImageId
			myImage = image
		}
		if templateId == "" {
			return errors.New("could not find chosen template")
		}
	}
	if myImage.Architecture != nil && v.isArm && !strings.Contains(*myImage.Architecture, "arm") {
		return fmt.Errorf("requested arm system with non-arm AMI")
	}
	if myImage.Architecture != nil && !v.isArm && strings.Contains(*myImage.Architecture, "arm") {
		return fmt.Errorf("requested non-arm system with arm AMI")
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
	var reservations []*ec2.Reservation
	var keyPath string
	for i := start; i < (nodeCount + start); i++ {
		// tag setup
		tgClusterName := ec2.Tag{}
		tgClusterName.Key = aws.String(awsTagClusterName)
		tgClusterName.Value = aws.String(name)
		tgNodeNumber := ec2.Tag{}
		tgNodeNumber.Key = aws.String(awsTagNodeNumber)
		tgNodeNumber.Value = aws.String(strconv.Itoa(i))
		tgUsedBy := ec2.Tag{}
		tgUsedBy.Key = aws.String(awsTagUsedBy)
		tgUsedBy.Value = aws.String(awsTagUsedByValue)
		tgName := ec2.Tag{}
		tgName.Key = aws.String("Name")
		tgName.Value = aws.String(fmt.Sprintf("aerolab4-%s_%d", name, i))
		tgs := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgUsedBy, &tgName}
		tgs = append(tgs, extraTags...)
		ts := ec2.TagSpecification{}
		ts.Tags = tgs
		ntype := "instance"
		ts.ResourceType = &ntype
		ts1 := ec2.TagSpecification{}
		ts1.Tags = tgs
		ntype1 := "volume"
		ts1.ResourceType = &ntype1
		tsa := []*ec2.TagSpecification{&ts, &ts1}
		// end tag setup
		input := ec2.RunInstancesInput{}
		input.DryRun = aws.Bool(false)
		printID := false
		if i == start {
			printID = true
		}
		extra.securityGroupID, extra.subnetID, err = d.resolveSecGroupAndSubnet(extra.securityGroupID, extra.subnetID, printID)
		if err != nil {
			return err
		}
		secgroupIds := strings.Split(extra.securityGroupID, ",")
		input.SecurityGroupIds = aws.StringSlice(secgroupIds)
		subnetId := extra.subnetID
		if subnetId != "" {
			input.SubnetId = &subnetId
		}
		input.ImageId = aws.String(templateId)
		input.InstanceType = aws.String(hostType)
		var keyname string
		keyname, keyPath, err = d.getKey(name)
		if err != nil {
			d.killKey(name)
			keyname, keyPath, err = d.makeKey(name)
			if err != nil {
				return fmt.Errorf("could not run getKey\n%s", err)
			}
		}
		input.KeyName = &keyname
		// number of EBS volumes of the right size
		// resolve EBS mapping first, to correctly handle splitting of disks
		bdms := []*ec2.BlockDeviceMapping{}
		if myImage.RootDeviceType != nil && myImage.RootDeviceName != nil && *myImage.RootDeviceType == ec2.RootDeviceTypeEbs {
			bdms = []*ec2.BlockDeviceMapping{
				&ec2.BlockDeviceMapping{
					DeviceName: myImage.RootDeviceName,
					Ebs: &ec2.EbsBlockDevice{
						DeleteOnTermination: aws.Bool(true),
						VolumeSize:          aws.Int64(disksInt[0]),
						VolumeType:          aws.String("gp2"),
					},
				},
			}
		}
		avnames := "bcdefghijklmnopqrstuvwxyz"
		for i, av := range disksInt {
			if i == 0 {
				continue
			}
			bdms = append(bdms, &ec2.BlockDeviceMapping{
				DeviceName: aws.String("/dev/xvd" + string(avnames[i])),
				Ebs: &ec2.EbsBlockDevice{
					DeleteOnTermination: aws.Bool(true),
					VolumeSize:          aws.Int64(int64(av)),
					VolumeType:          aws.String("gp2"),
				},
			})
		} // END EBS mapping
		input.BlockDeviceMappings = bdms
		//end ebs part
		one := int64(1)
		input.MaxCount = &one
		input.MinCount = &one
		input.TagSpecifications = tsa
		reservationsX, err := d.ec2svc.RunInstances(&input)
		if err != nil {
			return fmt.Errorf("could not run RunInstances\n%s", err)
		}
		reservations = append(reservations, reservationsX)
	}
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
				if nout == nil {
					return errors.New("aws output is empty(?) and no error happened")
				}
				if len(nout.Reservations) == 0 {
					return errors.New("aws reservations count == 0 and no error happened")
				}
				if len(nout.Reservations[0].Instances) == 0 {
					return errors.New("aws instances count == 0 in reservation[0] and no error happened")
				}
				if nout.Reservations[0].Instances[0].PublicIpAddress == nil {
					return errors.New("no public ip address assigned to the instance")
				}
				_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls")
				if err == nil {
					fmt.Println("Connection succeeded, continuing installation...")
					// sort out root/ubuntu issues
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
						if err != nil {
							return fmt.Errorf("mkdir .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
						if err != nil {
							return fmt.Errorf("chown .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
						if err != nil {
							return fmt.Errorf("chmod .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/")
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/")
						if err != nil {
							return fmt.Errorf("cp .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
						if err != nil {
							return fmt.Errorf("chmod auth_keys failed on %s: %s\n%s", *nout.Reservations[0].Instances[0].PublicIpAddress, string(out), err)
						}
					}
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
	if strings.HasSuffix(v.aerospikeVersion, "f") {
		log.Print("NOTE: Remember to harden the instance with FIPS as well.")
		if v.distroName == "amazon" && v.distroVersion == "2" {
			log.Print("AWS FIPS Enablement Manual: https://aws.amazon.com/blogs/publicsector/enabling-fips-mode-amazon-linux-2/")
		}
		log.Print("Each command can be run on all nodes at the same time, like so:")
		log.Print("  $ aerolab attach shell -n ClusterName -l all -- yum update -y")
	}
	return nil
}

func (d *backendAws) Upload(clusterName string, node int, source string, destination string, verbose bool) error {
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

func (d *backendAws) Download(clusterName string, node int, source string, destination string, verbose bool) error {
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
