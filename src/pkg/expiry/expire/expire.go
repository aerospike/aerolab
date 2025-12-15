package expire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// telemetryTagKey is the tag key used to identify resources that should have telemetry sent on expiry
const telemetryTagKey = "aerolab.telemetry"

// telemetryURL is the endpoint for sending telemetry data
const telemetryURL = "https://aerolab-telemetry-595313549904.us-central1.run.app"

// telemetryExpiryVersion is the version of the expiry telemetry format
const telemetryExpiryVersion = "5"

// expiryTelemetry represents the telemetry data sent when expiring resources
type expiryTelemetry struct {
	UUID          string
	Job           string
	Cloud         string
	Zone          string
	ResourceID    string
	ClusterUUID   string
	ResourceType  string
	ResourceName  string
	ClusterName   string
	NodeNo        string
	ExpiryVersion string
	CmdLine       []string
	Time          int64
	Tags          map[string]string
}

// telemetryWaitGroup is used to wait for all telemetry goroutines to complete
var telemetryWaitGroup = new(sync.WaitGroup)

type ExpiryHandler struct {
	Backend      backends.Backend
	ExpireEksctl bool
	CleanupDNS   bool
	lock         sync.Mutex
	Credentials  *clouds.Credentials
}

func (h *ExpiryHandler) Expire() error {
	log.Print("Expiry start")
	if !h.lock.TryLock() {
		log.Print("Another invocation is already running, skipping")
		return nil
	}
	defer h.lock.Unlock()

	// Ensure all telemetry goroutines complete before returning
	defer telemetryWaitGroup.Wait()

	log.Print("Lock acquired, listing inventory")
	err := h.Backend.ForceRefreshInventory()
	if err != nil {
		return err
	}
	inventory := h.Backend.GetInventory()

	instances := inventory.Instances.WithExpired(true).WithState(backends.LifeCycleStateRunning, backends.LifeCycleStateUnknown, backends.LifeCycleStateStopped, backends.LifeCycleStateFail, backends.LifeCycleStateCreated, backends.LifeCycleStateConfiguring).Describe()

	if len(instances) > 0 {
		logLine := fmt.Sprintf("Terminating %d instances: ", len(instances))
		for _, instance := range instances {
			logLine += fmt.Sprintf("clusterName=%s,nodeNo=%d,instanceID=%s;", instance.ClusterName, instance.NodeNo, instance.InstanceID)
		}
		log.Print(logLine)

		// Send telemetry for instances with aerolab.telemetry=true tag before termination
		for _, instance := range instances {
			if v, ok := instance.Tags[telemetryTagKey]; ok && v != "" {
				h.shipInstanceTelemetry(instance)
			}
		}

		err := instances.Terminate(10 * time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Terminated %d instances", len(instances))
	} else {
		log.Print("No instances to terminate")
	}

	volumes := inventory.Volumes.WithDeleteOnTermination(false).WithExpired(true).Describe()
	if len(volumes) > 0 {
		logLine := fmt.Sprintf("Deleting %d volumes: ", len(volumes))
		for _, volume := range volumes {
			logLine += fmt.Sprintf("volumeID=%s;", volume.FileSystemId)
		}
		log.Print(logLine)

		// Send telemetry for volumes with aerolab.telemetry=true tag before deletion
		for _, volume := range volumes {
			if v, ok := volume.Tags[telemetryTagKey]; ok && v != "" {
				h.shipVolumeTelemetry(volume)
			}
		}

		err := volumes.DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Deleted %d volumes", len(volumes))
	} else {
		log.Print("No volumes to delete")
	}

	if h.CleanupDNS {
		log.Print("Cleaning up stale DNS")
		err := h.Backend.CleanupDNS()
		if err != nil {
			return err
		}
	}

	if h.ExpireEksctl {
		regions, err := h.Backend.ListEnabledRegions(backends.BackendTypeAWS)
		if err != nil {
			return err
		}
		for _, region := range regions {
			err = h.expireEksctl(region)
			if err != nil {
				return err
			}
		}
	}

	log.Print("Expiry complete, releasing lock")
	return nil
}

func (h *ExpiryHandler) expireEksctl(region string) error {
	log.Println("EKS: Listing clusters")
	ekssvc, err := baws.GetEksClient(h.Credentials, &region)
	if err != nil {
		return fmt.Errorf("could not get EKS client: %w", err)
	}
	ec2svc, err := baws.GetEc2Client(h.Credentials, &region)
	if err != nil {
		return fmt.Errorf("could not get EC2 client: %w", err)
	}
	eksClusters, err := ekssvc.ListClusters(context.TODO(), &eks.ListClustersInput{})
	if err != nil {
		return fmt.Errorf("could not list EKS: %w", err)
	}
	now := time.Now()
	for _, eksClusterName := range eksClusters.Clusters {
		eksCluster, err := ekssvc.DescribeCluster(context.TODO(), &eks.DescribeClusterInput{Name: aws.String(eksClusterName)})
		if err != nil {
			return fmt.Errorf("could not describe EKS cluster: %w", err)
		}
		eksAt := eksCluster.Cluster.Tags["ExpireAt"]
		var expireAt time.Time
		if eksAt != "" {
			oldAtInt, err := strconv.Atoi(eksAt)
			if err == nil {
				expireAt = time.Unix(int64(oldAtInt), 0)
			}
		} else {
			initial := eksCluster.Cluster.Tags["initialExpiry"]
			if initial != "" {
				createTs := aws.ToTime(eksCluster.Cluster.CreatedAt)
				initialDuration, err := time.ParseDuration(initial)
				if err == nil {
					expireAt = createTs.Add(initialDuration)
				}
			}
		}
		if expireAt.IsZero() {
			log.Printf("EKS: Cluster=%s NoExpirySet", aws.ToString(eksCluster.Cluster.Name))
			continue
		}
		if expireAt.After(now) {
			log.Printf("EKS: Cluster=%s Expiry=%s NotExpired", aws.ToString(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))
			continue
		}
		log.Printf("EKS: Cluster=%s Expiry=%s Starting Expiry", aws.ToString(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))

		log.Print("EKS: Listing cloudformation stacks...")
		cfSvc, err := baws.GetCloudformationClient(h.Credentials, &region)
		if err != nil {
			return fmt.Errorf("could not get cloudformation client: %w", err)
		}
		stacks, err := cfSvc.ListStacks(context.TODO(), &cloudformation.ListStacksInput{})
		if err != nil {
			return fmt.Errorf("could not cloudformation.ListStacks: %w", err)
		}
		type astack struct {
			Name          string
			CreationTime  time.Time
			CurrentStatus string
		}
		stackList := []*astack{}
		for _, stack := range stacks.StackSummaries {
			if !strings.HasPrefix(aws.ToString(stack.StackName), "eksctl-"+aws.ToString(eksCluster.Cluster.Name)+"-") {
				continue
			}
			if stack.StackStatus == cftypes.StackStatusDeleteComplete {
				continue
			}
			stackList = append(stackList, &astack{
				Name:          aws.ToString(stack.StackName),
				CreationTime:  aws.ToTime(stack.CreationTime),
				CurrentStatus: string(stack.StackStatus),
			})
		}
		sort.Slice(stackList, func(i, j int) bool {
			return stackList[j].CreationTime.Before(stackList[i].CreationTime) // reverse sort since we want to delete in reverse order
		})
		for _, stack := range stackList {
			if stack.Name == "eksctl-"+aws.ToString(eksCluster.Cluster.Name)+"-cluster" {
				log.Print("EKS: Listing EBS Volumes")
				delvols, err := ec2svc.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
					Filters: []ec2types.Filter{
						{
							Name:   aws.String("tag:kubernetes.io/cluster/" + aws.ToString(eksCluster.Cluster.Name)),
							Values: []string{"owned"},
						}, {
							Name:   aws.String("tag:KubernetesCluster"),
							Values: []string{aws.ToString(eksCluster.Cluster.Name)},
						},
					},
				})
				if err != nil {
					return fmt.Errorf("could not ec2.ListVolumes: %w", err)
				}
				log.Printf("EKS: Deleting %d EBS volumes created using the ebs-csi driver (tag:KubernetesCluster={CLUSTERNAME} tag:kubernetes.io/cluster/{CLUSTERNAME}=owned)", len(delvols.Volumes))
				for _, delvol := range delvols.Volumes {
					log.Printf("Deleting %s", *delvol.VolumeId)
					_, err = ec2svc.DeleteVolume(context.TODO(), &ec2.DeleteVolumeInput{
						VolumeId: delvol.VolumeId,
					})
					if err != nil {
						return fmt.Errorf("could not ec2.DeleteVolume: %w", err)
					}
				}
				log.Print("Checking IAM Identity Providers (tag:alpha.eksctl.io/cluster-name={CLUSTERNAME})")
				iamsvc, err := baws.GetIamClient(h.Credentials, &region)
				if err != nil {
					return fmt.Errorf("could not get iam client: %w", err)
				}
				oidc, err := iamsvc.ListOpenIDConnectProviders(context.TODO(), &iam.ListOpenIDConnectProvidersInput{})
				if err != nil {
					return fmt.Errorf("could not iam.ListOpenIDConnectProviders: %w", err)
				}
				oidcDelList := []string{}
				for _, oidcItem := range oidc.OpenIDConnectProviderList {
					tags, err := iamsvc.ListOpenIDConnectProviderTags(context.TODO(), &iam.ListOpenIDConnectProviderTagsInput{
						OpenIDConnectProviderArn: oidcItem.Arn,
					})
					if err != nil {
						return fmt.Errorf("could not iam.ListOpenIDConnectProviderTags on %s: %w", *oidcItem.Arn, err)
					}
					tagList := make(map[string]string)
					for _, tag := range tags.Tags {
						tagList[*tag.Key] = *tag.Value
					}
					if tagList["alpha.eksctl.io/cluster-name"] == aws.ToString(eksCluster.Cluster.Name) {
						oidcDelList = append(oidcDelList, *oidcItem.Arn)
					}
				}
				log.Printf("Deleting %d IAM Identity Provider for the k8s cluster", len(oidcDelList))
				for _, oidcDel := range oidcDelList {
					_, err = iamsvc.DeleteOpenIDConnectProvider(context.TODO(), &iam.DeleteOpenIDConnectProviderInput{
						OpenIDConnectProviderArn: aws.String(oidcDel),
					})
					if err != nil {
						return fmt.Errorf("could not iam.DeleteOpenIDConnectProvider on %s: %w", oidcDel, err)
					}
				}
			}
			if stack.CurrentStatus != string(cftypes.StackStatusDeleteInProgress) {
				log.Printf("EKS: Scheduling deletion of stack %s", stack.Name)
				_, err = cfSvc.DeleteStack(context.TODO(), &cloudformation.DeleteStackInput{
					StackName: aws.String(stack.Name),
				})
				if err != nil {
					return fmt.Errorf("could not cloudformation.DeleteStack: %w", err)
				}
			}
			log.Printf("EKS: Waiting on %s to be deleted", stack.Name)
			waiter := cloudformation.NewStackDeleteCompleteWaiter(cfSvc)
			err = waiter.Wait(context.TODO(), &cloudformation.DescribeStacksInput{
				StackName: aws.String(stack.Name),
			}, 10*time.Minute)
			if err != nil {
				return fmt.Errorf("could not cloudformation.WaitUntilStackDeleteComplete: %w", err)
			}
		}
		log.Println("EKS: Cluster Expired")
	}
	return nil
}

