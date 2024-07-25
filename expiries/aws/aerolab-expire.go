package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/bestmethod/inslice"
)

type MyEvent struct{}

type MyResponse struct {
	Status  string
	Message string `json:"Message:"`
}

func HandleLambdaEvent(event MyEvent) (MyResponse, error) {
	// connect
	sess, err := session.NewSession()
	if err != nil {
		return makeErrorResponse("Could not create AWS session: ", err)
	}
	svc := ec2.New(sess)

	r53instanceList := []string{}
	// list instances
	instances, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: aws.StringSlice([]string{"pending", "running", "stopping", "stopped"}),
			},
		},
	})
	if err != nil {
		return makeErrorResponse("Could not list instances: ", err)
	}

	// check each instance for expiry
	now := time.Now()
	deleteList := []string{}
	deleteListForLog := []string{}
	enumCount := 0
	defer telemetryLock.Wait()
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			r53instanceList = append(r53instanceList, *instance.InstanceId)
			enumCount++
			tags := make(map[string]string)
			for _, tag := range instance.Tags {
				tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
			}
			if expires, ok := tags["aerolab4expires"]; ok && expires != "" {
				expiry, err := time.Parse(time.RFC3339, expires)
				if err != nil {
					log.Printf("Could not handle expiry for instance %s: %s: %s", aws.StringValue(instance.InstanceId), expires, err)
					continue
				}
				if expiry.Before(now) && expiry.After(time.Date(1985, time.April, 29, 0, 0, 0, 0, time.UTC)) {
					// test for termination protection
					attr, err := svc.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
						Attribute:  aws.String(ec2.InstanceAttributeNameDisableApiTermination),
						InstanceId: instance.InstanceId,
					})
					if err == nil && aws.BoolValue(attr.DisableApiTermination.Value) {
						log.Printf("Not terminating %s, ApiTermination protection is enabled", *instance.InstanceId)
						continue
					}
					deleteList = append(deleteList, aws.StringValue(instance.InstanceId))
					r53instanceList = r53instanceList[:len(r53instanceList)-1]
					name := tags["Aerolab4ClusterName"]
					node := tags["Aerolab4NodeNumber"]
					if name == "" || node == "" {
						name = tags["Aerolab4clientClusterName"]
						node = tags["Aerolab4clientNodeNumber"]
					}
					deleteListForLog = append(deleteListForLog, fmt.Sprintf("instanceId=%s clusterName=%s nodeNo=%s", aws.StringValue(instance.InstanceId), name, node))
					if _, ok := tags["telemetry"]; ok {
						telemetryShip(tags["telemetry"], aws.StringValue(instance.Placement.AvailabilityZone), aws.StringValue(instance.InstanceId), name, node)
					}
				}
			}
		}
	}

	// expire if found
	log.Printf("Enumerated through %d instances, shutting down %d instances", enumCount, len(deleteList))
	if len(deleteList) > 0 {
		// expire instances
		for _, del := range deleteListForLog {
			log.Printf("Removing: %s", del)
		}
		_, err = svc.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice(deleteList),
		})
		if err != nil {
			return makeErrorResponse("Could not terminate instances: ", err)
		}
	}

	//// EXPIRE EFS
	fsList := []string{}
	mtList := []string{}
	fsCount := 0
	efsSvc := efs.New(sess)
	err = efsSvc.DescribeFileSystemsPages(&efs.DescribeFileSystemsInput{}, func(vol *efs.DescribeFileSystemsOutput, lastPage bool) bool {
		for _, fs := range vol.FileSystems {
			fsCount++
			allTags := make(map[string]string)
			for _, tag := range fs.Tags {
				allTags[*tag.Key] = *tag.Value
			}
			if lastUsed, ok := allTags["lastUsed"]; ok {
				if expireDuration, ok := allTags["expireDuration"]; ok {
					lu, err := time.Parse(time.RFC3339, lastUsed)
					if err == nil && !lu.IsZero() {
						ed, err := time.ParseDuration(expireDuration)
						if err == nil && ed > 0 {
							expiresTime := lu.Add(ed)
							if expiresTime.Before(time.Now()) {
								fsList = append(fsList, aws.StringValue(fs.FileSystemId))
								if aws.Int64Value(fs.NumberOfMountTargets) > 0 {
									mts, err := efsSvc.DescribeMountTargets(&efs.DescribeMountTargetsInput{
										FileSystemId: aws.String(*fs.FileSystemId),
									})
									if err != nil {
										continue
									}
									for _, mt := range mts.MountTargets {
										mtList = append(mtList, aws.StringValue(mt.MountTargetId))
									}
								}
							}
						}
					}
				}
			}
		}
		return true
	})
	if err != nil {
		return makeErrorResponse("Could not list EFS: ", err)
	}

	log.Printf("Enumerated through %d EFS, shutting down %d EFS", fsCount, len(fsList))
	if len(fsList) > 0 {
		// expire volumes
		for _, del := range mtList {
			log.Printf("Removing: %s", del)
			_, err = efsSvc.DeleteMountTarget(&efs.DeleteMountTargetInput{
				MountTargetId: aws.String(del),
			})
			if err != nil {
				return makeErrorResponse("Could not delete EFS mount target: ", err)
			}
		}
		for _, del := range fsList {
			for {
				time.Sleep(5 * time.Second)
				targets, err := efsSvc.DescribeMountTargets(&efs.DescribeMountTargetsInput{
					FileSystemId: aws.String(del),
				})
				if err != nil {
					return makeErrorResponse("error waiting on mount targets to be deleted: ", err)
				}
				if len(targets.MountTargets) == 0 {
					break
				}
			}
		}
		for _, del := range fsList {
			log.Printf("Removing: %s", del)
			_, err = efsSvc.DeleteFileSystem(&efs.DeleteFileSystemInput{
				FileSystemId: aws.String(del),
			})
			if err != nil {
				return makeErrorResponse("Could not delete EFS volume: ", err)
			}
		}
	}

	log.Println("EKS: Listing clusters")
	ekssvc := eks.New(sess)
	eksClusters, err := ekssvc.ListClusters(&eks.ListClustersInput{})
	if err != nil {
		return makeErrorResponse("Could not list EKS: ", err)
	}
	now = time.Now()
	for _, eksClusterName := range eksClusters.Clusters {
		eksCluster, err := ekssvc.DescribeCluster(&eks.DescribeClusterInput{Name: eksClusterName})
		if err != nil {
			return makeErrorResponse("Could not describe EKS cluster: ", err)
		}
		eksAt := aws.StringValue(eksCluster.Cluster.Tags["ExpireAt"])
		var expireAt time.Time
		if eksAt != "" {
			oldAtInt, err := strconv.Atoi(eksAt)
			if err == nil {
				expireAt = time.Unix(int64(oldAtInt), 0)
			}
		} else {
			initial := aws.StringValue(eksCluster.Cluster.Tags["initialExpiry"])
			if initial != "" {
				createTs := aws.TimeValue(eksCluster.Cluster.CreatedAt)
				initialDuration, err := time.ParseDuration(initial)
				if err == nil {
					expireAt = createTs.Add(initialDuration)
				}
			}
		}
		if expireAt.IsZero() {
			log.Printf("EKS: Cluster=%s NoExpirySet", aws.StringValue(eksCluster.Cluster.Name))
			continue
		}
		if expireAt.After(now) {
			log.Printf("EKS: Cluster=%s Expiry=%s NotExpired", aws.StringValue(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))
			continue
		}
		log.Printf("EKS: Cluster=%s Expiry=%s Starting Expiry", aws.StringValue(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))

		log.Print("EKS: Listing cloudformation stacks...")
		cfSvc := cloudformation.New(sess)
		stacks, err := cfSvc.ListStacks(&cloudformation.ListStacksInput{})
		if err != nil {
			return makeErrorResponse("Could not cloudformation.ListStacks: ", err)
		}
		type astack struct {
			Name          string
			CreationTime  time.Time
			CurrentStatus string
		}
		stackList := []*astack{}
		for _, stack := range stacks.StackSummaries {
			if !strings.HasPrefix(aws.StringValue(stack.StackName), "eksctl-"+aws.StringValue(eksCluster.Cluster.Name)+"-") {
				continue
			}
			if aws.StringValue(stack.StackStatus) == cloudformation.StackStatusDeleteComplete {
				continue
			}
			stackList = append(stackList, &astack{
				Name:          aws.StringValue(stack.StackName),
				CreationTime:  aws.TimeValue(stack.CreationTime),
				CurrentStatus: aws.StringValue(stack.StackStatus),
			})
		}
		sort.Slice(stackList, func(i, j int) bool {
			return stackList[j].CreationTime.Before(stackList[i].CreationTime) // reverse sort since we want to delete in reverse order
		})
		for _, stack := range stackList {
			if stack.Name == "eksctl-"+aws.StringValue(eksCluster.Cluster.Name)+"-cluster" {
				log.Print("EKS: Listing EBS Volumes")
				delvols, err := svc.DescribeVolumes(&ec2.DescribeVolumesInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("tag:kubernetes.io/cluster/" + aws.StringValue(eksCluster.Cluster.Name)),
							Values: aws.StringSlice([]string{"owned"}),
						}, {
							Name:   aws.String("tag:KubernetesCluster"),
							Values: aws.StringSlice([]string{aws.StringValue(eksCluster.Cluster.Name)}),
						},
					},
				})
				if err != nil {
					return makeErrorResponse("Could not ec2.ListVolumes: ", err)
				}
				log.Printf("EKS: Deleting %d EBS volumes created using the ebs-csi driver (tag:KubernetesCluster={CLUSTERNAME} tag:kubernetes.io/cluster/{CLUSTERNAME}=owned)", len(delvols.Volumes))
				for _, delvol := range delvols.Volumes {
					log.Printf("Deleting %s", *delvol.VolumeId)
					_, err = svc.DeleteVolume(&ec2.DeleteVolumeInput{
						VolumeId: delvol.VolumeId,
					})
					if err != nil {
						return makeErrorResponse("Could not ec2.DeleteVolume: ", err)
					}
				}
				log.Print("Checking IAM Identity Providers (tag:alpha.eksctl.io/cluster-name={CLUSTERNAME})")
				iamsvc := iam.New(sess)
				oidc, err := iamsvc.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
				if err != nil {
					return makeErrorResponse("Could not iam.ListOpenIDConnectProviders: ", err)
				}
				oidcDelList := []string{}
				for _, oidcItem := range oidc.OpenIDConnectProviderList {
					tags, err := iamsvc.ListOpenIDConnectProviderTags(&iam.ListOpenIDConnectProviderTagsInput{
						OpenIDConnectProviderArn: oidcItem.Arn,
					})
					if err != nil {
						return makeErrorResponse("Could not iam.ListOpenIDConnectProviderTags on "+*oidcItem.Arn+": ", err)
					}
					tagList := make(map[string]string)
					for _, tag := range tags.Tags {
						tagList[*tag.Key] = *tag.Value
					}
					if tagList["alpha.eksctl.io/cluster-name"] == aws.StringValue(eksCluster.Cluster.Name) {
						oidcDelList = append(oidcDelList, *oidcItem.Arn)
					}
				}
				log.Printf("Deleting %d IAM Identity Provider for the k8s cluster", len(oidcDelList))
				for _, oidcDel := range oidcDelList {
					_, err = iamsvc.DeleteOpenIDConnectProvider(&iam.DeleteOpenIDConnectProviderInput{
						OpenIDConnectProviderArn: aws.String(oidcDel),
					})
					if err != nil {
						return makeErrorResponse("Could not iam.DeleteOpenIDConnectProvider on "+oidcDel+": ", err)
					}
				}
			}
			if stack.CurrentStatus != cloudformation.StackStatusDeleteInProgress {
				log.Printf("EKS: Scheduling deletion of stack %s", stack.Name)
				_, err = cfSvc.DeleteStack(&cloudformation.DeleteStackInput{
					StackName: aws.String(stack.Name),
				})
				if err != nil {
					return makeErrorResponse("Could not cloudformation.DeleteStack: ", err)
				}
			}
			log.Printf("EKS: Waiting on %s to be deleted", stack.Name)
			err = cfSvc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
				StackName: aws.String(stack.Name),
			})
			if err != nil {
				return makeErrorResponse("Could not cloudformation.WaitUntilStackDeleteComplete: ", err)
			}
		}
		log.Println("EKS: Cluster Expired")
	}

	// expire keys
	log.Print("Expiring SSH keys")
	clusters := []string{}
	err = svc.DescribeInstancesPages(&ec2.DescribeInstancesInput{}, func(out *ec2.DescribeInstancesOutput, nextPage bool) bool {
		for _, r := range out.Reservations {
			for _, instance := range r.Instances {
				tags := make(map[string]string)
				for _, tag := range instance.Tags {
					tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
				}
				name := tags["Aerolab4ClusterName"]
				if name == "" {
					name = tags["Aerolab4clientClusterName"]
				}
				if name != "" && !inslice.HasString(clusters, name) {
					clusters = append(clusters, name)
				}
			}
		}
		return true
	})
	log.Print(clusters)
	if err != nil {
		return makeErrorResponse("Could not DescribeInstancesPages: ", err)
	}
	keys, err := svc.DescribeKeyPairs(&ec2.DescribeKeyPairsInput{})
	if err != nil {
		return makeErrorResponse("Could not DescribeKeyPairs: ", err)
	}
	expRegion := os.Getenv("AWS_REGION")
	for _, key := range keys.KeyPairs {
		if !strings.HasPrefix(*key.KeyName, "aerolab-") {
			continue
		}
		if strings.HasPrefix(*key.KeyName, "aerolab-template") {
			continue
		}
		if !inslice.HasString(clusters, strings.TrimSuffix(strings.TrimPrefix(*key.KeyName, "aerolab-"), "_"+expRegion)) {
			svc.DeleteKeyPair(&ec2.DeleteKeyPairInput{
				KeyName: key.KeyName,
			})
		}
	}

	if os.Getenv("route53_zoneid") == "" {
		return MyResponse{Message: "Completed", Status: "OK"}, nil
	}

	log.Println("DNS Cleanup")
	zoneid := os.Getenv("route53_zoneid")
	region := os.Getenv("route53_region")
	var regionx *string
	if region != "" {
		regionx = &region
	}
	sessx, err := session.NewSession(&aws.Config{
		Region: regionx,
	})
	if err != nil {
		return makeErrorResponse("Could not route53 new session: ", err)
	}
	r53 := route53.New(sessx)
	hosts, err := DNSListHosts(r53, zoneid)
	if err != nil {
		return makeErrorResponse("Could not route53 list hosts: ", err)
	}
	changeBatch := route53.ChangeBatch{}
	// instance-id.region.agi.*
	// len=4+ [0]=instance-id [1]=region [2]=agi
	for _, host := range hosts {
		hostSplit := strings.Split(host.Name, ".")
		if len(hostSplit) < 4 {
			continue
		}
		if hostSplit[2] != "agi" {
			continue
		}
		if hostSplit[1] != os.Getenv("AWS_REGION") {
			continue
		}
		if inslice.HasString(r53instanceList, hostSplit[0]) {
			continue
		}
		ips := []*route53.ResourceRecord{}
		for _, ip := range host.IPs {
			ips = append(ips, &route53.ResourceRecord{
				Value: aws.String(ip),
			})
		}
		log.Printf("Removing %s (%v) TTL:%d", host.Name, host.IPs, host.TTL)
		changeBatch.Changes = append(changeBatch.Changes, &route53.Change{
			Action: aws.String("DELETE"),
			ResourceRecordSet: &route53.ResourceRecordSet{
				Name:            aws.String(host.Name),
				TTL:             aws.Int64(host.TTL),
				Type:            aws.String("A"),
				ResourceRecords: ips,
			},
		})
	}
	_, err = r53.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneid),
		ChangeBatch:  &changeBatch,
	})
	if err != nil {
		return makeErrorResponse("Could not route53 remove hosts: ", err)
	}

	return MyResponse{Message: "Completed", Status: "OK"}, nil
}

