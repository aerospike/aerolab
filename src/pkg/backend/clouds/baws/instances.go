package baws

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/parallelize"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	rtypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	"golang.org/x/crypto/ssh"
)

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

func (s *b) getInstanceDetails(inst types.Instance, zone string, volumes backend.VolumeList, networkList backend.NetworkList, firewallList backend.FirewallList) *backend.Instance {
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
	var customDns *backend.InstanceDNS
	if tags[TAG_DNS_DOMAIN_NAME] != "" {
		customDns = &backend.InstanceDNS{
			Name:       tags[TAG_DNS_NAME],
			DomainID:   tags[TAG_DNS_DOMAIN_ID],
			DomainName: tags[TAG_DNS_DOMAIN_NAME],
			Region:     tags[TAG_DNS_REGION],
		}
		if customDns.Name == "" {
			customDns.Name = aws.ToString(inst.InstanceId)
		}
	}
	return &backend.Instance{
		ClusterName:  tags[TAG_CLUSTER_NAME],
		ClusterUUID:  tags[TAG_CLUSTER_UUID],
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
		CustomDNS: customDns,
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
	}
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
			listFilters := []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{TAG_AEROLAB_VERSION},
				},
			}
			if !s.listAllProjects {
				listFilters = append(listFilters, types.Filter{
					Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
					Values: []string{s.project},
				})
			}
			paginator := ec2.NewDescribeInstancesPaginator(cli, &ec2.DescribeInstancesInput{
				Filters: listFilters,
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, res := range out.Reservations {
					for _, inst := range res.Instances {
						ilock.Lock()
						i = append(i, s.getInstanceDetails(inst, zone, volumes, networkList, firewallList))
						ilock.Unlock()
					}
				}
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.instances = i
	}
	return i, errs
}

func (s *b) InstancesAddTags(instances backend.InstanceList, tags map[string]string) error {
	log := s.log.WithPrefix("InstancesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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

	removeSSHKey := false
	if s.instances.WithBackendType(backend.BackendTypeAWS).WithNotState(backend.LifeCycleStateTerminating, backend.LifeCycleStateTerminated).Count() == instances.Count() {
		removeSSHKey = true
	}
	keyNames := []string{}
	for _, instance := range instances {
		if !slices.Contains(keyNames, instance.SSHKeyName) {
			keyNames = append(keyNames, instance.SSHKeyName)
		}
	}

	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backend.CacheInvalidateVolume)
	instanceIds := make(map[string][]string)
	clis := make(map[string]*ec2.Client)
	zoneDNS := []*backend.InstanceDNS{}
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
		if instance.CustomDNS != nil {
			zoneDNS = append(zoneDNS, instance.CustomDNS)
		}
	}

	// cleanup dns records in the background
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if len(zoneDNS) == 0 {
			return
		}
		cli, err := getRoute53Client(s.credentials, &zoneDNS[0].Region)
		if err != nil {
			log.Warn("Failed to get route53 client, DNS will not be cleaned up: %s", err)
			return
		}
		for _, dns := range zoneDNS {
			log.Detail("zone=%s start DNS cleanup", dns.Region)
			defer log.Detail("zone=%s end DNS cleanup", dns.Region)
			_, err = cli.ChangeResourceRecordSets(context.TODO(), &route53.ChangeResourceRecordSetsInput{
				HostedZoneId: aws.String(dns.DomainID),
				ChangeBatch: &rtypes.ChangeBatch{
					Changes: []rtypes.Change{
						{
							Action: rtypes.ChangeActionDelete,
							ResourceRecordSet: &rtypes.ResourceRecordSet{
								Name: aws.String(dns.GetFQDN()),
							},
						},
					},
				},
			})
			if err != nil {
				log.Warn("Failed to delete DNS record %s, DNS will not be cleaned up: %s", dns.GetFQDN(), err)
			}
		}
	}()
	defer wg.Wait()

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

	// if no more instances exist for this project, delete the ssh key from amazon and locally from filepath.Join(s.sshKeysDir, s.project)
	if removeSSHKey {
		log.Detail("Remove SSH keys as no more instances exist for this project")
		for _, keyName := range keyNames {
			os.Remove(filepath.Join(s.sshKeysDir, keyName))
			os.Remove(filepath.Join(s.sshKeysDir, keyName+".pub"))
		}
		log.Detail("SSH keys removed")
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
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, i.SSHKeyName))
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
			ConnectTimeout: e.ConnectTimeout,
		}
		execInput := &sshexec.ExecInput{
			ClientConf: clientConf,
			ExecDetail: e.ExecDetail,
		}
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_CLUSTER_NAME",
			Value: i.ClusterName,
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_NODE_NO",
			Value: strconv.Itoa(i.NodeNo),
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_PROJECT_NAME",
			Value: s.project,
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_OWNER",
			Value: i.Owner,
		})
		o := sshexec.Exec(execInput)
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
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, i.SSHKeyName))
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
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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
	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
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

