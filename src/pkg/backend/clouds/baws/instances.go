package baws

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// TODO: create instances call

type instanceDetail struct {
	SecurityGroups        []types.GroupIdentifier   `yaml:"securityGroups" json:"securityGroups"`
	ClientToken           string                    `yaml:"clientToken" json:"clientToken"`
	EnaSupport            bool                      `yaml:"enaSupport" json:"enaSupport"`
	IAMInstanceProfile    *types.IamInstanceProfile `yaml:"iamInstanceProfile" json:"iamInstanceProfile"`
	SpotInstanceRequestId string                    `yaml:"spotInstanceRequestID" json:"spotInstanceRequestID"`
	LifecycleType         string                    `yaml:"lifecycleType" json:"lifecycleType"`
}

func (s *b) GetInstances(volumes backend.VolumeList) (backend.InstanceList, error) {
	var i backend.InstanceList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errors.Join(errs, err)
				return
			}
			paginator := ec2.NewDescribeInstancesPaginator(cli, &ec2.DescribeInstancesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("tag-key"),
						Values: []string{TAG_AEROLAB_VERSION},
					}, {
						Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
						Values: []string{s.project},
					},
				},
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errors.Join(errs, err)
					return
				}
				for _, res := range out.Reservations {
					for _, inst := range res.Instances {
						tags := make(map[string]string)
						for _, t := range inst.Tags {
							tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
						}
						expires, _ := time.Parse(time.RFC3339, tags[TAG_EXPIRES])
						state := backend.LifeCycleStateUnknown
						switch inst.State.Name {
						case types.InstanceStateNamePending:
							state = backend.LifeCycleStateCreating
						case types.InstanceStateNameRunning:
							state = backend.LifeCycleStateRunning
						case types.InstanceStateNameShuttingDown:
							state = backend.LifeCycleStateTerminating
						case types.InstanceStateNameTerminated:
							state = backend.LifeCycleStateTerminated
						case types.InstanceStateNameStopping:
							state = backend.LifeCycleStateStopping
						case types.InstanceStateNameStopped:
							state = backend.LifeCycleStateStopped
						}
						firewalls := []string{}
						for _, f := range inst.SecurityGroups {
							if f.GroupName == nil || *f.GroupName == "" {
								firewalls = append(firewalls, aws.ToString(f.GroupId))
							} else {
								firewalls = append(firewalls, aws.ToString(f.GroupName))
							}
						}
						spot := false
						if inst.InstanceLifecycle != types.InstanceLifecycleTypeScheduled {
							spot = true
						}
						startTime := time.Time{}
						if tags[TAG_START_TIME] != "" {
							startTime, _ = time.Parse(time.RFC3339, tags[TAG_START_TIME])
						}
						costSoFar, _ := strconv.ParseFloat(tags[TAG_COST_SO_FAR], 64)
						pph, _ := strconv.ParseFloat(tags[TAG_COST_PPH], 64)
						volIDs := []string{}
						for _, v := range inst.BlockDeviceMappings {
							volIDs = append(volIDs, aws.ToString(v.Ebs.VolumeId))
						}
						vols := volumes.WithVolumeID(volIDs...).Describe()
						dvols := backend.CostVolumes{}
						avols := backend.CostVolumes{}
						for _, vol := range vols {
							cpg, _ := strconv.ParseFloat(vol.Tags[TAG_COST_GB], 64)
							volcost := backend.CostVolume{
								PricePerGBHour: cpg,
								SizeGB:         int64(vol.Size / backend.StorageGB),
								CreateTime:     vol.CreationTime,
							}
							if vol.DeleteOnTermination {
								dvols = append(dvols, volcost)
							} else {
								avols = append(avols, volcost)
							}
						}
						ilock.Lock()
						i = append(i, &backend.Instance{
							ClusterName:  tags[TAG_CLUSTER_NAME],
							NodeNo:       toInt(tags[TAG_NODE_NO]),
							InstanceID:   aws.ToString(inst.InstanceId),
							BackendType:  backend.BackendTypeAWS,
							InstanceType: string(inst.InstanceType),
							Name:         tags[TAG_NAME],
							Description:  tags[TAG_DESCRIPTION],
							ZoneName:     zone,
							ZoneID:       zone,
							CreationTime: aws.ToTime(inst.LaunchTime),
							Owner:        tags[TAG_OWNER],
							Tags:         tags,
							Expires:      expires,
							PublicIP:     aws.ToString(inst.PublicIpAddress),
							PrivateIP:    aws.ToString(inst.PrivateIpAddress),
							ImageID:      aws.ToString(inst.ImageId),
							SSHKeyName:   aws.ToString(inst.KeyName),
							SubnetID:     aws.ToString(inst.SubnetId),
							NetworkID:    aws.ToString(inst.VpcId),
							Architecture: string(inst.Architecture),
							OperatingSystem: backend.OS{
								Name:    tags[TAG_OS_NAME],
								Version: tags[TAG_OS_VERSION],
							},
							Firewalls:       firewalls,
							InstanceState:   state,
							SpotInstance:    spot,
							AttachedVolumes: vols,
							EstimatedCostUSD: backend.Cost{
								Instance: backend.CostInstance{
									RunningPricePerHour: pph,
									CostUntilLastStop:   costSoFar,
									LastStartTime:       startTime,
								},
								DeployedVolumes: dvols,
								AttachedVolumes: avols,
							},
							BackendSpecific: &instanceDetail{
								SecurityGroups:        inst.SecurityGroups,
								ClientToken:           aws.ToString(inst.ClientToken),
								EnaSupport:            aws.ToBool(inst.EnaSupport),
								IAMInstanceProfile:    inst.IamInstanceProfile,
								SpotInstanceRequestId: aws.ToString(inst.SpotInstanceRequestId),
								LifecycleType:         string(inst.InstanceLifecycle),
							},
						})
						ilock.Unlock()
					}
				}
			}
		}(zone)
	}
	wg.Wait()
	return i, errs
}

