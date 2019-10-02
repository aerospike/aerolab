package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type b_aws struct {
	sess     *session.Session
	ec2svc   *ec2.EC2
	host     []string
	hostType string
	disks    []string
}

// return slice of strings holding cluster names, or error
func (b b_aws) ClusterList() ([]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	clusterList := []string{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == "AerolabClusterName" {
						clusterList = append(clusterList, *tag.Value)
					}
				}
			}
		}
	}
	return clusterList, nil
}

// accept cluster name, return slice of int holding node numbers or error
func (b b_aws) NodeListInCluster(name string) ([]int, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	nodeList := []int{}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == "NodeNumber" {
						nodeNumber, err := strconv.Atoi(*tag.Value)
						if err != nil {
							return nil, errors.New("Problem with node numbers in the given cluster. Investigate manually.")
						}
						nodeList = append(nodeList, nodeNumber)
					}
				}
			}
		}
	}
	return nodeList, nil
}

// returns a string slice containing IPs of given cluster name - internal IPs
func (b b_aws) GetClusterNodeIps(name string) ([]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
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

// used by backend to configure itself for remote access. e.g. store b.user, b.host, b.pubkey from given parameters
func (b b_aws) ConfigRemote(host string, pubkey string) backend {
	b.host = strings.Split(host, ":")
	return b
}

// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
func (b b_aws) GetNodeIpMap(name string) (map[int]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	nodeList := make(map[int]string)
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == "NodeNumber" {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return nil, errors.New("Problem with node numbers in the given cluster. Investigate manually.")
						}
					}
				}
				nodeList[nodeNumber] = *instance.PublicIpAddress
			}
		}
	}
	return nodeList, nil
}

// get Backend type
func (b b_aws) GetBackendName() string {
	return "aws"
}

// used to initialize the backend, for example check if docker is installed and install it if not on linux (error on mac)
func (b b_aws) Init() (backend, error) {
	var err error

	b.sess, err = session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not establish a session to aws, make sure your ~/.aws/credentials file is correct\n%s", err))
	}

	svc := ec2.New(b.sess)
	_, err = svc.DescribeRegions(nil)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not establish a session to aws, make sure your ~/.aws/credentials file is correct\n%s", err))
	}

	b.ec2svc = svc

	return b, nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_aws) ClusterStart(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return errors.New(fmt.Sprintf("Could not get IP list\n%s", err))
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	var instList []*string
	var stateCodes []int64
	stateCodes = append(stateCodes, int64(0), int64(16))
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			var nodeNumber int
			for _, tag := range instance.Tags {
				if *tag.Key == "NodeNumber" {
					nodeNumber, err = strconv.Atoi(*tag.Value)
					if err != nil {
						return errors.New("Problem with node numbers in the given cluster. Investigate manually.")
					}
				}
			}
			if inArray(nodes, nodeNumber) != -1 {
				if instance.State.Code != &stateCodes[0] && instance.State.Code != &stateCodes[1] {
					input := &ec2.StartInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					result, err := b.ec2svc.StartInstances(input)
					if err != nil {
						return errors.New(fmt.Sprintf("Error starting instance %s\n%s\n%s", *instance.InstanceId, result, err))
					}
				}
				instList = append(instList, instance.InstanceId)
			}
		}
	}

	b.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instList,
	})
	return nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_aws) ClusterStop(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return errors.New(fmt.Sprintf("Could not get IP list\n%s", err))
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	var instList []*string
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			var nodeNumber int
			for _, tag := range instance.Tags {
				if *tag.Key == "NodeNumber" {
					nodeNumber, err = strconv.Atoi(*tag.Value)
					if err != nil {
						return errors.New("Problem with node numbers in the given cluster. Investigate manually.")
					}
				}
			}
			if inArray(nodes, nodeNumber) != -1 {
				input := &ec2.StopInstancesInput{
					InstanceIds: []*string{
						aws.String(*instance.InstanceId),
					},
					DryRun: aws.Bool(false),
				}
				instList = append(instList, instance.InstanceId)
				result, err := b.ec2svc.StopInstances(input)
				if err != nil {
					return errors.New(fmt.Sprintf("Error starting instance %s\n%s\n%s", *instance.InstanceId, result, err))
				}
			}
		}
	}

	b.ec2svc.WaitUntilInstanceStopped(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instList,
	})
	return nil
}