func (s *b) CreateInstancesGetPrice(input *backend.CreateInstanceInput) (costPPH, costGB float64, err error) {
	_, _, zone, err := s.resolveNetworkPlacement(input.NetworkPlacement)
	if err != nil {
		return 0, 0, err
	}
	instanceType, err := s.GetInstanceType(zone, input.InstanceType)
	if err != nil {
		return 0, 0, err
	}
	if input.SpotInstance {
		costPPH = instanceType.PricePerHour.Spot
	} else {
		costPPH = instanceType.PricePerHour.OnDemand
	}
	for _, diskDef := range input.Disks {
		parts := strings.Split(diskDef, ",")
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				return 0, 0, fmt.Errorf("invalid disk definition %s - each part must be key=value", diskDef)
			}
			switch strings.ToLower(kv[0]) {
			case "type":
				diskType := kv[1]
				volumePrice, err := s.GetVolumePrice(zone, diskType)
				if err != nil {
					return 0, 0, err
				}
				costGB += volumePrice.PricePerGBHour
			}
		}
	}
	return costPPH, costGB, nil
}

func (s *b) resolveNetworkPlacement(placement string) (vpc *backend.Network, subnet *backend.Subnet, zone string, err error) {
	switch {
	case strings.HasPrefix(placement, "vpc-"):
		for _, n := range s.networks {
			if n.NetworkId == placement {
				vpc = n
				if len(vpc.Subnets) > 0 {
					subnet = vpc.Subnets[0]
					zone = subnet.ZoneName
				}
				break
			}
		}
		if vpc == nil {
			return nil, nil, "", fmt.Errorf("vpc %s not found", placement)
		}
		if subnet == nil {
			return nil, nil, "", fmt.Errorf("no subnets found in vpc %s", placement)
		}

	case strings.HasPrefix(placement, "subnet-"):
		for _, n := range s.networks {
			for _, s := range n.Subnets {
				if s.SubnetId == placement {
					vpc = n
					subnet = s
					zone = subnet.ZoneName
					break
				}
			}
			if subnet != nil {
				break
			}
		}
		if subnet == nil {
			return nil, nil, "", fmt.Errorf("subnet %s not found", placement)
		}

	default:
		zone = placement
		for _, n := range s.networks {
			if !n.IsDefault {
				continue
			}
			for _, s := range n.Subnets {
				if s.ZoneName == zone {
					vpc = n
					subnet = s
					break
				}
			}
			if subnet != nil {
				break
			}
		}
		if subnet == nil {
			return nil, nil, "", fmt.Errorf("no default subnet found in zone %s", zone)
		}
	}
	return vpc, subnet, zone, nil
}