type DNSHost struct {
	Name string
	IPs  []string
	TTL  int64
}

func DNSListHosts(r53 *route53.Route53, zoneId string) (hosts []DNSHost, err error) {
	out, err := r53.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneId),
	})
	if err != nil {
		return nil, err
	}
	for _, rset := range out.ResourceRecordSets {
		if rset == nil || rset.Type == nil || rset.TTL == nil || rset.Name == nil {
			continue
		}
		if *rset.Type != "A" {
			continue
		}
		ips := []string{}
		for _, rr := range rset.ResourceRecords {
			ips = append(ips, *rr.Value)
		}
		hosts = append(hosts, DNSHost{
			Name: *rset.Name,
			IPs:  ips,
			TTL:  *rset.TTL,
		})
	}
	return hosts, nil
}

func main() {
	lambda.Start(HandleLambdaEvent)
}

func makeErrorResponse(prefix string, err error) (MyResponse, error) {
	return MyResponse{Message: prefix + err.Error(), Status: "ERROR"}, errors.New(prefix + err.Error())
}

type telemetry struct {
	UUID          string
	Job           string
	Cloud         string
	Zone          string
	Instance      string
	ClusterName   string
	NodeNo        string
	ExpiryVersion string
	CmdLine       []string
	Time          int64
}

var telemetryLock = new(sync.WaitGroup)

func telemetryShip(uuid string, zone string, instance string, clusterName string, nodeNo string) {
	telemetryLock.Add(1)
	go func() {
		defer telemetryLock.Done()
		err := telemetryShipDo(uuid, zone, instance, clusterName, nodeNo)
		if err != nil {
			log.Println(err)
		}
	}()
}

func telemetryShipDo(uuid string, zone string, instance string, clusterName string, nodeNo string) error {
	t := &telemetry{
		UUID:          uuid,
		Job:           "expire",
		Cloud:         "AWS",
		Zone:          zone,
		Instance:      instance,
		ClusterName:   clusterName,
		NodeNo:        nodeNo,
		ExpiryVersion: "2",
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
	}
	contents, err := json.Marshal(t)
	if err != nil {
		return err
	}
	url := "https://us-central1-aerospike-gaia.cloudfunctions.net/aerolab-telemetrics"
	ret, err := http.Post(url, "application/json", bytes.NewReader(contents))
	if err != nil {
		return err
	}
	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("returned ret code: %d:%s", ret.StatusCode, ret.Status)
	}
	return nil
}