// shipInstanceTelemetry sends telemetry data for an expiring instance asynchronously
func (h *ExpiryHandler) shipInstanceTelemetry(instance *backends.Instance) {
	telemetryWaitGroup.Add(1)
	go func() {
		defer telemetryWaitGroup.Done()
		err := h.shipInstanceTelemetrySync(instance)
		if err != nil {
			log.Printf("Telemetry: failed to send telemetry for instance %s: %s", instance.InstanceID, err)
		}
	}()
}

// shipInstanceTelemetrySync sends telemetry data for an expiring instance synchronously
func (h *ExpiryHandler) shipInstanceTelemetrySync(instance *backends.Instance) error {
	cloud := "unknown"
	switch instance.BackendType {
	case backends.BackendTypeAWS:
		cloud = "AWS"
	case backends.BackendTypeGCP:
		cloud = "GCP"
	case backends.BackendTypeDocker:
		cloud = "Docker"
	}

	t := &expiryTelemetry{
		UUID:          instance.Tags[telemetryTagKey],
		Job:           "expire",
		Cloud:         cloud,
		Zone:          instance.ZoneName,
		ResourceID:    instance.InstanceID,
		ClusterUUID:   instance.ClusterUUID,
		ResourceType:  "instance",
		ClusterName:   instance.ClusterName,
		NodeNo:        strconv.Itoa(instance.NodeNo),
		ResourceName:  instance.Name,
		ExpiryVersion: telemetryExpiryVersion,
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
		Tags:          instance.Tags,
	}

	contents, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	ret, err := http.Post(telemetryURL, "application/json", bytes.NewReader(contents))
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer ret.Body.Close()

	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("telemetry returned status code: %d:%s", ret.StatusCode, ret.Status)
	}

	log.Printf("Telemetry: sent for instance %s (cluster=%s, node=%d)", instance.InstanceID, instance.ClusterName, instance.NodeNo)
	return nil
}