// /stop/destroy/start cluster of given name. optional nodes slice to only start particular nodes.
func (b b_aws) ClusterDestroy(name string, nodes []int) error {
	var err error
	if len(nodes) == 0 {
		nodes, err = b.NodeListInCluster(name)
		if err != nil {
			return errors.New(fmt.Sprintf("Could not get IP list\n%s", err))
		}
	}
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	var instanceIds []*string

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			var nodeNumber int
			for _, tag := range instance.Tags {
				if *tag.Key == "NodeNumber" {
					nodeNumber, err = strconv.Atoi(*tag.Value)
					if err != nil {
						return errors.New("Problem with node numbers in the given cluster. Investigate manually.")
					}
				}
			}
			if inArray(nodes, nodeNumber) != -1 {
				instanceIds = append(instanceIds, instance.InstanceId)
				input := &ec2.TerminateInstancesInput{
					InstanceIds: []*string{
						aws.String(*instance.InstanceId),
					},
					DryRun: aws.Bool(false),
				}
				result, err := b.ec2svc.TerminateInstances(input)
				if err != nil {
					return errors.New(fmt.Sprintf("Error starting instance %s\n%s\n%s", *instance.InstanceId, result, err))
				}
			}
		}
	}
	b.ec2svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instanceIds,
	})
	cl, err := b.ClusterList()
	if err != nil {
		return makeError("Could not kill key as ClusterList failed: %s", err)
	}
	if inArray(cl, name) == -1 {
		b.killKey(name)
	}

	return nil
}

// returns an unformatted string with list of clusters, to be printed to user
func (b b_aws) ClusterListFull() (string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	result := "Clusters:\n\nCLUSTER_NAME\t\tNODE_NO\t\tIP_ADDRESS\n------------------------------------------\n"
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var clusterName string
				var nodeNumber string
				var ipAddress string
				for _, tag := range instance.Tags {
					if *tag.Key == "AerolabClusterName" {
						clusterName = *tag.Value
					}
					if *tag.Key == "NodeNumber" {
						nodeNumber = *tag.Value
					}
				}
				if instance.PrivateIpAddress != nil {
					ipAddress = *instance.PrivateIpAddress
				} else {
					ipAddress = ""
				}
				result = fmt.Sprintf("%s%s\t\t%s\t\t%s\n", result, clusterName, nodeNumber, ipAddress)
			}
		}
	}
	// get templates list and return here as well
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	result = fmt.Sprintf("%s\n====================\n\nTemplates:\n\nOS_NAME\t\tOS_VER\t\tAEROSPIKE_VERSION\n-----------------------------------------\n", result)
	images, err := b.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Could not run DescribeImages\n%s", err))
	}
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == "OperatingSystem" {
				imOs = *tag.Value
			}
			if *tag.Key == "OperatingSystemVersion" {
				imOsVer = *tag.Value
			}
			if *tag.Key == "AerospikeVersion" {
				imAerVer = *tag.Value
			}
		}
		result = fmt.Sprintf("%s%s\t\t%s\t\t%s\n", result, imOs, imOsVer, imAerVer)
	}
	return result, nil
}

// return a slice of 'version' structs containing versions of templates available
func (b b_aws) ListTemplates() ([]version, error) {
	v := []version{}
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	images, err := b.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return v, errors.New(fmt.Sprintf("Could not run DescribeImages\n%s", err))
	}
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == "OperatingSystem" {
				imOs = *tag.Value
			}
			if *tag.Key == "OperatingSystemVersion" {
				imOsVer = *tag.Value
			}
			if *tag.Key == "AerospikeVersion" {
				imAerVer = *tag.Value
			}
		}
		v = append(v, version{imOs, imOsVer, imAerVer})
	}
	return v, nil
}