func (s *b) CreateInstances(input *backend.CreateInstanceInput, waitDur time.Duration) (output *backend.CreateInstanceOutput, err error) {
	// resolve network placement using s.networks, so we have VPC, Subnet and Zone from it, user provided either vpc- or subnet- or zone name
	log := s.log.WithPrefix("CreateInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// early check - DNS
	if input.CustomDNS != nil && input.CustomDNS.Name != "" && input.Nodes > 1 {
		return nil, fmt.Errorf("DNS name %s is set, but nodes > 1, this is not allowed as AWS Route53 does not support creating CNAME records for multiple nodes", input.CustomDNS.Name)
	}

	vpc, subnet, zone, err := s.resolveNetworkPlacement(input.NetworkPlacement)
	if err != nil {
		return nil, err
	}

	log.Detail("Selected network placement: zone=%s vpc=%s subnet=%s", zone, vpc.NetworkId, subnet.SubnetId)

	// if cluster with given ClusterName already exists in s.instances, find last node number, so we know where to count up for the instances we will be creating
	lastNodeNo := 0
	clusterUUID := uuid.New().String()
	for _, instance := range s.instances.WithClusterName(input.ClusterName).Describe() {
		clusterUUID = instance.ClusterUUID
		if instance.NodeNo > lastNodeNo {
			lastNodeNo = instance.NodeNo
		}
	}
	log.Detail("Current last node number in cluster %s: %d", input.ClusterName, lastNodeNo)

	// resolve firewalls from s.firewalls so we know they are in the right VPC
	firewallIds := make(map[string]string) // map of firewallID -> name
	securityGroupIds := []string{}
	for _, fwNameOrId := range input.Firewalls {
		isId := false
		if strings.HasPrefix(fwNameOrId, "sg-") {
			isId = true
		}
		found := false
		for _, fw := range s.firewalls {
			if (isId && fw.FirewallID == fwNameOrId) || (!isId && fw.Name == fwNameOrId) {
				if fw.Network.NetworkId != vpc.NetworkId {
					return nil, fmt.Errorf("firewall %s exists but in different VPC than selected subnet", fwNameOrId)
				}
				firewallIds[fw.FirewallID] = fw.Name
				securityGroupIds = append(securityGroupIds, fw.FirewallID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("firewall %s not found", fwNameOrId)
		}
	}
	log.Detail("Found security groups: %v", firewallIds)

	// parse disks into ec2.CreateInstancesInput so we know the definitions are fine and have a block device mapping done
	blockDeviceMappings := []types.BlockDeviceMapping{}
	lastDisk := 'a' - 1
	nextLetter := 'a'
	for _, diskDef := range input.Disks {
		parts := strings.Split(diskDef, ",")
		var diskType, diskSize, diskIops, diskThroughput, diskCount string
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid disk definition %s - each part must be key=value", diskDef)
			}
			switch strings.ToLower(kv[0]) {
			case "type":
				diskType = kv[1]
			case "size":
				diskSize = kv[1]
			case "iops":
				diskIops = kv[1]
			case "throughput":
				diskThroughput = kv[1]
			case "count":
				diskCount = kv[1]
			default:
				return nil, fmt.Errorf("invalid disk definition %s - unknown key %s", diskDef, kv[0])
			}
		}

		if diskType == "" {
			return nil, fmt.Errorf("invalid disk definition %s - type is required", diskDef)
		}
		if diskSize == "" {
			return nil, fmt.Errorf("invalid disk definition %s - size is required", diskDef)
		}

		size, err := strconv.ParseInt(diskSize, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid disk definition %s - size must be a number", diskDef)
		}

		count := int64(1)
		if diskCount != "" {
			count, err = strconv.ParseInt(diskCount, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid disk definition %s - count must be a number", diskDef)
			}
		}

		var iops, throughput *int32
		if diskIops != "" {
			i, err := strconv.ParseInt(diskIops, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid disk definition %s - iops must be a number", diskDef)
			}
			iops = aws.Int32(int32(i))
		}
		if diskThroughput != "" {
			t, err := strconv.ParseInt(diskThroughput, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid disk definition %s - throughput must be a number", diskDef)
			}
			throughput = aws.Int32(int32(t))
		}

		for i := int64(0); i < count; i++ {
			deviceName := fmt.Sprintf("/dev/xvd%c", nextLetter)
			if lastDisk != 'a'-1 {
				deviceName = fmt.Sprintf("/dev/xvd%c%c", lastDisk, nextLetter)
			}
			nextLetter++
			if nextLetter > 'z' {
				nextLetter = 'a'
				lastDisk++
			}

			blockDeviceMappings = append(blockDeviceMappings, types.BlockDeviceMapping{
				DeviceName: aws.String(deviceName),
				Ebs: &types.EbsBlockDevice{
					DeleteOnTermination: aws.Bool(true),
					VolumeSize:          aws.Int32(int32(size)),
					VolumeType:          types.VolumeType(diskType),
					Iops:                iops,
					Throughput:          throughput,
				},
			})
		}
	}

	log.Detail("Block device mappings: %v", blockDeviceMappings)

	// get prices
	var costPPH, costGB float64
	instanceType, err := s.GetInstanceType(zone, input.InstanceType)
	if err != nil {
		log.Warn("Failed to get instance price: %v", err)
	} else {
		if input.SpotInstance {
			costPPH = instanceType.PricePerHour.Spot
		} else {
			costPPH = instanceType.PricePerHour.OnDemand
		}
	}
	volumePrice, err := s.GetVolumePrice(zone, input.Disks[0])
	if err != nil {
		log.Warn("Failed to get volume price: %v", err)
	} else {
		costGB = volumePrice.PricePerGBHour
	}

	// create aws tags for ec2.CreateInstancesInput
	awsTags := []types.Tag{
		{
			Key:   aws.String(TAG_NAME),
			Value: aws.String(input.Name),
		},
		{
			Key:   aws.String(TAG_OWNER),
			Value: aws.String(input.Owner),
		},
		{
			Key:   aws.String(TAG_CLUSTER_NAME),
			Value: aws.String(input.ClusterName),
		},
		{
			Key:   aws.String(TAG_DESCRIPTION),
			Value: aws.String(input.Description),
		},
		{
			Key:   aws.String(TAG_EXPIRES),
			Value: aws.String(input.Expires.Format(time.RFC3339)),
		},
		{
			Key:   aws.String(TAG_AEROLAB_PROJECT),
			Value: aws.String(s.project),
		},
		{
			Key:   aws.String(TAG_AEROLAB_VERSION),
			Value: aws.String(s.aerolabVersion),
		},
		{
			Key:   aws.String(TAG_OS_NAME),
			Value: aws.String(input.Image.OSName),
		},
		{
			Key:   aws.String(TAG_OS_VERSION),
			Value: aws.String(input.Image.OSVersion),
		},
		{
			Key:   aws.String(TAG_COST_PPH),
			Value: aws.String(fmt.Sprintf("%f", costPPH)),
		},
		{
			Key:   aws.String(TAG_COST_GB),
			Value: aws.String(fmt.Sprintf("%f", costGB)),
		},
		{
			Key:   aws.String(TAG_COST_SO_FAR),
			Value: aws.String("0"),
		},
		{
			Key:   aws.String(TAG_START_TIME),
			Value: aws.String(time.Now().Format(time.RFC3339)),
		},
		{
			Key:   aws.String(TAG_CLUSTER_UUID),
			Value: aws.String(clusterUUID),
		},
	}
	if input.CustomDNS != nil {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(TAG_DNS_NAME),
			Value: aws.String(input.CustomDNS.Name),
		})
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(TAG_DNS_REGION),
			Value: aws.String(input.CustomDNS.Region),
		})
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(TAG_DNS_DOMAIN_ID),
			Value: aws.String(input.CustomDNS.DomainID),
		})
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(TAG_DNS_DOMAIN_NAME),
			Value: aws.String(input.CustomDNS.DomainName),
		})

	}
	for k, v := range input.Tags {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	defer s.invalidateCacheFunc(backend.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backend.CacheInvalidateVolume)
	// connect
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return nil, err
	}

	// resolve SSHKeyName
	sshKeyName := input.SSHKeyName
	if input.SSHKeyName == "" {
		sshKeyName = s.project
	}
	sshKeyPath := filepath.Join(s.sshKeysDir, sshKeyName)

	// if key does not exist in aws, create it
	var publicKeyBytes []byte
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		log.Detail("SSH key %s does not exist, creating it", sshKeyName)
		// generate new SSH key pair
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %v", err)
		}

		// encode public key
		publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create public key: %v", err)
		}
		publicKeyBytes = ssh.MarshalAuthorizedKey(publicKey)

		// save private key to file
		privateKeyBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})

		if _, err := os.Stat(s.sshKeysDir); os.IsNotExist(err) {
			err = os.MkdirAll(s.sshKeysDir, 0700)
			if err != nil {
				return nil, fmt.Errorf("failed to create ssh keys directory: %v", err)
			}
		}

		err = os.WriteFile(sshKeyPath, privateKeyBytes, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to save private key: %v", err)
		}

		err = os.WriteFile(sshKeyPath+".pub", publicKeyBytes, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to save public key: %v", err)
		}
	} else {
		publicKeyBytes, err = os.ReadFile(sshKeyPath + ".pub")
		if err != nil {
			return nil, fmt.Errorf("failed to read public key: %v", err)
		}
	}

	// Create instances
	runResults := []types.Instance{}
	marketType := types.MarketTypeCapacityBlock
	if input.SpotInstance {
		marketType = types.MarketTypeSpot
	}
	shutdownBehavior := types.ShutdownBehaviorStop
	if input.TerminateOnStop {
		shutdownBehavior = types.ShutdownBehaviorTerminate
	}
	var iam *types.IamInstanceProfileSpecification
	if input.IAMInstanceProfile != "" {
		if strings.HasPrefix(input.IAMInstanceProfile, "arn:aws:iam::") {
			iam = &types.IamInstanceProfileSpecification{
				Arn: aws.String(input.IAMInstanceProfile),
			}
		} else {
			iam = &types.IamInstanceProfileSpecification{
				Name: aws.String(input.IAMInstanceProfile),
			}
		}
	}

	// userdata read from embedded file
	userData, err := scripts.ReadFile("scripts/userdata.sh")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded userdata: %v", err)
	}

	log.Detail("Creating %d instances", input.Nodes)
	for i := lastNodeNo; i < lastNodeNo+input.Nodes; i++ {
		// Add node number tag
		nodeTags := make([]types.Tag, len(awsTags))
		copy(nodeTags, awsTags)
		nodeTags = append(nodeTags, types.Tag{
			Key:   aws.String(TAG_NODE_NO),
			Value: aws.String(fmt.Sprintf("%d", i+1)),
		})
		nodeVolumeTags := make([]types.Tag, len(awsTags))
		copy(nodeVolumeTags, awsTags)
		nodeVolumeTags = append(nodeVolumeTags, types.Tag{
			Key:   aws.String(TAG_NODE_NO),
			Value: aws.String(fmt.Sprintf("%d", i+1)),
		})

		// Create instance
		runResult, err := cli.RunInstances(context.Background(), &ec2.RunInstancesInput{
			ImageId:                           aws.String(input.Image.ImageId),
			InstanceType:                      types.InstanceType(input.InstanceType),
			MinCount:                          aws.Int32(1),
			MaxCount:                          aws.Int32(1),
			IamInstanceProfile:                iam,
			InstanceInitiatedShutdownBehavior: shutdownBehavior,
			InstanceMarketOptions: &types.InstanceMarketOptionsRequest{
				MarketType: marketType,
			},
			NetworkInterfaces: []types.InstanceNetworkInterfaceSpecification{
				{
					DeviceIndex:              aws.Int32(0),
					SubnetId:                 aws.String(subnet.SubnetId),
					Groups:                   securityGroupIds,
					AssociatePublicIpAddress: aws.Bool(!input.DisablePublicIP),
				},
			},
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeInstance,
					Tags:         nodeTags,
				},
				{
					ResourceType: types.ResourceTypeVolume,
					Tags:         nodeVolumeTags,
				},
			},
			BlockDeviceMappings: blockDeviceMappings,
			UserData:            aws.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(string(userData), string(publicKeyBytes))))),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %d: %v", i+1, err)
		}

		runResults = append(runResults, runResult.Instances[0])
	}

	// wait for instances to be running
	instanceIds := make([]string, len(runResults))
	for i, instance := range runResults {
		instanceIds[i] = *instance.InstanceId
	}

	durAdjust := time.Now()
	waiter := ec2.NewInstanceRunningWaiter(cli)
	log.Detail("Waiting for instances to be in running state")
	err = waiter.Wait(context.Background(), &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}, waitDur)
	if err != nil {
		return nil, fmt.Errorf("error waiting for instances to be running: %v", err)
	}
	waitDur -= time.Since(durAdjust)

	// get final instance details
	log.Detail("Getting final instance details")
	describeResult, err := cli.DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	})
	if err != nil {
		return nil, fmt.Errorf("error describing created instances: %v", err)
	}

	runResults = []types.Instance{}
	for _, reservation := range describeResult.Reservations {
		runResults = append(runResults, reservation.Instances...)
	}

	// fill output
	output = &backend.CreateInstanceOutput{
		Instances: make(backend.InstanceList, len(runResults)),
	}
	for i, instance := range runResults {
		output.Instances[i] = s.getInstanceDetails(instance, zone, s.volumes, s.networks, s.firewalls)
	}

	// handle DNS creation if required
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if input.CustomDNS == nil {
			return
		}
		log.Detail("Creating DNS records start")
		defer log.Detail("Creating DNS records end")
		cli, err := getRoute53Client(s.credentials, &input.CustomDNS.Region)
		if err != nil {
			log.Warn("Failed to get route53 client, DNS will not be created: %s", err)
			return
		}
		_, err = cli.ChangeTagsForResource(context.Background(), &route53.ChangeTagsForResourceInput{
			ResourceType: rtypes.TagResourceTypeHostedzone,
			ResourceId:   aws.String(input.CustomDNS.DomainID),
			AddTags: []rtypes.Tag{
				{Key: aws.String(TAG_AEROLAB_PROJECT), Value: aws.String(s.project)},
				{Key: aws.String(TAG_AEROLAB_VERSION), Value: aws.String(s.aerolabVersion)},
			},
		})
		if err != nil {
			log.Detail("WARNING: Failed to add tags to hosted zone, auto cleanup in expiry system will not work: %s", err)
		}
		var changes []rtypes.Change
		for _, instance := range output.Instances {
			if instance.CustomDNS != nil {
				changes = append(changes, rtypes.Change{
					Action: rtypes.ChangeActionCreate,
					ResourceRecordSet: &rtypes.ResourceRecordSet{
						Name: aws.String(instance.CustomDNS.GetFQDN()),
						Type: rtypes.RRTypeA,
						ResourceRecords: []rtypes.ResourceRecord{
							{Value: aws.String(instance.IP.Routable())},
						},
					},
				})
			}
		}
		change, err := cli.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(input.CustomDNS.DomainID),
			ChangeBatch: &rtypes.ChangeBatch{
				Changes: changes,
			},
		})
		if err != nil {
			log.Warn("Failed to create DNS records: %s", err)
		}
		if waitDur > 0 {
			waiter := route53.NewResourceRecordSetsChangedWaiter(cli)
			err = waiter.Wait(context.Background(), &route53.GetChangeInput{
				Id: change.ChangeInfo.Id,
			}, waitDur)
			if err != nil {
				log.Warn("Failed to wait for DNS records to be created: %s", err)
			}
		}
	}()
	defer wg.Wait()

	// using ssh, wait for the instances to be ready
	log.Detail("Waiting for instances to be ssh-ready")
	for waitDur > 0 {
		now := time.Now()
		success := true
		out := output.Instances.Exec(&backend.ExecInput{
			Username:        input.Image.Username,
			ParallelThreads: input.ParallelSSHThreads,
			ConnectTimeout:  5 * time.Second,
			ExecDetail: sshexec.ExecDetail{
				Command: []string{"ls", "/"},
			},
		})
		if len(out) != len(output.Instances) {
			success = false
		}
		for _, o := range out {
			if o.Output.Err != nil {
				success = false
				log.Detail("Waiting for instance %s to be ready: %s: %s", o.Instance.InstanceID, o.Output.Err, o.Output.Stdout)
			}
		}
		if success {
			break
		}
		waitDur -= time.Since(now)
		if waitDur > 0 {
			time.Sleep(1 * time.Second)
		}
	}

	if waitDur <= 0 {
		log.Detail("Instances failed to initialize ssh")
		return nil, fmt.Errorf("instances failed to initialize ssh")
	}

	// return
	return output, nil
}