func (s *b) InstancesAddTags(instances backend.InstanceList, tags map[string]string) error {
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	tagsOut := []types.Tag{}
	for k, v := range tags {
		tagsOut = append(tagsOut, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	for zone, ids := range instanceIds {
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			return err
		}
		_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
			Resources: ids,
			Tags:      tagsOut,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesRemoveTags(instances backend.InstanceList, tagKeys []string) error {
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	tagsOut := []types.Tag{}
	for _, k := range tagKeys {
		tagsOut = append(tagsOut, types.Tag{
			Key: aws.String(k),
		})
	}
	for zone, ids := range instanceIds {
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			return err
		}
		_, err = cli.DeleteTags(context.TODO(), &ec2.DeleteTagsInput{
			Resources: ids,
			Tags:      tagsOut,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesTerminate(instances backend.InstanceList, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]string)
	clis := make(map[string]*ec2.Client)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
			cli, err := getEc2Client(s.credentials, &instance.ZoneID)
			if err != nil {
				return err
			}
			clis[instance.ZoneID] = cli
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	for zone, ids := range instanceIds {
		_, err := clis[zone].TerminateInstances(context.TODO(), &ec2.TerminateInstancesInput{
			InstanceIds: ids,
		})
		if err != nil {
			return err
		}
	}
	if waitDur > 0 {
		for zone, ids := range instanceIds {
			w := time.Now()
			waiter := ec2.NewInstanceTerminatedWaiter(clis[zone])
			err := waiter.Wait(context.TODO(), &ec2.DescribeInstancesInput{
				InstanceIds: ids,
			}, waitDur)
			if err != nil {
				return err
			}
			waitDur -= time.Since(w)
			if waitDur < time.Second {
				return errors.New("wait timeout")
			}
		}
	}
	return nil
}

func (s *b) InstancesStop(instances backend.InstanceList, force bool, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]string)
	clis := make(map[string]*ec2.Client)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
			cli, err := getEc2Client(s.credentials, &instance.ZoneID)
			if err != nil {
				return err
			}
			clis[instance.ZoneID] = cli
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	for zone, ids := range instanceIds {
		_, err := clis[zone].StopInstances(context.TODO(), &ec2.StopInstancesInput{
			InstanceIds: ids,
			Force:       &force,
		})
		if err != nil {
			return err
		}
	}
	// for each instance, update cost so far, while we wait
	var reterr error
	retLock := new(sync.Mutex)
	retWait := new(sync.WaitGroup)
	retWait.Add(1)
	go func() {
		defer retWait.Done()
		for _, instance := range instances {
			err := s.InstancesAddTags(backend.InstanceList{instance}, map[string]string{
				TAG_COST_SO_FAR: strconv.FormatFloat(instance.EstimatedCostUSD.Instance.AccruedCost(), 'f', 4, 64),
				TAG_START_TIME:  "",
			})
			if err != nil {
				retLock.Lock()
				reterr = errors.Join(reterr, err)
				retLock.Unlock()
			}
		}
	}()
	// wait
	if waitDur > 0 {
		for zone, ids := range instanceIds {
			w := time.Now()
			waiter := ec2.NewInstanceStoppedWaiter(clis[zone])
			err := waiter.Wait(context.TODO(), &ec2.DescribeInstancesInput{
				InstanceIds: ids,
			}, waitDur)
			if err != nil {
				return err
			}
			waitDur -= time.Since(w)
			if waitDur < time.Second {
				return errors.New("wait timeout")
			}
		}
	}
	retWait.Wait()
	return reterr
}

func (s *b) InstancesStart(instances backend.InstanceList, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]string)
	clis := make(map[string]*ec2.Client)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
			cli, err := getEc2Client(s.credentials, &instance.ZoneID)
			if err != nil {
				return err
			}
			clis[instance.ZoneID] = cli
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	for zone, ids := range instanceIds {
		_, err := clis[zone].StartInstances(context.TODO(), &ec2.StartInstancesInput{
			InstanceIds: ids,
		})
		if err != nil {
			return err
		}
	}
	// tag instances, even if we are waiting
	wg := new(sync.WaitGroup)
	wg.Add(1)
	var reterr error
	go func() {
		defer wg.Done()
		reterr = s.InstancesAddTags(instances, map[string]string{
			TAG_START_TIME: time.Now().Format(time.RFC3339),
		})
	}()
	// wait
	if waitDur > 0 {
		for zone, ids := range instanceIds {
			w := time.Now()
			waiter := ec2.NewInstanceRunningWaiter(clis[zone])
			err := waiter.Wait(context.TODO(), &ec2.DescribeInstancesInput{
				InstanceIds: ids,
			}, waitDur)
			if err != nil {
				return err
			}
			waitDur -= time.Since(w)
			if waitDur < time.Second {
				return errors.New("wait timeout")
			}
		}
	}
	wg.Wait()
	return reterr
}