// destroy template for a given version
func (b b_aws) TemplateDestroy(v version) error {
	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	images, err := b.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run DescribeImages\n%s", err))
	}
	templateId := ""
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == "OperatingSystem" {
				imOs = *tag.Value
			}
			if *tag.Key == "OperatingSystemVersion" {
				imOsVer = *tag.Value
			}
			if *tag.Key == "AerospikeVersion" {
				imAerVer = *tag.Value
			}
		}
		if v.distroName == imOs && v.distroVersion == imOsVer && v.aerospikeVersion == imAerVer {
			templateId = *image.ImageId
			break
		}
	}
	if templateId == "" {
		return errors.New("Could not find chosen template")
	}
	rem := ec2.DeregisterImageInput{}
	rem.ImageId = aws.String(templateId)
	rem.DryRun = aws.Bool(false)
	result, err := b.ec2svc.DeregisterImage(&rem)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not deregister image:%s\n%s\n", result, err))
	}
	return nil
}

// deploy cluster from template, requires version, name of new cluster and node count to deploy
func (b b_aws) DeployCluster(v version, name string, nodeCount int, exposePorts []string) error {

	if len(b.host) < 2 {
		return errors.New("Host must be set to type:DISK_SIZE at least, in GB. E.g.: t2.micro:10. Using t2.micro:10:20 would create 2 EBS voluems, size 10GB and 20GB. First is OS volume.")
	}
	b.hostType = b.host[0]
	b.disks = b.host[1:]

	filterA := ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:UsedBy"),
				Values: []*string{aws.String("aerolab")},
			},
		},
	}
	images, err := b.ec2svc.DescribeImages(&filterA)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run DescribeImages\n%s", err))
	}
	templateId := ""
	for _, image := range images.Images {
		var imOs string
		var imOsVer string
		var imAerVer string
		for _, tag := range image.Tags {
			if *tag.Key == "OperatingSystem" {
				imOs = *tag.Value
			}
			if *tag.Key == "OperatingSystemVersion" {
				imOsVer = *tag.Value
			}
			if *tag.Key == "AerospikeVersion" {
				imAerVer = *tag.Value
			}
		}
		if v.distroName == imOs && v.distroVersion == imOsVer && v.aerospikeVersion == imAerVer {
			templateId = *image.ImageId
			break
		}
	}
	if templateId == "" {
		return errors.New("Could not find chosen template")
	}
	nodes, err := b.NodeListInCluster(name)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run NodeListInCluster\n%s", err))
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
		tgClusterName.Key = aws.String("AerolabClusterName")
		tgClusterName.Value = aws.String(name)
		tgNodeNumber := ec2.Tag{}
		tgNodeNumber.Key = aws.String("NodeNumber")
		tgNodeNumber.Value = aws.String(strconv.Itoa(i))
		tgUsedBy := ec2.Tag{}
		tgUsedBy.Key = aws.String("UsedBy")
		tgUsedBy.Value = aws.String("aerolab")
		tgName := ec2.Tag{}
		tgName.Key = aws.String("Name")
		tgName.Value = aws.String(fmt.Sprintf("aerolab-%s_%d", name, i))
		tgs := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgUsedBy, &tgName}
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
		secgroupIds := os.Getenv("aerolabSecurityGroupId")
		input.SecurityGroupIds = []*string{&secgroupIds}
		input.ImageId = aws.String(templateId)
		input.InstanceType = aws.String(b.hostType)
		var keyname string
		keyname, keyPath, err = b.getKey(name)
		if err != nil {
			keyname, keyPath, err = b.makeKey(name)
			if err != nil {
				return errors.New(fmt.Sprintf("Could not run getKey\n%s", err))
			}
		}
		input.KeyName = &keyname
		// number of EBS volumes of the right size
		var bdms []*ec2.BlockDeviceMapping
		for ni, device := range b.disks {
			ebd := ec2.EbsBlockDevice{}
			ebd.DeleteOnTermination = aws.Bool(true)
			// Iops and VolumeType available here
			vsize, err := strconv.Atoi(device)
			if err != nil {
				return errors.New(fmt.Sprintf("Could not convert device size to int\n%s", err))
			}
			vsize64 := int64(vsize)
			ebd.VolumeSize = &vsize64

			bdm := ec2.BlockDeviceMapping{}
			bdm.Ebs = &ebd
			var dn string
			if ni == 0 {
				dn = "/dev/sda1"
			} else {
				dn = fmt.Sprintf("/dev/xvd%s", string('a'+ni-1))
			}
			bdm.DeviceName = &dn
			bdms = append(bdms, &bdm)
		}
		input.BlockDeviceMappings = bdms
		//end ebs part
		one := int64(1)
		input.MaxCount = &one
		input.MinCount = &one
		input.TagSpecifications = tsa
		reservationsX, err := b.ec2svc.RunInstances(&input)
		if err != nil {
			return errors.New(fmt.Sprintf("Could not run RunInstances\n%s", err))
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
				err := b.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
					InstanceIds: []*string{instance.InstanceId},
				})
				if err != nil {
					return makeError("Failed on waiting for instance to be running: %s", err)
				}
				// get PublicIpAddress as it's not assigned at first so the field is empty
				for ntry := 1; ntry <= 3; ntry += 1 {
					nout, err = b.ec2svc.DescribeInstances(&ec2.DescribeInstancesInput{
						InstanceIds: []*string{instance.InstanceId},
					})
					if err != nil {
						if strings.Contains(err.Error(), "Request limit exceeded") && ntry < 3 {
							time.Sleep(time.Duration(ntry*10) * time.Second)
						} else {
							return makeError("Failed on describe instance which is running: %s", err)
						}
					} else {
						break
					}
				}
				_, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls")
				if err == nil {
					// sort out root/ubuntu issues
					out, err := remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
					if err != nil {
						return makeError("mkdir .ssh failed: %s\n%s", string(out), err)
					}
					out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
					if err != nil {
						return makeError("chown .ssh failed: %s\n%s", string(out), err)
					}
					out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
					if err != nil {
						return makeError("chmod .ssh failed: %s\n%s", string(out), err)
					}
					out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo cp /home/ubuntu/.ssh/authorized_keys /root/.ssh/")
					if err != nil {
						return makeError("cp .ssh failed: %s\n%s", string(out), err)
					}
					out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
					if err != nil {
						return makeError("chmod auth_keys failed: %s\n%s", string(out), err)
					}
					instanceReady = instanceReady + 1
				} else {
					fmt.Println(err)
					time.Sleep(time.Second)
				}
			}
		}
		td := time.Now().Sub(timeStart)
		if td.Seconds() > 600 {
			return errors.New("Waited for 600 seconds for SSH to come up, but it hasn't. Giving up.")
		}
	}
	return nil
}

