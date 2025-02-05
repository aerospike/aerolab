package baws

import (
	"context"
	"errors"
	"os"
	"path"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/parallelize"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
)

// TODO: create instances call

type instanceDetail struct {
	SecurityGroups        []types.GroupIdentifier   `yaml:"securityGroups" json:"securityGroups"`
	ClientToken           string                    `yaml:"clientToken" json:"clientToken"`
	EnaSupport            bool                      `yaml:"enaSupport" json:"enaSupport"`
	IAMInstanceProfile    *types.IamInstanceProfile `yaml:"iamInstanceProfile" json:"iamInstanceProfile"`
	SpotInstanceRequestId string                    `yaml:"spotInstanceRequestID" json:"spotInstanceRequestID"`
	LifecycleType         string                    `yaml:"lifecycleType" json:"lifecycleType"`
	Volumes               []instanceVolume          `yaml:"volumes" json:"volumes"`
	FirewallList          backend.FirewallList      `yaml:"firewallList" json:"firewallList"`
	Network               *backend.Network          `yaml:"network" json:"network"`
	Subnet                *backend.Subnet           `yaml:"subnet" json:"subnet"`
}

type instanceVolume struct {
	Device   string `yaml:"device" json:"device"`
	VolumeID string `yaml:"volumeID" json:"volumeID"`
}

