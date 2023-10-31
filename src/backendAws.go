package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/aws/aws-sdk-go/service/scheduler"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/bestmethod/inslice"
	"github.com/google/uuid"
)

func (d *backendAws) Arch() TypeArch {
	return TypeArchUndef
}

type backendAws struct {
	sess      *session.Session
	ec2svc    *ec2.EC2
	lambda    *lambda.Lambda
	scheduler *scheduler.Scheduler
	iam       *iam.IAM
	sts       *sts.STS
	efs       *efs.EFS
	server    bool
	client    bool
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
	awsClientTagClientType       = "Aerolab4clientType"
)

var (
	awsTagUsedBy           = awsServerTagUsedBy
	awsTagUsedByValue      = awsServerTagUsedByValue
	awsTagClusterName      = awsServerTagClusterName
	awsTagNodeNumber       = awsServerTagNodeNumber
	awsTagOperatingSystem  = awsServerTagOperatingSystem
	awsTagOSVersion        = awsServerTagOSVersion
	awsTagAerospikeVersion = awsServerTagAerospikeVersion
	awsTagCostPerHour      = "Aerolab4CostPerHour"
	awsTagCostLastRun      = "Aerolab4CostSoFar"
	awsTagCostStartTime    = "Aerolab4CostStartTime"
	awsTagEFSKey           = "UsedBy"
	awsTagEFSValue         = "aerolab7"
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

func (d *backendAws) CreateMountTarget(volume *inventoryVolume, subnet string, secGroups []string) (inventoryMountTarget, error) {
	out, err := d.efs.CreateMountTarget(&efs.CreateMountTargetInput{
		FileSystemId:   aws.String(volume.FileSystemId),
		SecurityGroups: aws.StringSlice(secGroups),
		SubnetId:       aws.String(subnet),
	})
	if err != nil {
		return inventoryMountTarget{}, err
	}
	return inventoryMountTarget{
		AvailabilityZoneId:   aws.StringValue(out.AvailabilityZoneId),
		AvailabilityZoneName: aws.StringValue(out.AvailabilityZoneName),
		FileSystemId:         aws.StringValue(out.FileSystemId),
		IpAddress:            aws.StringValue(out.IpAddress),
		LifeCycleState:       aws.StringValue(out.LifeCycleState),
		MountTargetId:        aws.StringValue(out.MountTargetId),
		NetworkInterfaceId:   aws.StringValue(out.NetworkInterfaceId),
		AWSOwnerId:           aws.StringValue(out.OwnerId),
		SubnetId:             aws.StringValue(out.SubnetId),
		VpcId:                aws.StringValue(out.VpcId),
		SecurityGroups:       secGroups,
	}, nil
}

func (d *backendAws) MountTargetAddSecurityGroup(mountTarget *inventoryMountTarget, volume *inventoryVolume, addGroups []string) error {
	_, err := d.efs.ModifyMountTargetSecurityGroups(&efs.ModifyMountTargetSecurityGroupsInput{
		MountTargetId:  aws.String(mountTarget.MountTargetId),
		SecurityGroups: aws.StringSlice(addGroups),
	})
	return err
}

func (d *backendAws) DeleteVolume(name string) error {
	vols, err := b.Inventory("", []int{InventoryItemVolumes})
	if err != nil {
		return fmt.Errorf("could not enumerate through volumes: %s", err)
	}
	for _, vol := range vols.Volumes {
		if vol.Name != name {
			continue
		}
		if vol.NumberOfMountTargets > 0 {
			log.Println("Deleting mount targets")
			for _, mt := range vol.MountTargets {
				_, err = d.efs.DeleteMountTarget(&efs.DeleteMountTargetInput{
					MountTargetId: aws.String(mt.MountTargetId),
				})
				if err != nil {
					return fmt.Errorf("failed to remove mount target: %s", err)
				}
			}
			log.Println("Waiting for mount targets to be deleted by AWS")
			for {
				time.Sleep(5 * time.Second)
				targets, err := d.efs.DescribeMountTargets(&efs.DescribeMountTargetsInput{
					FileSystemId: aws.String(vol.FileSystemId),
				})
				if err != nil {
					return fmt.Errorf("error waiting on mount targets to be deleted: %s", err)
				}
				if len(targets.MountTargets) == 0 {
					break
				}
			}
		}
		log.Println("Deleting volume")
		_, err = d.efs.DeleteFileSystem(&efs.DeleteFileSystemInput{
			FileSystemId: aws.String(vol.FileSystemId),
		})
		if err != nil {
			return err
		}
		log.Println("Delete sent successfully to AWS, volume should disappear shortly.")
	}
	return nil
}

func (d *backendAws) TagVolume(fsId string, tagName string, tagValue string) error {
	_, err := d.efs.TagResource(&efs.TagResourceInput{
		ResourceId: aws.String(fsId),
		Tags: []*efs.Tag{
			{
				Key:   aws.String(tagName),
				Value: aws.String(tagValue),
			},
		},
	})
	return err
}

func (d *backendAws) CreateVolume(name string, zone string, tags []string, expires time.Duration) error {
	var az *string
	if zone != "" {
		az = &zone
	}
	tag := []*efs.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String(name),
		}, {
			Key:   aws.String(awsTagEFSKey),
			Value: aws.String(awsTagEFSValue),
		},
	}
	for _, t := range tags {
		kv := strings.Split(t, "=")
		if len(kv) < 2 {
			return errors.New("tag format incorrect")
		}
		tag = append(tag, &efs.Tag{
			Key:   aws.String(kv[0]),
			Value: aws.String(strings.Join(kv[1:], "=")),
		})
	}
	if expires != 0 {
		err := d.ExpiriesSystemInstall(10, "")
		if err != nil && err.Error() != "EXISTS" {
			log.Printf("WARNING: Failed to install the expiry system, EFS will not expire: %s", err)
		}
		tag = append(tag, &efs.Tag{
			Key:   aws.String("lastUsed"),
			Value: aws.String(time.Now().Format(time.RFC3339)),
		}, &efs.Tag{
			Key:   aws.String("expireDuration"),
			Value: aws.String(expires.String()),
		})
	}
	_, err := d.efs.CreateFileSystem(&efs.CreateFileSystemInput{
		AvailabilityZoneName: az,
		Backup:               aws.Bool(false),
		CreationToken:        aws.String(name),
		Encrypted:            aws.Bool(true),
		PerformanceMode:      aws.String(efs.PerformanceModeGeneralPurpose),
		ThroughputMode:       aws.String(efs.ThroughputModeElastic),
		Tags:                 tag,
	})
	return err
}