// attach to a node in cluster and run a single command. does not return output of command.
func (b b_aws) AttachAndRun(clusterName string, node int, command []string) (err error) {
	clusters, err := b.ClusterList()
	if err != nil {
		return makeError("Could not get cluster list: %s", err)
	}
	if inArray(clusters, clusterName) == -1 {
		return makeError("Cluster not found")
	}
	nodes, err := b.NodeListInCluster(clusterName)
	if err != nil {
		return makeError("Could not get node list: %s", err)
	}
	if inArray(nodes, node) == -1 {
		return makeError("Node not found")
	}
	nodeIp, err := b.GetNodeIpMap(clusterName)
	if err != nil {
		return makeError("Could not get node ip map: %s", err)
	}
	_, keypath, err := b.getKey(clusterName)
	if err != nil {
		return makeError("Could not get key path: %s", err)
	}
	var comm string
	if len(command) > 0 {
		comm = command[0]
		for _, c := range command[1:] {
			if strings.Contains(c, " ") {
				comm = comm + " \"" + c + "\""
			} else {
				comm = comm + " " + c
			}
		}
	} else {
		comm = "bash"
	}
	err = remoteAttachAndRun("root", fmt.Sprintf("%s:22", nodeIp[node]), keypath, comm)
	return err
}

// run command(s) inside node(s) in cluster. Requires cluster name, commands as slice of command slices, and nodes list slice
// returns a slice of byte slices containing each node/command output and error
func (b b_aws) RunCommand(clusterName string, commands [][]string, nodes []int) ([][]byte, error) {
	var fout [][]byte
	var err error
	if nodes == nil {
		nodes, err = b.NodeListInCluster(clusterName)
		if err != nil {
			return nil, err
		}
	}
	_, keypath, err := b.getKey(clusterName)
	if err != nil {
		return nil, makeError("Could not get key:%s", err)
	}
	nodeIps, err := b.GetNodeIpMap(clusterName)
	if err != nil {
		return nil, makeError("Could not get node ip map:%s", err)
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
			if check_exec_retcode(err) != 0 {
				return fout, errors.New(fmt.Sprintf("ERROR running `%s`: %s\n", comm, err))
			}
		}
	}
	return fout, nil
}