func (s *b) GetInstances(volumes backend.VolumeList, networkList backend.NetworkList, firewallList backend.FirewallList) (backend.InstanceList, error) {
	log := s.log.WithPrefix("GetInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backend.InstanceList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
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
					errs = errors.Join(errs, err)
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
							firewalls = append(firewalls, aws.ToString(f.GroupId))
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
							if vol.DeleteOnTermination {
								dvols = append(dvols, vol.EstimatedCostUSD)
							} else {
								avols = append(avols, vol.EstimatedCostUSD)
							}
						}
						volslist := []instanceVolume{}
						for _, v := range inst.BlockDeviceMappings {
							volslist = append(volslist, instanceVolume{
								Device:   aws.ToString(v.DeviceName),
								VolumeID: aws.ToString(v.Ebs.VolumeId),
							})
						}
						arch := backend.ArchitectureARM64
						if inst.Architecture == types.ArchitectureValuesX8664 {
							arch = backend.ArchitectureX8664
						}
						net := &backend.Network{}
						sub := &backend.Subnet{}
						nets := networkList.WithNetID(aws.ToString(inst.VpcId))
						if nets.Count() > 0 {
							nnet := nets.Describe()[0]
							net = nnet
							ssub := nnet.Subnets.WithSubnetId(aws.ToString(inst.SubnetId))
							if len(ssub) > 0 {
								sub = ssub[0]
							}
						}
						fwPointers := backend.FirewallList{}
						for _, fw := range firewalls {
							fwx := firewallList.WithFirewallID(fw)
							if fwx.Count() > 0 {
								fwPointers = append(fwPointers, fwx.Describe()[0])
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
							IP: backend.IP{
								Public:  aws.ToString(inst.PublicIpAddress),
								Private: aws.ToString(inst.PrivateIpAddress),
							},
							ImageID:      aws.ToString(inst.ImageId),
							SSHKeyName:   aws.ToString(inst.KeyName),
							SubnetID:     aws.ToString(inst.SubnetId),
							NetworkID:    aws.ToString(inst.VpcId),
							Architecture: arch,
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
								Volumes:               volslist,
								FirewallList:          fwPointers,
								Network:               net,
								Subnet:                sub,
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
	log := s.log.WithPrefix("InstancesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
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
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
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
	log := s.log.WithPrefix("InstancesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
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
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
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
	log := s.log.WithPrefix("InstancesTerminate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
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
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		_, err := clis[zone].TerminateInstances(context.TODO(), &ec2.TerminateInstancesInput{
			InstanceIds: ids,
		})
		if err != nil {
			return err
		}
	}
	if waitDur > 0 {
		for zone, ids := range instanceIds {
			log.Detail("zone=%s wait: start", zone)
			defer log.Detail("zone=%s wait: end", zone)
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
	log := s.log.WithPrefix("InstancesStop: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
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
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
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
		log.Detail("tag instances start")
		defer log.Detail("tag instances end")
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
			log.Detail("zone=%s wait: start", zone)
			defer log.Detail("zone=%s wait: end", zone)
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
	log := s.log.WithPrefix("InstancesStart: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
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
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
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
		log.Detail("tag-instances start")
		defer log.Detail("tag-instances end")
		reterr = s.InstancesAddTags(instances, map[string]string{
			TAG_START_TIME: time.Now().Format(time.RFC3339),
		})
	}()
	// wait
	if waitDur > 0 {
		for zone, ids := range instanceIds {
			log.Detail("zone=%s wait: start", zone)
			defer log.Detail("zone=%s wait: end", zone)
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

func (s *b) InstancesExec(instances backend.InstanceList, e *backend.ExecInput) []*backend.ExecOutput {
	log := s.log.WithPrefix("InstancesExec: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	if e.ParallelThreads == 0 {
		e.ParallelThreads = len(instances)
	}
	out := []*backend.ExecOutput{}
	outl := new(sync.Mutex)
	parallelize.ForEachLimit(instances, e.ParallelThreads, func(i *backend.Instance) {
		if i.InstanceState != backend.LifeCycleStateRunning {
			outl.Lock()
			out = append(out, &backend.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: errors.New("instance not running"),
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, i.ClusterName))
		if err != nil {
			outl.Lock()
			out = append(out, &backend.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: err,
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		clientConf := sshexec.ClientConf{
			Host:           i.IP.Routable(),
			Port:           22,
			Username:       e.Username,
			PrivateKey:     nKey,
			ConnectTimeout: 30 * time.Second,
		}
		o := sshexec.Exec(&sshexec.ExecInput{
			ClientConf: clientConf,
			ExecDetail: e.ExecDetail,
		})
		outl.Lock()
		out = append(out, &backend.ExecOutput{
			Output:   o,
			Instance: i,
		})
		outl.Unlock()
	})
	return out
}

func (s *b) InstancesGetSftpConfig(instances backend.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	log := s.log.WithPrefix("InstancesGetSftpConfig: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	confs := []*sshexec.ClientConf{}
	for _, i := range instances {
		if i.InstanceState != backend.LifeCycleStateRunning {
			return nil, errors.New("instance not running")
		}
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, i.ClusterName))
		if err != nil {
			return nil, errors.New("required key not found")
		}
		clientConf := &sshexec.ClientConf{
			Host:           i.IP.Routable(),
			Port:           22,
			Username:       username,
			PrivateKey:     nKey,
			ConnectTimeout: 30 * time.Second,
		}
		confs = append(confs, clientConf)
	}
	return confs, nil
}

func (s *b) InstancesAssignFirewalls(instances backend.InstanceList, fw backend.FirewallList) error {
	log := s.log.WithPrefix("InstancesAssignFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]*backend.Instance)
	clis := make(map[string]*ec2.Client)
	for _, instance := range instances {
		instance := instance
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []*backend.Instance{}
			cli, err := getEc2Client(s.credentials, &instance.ZoneID)
			if err != nil {
				return err
			}
			clis[instance.ZoneID] = cli
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance)
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range instanceIds {
		wg.Add(1)
		go func(zone string, ids []*backend.Instance) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			for _, id := range ids {
				allGroups := id.Firewalls
				for _, f := range fw {
					if !slices.Contains(allGroups, f.FirewallID) {
						allGroups = append(allGroups, f.FirewallID)
					}
				}
				_, err := clis[zone].ModifyInstanceAttribute(context.TODO(), &ec2.ModifyInstanceAttributeInput{
					InstanceId: aws.String(id.InstanceID),
					Groups:     allGroups,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

func (s *b) InstancesRemoveFirewalls(instances backend.InstanceList, fw backend.FirewallList) error {
	log := s.log.WithPrefix("InstancesRemoveFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	instanceIds := make(map[string][]*backend.Instance)
	clis := make(map[string]*ec2.Client)
	for _, instance := range instances {
		instance := instance
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []*backend.Instance{}
			cli, err := getEc2Client(s.credentials, &instance.ZoneID)
			if err != nil {
				return err
			}
			clis[instance.ZoneID] = cli
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance)
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range instanceIds {
		wg.Add(1)
		go func(zone string, ids []*backend.Instance) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			for _, id := range ids {
				removeGroups := []string{}
				for _, f := range fw {
					if !slices.Contains(removeGroups, f.FirewallID) {
						removeGroups = append(removeGroups, f.FirewallID)
					}
				}
				allGroups := []string{}
				for _, n := range id.Firewalls {
					if !slices.Contains(removeGroups, n) {
						allGroups = append(allGroups, n)
					}
				}
				_, err := clis[zone].ModifyInstanceAttribute(context.TODO(), &ec2.ModifyInstanceAttributeInput{
					InstanceId: aws.String(id.InstanceID),
					Groups:     allGroups,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}