func (d *backendAws) SetLabel(clusterName string, key string, value string, gzpZone string) error {
	vols, err := d.Inventory("", []int{InventoryItemVolumes})
	if err == nil {
		for _, vol := range vols.Volumes {
			if vol.Name != clusterName {
				continue
			}
			d.efs.TagResource(&efs.TagResourceInput{
				ResourceId: aws.String(vol.FileSystemId),
				Tags: []*efs.Tag{
					{
						Key:   aws.String(key),
						Value: aws.String(value),
					},
				},
			})
		}
	}
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
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.State.Code == int64(48) {
				continue
			}
			_, err := d.ec2svc.CreateTags(&ec2.CreateTagsInput{
				Resources: aws.StringSlice([]string{*instance.InstanceId}),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(key),
						Value: aws.String(value),
					},
				},
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *backendAws) ExpiriesSystemInstall(intervalMinutes int, deployRegion string) error {
	// if a scheduler already exists, return EXISTS as it's all been already made
	q, err := d.scheduler.ListSchedules(&scheduler.ListSchedulesInput{
		NamePrefix: aws.String("aerolab-expiries"),
	})
	if err == nil && len(q.Schedules) > 0 {
		return errors.New("EXISTS")
	}
	log.Println("Installing cluster expiry system")
	// attempt to remove any garbage left behind by previous installation efforts
	d.ExpiriesSystemRemove()

	ident, err := d.sts.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	accountId := *ident.Account

	lambdaRole, err := d.iam.CreateRole(&iam.CreateRoleInput{
		RoleName:                 aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
		AssumeRolePolicyDocument: aws.String(`{"Statement":[{"Action":"sts:AssumeRole","Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"}}],"Version":"2012-10-17"}`),
	})
	if err != nil {
		return err
	}
	err = d.iam.WaitUntilRoleExists(&iam.GetRoleInput{
		RoleName: lambdaRole.Role.RoleName,
	})
	if err != nil {
		return err
	}
	_, err = d.iam.PutRolePolicy(&iam.PutRolePolicyInput{
		PolicyName:     aws.String("aerolab-expiries-lambda-policy-" + a.opts.Config.Backend.Region),
		RoleName:       aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
		PolicyDocument: aws.String(fmt.Sprintf(`{"Statement":[{"Action":"logs:CreateLogGroup","Effect":"Allow","Resource":"arn:aws:logs:%s:%s:*"},{"Action":["logs:CreateLogStream","logs:PutLogEvents"],"Effect":"Allow","Resource":["arn:aws:logs:%s:%s:log-group:/aws/lambda/aerolab-expiries:*"]}],"Version":"2012-10-17"}`, a.opts.Config.Backend.Region, accountId, a.opts.Config.Backend.Region, accountId)),
	})
	if err != nil {
		return err
	}
	_, err = d.iam.AttachRolePolicy(&iam.AttachRolePolicyInput{
		RoleName:  aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonEC2FullAccess"),
	})
	if err != nil {
		return err
	}
	_, err = d.iam.AttachRolePolicy(&iam.AttachRolePolicyInput{
		RoleName:  aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonElasticFileSystemFullAccess"),
	})
	if err != nil {
		return err
	}

	schedRole, err := d.iam.CreateRole(&iam.CreateRoleInput{
		RoleName:                 aws.String("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region),
		AssumeRolePolicyDocument: aws.String(fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect": "Allow","Principal":{"Service":"scheduler.amazonaws.com"},"Action":"sts:AssumeRole","Condition":{"StringEquals":{"aws:SourceAccount":"%s"}}}]}`, accountId)),
	})
	if err != nil {
		return err
	}
	err = d.iam.WaitUntilRoleExists(&iam.GetRoleInput{
		RoleName: schedRole.Role.RoleName,
	})
	if err != nil {
		return err
	}
	_, err = d.iam.PutRolePolicy(&iam.PutRolePolicyInput{
		PolicyName:     aws.String("aerolab-expiries-scheduler-policy-" + a.opts.Config.Backend.Region),
		RoleName:       aws.String("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region),
		PolicyDocument: aws.String(fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["lambda:InvokeFunction"],"Resource":["arn:aws:lambda:%s:%s:function:aerolab-expiries:*","arn:aws:lambda:%s:%s:function:aerolab-expiries"]}]}`, a.opts.Config.Backend.Region, accountId, a.opts.Config.Backend.Region, accountId)),
	})
	if err != nil {
		return err
	}

	function, err := d.lambda.CreateFunction(&lambda.CreateFunctionInput{
		Code: &lambda.FunctionCode{
			ZipFile: expiriesCodeAws,
		},
		FunctionName: aws.String("aerolab-expiries"),
		Handler:      aws.String("bootstrap"),
		PackageType:  aws.String("Zip"),
		Timeout:      aws.Int64(60),
		Publish:      aws.Bool(true),
		Runtime:      aws.String("provided.al2"),
		Role:         lambdaRole.Role.Arn,
	})
	if err != nil && strings.Contains(err.Error(), "InvalidParameterValueException: The role defined for the function cannot be assumed by Lambda") {
		retries := 0
		for {
			retries++
			log.Println("IAM not ready, waiting for IAM and retrying to create Lambda")
			time.Sleep(5 * time.Second)
			function, err = d.lambda.CreateFunction(&lambda.CreateFunctionInput{
				Code: &lambda.FunctionCode{
					ZipFile: expiriesCodeAws,
				},
				FunctionName: aws.String("aerolab-expiries"),
				Handler:      aws.String("bootstrap"),
				PackageType:  aws.String("Zip"),
				Timeout:      aws.Int64(60),
				Publish:      aws.Bool(true),
				Runtime:      aws.String("provided.al2"),
				Role:         lambdaRole.Role.Arn,
			})
			if err != nil && !strings.Contains(err.Error(), "InvalidParameterValueException: The role defined for the function cannot be assumed by Lambda") {
				return err
			} else if err == nil {
				break
			} else if retries > 3 {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	_, err = d.scheduler.CreateSchedule(&scheduler.CreateScheduleInput{
		Name:               aws.String("aerolab-expiries"),
		ScheduleExpression: aws.String("rate(" + strconv.Itoa(intervalMinutes) + " minutes)"),
		State:              aws.String("ENABLED"),
		ClientToken:        aws.String("aerolab-expiries-" + a.opts.Config.Backend.Region),
		FlexibleTimeWindow: &scheduler.FlexibleTimeWindow{
			Mode: aws.String(scheduler.FlexibleTimeWindowModeOff),
		},
		Target: &scheduler.Target{
			Arn:     function.FunctionArn,
			RoleArn: schedRole.Role.Arn,
		},
	})
	if err != nil {
		return err
	}

	log.Println("Cluster expiry system installed")
	return nil
}

func (d *backendAws) EnableServices() error {
	return nil
}

func (d *backendAws) ClusterExpiry(zone string, clusterName string, expiry time.Duration, nodes []int) error {
	var instances []string
	if d.server {
		j, err := d.Inventory("", []int{InventoryItemClusters})
		d.WorkOnServers()
		if err != nil {
			return err
		}
		for _, jj := range j.Clusters {
			nodeNo, _ := strconv.Atoi(jj.NodeNo)
			if jj.ClusterName == clusterName && (len(nodes) == 0 || inslice.HasInt(nodes, nodeNo)) {
				instances = append(instances, jj.InstanceId)
			}
		}
	} else {
		j, err := d.Inventory("", []int{InventoryItemClients})
		d.WorkOnClients()
		if err != nil {
			return err
		}
		for _, jj := range j.Clients {
			nodeNo, _ := strconv.Atoi(jj.NodeNo)
			if jj.ClientName == clusterName && (len(nodes) == 0 || inslice.HasInt(nodes, nodeNo)) {
				instances = append(instances, jj.InstanceId)
			}
		}
	}
	if len(instances) == 0 {
		return errors.New("not found any instances for the given name")
	}
	var expiresTime time.Time
	if expiry != 0 {
		expiresTime = time.Now().Add(expiry)
	}
	_, err := d.ec2svc.CreateTags(&ec2.CreateTagsInput{
		Resources: aws.StringSlice(instances),
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("aerolab4expires"),
				Value: aws.String(expiresTime.Format(time.RFC3339)),
			},
		},
	})
	return err
}

func (d *backendAws) ExpiriesSystemRemove() error {
	var ret []string
	_, err := d.scheduler.DeleteSchedule(&scheduler.DeleteScheduleInput{
		Name:        aws.String("aerolab-expiries"),
		ClientToken: aws.String(uuid.New().String()),
	})
	if err != nil && !strings.Contains(err.Error(), "Schedule aerolab-expiries does not exist") {
		ret = append(ret, err.Error())
	}

	_, err = d.lambda.DeleteFunction(&lambda.DeleteFunctionInput{
		FunctionName: aws.String("aerolab-expiries"),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceNotFoundException: Function not found") {
		ret = append(ret, err.Error())
	}

	_, err = d.iam.DetachRolePolicy(&iam.DetachRolePolicyInput{
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonElasticFileSystemFullAccess"),
		RoleName:  aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}
	_, err = d.iam.DetachRolePolicy(&iam.DetachRolePolicyInput{
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonEC2FullAccess"),
		RoleName:  aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}
	_, err = d.iam.DeleteRolePolicy(&iam.DeleteRolePolicyInput{
		PolicyName: aws.String("aerolab-expiries-lambda-policy-" + a.opts.Config.Backend.Region),
		RoleName:   aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}
	_, err = d.iam.DeleteRole(&iam.DeleteRoleInput{
		RoleName: aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}

	_, err = d.iam.DeleteRolePolicy(&iam.DeleteRolePolicyInput{
		PolicyName: aws.String("aerolab-expiries-scheduler-policy-" + a.opts.Config.Backend.Region),
		RoleName:   aws.String("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}
	_, err = d.iam.DeleteRole(&iam.DeleteRoleInput{
		RoleName: aws.String("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		ret = append(ret, err.Error())
	}

	if len(ret) != 0 {
		return errors.New(strings.Join(ret, "\n\n"))
	}
	return nil
}

func (d *backendAws) ExpiriesSystemFrequency(intervalMinutes int) error {
	out, err := d.scheduler.ListSchedules(&scheduler.ListSchedulesInput{
		NamePrefix: aws.String("aerolab-expiries"),
	})
	if err != nil {
		return err
	}
	if len(out.Schedules) == 0 {
		return errors.New("schedule not found")
	}
	roleArn := ""
	err = d.iam.ListRolesPages(&iam.ListRolesInput{}, func(o *iam.ListRolesOutput, lastPage bool) (nextPage bool) {
		for _, role := range o.Roles {
			if *role.RoleName == ("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region) {
				roleArn = *role.Arn
				return false
			}
		}
		return true
	})
	if err != nil {
		return err
	}
	_, err = d.scheduler.UpdateSchedule(&scheduler.UpdateScheduleInput{
		Name:               aws.String("aerolab-expiries"),
		ScheduleExpression: aws.String("rate(" + strconv.Itoa(intervalMinutes) + " minutes)"),
		State:              aws.String("ENABLED"),
		FlexibleTimeWindow: &scheduler.FlexibleTimeWindow{
			Mode: aws.String(scheduler.FlexibleTimeWindowModeOff),
		},
		Target: &scheduler.Target{
			Arn:     out.Schedules[0].Target.Arn,
			RoleArn: aws.String(roleArn),
		},
	})
	return err
}

func (d *backendAws) getInstanceTypesFromCache() ([]instanceType, error) {
	cacheFile, err := a.aerolabRootDir()
	if err != nil {
		return nil, err
	}
	cacheFile = path.Join(cacheFile, "cache", "aws.instance-types."+a.opts.Config.Backend.Region+".json")
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	ita := &instanceTypesCache{}
	err = dec.Decode(ita)
	if err != nil {
		return nil, err
	}
	if ita.Expires.Before(time.Now()) {
		return ita.InstanceTypes, errors.New("cache expired")
	}
	return ita.InstanceTypes, nil
}

func (d *backendAws) saveInstanceTypesToCache(it []instanceType) error {
	cacheDir, err := a.aerolabRootDir()
	if err != nil {
		return err
	}
	cacheDir = path.Join(cacheDir, "cache")
	cacheFile := path.Join(cacheDir, "aws.instance-types."+a.opts.Config.Backend.Region+".json")
	if _, err := os.Stat(cacheDir); err != nil {
		if err = os.Mkdir(cacheDir, 0700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(cacheFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	ita := &instanceTypesCache{
		Expires:       time.Now().Add(24 * time.Hour),
		InstanceTypes: it,
	}
	err = enc.Encode(ita)
	if err != nil {
		os.Remove(cacheFile)
		return err
	}
	return nil
}

func (d *backendAws) getInstanceTypes() ([]instanceType, error) {
	it, err := d.getInstanceTypesFromCache()
	if err == nil {
		return it, err
	}
	saveCache := true
	prices, err := d.getInstancePricesPerHour()
	if err != nil {
		log.Printf("WARN: pricing error: %s", err)
		saveCache = false
	}
	spotPrices, err := d.getSpotPricesPerHour()
	if err != nil {
		log.Printf("WARN: pricing error: %s", err)
		saveCache = false
	}
	it = []instanceType{}
	err = d.ec2svc.DescribeInstanceTypesPages(&ec2.DescribeInstanceTypesInput{}, func(ec2Types *ec2.DescribeInstanceTypesOutput, lastPage bool) bool {
		for _, iType := range ec2Types.InstanceTypes {
			sarch := []string{}
			for _, sar := range iType.ProcessorInfo.SupportedArchitectures {
				if sar != nil && *sar != "i386" {
					sarch = append(sarch, *sar)
				}
			}
			if len(sarch) == 0 {
				continue
			}
			isArm := false
			isX86 := false
			sarchString := strings.Join(sarch, ",")
			if strings.Contains(sarchString, "amd64") || strings.Contains(sarchString, "x86") || strings.Contains(sarchString, "x64") {
				isX86 = true
			}
			if strings.Contains(sarchString, "arm64") || strings.Contains(sarchString, "aarch64") {
				isArm = true
			}
			eph := 0
			ephs := float64(0)
			if iType.InstanceStorageSupported != nil && *iType.InstanceStorageSupported && iType.InstanceStorageInfo != nil {
				ephs = float64(aws.Int64Value(iType.InstanceStorageInfo.TotalSizeInGB))
				for _, d := range iType.InstanceStorageInfo.Disks {
					eph = eph + int(*d.Count)
				}
			}
			price := prices[*iType.InstanceType]
			if price <= 0 {
				price = -1
			}
			spotPrice := spotPrices[*iType.InstanceType]
			if spotPrice <= 0 {
				spotPrice = -1
			}
			it = append(it, instanceType{
				InstanceName:             *iType.InstanceType,
				CPUs:                     int(*iType.VCpuInfo.DefaultVCpus),
				RamGB:                    float64(int(*iType.MemoryInfo.SizeInMiB)) / 1024,
				EphemeralDisks:           eph,
				EphemeralDiskTotalSizeGB: ephs,
				PriceUSD:                 price,
				SpotPriceUSD:             spotPrice,
				IsArm:                    isArm,
				IsX86:                    isX86,
			})
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if saveCache {
		err = d.saveInstanceTypesToCache(it)
		if err != nil {
			log.Printf("WARN: failed to save instance types to cache: %s", err)
		}
	}
	return it, nil
}

func (d *backendAws) GetInstanceTypes(minCpu int, maxCpu int, minRam float64, maxRam float64, minDisks int, maxDisks int, findArm bool, gcpZone string) ([]instanceType, error) {
	ita, err := d.getInstanceTypes()
	if err != nil {
		return ita, err
	}
	it := []instanceType{}
	for _, i := range ita {
		if findArm && !i.IsArm {
			continue
		}
		if !findArm && !i.IsX86 {
			continue
		}
		if (minCpu > 0 && i.CPUs < minCpu) || (maxCpu > 0 && i.CPUs > maxCpu) {
			continue
		}
		if (minRam > 0 && i.RamGB < minRam) || (maxRam > 0 && i.RamGB > maxRam) {
			continue
		}
		if (minDisks > 0 && i.EphemeralDisks < minDisks) || (maxDisks > 0 && i.EphemeralDisks > maxDisks) {
			continue
		}
		it = append(it, i)
	}
	return it, nil
}

// map[instanceType]priceUSD
func (d *backendAws) getSpotPricesPerHour() (map[string]float64, error) {
	prices := make(map[string]float64)
	utc, _ := time.LoadLocation("UTC")
	endTime := time.Now().In(utc)
	startTime := endTime.Add(-1 * time.Minute) // 1 minute ago
	input := &ec2.DescribeSpotPriceHistoryInput{
		StartTime:           &startTime,
		EndTime:             &endTime,
		MaxResults:          aws.Int64(100),
		ProductDescriptions: []*string{aws.String("Linux/UNIX")},
	}
	err := d.ec2svc.DescribeSpotPriceHistoryPages(input, func(out *ec2.DescribeSpotPriceHistoryOutput, lastPage bool) bool {
		for _, hist := range out.SpotPriceHistory {
			newPrice, err := strconv.ParseFloat(*hist.SpotPrice, 64)
			if err != nil {
				continue
			}
			if price, ok := prices[*hist.InstanceType]; !ok || price < newPrice {
				prices[*hist.InstanceType] = newPrice
			}
		}
		return true
	})
	if err != nil {
		return prices, err
	}
	return prices, nil
}

// map[instanceType]priceUSD
func (d *backendAws) getInstancePricesPerHour() (map[string]float64, error) {
	prices := make(map[string]float64)
	svc := pricing.New(d.sess, aws.NewConfig().WithRegion("us-east-1"))
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		MaxResults:  aws.Int64(100),
		Filters: []*pricing.Filter{
			{
				Field: aws.String("regionCode"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String(a.opts.Config.Backend.Region),
			},
			{
				Field: aws.String("marketoption"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("OnDemand"),
			},
			{
				Field: aws.String("tenancy"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("Shared"), // other values: Dedicated, Host
			},
			{
				Field: aws.String("capacitystatus"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("Used"), // other values: AllocatedCapacityReservation, UnusedCapacityReservation
			},
			{
				Field: aws.String("preInstalledSw"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("NA"),
			},
			{
				Field: aws.String("operatingSystem"),
				Type:  aws.String("TERM_MATCH"),
				Value: aws.String("Linux"),
			},
		},
	}

	result, err := svc.GetProducts(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case pricing.ErrCodeInternalErrorException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInternalErrorException, aerr.Error())
			case pricing.ErrCodeInvalidParameterException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidParameterException, aerr.Error())
			case pricing.ErrCodeNotFoundException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeNotFoundException, aerr.Error())
			case pricing.ErrCodeInvalidNextTokenException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidNextTokenException, aerr.Error())
			case pricing.ErrCodeExpiredNextTokenException:
				return nil, fmt.Errorf("%s: %s", pricing.ErrCodeExpiredNextTokenException, aerr.Error())
			default:
				return nil, fmt.Errorf("%s", aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			return nil, fmt.Errorf("%s", err.Error())
		}
	}
	for {
		for _, price := range result.PriceList {
			/*
				ret, err := json.MarshalIndent(price["terms"].(map[string]interface{})["OnDemand"].(map[string]interface{}), "", "    ")
				if err != nil {
					fmt.Println(err.Error())
				}
				fmt.Println(string(ret))
			*/
			for _, v := range price["terms"].(map[string]interface{})["OnDemand"].(map[string]interface{}) {
				for _, vv := range v.(map[string]interface{})["priceDimensions"].(map[string]interface{}) {
					if vv.(map[string]interface{})["unit"].(string) != "Hrs" {
						return nil, fmt.Errorf("price format incorrect:%s", vv.(map[string]interface{})["unit"].(string))
					}
					nType := vv.(map[string]interface{})["description"].(string)
					nTypes := strings.Split(nType, " per On Demand Linux ")
					if len(nTypes) != 2 {
						continue
					}
					if !strings.HasSuffix(nTypes[1], " Instance Hour") {
						continue
					}
					nPriceStr := vv.(map[string]interface{})["pricePerUnit"].(map[string]interface{})["USD"].(string)
					nPrice, err := strconv.ParseFloat(nPriceStr, 64)
					if err != nil {
						return nil, fmt.Errorf("price is NaN:%s: '%s'", err, nPriceStr)
					}
					prices[strings.TrimSuffix(nTypes[1], " Instance Hour")] = nPrice
				}
			}
		}
		input.NextToken = result.NextToken
		if input.NextToken == nil || *input.NextToken == "" {
			break
		}
		result, err = svc.GetProducts(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case pricing.ErrCodeInternalErrorException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInternalErrorException, aerr.Error())
				case pricing.ErrCodeInvalidParameterException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidParameterException, aerr.Error())
				case pricing.ErrCodeNotFoundException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeNotFoundException, aerr.Error())
				case pricing.ErrCodeInvalidNextTokenException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeInvalidNextTokenException, aerr.Error())
				case pricing.ErrCodeExpiredNextTokenException:
					return nil, fmt.Errorf("%s: %s", pricing.ErrCodeExpiredNextTokenException, aerr.Error())
				default:
					return nil, fmt.Errorf("%s", aerr.Error())
				}
			} else {
				return nil, fmt.Errorf("%s", err.Error())
			}
		}
	}
	return prices, nil
}

func (d *backendAws) InventoryAllRegions(filterOwner string, inventoryItems []int) (inventoryJson, error) {
	ij := inventoryJson{}
	regions, err := d.ec2svc.DescribeRegions(nil)
	if err != nil {
		return ij, fmt.Errorf("could not establish a session to aws, make sure your ~/.aws/credentials file is correct\n%s", err)
	}
	setRegion := a.opts.Config.Backend.Region
	defer func() {
		a.opts.Config.Backend.Region = setRegion
	}()
	regionCount := len(regions.Regions)
	for i, region := range regions.Regions {
		log.Printf("Pulling from %s (%d/%d)", *region.RegionName, i+1, regionCount)
		a.opts.Config.Backend.Region = *region.RegionName
		err = d.Init()
		if err != nil {
			return ij, fmt.Errorf("could not connect to region %s: %s", *region.RegionName, err)
		}
		inv, err := d.Inventory(filterOwner, inventoryItems)
		if err != nil {
			return ij, err
		}
		ij.Clients = append(ij.Clients, inv.Clients...)
		ij.Clusters = append(ij.Clusters, inv.Clusters...)
		ij.ExpirySystem = append(ij.ExpirySystem, inv.ExpirySystem...)
		ij.FirewallRules = append(ij.FirewallRules, inv.FirewallRules...)
		ij.Subnets = append(ij.Subnets, inv.Subnets...)
		ij.Templates = append(ij.Templates, inv.Templates...)
	}
	return ij, nil
}

func (d *backendAws) Inventory(filterOwner string, inventoryItems []int) (inventoryJson, error) {
	if inslice.HasInt(inventoryItems, InventoryItemAWSAllRegions) {
		items := []int{}
		for _, item := range inventoryItems {
			if item != InventoryItemAWSAllRegions {
				items = append(items, item)
			}
		}
		return d.InventoryAllRegions(filterOwner, items)
	}
	ij := inventoryJson{}

	if inslice.HasInt(inventoryItems, InventoryItemExpirySystem) {
		ij.ExpirySystem = append(ij.ExpirySystem, inventoryExpiry{})
		q, err := d.scheduler.GetSchedule(&scheduler.GetScheduleInput{
			Name: aws.String("aerolab-expiries"),
		})
		if err == nil {
			ij.ExpirySystem[0].Scheduler = *q.Arn
			ij.ExpirySystem[0].Schedule = *q.ScheduleExpression
		}
		q2, err := d.lambda.GetFunction(&lambda.GetFunctionInput{
			FunctionName: aws.String("aerolab-expiries"),
		})
		if err == nil {
			ij.ExpirySystem[0].Function = *q2.Configuration.FunctionArn
		}
		q3, err := d.iam.GetRole(&iam.GetRoleInput{
			RoleName: aws.String("aerolab-expiries-scheduler-" + a.opts.Config.Backend.Region),
		})
		if err == nil {
			ij.ExpirySystem[0].IAMScheduler = *q3.Role.Arn
		}
		q4, err := d.iam.GetRole(&iam.GetRoleInput{
			RoleName: aws.String("aerolab-expiries-lambda-" + a.opts.Config.Backend.Region),
		})
		if err == nil {
			ij.ExpirySystem[0].IAMFunction = *q4.Role.Arn
		}
	}

	if inslice.HasInt(inventoryItems, InventoryItemTemplates) {
		tmpl, err := d.ListTemplates()
		if err != nil {
			return ij, err
		}
		for _, d := range tmpl {
			arch := "amd64"
			if d.isArm {
				arch = "arm64"
			}
			ij.Templates = append(ij.Templates, inventoryTemplate{
				AerospikeVersion: d.aerospikeVersion,
				Distribution:     d.distroName,
				OSVersion:        d.distroVersion,
				Arch:             arch,
				Region:           a.opts.Config.Backend.Region,
			})
		}
	}

	if inslice.HasInt(inventoryItems, InventoryItemFirewalls) {
		var err error
		ij.FirewallRules, err = d.listSecurityGroups(false)
		if err != nil {
			return ij, err
		}

		ijSubnets, err := d.listSubnets(false)
		if err != nil {
			return ij, err
		}
		for _, ijs := range ijSubnets {
			ij.Subnets = append(ij.Subnets, inventorySubnet{
				AWS: ijs,
			})
		}
	}

	nCheckList := []int{}
	if inslice.HasInt(inventoryItems, InventoryItemClusters) {
		nCheckList = []int{1}
	}
	if inslice.HasInt(inventoryItems, InventoryItemClients) {
		nCheckList = append(nCheckList, 2)
	}
	for _, i := range nCheckList {
		if i == 1 {
			d.WorkOnServers()
		} else {
			d.WorkOnClients()
		}
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
			return ij, fmt.Errorf("could not run DescribeInstances\n%s", err)
		}
		for _, reservation := range instances.Reservations {
			for _, instance := range reservation.Instances {
				if *instance.State.Code != int64(48) {
					clusterName := ""
					nodeNo := ""
					os := ""
					osVer := ""
					asdVer := ""
					publicIp := ""
					privateIp := ""
					instanceId := ""
					imageId := ""
					state := ""
					arch := ""
					clientType := ""
					startTime := int(0)
					lastRunCost := float64(0)
					pricePerHour := float64(0)
					if instance.PublicIpAddress != nil {
						publicIp = *instance.PublicIpAddress
					}
					if instance.PrivateIpAddress != nil {
						privateIp = *instance.PrivateIpAddress
					}
					if instance.InstanceId != nil {
						instanceId = *instance.InstanceId
					}
					if instance.ImageId != nil {
						imageId = *instance.ImageId
					}
					if instance.State != nil && instance.State.Name != nil {
						state = *instance.State.Name
					}
					if instance.Architecture != nil {
						arch = *instance.Architecture
					}
					owner := ""
					expires := ""
					features := 0
					allTags := make(map[string]string)
					for _, tag := range instance.Tags {
						allTags[*tag.Key] = *tag.Value
					}
					for _, tag := range instance.Tags {
						if *tag.Key == awsTagClusterName {
							clusterName = *tag.Value
						} else if *tag.Key == "owner" {
							owner = *tag.Value
						} else if *tag.Key == awsTagNodeNumber {
							nodeNo = *tag.Value
						} else if *tag.Key == awsTagOperatingSystem {
							os = *tag.Value
						} else if *tag.Key == awsTagOSVersion {
							osVer = *tag.Value
						} else if *tag.Key == awsTagAerospikeVersion {
							asdVer = *tag.Value
						} else if *tag.Key == awsClientTagClientType {
							clientType = *tag.Value
						} else if *tag.Key == "aerolab4expires" {
							expires = *tag.Value
						} else if *tag.Key == "aerolab4features" {
							features, _ = strconv.Atoi(*tag.Value)
						} else if *tag.Key == awsTagCostLastRun {
							lastRunCost, err = strconv.ParseFloat(*tag.Value, 64)
							if err != nil {
								lastRunCost = 0
							}
						} else if *tag.Key == awsTagCostPerHour {
							pricePerHour, err = strconv.ParseFloat(*tag.Value, 64)
							if err != nil {
								pricePerHour = 0
							}
						} else if *tag.Key == awsTagCostStartTime {
							startTime, err = strconv.Atoi(*tag.Value)
							if err != nil {
								startTime = int(instance.LaunchTime.Unix())
							}
						}
					}
					if filterOwner != "" {
						if owner != filterOwner {
							continue
						}
					}
					sgs := []string{}
					for _, sgss := range instance.SecurityGroups {
						sgs = append(sgs, *sgss.GroupName)
					}
					currentCost := lastRunCost
					if startTime != 0 {
						now := int(time.Now().Unix())
						delta := now - startTime
						deltaH := float64(delta) / 3600
						currentCost = lastRunCost + (pricePerHour * deltaH)
					}
					if expires == "0001-01-01T00:00:00Z" {
						expires = ""
					}
					secGroups := []string{}
					for _, secGroup := range instance.SecurityGroups {
						secGroups = append(secGroups, aws.StringValue(secGroup.GroupId))
					}
					isSpot := false
					if spot, ok := allTags["aerolab7spot"]; ok && spot == "true" {
						isSpot = true
					}
					if i == 1 {
						ij.Clusters = append(ij.Clusters, inventoryCluster{
							ClusterName:         clusterName,
							NodeNo:              nodeNo,
							PublicIp:            publicIp,
							PrivateIp:           privateIp,
							InstanceId:          instanceId,
							ImageId:             imageId,
							State:               state,
							Arch:                arch,
							Distribution:        os,
							OSVersion:           osVer,
							AerospikeVersion:    asdVer,
							Zone:                a.opts.Config.Backend.Region,
							Firewalls:           sgs,
							InstanceRunningCost: currentCost,
							Owner:               owner,
							Expires:             expires,
							Features:            FeatureSystem(features),
							AGILabel:            allTags["agiLabel"],
							awsTags:             allTags,
							awsSubnet:           aws.StringValue(instance.SubnetId),
							awsSecGroups:        secGroups,
							AwsIsSpot:           isSpot,
						})
					} else {
						ij.Clients = append(ij.Clients, inventoryClient{
							ClientName:          clusterName,
							NodeNo:              nodeNo,
							PublicIp:            publicIp,
							PrivateIp:           privateIp,
							InstanceId:          instanceId,
							ImageId:             imageId,
							State:               state,
							Arch:                arch,
							Distribution:        os,
							OSVersion:           osVer,
							AerospikeVersion:    asdVer,
							ClientType:          clientType,
							Zone:                a.opts.Config.Backend.Region,
							Firewalls:           sgs,
							InstanceRunningCost: currentCost,
							Owner:               owner,
							Expires:             expires,
							awsTags:             allTags,
							awsSubnet:           aws.StringValue(instance.SubnetId),
							awsSecGroups:        secGroups,
							AwsIsSpot:           isSpot,
						})
					}
				}
			}
		}
	}
	if inslice.HasInt(inventoryItems, InventoryItemVolumes) {
		err := d.efs.DescribeFileSystemsPages(&efs.DescribeFileSystemsInput{}, func(e *efs.DescribeFileSystemsOutput, t bool) bool {
			for _, volume := range e.FileSystems {
				tags := make(map[string]string)
				for _, tag := range volume.Tags {
					tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
				}
				if tag, ok := tags[awsTagEFSKey]; !ok || tag != awsTagEFSValue {
					continue
				}
				vol := inventoryVolume{
					AvailabilityZoneId:   aws.StringValue(volume.AvailabilityZoneId),
					AvailabilityZoneName: aws.StringValue(volume.AvailabilityZoneName),
					CreationTime:         aws.TimeValue(volume.CreationTime),
					CreationToken:        aws.StringValue(volume.CreationToken),
					Encrypted:            aws.BoolValue(volume.Encrypted),
					FileSystemArn:        aws.StringValue(volume.FileSystemArn),
					FileSystemId:         aws.StringValue(volume.FileSystemId),
					LifeCycleState:       aws.StringValue(volume.LifeCycleState),
					Name:                 aws.StringValue(volume.Name),
					NumberOfMountTargets: int(aws.Int64Value(volume.NumberOfMountTargets)),
					AWSOwnerId:           aws.StringValue(volume.OwnerId),
					PerformanceMode:      aws.StringValue(volume.PerformanceMode),
					ThroughputMode:       aws.StringValue(volume.ThroughputMode),
					SizeBytes:            int(aws.Int64Value(volume.SizeInBytes.Value)),
					Owner:                tags["aerolab7owner"],
					Tags:                 tags,
				}
				if vol.NumberOfMountTargets > 0 {
					mt, err := d.efs.DescribeMountTargets(&efs.DescribeMountTargetsInput{
						FileSystemId: aws.String(vol.FileSystemId),
					})
					if err != nil {
						log.Printf("WARNING: Failed to get mount targets for %s: %s", vol.FileSystemId, err)
						ij.Volumes = append(ij.Volumes, vol)
						continue
					}
					for _, target := range mt.MountTargets {
						var sg []string
						if aws.StringValue(target.LifeCycleState) != efs.LifeCycleStateDeleting && aws.StringValue(target.LifeCycleState) != efs.LifeCycleStateDeleted {
							secGroups, err := d.efs.DescribeMountTargetSecurityGroups(&efs.DescribeMountTargetSecurityGroupsInput{
								MountTargetId: target.MountTargetId,
							})
							if err != nil {
								log.Printf("Could not get security groups for mount target: %s", err)
							}
							sg = aws.StringValueSlice(secGroups.SecurityGroups)
						}
						vol.MountTargets = append(vol.MountTargets, inventoryMountTarget{
							AvailabilityZoneId:   aws.StringValue(target.AvailabilityZoneId),
							AvailabilityZoneName: aws.StringValue(target.AvailabilityZoneName),
							FileSystemId:         aws.StringValue(target.FileSystemId),
							IpAddress:            aws.StringValue(target.IpAddress),
							LifeCycleState:       aws.StringValue(target.LifeCycleState),
							MountTargetId:        aws.StringValue(target.MountTargetId),
							NetworkInterfaceId:   aws.StringValue(target.NetworkInterfaceId),
							AWSOwnerId:           aws.StringValue(target.OwnerId),
							SubnetId:             aws.StringValue(target.SubnetId),
							VpcId:                aws.StringValue(target.VpcId),
							SecurityGroups:       sg,
						})
					}
				}
				ij.Volumes = append(ij.Volumes, vol)
			}
			return true
		})
		if err != nil {
			return ij, err
		}
	}
	return ij, nil
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
		Profile:           a.opts.Config.Backend.AWSProfile,
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

	var lambdaSvc *lambda.Lambda
	if a.opts.Config.Backend.Region == "" {
		lambdaSvc = lambda.New(d.sess)
	} else {
		lambdaSvc = lambda.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	var schedulerSvc *scheduler.Scheduler
	if a.opts.Config.Backend.Region == "" {
		schedulerSvc = scheduler.New(d.sess)
	} else {
		schedulerSvc = scheduler.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	var iamSvc *iam.IAM
	if a.opts.Config.Backend.Region == "" {
		iamSvc = iam.New(d.sess)
	} else {
		iamSvc = iam.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	var stsSvc *sts.STS
	if a.opts.Config.Backend.Region == "" {
		stsSvc = sts.New(d.sess)
	} else {
		stsSvc = sts.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	var efsSvc *efs.EFS
	if a.opts.Config.Backend.Region == "" {
		efsSvc = efs.New(d.sess)
	} else {
		efsSvc = efs.New(d.sess, aws.NewConfig().WithRegion(a.opts.Config.Backend.Region))
	}

	d.scheduler = schedulerSvc
	d.lambda = lambdaSvc
	d.ec2svc = svc
	d.iam = iamSvc
	d.sts = stsSvc
	d.efs = efsSvc
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
	fr := []fileListReader{}
	for _, f := range files {
		fr = append(fr, fileListReader{
			filePath:     f.filePath,
			fileSize:     f.fileSize,
			fileContents: strings.NewReader(f.fileContents),
		})
	}
	return d.CopyFilesToClusterReader(name, fr, nodes)
}

func (d *backendAws) CopyFilesToClusterReader(name string, files []fileListReader, nodes []int) error {
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
			out, err = remoteRun("root", fmt.Sprintf("%s:22", nodeIps[node]), keypath, comm, node)
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
				startTime := 0
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					} else if *tag.Key == awsTagCostStartTime {
						startTime, err = strconv.Atoi(*tag.Value)
						if err != nil {
							startTime = 0
						}
					}
				}
				if inslice.HasInt(nodes, nodeNumber) {
					if instance.State.Code != &stateCodes[0] && instance.State.Code != &stateCodes[1] {
						if startTime == 0 {
							tagi := &ec2.CreateTagsInput{
								Resources: aws.StringSlice([]string{*instance.InstanceId}),
								Tags: []*ec2.Tag{
									{
										Key:   aws.String(awsTagCostStartTime),
										Value: aws.String(strconv.Itoa(int(time.Now().Unix()))),
									}},
							}
							_, err := d.ec2svc.CreateTags(tagi)
							if err != nil {
								return fmt.Errorf("could not update instance start time tags: %s", err)
							}
						}
						input := &ec2.StartInstancesInput{
							InstanceIds: []*string{
								aws.String(*instance.InstanceId),
							},
							DryRun: aws.Bool(false),
						}
						result, err := d.ec2svc.StartInstances(input)
						if err != nil && !strings.Contains(err.Error(), "UnsupportedOperation") {
							return fmt.Errorf("error starting instance %s\n%s\n%s", *instance.InstanceId, result, err)
						}
					}
					instList = append(instList, instance.InstanceId)
				}
			}
		}
	}

	log.Println("Waiting for Instance to be UP")
	d.ec2svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: instList,
	})

	start := time.Now()
	// wait for IPs
	log.Println("Waiting for Public IPs")
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
	log.Println("Waiting for SSH to be available")
	_, keyPath, err := d.getKey(name)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		err = errors.New("waiting")
		for err != nil {
			_, err = remoteRun("root", fmt.Sprintf("%s:22", nip[node]), keyPath, "ls", node)
			if err != nil {
				if time.Since(start) > time.Second*600 {
					return errors.New("didn't get SSH for 10 minutes, giving up")
				}
				fmt.Printf("Not up yet, waiting (%s:22 using %s): %s\n", nip[node], keyPath, err)
				time.Sleep(time.Second)
			}
		}
	}
	log.Println("Instance UP, continuing")
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
				lastRunCost := float64(0)
				pricePerHour := float64(0)
				startTime := 0
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagNodeNumber {
						nodeNumber, err = strconv.Atoi(*tag.Value)
						if err != nil {
							return errors.New("problem with node numbers in the given cluster. Investigate manually")
						}
					} else if *tag.Key == awsTagCostLastRun {
						lastRunCost, err = strconv.ParseFloat(*tag.Value, 64)
						if err != nil {
							lastRunCost = 0
						}
					} else if *tag.Key == awsTagCostPerHour {
						pricePerHour, err = strconv.ParseFloat(*tag.Value, 64)
						if err != nil {
							pricePerHour = 0
						}
					} else if *tag.Key == awsTagCostStartTime {
						startTime, err = strconv.Atoi(*tag.Value)
						if err != nil {
							startTime = int(instance.LaunchTime.Unix())
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
					// on cluster stop, calculate and set the lastRun tag, and set startTime to 0
					if startTime != 0 {
						lastRunCost = lastRunCost + (pricePerHour * (float64((int(time.Now().Unix()) - startTime)) / 3600))
						tagi := &ec2.CreateTagsInput{
							Resources: aws.StringSlice([]string{*instance.InstanceId}),
							Tags: []*ec2.Tag{
								{
									Key:   aws.String(awsTagCostStartTime),
									Value: aws.String("0"),
								}, {
									Key:   aws.String(awsTagCostLastRun),
									Value: aws.String(strconv.FormatFloat(lastRunCost, 'f', 8, 64)),
								}},
						}
						_, err = d.ec2svc.CreateTags(tagi)
						if err != nil {
							return fmt.Errorf("could not update instance start time tags: %s", err)
						}
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
					// test for termination protection
					attr, err := d.ec2svc.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
						Attribute:  aws.String(ec2.InstanceAttributeNameDisableApiTermination),
						InstanceId: instance.InstanceId,
					})
					if err == nil && aws.BoolValue(attr.DisableApiTermination.Value) {
						log.Printf("Not terminating %s, ApiTermination protection is enabled", *instance.InstanceId)
						continue
					}
					instanceIds = append(instanceIds, instance.InstanceId)
					input := &ec2.TerminateInstancesInput{
						InstanceIds: []*string{
							aws.String(*instance.InstanceId),
						},
						DryRun: aws.Bool(false),
					}
					result, err := d.ec2svc.TerminateInstances(input)
					if err != nil {
						return fmt.Errorf("error terminating instance %s\n%s\n%s", *instance.InstanceId, result, err)
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
	isInteractive := true
	if len(command) > 0 {
		isInteractive = false
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
	err = remoteAttachAndRun("root", fmt.Sprintf("%s:22", nodeIp[node]), keypath, comm, stdin, stdout, stderr, node, isInteractive)
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

func (d *backendAws) ClusterListFull(isJson bool, owner string, pager bool, isPretty bool, sort []string) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Owner = owner
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	return "", a.opts.Inventory.List.run(d.server, d.client, false, false, false)
}

func (d *backendAws) TemplateListFull(isJson bool, pager bool, isPretty bool, sort []string) (string, error) {
	a.opts.Inventory.List.Json = isJson
	a.opts.Inventory.List.Pager = pager
	a.opts.Inventory.List.JsonPretty = isPretty
	a.opts.Inventory.List.SortBy = sort
	return "", a.opts.Inventory.List.run(false, false, true, false, false)
}

var deployAwsTemplateShutdownMaking = make(chan int, 1)

func (d *backendAws) DeployTemplate(v backendVersion, script string, files []fileListReader, extra *backendExtra) error {
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
	var templateId string
	if extra.ami != "" {
		templateId = extra.ami
	} else {
		templateId, err = d.getAmi(a.opts.Config.Backend.Region, v)
		if err != nil {
			return err
		}
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
	extra.securityGroupID, extra.subnetID, err = d.resolveSecGroupAndSubnet(extra.securityGroupID, extra.subnetID, true, extra.firewallNamePrefix)
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
	files = append(files, fileListReader{"/root/installer.sh", strings.NewReader(script), len(script)})
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

func (d *backendAws) DeployCluster(v backendVersion, name string, nodeCount int, extra *backendExtra) error {
	name = strings.Trim(name, "\r\n\t ")
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
		if extra.ami != "" {
			templateId = extra.ami
		} else {
			templateId, err = d.getAmi(a.opts.Config.Backend.Region, v)
			if err != nil {
				return err
			}
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
	price := float64(-1)
	it, err := d.getInstanceTypes()
	if err == nil {
		for _, iti := range it {
			if iti.InstanceName == extra.instanceType {
				if !extra.spotInstance {
					price = iti.PriceUSD
				} else {
					price = iti.SpotPriceUSD
				}
			}
		}
	}
	if !extra.expiresTime.IsZero() {
		err = d.ExpiriesSystemInstall(10, "")
		if err != nil && err.Error() != "EXISTS" {
			log.Printf("WARNING: Failed to install the expiry system, clusters will not expire: %s", err)
		}
	}
	for i := start; i < (nodeCount + start); i++ {
		// tag setup
		if extra.clientType == "" {
			extra.clientType = "N/A"
		}
		tgClientType := ec2.Tag{}
		tgClientType.Key = aws.String(awsClientTagClientType)
		tgClientType.Value = aws.String(extra.clientType)
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
		tgs := []*ec2.Tag{&tgClusterName, &tgNodeNumber, &tgUsedBy, &tgName, &tgClientType,
			{
				Key:   aws.String(awsTagOperatingSystem),
				Value: aws.String(v.distroName),
			},
			{
				Key:   aws.String(awsTagOSVersion),
				Value: aws.String(v.distroVersion),
			},
			{
				Key:   aws.String(awsTagAerospikeVersion),
				Value: aws.String(v.aerospikeVersion),
			},
			{
				Key:   aws.String(awsTagCostPerHour),
				Value: aws.String(strconv.FormatFloat(price, 'f', 8, 64)),
			},
			{
				Key:   aws.String(awsTagCostLastRun),
				Value: aws.String(strconv.FormatFloat(0, 'f', 8, 64)),
			},
			{
				Key:   aws.String(awsTagCostStartTime),
				Value: aws.String(strconv.Itoa(int(time.Now().Unix()))),
			},
		}
		if extra.spotInstance {
			tgs = append(tgs, &ec2.Tag{
				Key:   aws.String("aerolab7spot"),
				Value: aws.String("true"),
			})
		}
		tgs = append(tgs, extraTags...)
		expiryTelemetryLock.Lock()
		if expiryTelemetryUUID != "" {
			tgs = append(tgs, &ec2.Tag{
				Key:   aws.String("telemetry"),
				Value: aws.String(expiryTelemetryUUID),
			})
		}
		expiryTelemetryLock.Unlock()
		if !extra.expiresTime.IsZero() {
			tgs = append(tgs, &ec2.Tag{
				Key:   aws.String("aerolab4expires"),
				Value: aws.String(extra.expiresTime.Format(time.RFC3339)),
			})
		}
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
		extra.securityGroupID, extra.subnetID, err = d.resolveSecGroupAndSubnet(extra.securityGroupID, extra.subnetID, printID, extra.firewallNamePrefix)
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
				{
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
		if extra.spotInstance {
			input.InstanceMarketOptions = &ec2.InstanceMarketOptionsRequest{
				MarketType: aws.String(ec2.MarketTypeSpot),
				SpotOptions: &ec2.SpotMarketOptions{
					InstanceInterruptionBehavior: aws.String(ec2.InstanceInterruptionBehaviorStop),
					SpotInstanceType:             aws.String(ec2.SpotInstanceTypePersistent),
				},
			}
		}
		input.InstanceInitiatedShutdownBehavior = aws.String(ec2.ShutdownBehaviorStop)
		if extra.terminateOnPoweroff {
			if extra.spotInstance {
				input.InstanceMarketOptions.SpotOptions.InstanceInterruptionBehavior = aws.String(ec2.InstanceInterruptionBehaviorTerminate)
				input.InstanceMarketOptions.SpotOptions.SpotInstanceType = aws.String(ec2.SpotInstanceTypeOneTime)
			}
			input.InstanceInitiatedShutdownBehavior = aws.String(ec2.ShutdownBehaviorTerminate)
		}
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
				_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "ls", 0)
				if err == nil {
					fmt.Println("Connection succeeded, continuing installation...")
					// sort out root/ubuntu issues
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo mkdir -p /root/.ssh", 0)
						if err != nil {
							return fmt.Errorf("mkdir .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chown root:root /root/.ssh", 0)
						if err != nil {
							return fmt.Errorf("chown .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 750 /root/.ssh", 0)
						if err != nil {
							return fmt.Errorf("chmod .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo cp /home/"+d.getUser(v)+"/.ssh/authorized_keys /root/.ssh/", 0)
						if err != nil {
							return fmt.Errorf("cp .ssh failed: %s\n%s", string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo chmod 640 /root/.ssh/authorized_keys", 0)
						if err != nil {
							return fmt.Errorf("chmod auth_keys failed on %s: %s\n%s", *nout.Reservations[0].Instances[0].PublicIpAddress, string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "echo \"AcceptEnv NODE\" |sudo tee -a /etc/ssh/sshd_config", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "echo \"AcceptEnv NODE\" |sudo tee -a /etc/ssh/sshd_config", 0)
						if err != nil {
							return fmt.Errorf("chmod auth_keys failed on %s: %s\n%s", *nout.Reservations[0].Instances[0].PublicIpAddress, string(out), err)
						}
					}
					_, err = remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo service ssh restart || sudo service sshd restart", 0)
					if err != nil {
						out, err := remoteRun(d.getUser(v), fmt.Sprintf("%s:22", *nout.Reservations[0].Instances[0].PublicIpAddress), keyPath, "sudo service ssh restart || sudo service sshd restart", 0)
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

func (d *backendAws) Upload(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
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

func (d *backendAws) Download(clusterName string, node int, source string, destination string, verbose bool, legacy bool) error {
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