func (s *b) CleanupDNS() error {
	// connect to route53
	cli, err := getRoute53Client(s.credentials, &s.project)
	if err != nil {
		return fmt.Errorf("failed to get route53 client: %v", err)
	}
	// list all hosted zones
	paginator := route53.NewListHostedZonesPaginator(cli, &route53.ListHostedZonesInput{})
	for paginator.HasMorePages() {
		zones, err := paginator.NextPage(context.Background())
		if err != nil {
			return fmt.Errorf("failed to list hosted zones: %v", err)
		}
		for _, zone := range zones.HostedZones {
			// get zone tags
			tags, err := cli.ListTagsForResource(context.Background(), &route53.ListTagsForResourceInput{
				ResourceType: rtypes.TagResourceTypeHostedzone,
				ResourceId:   zone.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to list tags for hosted zone: %v", err)
			}
			tagsMap := make(map[string]string)
			for _, tag := range tags.ResourceTagSet.Tags {
				tagsMap[*tag.Key] = *tag.Value
			}
			if tagProject, ok := tagsMap[TAG_AEROLAB_PROJECT]; (tagProject != s.project && !s.listAllProjects) || (s.listAllProjects && !ok) {
				continue
			}
			// for each hosted zone, list all resource record sets
			records, err := cli.ListResourceRecordSets(context.Background(), &route53.ListResourceRecordSetsInput{
				HostedZoneId: zone.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to list resource record sets: %v", err)
			}
			changes := []rtypes.Change{}
			for _, record := range records.ResourceRecordSets {
				if record.Type != rtypes.RRTypeA {
					continue
				}
				if record.Name == nil {
					continue
				}
				if strings.HasPrefix(*record.Name, "i-") {
					split := strings.Split(*record.Name, ".")
					if len(split) < 2 {
						continue
					}
					tail := strings.Join(split[1:], ".")
					if tail == "" {
						continue
					}
					if tail != *zone.Name {
						continue
					}
					instanceId := split[0]
					// if the instance does not exist, delete the record
					inst := s.instances.WithNotState(backend.LifeCycleStateTerminated).WithInstanceID(instanceId).Describe()
					if len(inst) == 0 {
						// delete the record
						changes = append(changes, rtypes.Change{
							Action: rtypes.ChangeActionDelete,
							ResourceRecordSet: &rtypes.ResourceRecordSet{
								Name: record.Name,
								Type: record.Type,
							},
						})
					}
				}
			}
			if len(changes) > 0 {
				_, err := cli.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
					HostedZoneId: zone.Id,
					ChangeBatch:  &rtypes.ChangeBatch{Changes: changes},
				})
				if err != nil {
					return fmt.Errorf("failed to change resource record sets: %v", err)
				}
			}
		}
	}

	return nil
}
