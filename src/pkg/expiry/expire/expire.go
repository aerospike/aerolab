package expire

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
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

type ExpiryHandler struct {
	Backend      backend.Backend
	ExpireEksctl bool
	CleanupDNS   bool
	lock         sync.Mutex
	Credentials  *clouds.Credentials
}

func (h *ExpiryHandler) Expire() error {
	log.Print("Expiry start")
	h.lock.Lock()
	defer h.lock.Unlock()

	log.Print("Lock acquired, listing inventory")
	inventory, err := h.Backend.GetInventory()
	if err != nil {
		return err
	}

	instances := inventory.Instances.WithExpired(true).WithState(backend.LifeCycleStateRunning, backend.LifeCycleStateUnknown, backend.LifeCycleStateStopped, backend.LifeCycleStateFail, backend.LifeCycleStateCreated, backend.LifeCycleStateConfiguring).Describe()

	if len(instances) > 0 {
		logLine := fmt.Sprintf("Terminating %d instances: ", len(instances))
		for _, instance := range instances {
			logLine += fmt.Sprintf("clusterName=%s,nodeNo=%d,instanceID=%s;", instance.ClusterName, instance.NodeNo, instance.InstanceID)
		}
		log.Print(logLine)
		err = instances.Terminate(10 * time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Terminated %d instances", len(instances))
	} else {
		log.Print("No instances to terminate")
	}

	volumes := inventory.Volumes.WithExpired(true).Describe()
	if len(volumes) > 0 {
		logLine := fmt.Sprintf("Deleting %d volumes: ", len(volumes))
		for _, volume := range volumes {
			logLine += fmt.Sprintf("volumeID=%s;", volume.FileSystemId)
		}
		log.Print(logLine)
		err = volumes.DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
		if err != nil {
			return err
		}
		log.Printf("Deleted %d volumes", len(volumes))
	} else {
		log.Print("No volumes to delete")
	}

	if h.CleanupDNS {
		log.Print("Cleaning up stale DNS")
		err = h.Backend.CleanupDNS()
		if err != nil {
			return err
		}
	}

	if h.ExpireEksctl {
		regions, err := h.Backend.ListEnabledRegions(backend.BackendTypeAWS)
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