// deploy cluster from template, requires version, name of new cluster and node count to deploy. accept and use limits
func (b b_aws) DeployClusterWithLimits(v version, name string, nodeCount int, exposePorts []string, cpuLimit string, ramLimit string, swapLimit string, privileged bool) error {
	return b.DeployCluster(v, name, nodeCount, exposePorts)
}

// copy files to cluster, requires cluster name, list of files to copy and list of nodes in cluster to copy to
func (b b_aws) CopyFilesToCluster(name string, files []fileList, nodes []int) error {
	var err error
	nodeIps, err := b.GetNodeIpMap(name)
	if err != nil {
		return makeError("Could not get node ip map for cluster: %s", err)
	}
	_, keypath, err := b.getKey(name)
	if err != nil {
		return makeError("Could not get key: %s", err)
	}
	for _, node := range nodes {
		err = scp("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, files)
		if err != nil {
			return makeError("scp failed: %s", err)
		}
	}
	return err
}

// get KeyPair
func (b b_aws) getKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
	keyPath = path.Join(os.Getenv("aerolabAWSkeys"), keyName)
	// check keyName exists, if not, error
	filter := ec2.DescribeKeyPairsInput{}
	filter.KeyNames = []*string{&keyName}
	keys, err := b.ec2svc.DescribeKeyPairs(&filter)
	if err != nil {
		err = makeError("Could not DescribeKeypairs: %s", err)
	}
	if len(keys.KeyPairs) != 1 {
		err = makeError("Key pair does not exist in AWS")
	}
	// check keypath exists, if not, error
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		err = makeError("Key does not exist in given location: %s", keyPath)
	}
	return
}

// get KeyPair
func (b b_aws) makeKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
	keyPath = path.Join(os.Getenv("aerolabAWSkeys"), keyName)
	_, _, err = b.getKey(clusterName)
	if err == nil {
		return
	}
	// check keypath exists, if not, make
	if _, err := os.Stat(os.Getenv("aerolabAWSkeys")); os.IsNotExist(err) {
		os.MkdirAll(os.Getenv("aerolabAWSkeys"), 0755)
	}
	// generate keypair
	filter := ec2.CreateKeyPairInput{}
	filter.DryRun = aws.Bool(false)
	filter.KeyName = &keyName
	out, err := b.ec2svc.CreateKeyPair(&filter)
	if err != nil {
		err = makeError("Could not generate keypair: %s", err)
		return
	}
	err = ioutil.WriteFile(keyPath, []byte(*out.KeyMaterial), 0600)
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
	keyPath = path.Join(os.Getenv("aerolabAWSkeys"), keyName)
	return
}

// get KeyPair
func (b b_aws) killKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s", clusterName)
	keyPath = path.Join(os.Getenv("aerolabAWSkeys"), keyName)
	os.Remove(keyPath)
	filter := ec2.DeleteKeyPairInput{}
	filter.DryRun = aws.Bool(false)
	filter.KeyName = &keyName
	b.ec2svc.DeleteKeyPair(&filter)
	return
}

func (b b_aws) getAmi(filter string) (ami string, err error) {
	out, err := exec.Command("aws", "ec2", "describe-images", "--owners", "099720109477", "--filters", filter, "Name=state,Values=available", "--query", "reverse(sort_by(Images, &CreationDate))[:1].ImageId", "--output", "text").CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return strings.Trim(string(out), " \r\n"), nil
}