// shipVolumeTelemetry sends telemetry data for an expiring volume asynchronously
func (h *ExpiryHandler) shipVolumeTelemetry(volume *backends.Volume) {
	telemetryWaitGroup.Add(1)
	go func() {
		defer telemetryWaitGroup.Done()
		err := h.shipVolumeTelemetrySync(volume)
		if err != nil {
			log.Printf("Telemetry: failed to send telemetry for volume %s: %s", volume.FileSystemId, err)
		}
	}()
}

// shipVolumeTelemetrySync sends telemetry data for an expiring volume synchronously
func (h *ExpiryHandler) shipVolumeTelemetrySync(volume *backends.Volume) error {
	cloud := "unknown"
	switch volume.BackendType {
	case backends.BackendTypeAWS:
		cloud = "AWS"
	case backends.BackendTypeGCP:
		cloud = "GCP"
	case backends.BackendTypeDocker:
		cloud = "Docker"
	}

	t := &expiryTelemetry{
		UUID:          volume.Tags[telemetryTagKey],
		Job:           "expire",
		Cloud:         cloud,
		Zone:          volume.ZoneName,
		ResourceID:    volume.FileSystemId,
		ResourceType:  "volume",
		ResourceName:  volume.Name,
		ClusterName:   "",
		NodeNo:        "",
		ExpiryVersion: telemetryExpiryVersion,
		Time:          time.Now().UnixMicro(),
		CmdLine:       []string{"EXPIRY"},
		Tags:          volume.Tags,
	}

	contents, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	ret, err := http.Post(telemetryURL, "application/json", bytes.NewReader(contents))
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer ret.Body.Close()

	if ret.StatusCode < 200 || ret.StatusCode > 299 {
		return fmt.Errorf("telemetry returned status code: %d:%s", ret.StatusCode, ret.Status)
	}

	log.Printf("Telemetry: sent for volume %s (name=%s)", volume.FileSystemId, volume.Name)
	return nil
}