// deploy a template, naming it with version, running 'script' inside for installation and copying 'files' into it
func (b b_aws) DeployTemplate(v version, script string, files []fileList) error {
	keyName, keyPath, err := b.getKey("template")
	if err != nil {
		keyName, keyPath, err = b.makeKey("template")
		if err != nil {
			return makeError("Could not obtain or make 'template' key:%s", err)
		}
	}
	ami := make(map[string]string)
	ami["ubuntubionic"], err = b.getAmi("Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-*-18.04-amd64-server-????????")
	if err != nil {
		return makeError("ERROR performing AMI discovery: %s\n%s", err, ami["ubuntubionic"])
	}
	ami["ubuntu18.04"], err = b.getAmi("Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-*-18.04-amd64-server-????????")
	if err != nil {
		return makeError("ERROR performing AMI discovery: %s\n%s", err, ami["ubuntu18.04"])
	}
	ami["ubuntu16.04"], err = b.getAmi("Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-*-16.04-amd64-server-????????")
	if err != nil {
		return makeError("ERROR performing AMI discovery: %s\n%s", err, ami["ubuntu16.04"])
	}
	ami["ubuntu14.04"], err = b.getAmi("Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-*-14.04-amd64-server-????????")
	if err != nil {
		return makeError("ERROR performing AMI discovery: %s\n%s", err, ami["ubuntu14.04"])
	}
	ami["ubuntu12.04"], err = b.getAmi("Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-*-14.04-amd64-server-????????")
	if err != nil {
		return makeError("ERROR performing AMI discovery: %s\n%s", err, ami["ubuntu14.04"])
	}

	// start VM
	templateId := ami[fmt.Sprintf("%s%s", v.distroName, v.distroVersion)]
	var reservations []*ec2.Reservation
	// tag setup
	tgClusterName := ec2.Tag{}
	tgClusterName.Key = aws.String("OperatingSystem")
	tgClusterName.Value = aws.String(v.distroName)
	tgNodeNumber := ec2.Tag{}
	tgNodeNumber.Key = aws.String("OperatingSystemVersion")
	tgNodeNumber.Value = aws.String(v.distroVersion)
	tgNodeNumber1 := ec2.Tag{}
	tgNodeNumber1.Key = aws.String("AerospikeVersion")
	tgNodeNumber1.Value = aws.String(v.aerospikeVersion)
	tgUsedBy := ec2.Tag{}
	tgUsedBy.Key = aws.String("UsedBy")
	tgUsedBy.Value = aws.String("aerolab")
	tgName := ec2.Tag{}
	tgName.Key = aws.String("Name")
	tgName.Value = aws.String(fmt.Sprintf("aerolab-template-%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion))
	tgs := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgNodeNumber1, &tgUsedBy, &tgName}
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
	secgroupIds := os.Getenv("aerolabSecurityGroupId")
	subnetId := os.Getenv("aerolabSubnetId")
	input.SecurityGroupIds = []*string{&secgroupIds}
	if subnetId != "" {
		input.SubnetId = &subnetId
	}
	input.DryRun = aws.Bool(false)
	input.ImageId = aws.String(templateId)
	input.InstanceType = aws.String("t2.micro")
	keyname := keyName
	input.KeyName = &keyname
	// number of EBS volumes of the right size
	var bdms []*ec2.BlockDeviceMapping
	ebd := ec2.EbsBlockDevice{}
	ebd.DeleteOnTermination = aws.Bool(true)
	// Iops and VolumeType available here
	vsize64 := int64(8)
	ebd.VolumeSize = &vsize64
	bdm := ec2.BlockDeviceMapping{}
	bdm.Ebs = &ebd
	dn := "/dev/sda1"
	bdm.DeviceName = &dn
	bdms = append(bdms, &bdm)
	input.BlockDeviceMappings = bdms
	//end ebs part
	one := int64(1)
	input.MaxCount = &one
	input.MinCount = &one
	input.TagSpecifications = tsa
	reservationsX, err := b.ec2svc.RunInstances(&input)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not run RunInstances\n%s", err))
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
				err := b.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
					InstanceIds: []*string{instance.InstanceId},
				})
				if err != nil {
					return makeError("Failed on waiting for instance to be running: %s", err)
				}
				// get PublicIpAddress as it's not assigned at first so the field is empty
				for ntry := 1; ntry <= 3; ntry += 1 {
					nout, err = b.ec2svc.DescribeInstances(&ec2.DescribeInstancesInput{
						InstanceIds: []*string{instance.InstanceId},
					})
					if err != nil {
						if strings.Contains(err.Error(), "Request limit exceeded") && ntry < 3 {
							time.Sleep(time.Duration(ntry*10) * time.Second)
						} else {
							return makeError("Failed on describe instance which is running: %s", err)
						}
					} else {
						break
					}
				}

				_, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls")
				if err == nil {
					instanceReady = instanceReady + 1
				} else {
					fmt.Println(err)
					time.Sleep(time.Second)
				}
			}
		}
		td := time.Now().Sub(timeStart)
		if td.Seconds() > 600 {
			return errors.New("Waited for 600 seconds for SSH to come up, but it hasn't. Giving up.")
		}
	}

	instance := nout.Reservations[0].Instances[0]

	// sort out root/ubuntu issues
	out, err := remoteRun("ubuntu", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh")
	if err != nil {
		return makeError("mkdir .ssh failed: %s\n%s", string(out), err)
	}
	out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh")
	if err != nil {
		return makeError("chown .ssh failed: %s\n%s", string(out), err)
	}
	out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh")
	if err != nil {
		return makeError("chmod .ssh failed: %s\n%s", string(out), err)
	}
	out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo cp /home/ubuntu/.ssh/authorized_keys /root/.ssh/")
	if err != nil {
		return makeError("cp .ssh failed: %s\n%s", string(out), err)
	}
	out, err = remoteRun("ubuntu", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys")
	if err != nil {
		return makeError("chmod auth_keys failed: %s\n%s", string(out), err)
	}

	// copy files as required to VM
	files = append(files, fileList{"/root/installer.sh", []byte(script)})
	err = scp("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, files)
	if err != nil {
		return makeError("scp failed: %s", err)
	}

	// run script as required
	out, err = remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "chmod 755 /root/installer.sh")
	if err != nil {
		return makeError("chmod failed: %s\n%s", string(out), err)
	}
	out, err = remoteRun("root", fmt.Sprintf("%s:22", *instance.PublicIpAddress), keyPath, "/bin/bash -c /root/installer.sh")
	if err != nil {
		return makeError("/root/installer.sh failed: %s\n%s", string(out), err)
	}

	// shutdown VM
	input1 := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			aws.String(*instance.InstanceId),
		},
		DryRun: aws.Bool(false),
	}
	result, err := b.ec2svc.StopInstances(input1)
	if err != nil {
		return errors.New(fmt.Sprintf("Error stopping instance %s\n%s\n%s", *instance.InstanceId, result, err))
	}
	b.ec2svc.WaitUntilInstanceStopped(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{instance.InstanceId},
	})

	// create ami from VM
	cii := ec2.CreateImageInput{}
	cii.InstanceId = instance.InstanceId
	cii.DryRun = aws.Bool(false)
	cii.Name = aws.String(fmt.Sprintf("aerolab-template-%s_%s_%s", v.distroName, v.distroVersion, v.aerospikeVersion))
	cii.NoReboot = aws.Bool(true)
	cio, err := b.ec2svc.CreateImage(&cii)
	if err != nil {
		return errors.New(fmt.Sprintf("Error creating AMI from instance %s", err))
	}
	imageId := cio.ImageId
	b.ec2svc.WaitUntilImageAvailable(&ec2.DescribeImagesInput{
		DryRun:   aws.Bool(false),
		ImageIds: []*string{imageId},
	})
	// add tags to the image
	cti := ec2.CreateTagsInput{
		DryRun:    aws.Bool(false),
		Resources: []*string{imageId},
		Tags:      []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgNodeNumber1, &tgUsedBy, &tgName},
	}
	b.ec2svc.CreateTags(&cti)

	// erase VM
	input2 := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(*instance.InstanceId),
		},
		DryRun: aws.Bool(false),
	}
	result1, err := b.ec2svc.TerminateInstances(input2)
	if err != nil {
		return errors.New(fmt.Sprintf("Error starting instance %s\n%s\n%s", *instance.InstanceId, result1, err))
	}
	return nil
}

// returns a map of [int]string for a given cluster, where int is node number and string is the IP of said node
func (b b_aws) GetNodeIpMapInternal(name string) (map[int]string, error) {
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:AerolabClusterName"),
				Values: []*string{aws.String(name)},
			},
		},
	}
	instances, err := b.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not run DescribeInstances\n%s", err))
	}
	nodeList := make(map[int]string)
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code != int64(48) {
				var nodeNumber int
				for _, tag := range instance.Tags {
					if *tag.Key == "NodeNumber" {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return nil, errors.New("Problem with node numbers in the given cluster. Investigate manually.")
						}
					}
				}
				nodeList[nodeNumber] = *instance.PrivateIpAddress
			}
		}
	}
	return nodeList, nil
}
