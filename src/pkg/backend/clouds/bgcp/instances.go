package bgcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
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

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/aerospike/aerolab/pkg/utils/structtags"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"golang.org/x/crypto/ssh"
	"golang.org/x/exp/maps"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

type CreateInstanceParams struct {
	// the image to use for the instances(nodes)
	Image *backends.Image `yaml:"image" json:"image"`
	// specify the zone for placement, e.g. us-central1-a
	NetworkPlacement string `yaml:"networkPlacement" json:"networkPlacement" required:"true"`
	// instance type
	InstanceType string `yaml:"instanceType" json:"instanceType" required:"true"`
	// volume types and sizes, backend-specific definitions
	//
	// gcp format:
	//   type={pd-*,hyperdisk-*,local-ssd}[,size={GB}][,iops={cnt}][,throughput={mb/s}][,count=5]
	//   example: type=pd-ssd,size=20 type=hyperdisk-balanced,size=20,iops=3060,throughput=155,count=2
	//
	// first specified volume is the root volume, all subsequent volumes are additional attached volumes
	Disks []string `yaml:"disks" json:"disks" required:"true"`
	// optional: names of firewalls to assign to the instances(nodes)
	//
	// will always create a project-wide firewall and assign it to the instances(nodes); this firewall allows communication between the instances(nodes) and port 22/tcp from the outside
	Firewalls []string `yaml:"firewalls" json:"firewalls"`
	// optional: if true, the instances(nodes) will be created as spot instances
	SpotInstance bool `yaml:"spotInstance" json:"spotInstance"`
	// optional: the IAM instance profile to use for the instance(node)
	IAMInstanceProfile string `yaml:"iamInstanceProfile" json:"iamInstanceProfile"`
	// optional: the custom DNS to use for the instance(node); if not set, will not create a custom DNS
	CustomDNS *backends.InstanceDNS `yaml:"customDNS" json:"customDNS"`
	// optional: the minimum CPU platform to use for the instance(node); if not set, will not create a minimum CPU platform
	MinCpuPlatform string `yaml:"minCpuPlatform" json:"minCpuPlatform"`
	// optional: if specified, and Image==nil, will lookup this image ID and use it (for custom images)
	// format: projects/<project>/global/images/<image>
	CustomImageID string `yaml:"customImageID" json:"customImageID"`
}

type InstanceDetail struct {
	FirewallTags     []string         `yaml:"firewallTags" json:"firewallTags"`
	TagFingerprint   string           `yaml:"tagFingerprint" json:"tagFingerprint"`
	LabelFingerprint string           `yaml:"labelFingerprint" json:"labelFingerprint"`
	Volumes          []instanceVolume `yaml:"volumes" json:"volumes"`
	FirewallIDs      []string         `yaml:"firewallIDs" json:"firewallIDs"`
	NetworkID        string           `yaml:"networkID" json:"networkID"`
	SubnetID         string           `yaml:"subnetID" json:"subnetID"`
}

type instanceVolume struct {
	Device   string `yaml:"device" json:"device"`
	VolumeID string `yaml:"volumeID" json:"volumeID"`
}

func getArchitecture(instance *computepb.Instance) backends.Architecture {
	// Check machine type for T2A (ARM) instances
	if strings.Contains(instance.GetMachineType(), "t2a-") {
		return backends.ArchitectureARM64
	}

	// Check CPU platform
	if strings.Contains(strings.ToLower(instance.GetCpuPlatform()), "arm") {
		return backends.ArchitectureARM64
	}

	// Default to x86_64
	return backends.ArchitectureX8664
}

func (s *b) getInstanceDetails(log *logger.Logger, inst *computepb.Instance, volumes backends.VolumeList, networkList backends.NetworkList) *backends.Instance {
	tags, err := decodeFromLabels(inst.Labels)
	if err != nil {
		log.Detail("Error decoding labels: %s", err)
		return nil
	}
	expires, _ := time.Parse(time.RFC3339, tags[TAG_AEROLAB_EXPIRES])
	state := backends.LifeCycleStateUnknown
	instanceStatus := inst.GetStatus()
	switch instanceStatus {
	case "PROVISIONING":
		state = backends.LifeCycleStateCreating
	case "RUNNING":
		state = backends.LifeCycleStateRunning
	case "DELETING":
		state = backends.LifeCycleStateTerminating
	case "STOPPING":
		state = backends.LifeCycleStateStopping
	case "TERMINATED":
		state = backends.LifeCycleStateStopped
	}
	firewalls := inst.GetTags().Items
	spot := inst.GetScheduling().GetProvisioningModel() == "SPOT"
	startTime := time.Time{}
	if tags[TAG_START_TIME] != "" {
		startTime, _ = time.Parse(time.RFC3339, tags[TAG_START_TIME])
	}
	costSoFar, _ := strconv.ParseFloat(tags[TAG_COST_SO_FAR], 64)
	pph, _ := strconv.ParseFloat(tags[TAG_COST_PPH], 64)
	volIDs := []string{}
	for _, v := range inst.GetDisks() {
		volIDs = append(volIDs, getValueFromURL(v.GetSource()))
	}
	vols := volumes.WithName(volIDs...).Describe()
	dvols := backends.CostVolumes{}
	avols := backends.CostVolumes{}
	for _, vol := range vols {
		if vol.DeleteOnTermination {
			dvols = append(dvols, vol.EstimatedCostUSD)
		} else {
			avols = append(avols, vol.EstimatedCostUSD)
		}
	}
	volslist := []instanceVolume{}
	for _, v := range inst.GetDisks() {
		volslist = append(volslist, instanceVolume{
			Device:   v.GetDeviceName(),
			VolumeID: v.GetSource(),
		})
	}
	arch := getArchitecture(inst)
	net := &backends.Network{}
	sub := &backends.Subnet{}
	nets := networkList.WithNetID(inst.GetNetworkInterfaces()[0].GetNetwork())
	if nets.Count() > 0 {
		nnet := nets.Describe()[0]
		net = nnet
		ssub := nnet.Subnets.WithSubnetId(inst.GetNetworkInterfaces()[0].GetSubnetwork())
		if len(ssub) > 0 {
			sub = ssub[0]
		}
	}
	var customDns *backends.InstanceDNS
	if tags[TAG_DNS_DOMAIN_NAME] != "" {
		customDns = &backends.InstanceDNS{
			Name:       tags[TAG_DNS_NAME],
			DomainID:   tags[TAG_DNS_DOMAIN_ID],
			DomainName: tags[TAG_DNS_DOMAIN_NAME],
			Region:     tags[TAG_DNS_REGION],
		}
		if customDns.Name == "" {
			customDns.Name = fmt.Sprintf("i-%x", sha256.Sum224([]byte(inst.GetName())))
		}
	}
	creationTime, _ := time.Parse(time.RFC3339, inst.GetCreationTimestamp())
	return &backends.Instance{
		ClusterName:  tags[TAG_CLUSTER_NAME],
		ClusterUUID:  tags[TAG_CLUSTER_UUID],
		NodeNo:       toInt(tags[TAG_NODE_NO]),
		InstanceID:   inst.GetName(),
		BackendType:  backends.BackendTypeGCP,
		InstanceType: string(inst.GetMachineType()),
		Name:         tags[TAG_NAME],
		Description:  tags[TAG_AEROLAB_DESCRIPTION],
		ZoneName:     getValueFromURL(inst.GetZone()),
		ZoneID:       inst.GetZone(),
		CreationTime: creationTime,
		Owner:        tags[TAG_AEROLAB_OWNER],
		Tags:         tags,
		Expires:      expires,
		IP: backends.IP{
			Public:  inst.GetNetworkInterfaces()[0].GetAccessConfigs()[0].GetNatIP(),
			Private: inst.GetNetworkInterfaces()[0].GetNetworkIP(),
		},
		ImageID:      inst.GetDisks()[0].GetSource(),
		SubnetID:     inst.GetNetworkInterfaces()[0].GetSubnetwork(),
		NetworkID:    inst.GetNetworkInterfaces()[0].GetNetwork(),
		Architecture: arch,
		OperatingSystem: backends.OS{
			Name:    tags[TAG_OS_NAME],
			Version: tags[TAG_OS_VERSION],
		},
		Firewalls:       firewalls,
		InstanceState:   state,
		SpotInstance:    spot,
		AttachedVolumes: vols,
		EstimatedCostUSD: backends.Cost{
			Instance: backends.CostInstance{
				RunningPricePerHour: pph,
				CostUntilLastStop:   costSoFar,
				LastStartTime:       startTime,
			},
			DeployedVolumes: dvols,
			AttachedVolumes: avols,
		},
		CustomDNS: customDns,
		BackendSpecific: &InstanceDetail{
			Volumes:          volslist,
			FirewallIDs:      firewalls,
			NetworkID:        net.NetworkId,
			SubnetID:         sub.SubnetId,
			LabelFingerprint: inst.GetLabelFingerprint(),
			FirewallTags:     inst.GetTags().GetItems(),
			TagFingerprint:   inst.GetTags().GetFingerprint(),
		},
	}
}

func (s *b) GetInstances(volumes backends.VolumeList, networkList backends.NetworkList, firewallList backends.FirewallList) (backends.InstanceList, error) {
	log := s.log.WithPrefix("GetInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	enabledRegions, err := s.ListEnabledZones()
	if err != nil {
		return nil, err
	}

	var i backends.InstanceList
	it := client.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project: s.credentials.Project,
		Filter:  proto.String(LABEL_FILTER_AEROLAB),
	})
	for {
		inst, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances := inst.Value.Instances
		for _, instance := range instances {
			iData := s.getInstanceDetails(log, instance, volumes, networkList)
			if iData == nil {
				continue
			}
			if !s.listAllProjects {
				if iData.Tags[TAG_AEROLAB_PROJECT] != s.project {
					continue
				}
			}
			if !slices.Contains(enabledRegions, zoneToRegion(iData.ZoneName)) {
				continue
			}
			i = append(i, iData)
		}
	}
	return i, nil
}

func (s *b) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	log := s.log.WithPrefix("InstancesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	log.Detail("Adding tags to instances")
	for _, instance := range instances {
		newTags := make(map[string]string)
		for k, v := range instance.Tags {
			newTags[k] = v
		}
		for k, v := range tags {
			newTags[k] = v
		}
		labels := encodeToLabels(newTags)
		labels["usedby"] = "aerolab"
		op, err := client.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
			InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
				LabelFingerprint: proto.String(instance.BackendSpecific.(*InstanceDetail).LabelFingerprint),
				Labels:           labels,
			},
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	log.Detail("Waiting for operations to complete")
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	log := s.log.WithPrefix("InstancesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	log.Detail("Removing tags from instances")
	for _, instance := range instances {
		newTags := make(map[string]string)
		for k, v := range instance.Tags {
			if slices.Contains(tagKeys, k) {
				continue
			}
			newTags[k] = v
		}
		labels := encodeToLabels(newTags)
		labels["usedby"] = "aerolab"
		op, err := client.SetLabels(ctx, &computepb.SetLabelsInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
			InstancesSetLabelsRequestResource: &computepb.InstancesSetLabelsRequest{
				LabelFingerprint: proto.String(instance.BackendSpecific.(*InstanceDetail).LabelFingerprint),
				Labels:           labels,
			},
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	log.Detail("Waiting for operations to complete")
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesTerminate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}

	removeSSHKey := false
	if s.instances.WithBackendType(backends.BackendTypeGCP).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Count() == instances.Count() {
		removeSSHKey = true
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		dnsPerDomainID := make(map[string][]*backends.InstanceDNS)
		for _, dns := range instances {
			if dns.CustomDNS == nil {
				continue
			}
			if _, ok := dnsPerDomainID[dns.CustomDNS.DomainID]; !ok {
				dnsPerDomainID[dns.CustomDNS.DomainID] = []*backends.InstanceDNS{}
			}
			dnsPerDomainID[dns.CustomDNS.DomainID] = append(dnsPerDomainID[dns.CustomDNS.DomainID], dns.CustomDNS)
			// DomainID - DNS Zone Name - ex aerospikeme
			// DomainName - The parent domain name - ex aerospike.me
			// Name - The tailing name of the record - ex aerolab-test-project-1-mydc-1
			// Region - unused, DNS is global
			// GetFQDN() - Full DNS Name of record - i.Name "." i.DomainName
		}
		if len(dnsPerDomainID) == 0 {
			return
		}
		log.Detail("Cleaning up DNS records")
		client, err := dns.NewService(ctx, option.WithHTTPClient(cli))
		if err != nil {
			log.Warn("Failed to get dns client, DNS will not be cleaned up: %s", err)
			return
		}
		for domainID, dnsList := range dnsPerDomainID {
			for _, dnsItem := range dnsList {
				_, err = client.ResourceRecordSets.Delete(s.credentials.Project, domainID, strings.TrimSuffix(dnsItem.GetFQDN(), ".")+".", "A").Do()
				if err != nil {
					log.Warn("Failed to delete DNS records for domain %s, DNS will not be cleaned up: %s", domainID, err)
				}
			}
		}
	}()

	ops := []*compute.Operation{}
	for _, instance := range instances {
		op, err := client.Delete(ctx, &computepb.DeleteInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}

	if waitDur > 0 {
		log.Detail("Waiting for operations to complete")
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
		}
	}

	// if no more instances exist for this project, delete the ssh key from amazon and locally from filepath.Join(s.sshKeysDir, s.project)
	if removeSSHKey && s.createInstanceCount.Get() == 0 {
		log.Detail("Remove SSH keys as no more instances exist for this project")
		os.Remove(filepath.Join(s.sshKeysDir, s.project))
		os.Remove(filepath.Join(s.sshKeysDir, s.project+".pub"))
		log.Detail("SSH keys removed")
	}

	wg.Wait()
	return nil
}

func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStop: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	ops := []*compute.Operation{}
	for _, instance := range instances {
		op, err := client.Stop(ctx, &computepb.StopInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
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
			err := s.InstancesAddTags(backends.InstanceList{instance}, map[string]string{
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

	if waitDur > 0 {
		log.Detail("Waiting for operations to complete")
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
		}
	}
	retWait.Wait()
	return reterr
}

func (s *b) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStart: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	ops := []*compute.Operation{}
	for _, instance := range instances {
		op, err := client.Start(ctx, &computepb.StartInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
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
			err := s.InstancesAddTags(backends.InstanceList{instance}, map[string]string{
				TAG_START_TIME: time.Now().Format(time.RFC3339),
			})
			if err != nil {
				retLock.Lock()
				reterr = errors.Join(reterr, err)
				retLock.Unlock()
			}
		}
	}()

	if waitDur > 0 {
		log.Detail("Waiting for operations to complete")
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
		}
	}

	retWait.Wait()
	return reterr
}

func (s *b) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	log := s.log.WithPrefix("InstancesExec: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	if e.ParallelThreads == 0 {
		e.ParallelThreads = len(instances)
	}
	out := []*backends.ExecOutput{}
	outl := new(sync.Mutex)
	parallelize.ForEachLimit(instances, e.ParallelThreads, func(i *backends.Instance) {
		if i.InstanceState != backends.LifeCycleStateRunning {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: errors.New("instance not running"),
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, s.project))
		if err != nil {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
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
		session, conn, err := sshexec.ExecPrepare(execInput)
		if err != nil {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: err,
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		isInterrupted := false
		shutdown.AddEarlyCleanupJob("ssh-exec-"+i.InstanceID, func(isSignal bool) {
			if isSignal {
				isInterrupted = true
				session.Close()
				conn.Close()
			}
		})
		o := sshexec.ExecRun(session, conn, execInput)
		if isInterrupted {
			o.Err = errors.New("interrupted")
		}
		outl.Lock()
		out = append(out, &backends.ExecOutput{
			Output:   o,
			Instance: i,
		})
		outl.Unlock()
	})
	return out
}

func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	log := s.log.WithPrefix("InstancesGetSSHKeyPath: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	out := []string{}
	for range instances {
		out = append(out, path.Join(s.sshKeysDir, s.project))
	}
	return out
}

func (s *b) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	log := s.log.WithPrefix("InstancesGetSftpConfig: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	confs := []*sshexec.ClientConf{}
	for _, i := range instances {
		if i.InstanceState != backends.LifeCycleStateRunning {
			return nil, errors.New("instance not running")
		}
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, s.project))
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

func (s *b) InstancesAssignFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	log := s.log.WithPrefix("InstancesAssignFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	for _, instance := range instances {
		newTags := instance.BackendSpecific.(*InstanceDetail).FirewallTags
		for _, f := range fw {
			if !slices.Contains(newTags, f.Name) {
				newTags = append(newTags, f.Name)
			}
		}
		_, err := client.SetTags(ctx, &computepb.SetTagsInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
			TagsResource: &computepb.Tags{
				Items:       newTags,
				Fingerprint: proto.String(instance.BackendSpecific.(*InstanceDetail).TagFingerprint),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) InstancesRemoveFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	log := s.log.WithPrefix("InstancesRemoveFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	fwRemoveList := []string{}
	for _, f := range fw {
		fwRemoveList = append(fwRemoveList, f.Name)
	}
	for _, instance := range instances {
		newTags := []string{}
		for _, f := range instance.BackendSpecific.(*InstanceDetail).FirewallTags {
			if !slices.Contains(fwRemoveList, f) {
				newTags = append(newTags, f)
			}
		}
		_, err := client.SetTags(ctx, &computepb.SetTagsInstanceRequest{
			Instance: instance.InstanceID,
			Project:  s.credentials.Project,
			Zone:     instance.ZoneName,
			TagsResource: &computepb.Tags{
				Items:       newTags,
				Fingerprint: proto.String(instance.BackendSpecific.(*InstanceDetail).TagFingerprint),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	// resolve backend-specific parameters
	backendSpecificParams := &CreateInstanceParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeGCP]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeGCP].(type) {
			case *CreateInstanceParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeGCP].(*CreateInstanceParams)
			case CreateInstanceParams:
				item := input.BackendSpecificParams[backends.BackendTypeGCP].(CreateInstanceParams)
				backendSpecificParams = &item
			default:
				return 0, 0, fmt.Errorf("invalid backend-specific parameters for gcp")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return 0, 0, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	instanceType, err := s.GetInstanceType(backendSpecificParams.NetworkPlacement, backendSpecificParams.InstanceType)
	if err != nil {
		return 0, 0, err
	}
	if backendSpecificParams.SpotInstance {
		costPPH = instanceType.PricePerHour.Spot
	} else {
		costPPH = instanceType.PricePerHour.OnDemand
	}
	for _, diskDef := range backendSpecificParams.Disks {
		parts := strings.Split(diskDef, ",")
		addCostGB := float64(0)
		count := int64(1)
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				return 0, 0, fmt.Errorf("invalid disk definition %s - each part must be key=value", diskDef)
			}
			switch strings.ToLower(kv[0]) {
			case "type":
				diskType := kv[1]
				volumePrice, err := s.GetVolumePrice(backendSpecificParams.NetworkPlacement, diskType)
				if err != nil {
					return 0, 0, err
				}
				addCostGB += volumePrice.PricePerGBHour
			case "count":
				count, err = strconv.ParseInt(kv[1], 10, 64)
				if err != nil {
					return 0, 0, fmt.Errorf("invalid disk definition %s - count must be a number", diskDef)
				}
			}
		}
		costGB += (addCostGB * float64(count))
	}
	return costPPH * float64(input.Nodes), costGB * float64(input.Nodes), nil
}

// resolve network placement based on placement string
func (s *b) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	if strings.Count(placement, "-") == 2 {
		parts := strings.Split(placement, "-")
		placement = parts[0] + "-" + parts[1]
	}
	for _, n := range s.networks {
		if !n.IsDefault {
			continue
		}
		for _, s := range n.Subnets {
			if s.ZoneName == placement || s.ZoneID == placement {
				vpc = n
				subnet = s
				zone = s.ZoneID
				break
			}
		}
		if subnet != nil {
			break
		}
	}
	if subnet == nil {
		return nil, nil, "", fmt.Errorf("no default subnet found in zone %s", placement)
	}
	return vpc, subnet, zone, nil
}

func getDeviceMappings(disks []string, nodeVolumeTagsEncoded map[string]string, zone string, backendSpecificParams *CreateInstanceParams) ([]*computepb.AttachedDisk, error) {
	// parse disks into ec2.CreateInstancesInput so we know the definitions are fine and have a block device mapping done
	disksList := []*computepb.AttachedDisk{}
	lastDisk := 'a' - 1
	nextLetter := 'a'
	nI := 0
	for _, diskDef := range disks {
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
			iops = proto.Int32(int32(i))
		}
		if diskThroughput != "" {
			t, err := strconv.ParseInt(diskThroughput, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid disk definition %s - throughput must be a number", diskDef)
			}
			throughput = proto.Int32(int32(t))
		}

		for i := int64(0); i < count; i++ {
			boot := false
			var simage *string
			if nI == 0 {
				if !strings.HasPrefix(diskType, "pd-") && !strings.HasPrefix(diskType, "hyperdisk-") {
					return nil, fmt.Errorf("first (root volume) disk must be of type pd-* or hyperdisk-*")
				}
				simage = proto.String(backendSpecificParams.Image.ImageId)
				boot = true
				nI++
			}
			deviceName := fmt.Sprintf("xvd%c", nextLetter)
			if lastDisk != 'a'-1 {
				deviceName = fmt.Sprintf("xvd%c%c", lastDisk, nextLetter)
			}
			nextLetter++
			if nextLetter > 'z' {
				nextLetter = 'a'
				lastDisk++
			}

			diskTypeFull := fmt.Sprintf("zones/%s/diskTypes/%s", zone, diskType)
			attachmentType := proto.String(computepb.AttachedDisk_SCRATCH.String())
			var devIface *string
			var piops *int64
			var pput *int64
			if strings.HasPrefix(diskType, "pd-") || strings.HasPrefix(diskType, "hyperdisk-") {
				devIface = nil
				attachmentType = proto.String(computepb.AttachedDisk_PERSISTENT.String())
				if diskThroughput != "" {
					put := int64(*throughput)
					pput = &put
				}
				if diskIops != "" {
					iops := int64(*iops)
					piops = &iops
				}
			} else {
				devIface = proto.String(computepb.AttachedDisk_NVME.String())
			}
			disksList = append(disksList, &computepb.AttachedDisk{
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:            &size,
					SourceImage:           simage,
					DiskType:              proto.String(diskTypeFull),
					ProvisionedIops:       piops,
					ProvisionedThroughput: pput,
					Labels:                nodeVolumeTagsEncoded,
				},
				AutoDelete: proto.Bool(true),
				Boot:       proto.Bool(boot),
				Type:       attachmentType,
				Interface:  devIface,
				DeviceName: proto.String(deviceName),
			})
		}
	}
	return disksList, nil
}

func (s *b) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (output *backends.CreateInstanceOutput, err error) {
	// resolve network placement using s.networks, so we have VPC, Subnet and Zone from it, user provided either vpc- or subnet- or zone name
	log := s.log.WithPrefix("CreateInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	s.createInstanceCount.Inc()
	defer s.createInstanceCount.Dec()

	// resolve backend-specific parameters
	backendSpecificParams := &CreateInstanceParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeGCP]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeGCP].(type) {
			case *CreateInstanceParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeGCP].(*CreateInstanceParams)
			case CreateInstanceParams:
				item := input.BackendSpecificParams[backends.BackendTypeGCP].(CreateInstanceParams)
				backendSpecificParams = &item
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for gcp")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	// early check - DNS
	if backendSpecificParams.CustomDNS != nil && backendSpecificParams.CustomDNS.Name != "" && input.Nodes > 1 {
		return nil, fmt.Errorf("DNS name %s is set, but nodes > 1, this is not allowed as GCP Domains does not support creating CNAME records for multiple nodes", backendSpecificParams.CustomDNS.Name)
	}

	vpc, subnet, az, err := s.ResolveNetworkPlacement(backendSpecificParams.NetworkPlacement)
	if err != nil {
		return nil, err
	}
	zone := backendSpecificParams.NetworkPlacement

	log.Detail("Selected network placement: zone=%s az=%s vpc=%s subnet=%s", zone, az, vpc.NetworkId, subnet.SubnetId)

	// custom image lookup
	if backendSpecificParams.Image == nil && backendSpecificParams.CustomImageID != "" {
		backendSpecificParams.Image = &backends.Image{
			ImageId:     backendSpecificParams.CustomImageID,
			Username:    "root",
			OSName:      "custom",
			OSVersion:   "custom",
			BackendType: backends.BackendTypeGCP,
			ZoneName:    zone,
			ZoneID:      zone,
			Public:      true,
			InAccount:   false,
		}
	}

	if backendSpecificParams.Image == nil {
		return nil, fmt.Errorf("image not found")
	}

	// if cluster with given ClusterName already exists in s.instances, find last node number, so we know where to count up for the instances we will be creating
	lastNodeNo := 0
	clusterUUID := uuid.New().String()
	for _, instance := range s.instances.WithNotState(backends.LifeCycleStateTerminated).WithClusterName(input.ClusterName).Describe() {
		clusterUUID = instance.ClusterUUID
		if instance.NodeNo > lastNodeNo {
			lastNodeNo = instance.NodeNo
		}
	}
	log.Detail("Current last node number in cluster %s: %d", input.ClusterName, lastNodeNo)

	// resolve firewalls from s.firewalls so we know they are in the right VPC
	securityGroupIds := []string{}
	for _, fwNameOrId := range backendSpecificParams.Firewalls {
		found := false
		for _, fw := range s.firewalls {
			if fw.Name == fwNameOrId {
				if fw.Network.NetworkId != vpc.NetworkId {
					return nil, fmt.Errorf("firewall %s exists but in different VPC than selected subnet", fwNameOrId)
				}
				securityGroupIds = append(securityGroupIds, fw.Name)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("firewall %s not found", fwNameOrId)
		}
	}
	log.Detail("Found security groups: %v", securityGroupIds)

	// default project-VPC firewall if it does not exist
	err = func() error {
		defaultFwName := sanitize(TAG_FIREWALL_NAME_PREFIX+vpc.Name, false)
		defaultFwInternalName := sanitize(TAG_FIREWALL_NAME_PREFIX_INTERNAL+vpc.Name, false)
		s.defaultFWCreateLock.Lock()
		defer s.defaultFWCreateLock.Unlock()
		getFirewalls := false
		defer func() {
			if getFirewalls {
				_, err := s.GetFirewalls(s.networks)
				if err != nil {
					log.Error("Failed to refresh firewalls after creation: %v", err)
				}
			}
		}()
		if s.firewalls.WithName(defaultFwName).Count() == 0 {
			getFirewalls = true
			fw, err := s.CreateFirewall(&backends.CreateFirewallInput{
				BackendType: backends.BackendTypeGCP,
				Name:        defaultFwName,
				Description: "AeroLab default project-VPC firewall",
				Ports: []*backends.Port{
					{
						FromPort:   22,
						ToPort:     22,
						SourceCidr: "0.0.0.0/0",
						SourceId:   "",
						Protocol:   backends.ProtocolTCP,
					},
				},
				Network: vpc,
			}, waitDur)
			if err != nil {
				if strings.Contains(err.Error(), "already exists") {
					// retrieve the existing firewall
					_, err := s.GetFirewalls(s.networks)
					if err != nil {
						return err
					}
					defaultFw := s.firewalls.WithName(defaultFwName).Describe()[0]
					securityGroupIds = append(securityGroupIds, defaultFw.Name)
				} else {
					return err
				}
			} else {
				securityGroupIds = append(securityGroupIds, fw.Firewall.Name)
			}
		} else {
			defaultFw := s.firewalls.WithName(defaultFwName).Describe()[0]
			securityGroupIds = append(securityGroupIds, defaultFw.Name)
		}
		if s.firewalls.WithName(defaultFwInternalName).Count() == 0 {
			getFirewalls = true
			fw, err := s.CreateFirewall(&backends.CreateFirewallInput{
				BackendType: backends.BackendTypeGCP,
				Name:        defaultFwInternalName,
				Description: "AeroLab default project-VPC internal firewall",
				Ports: []*backends.Port{
					{
						FromPort:   -1,
						ToPort:     -1,
						SourceCidr: "",
						SourceId:   "self",
						Protocol:   backends.ProtocolAll,
					},
				},
				Network: vpc,
			}, waitDur)
			if err != nil {
				if strings.Contains(err.Error(), "already exists") {
					// retrieve the existing firewall
					_, err := s.GetFirewalls(s.networks)
					if err != nil {
						return err
					}
					defaultFw := s.firewalls.WithName(defaultFwInternalName).Describe()[0]
					securityGroupIds = append(securityGroupIds, defaultFw.Name)
				} else {
					return err
				}
			} else {
				securityGroupIds = append(securityGroupIds, fw.Firewall.Name)
			}
		} else {
			defaultFw := s.firewalls.WithName(defaultFwInternalName).Describe()[0]
			securityGroupIds = append(securityGroupIds, defaultFw.Name)
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	// get prices
	costPPH, costGB, err := s.CreateInstancesGetPrice(input)
	if err != nil {
		return nil, err
	}

	// create gcp tags for ec2.CreateInstancesInput
	gcpTags := map[string]string{
		TAG_AEROLAB_OWNER:         input.Owner,
		TAG_CLUSTER_NAME:          input.ClusterName,
		TAG_AEROLAB_DESCRIPTION:   input.Description,
		TAG_AEROLAB_EXPIRES:       input.Expires.Format(time.RFC3339),
		TAG_AEROLAB_PROJECT:       s.project,
		TAG_AEROLAB_VERSION:       s.aerolabVersion,
		TAG_OS_NAME:               backendSpecificParams.Image.OSName,
		TAG_OS_VERSION:            backendSpecificParams.Image.OSVersion,
		TAG_COST_PPH:              fmt.Sprintf("%f", costPPH),
		TAG_COST_PER_GB:           fmt.Sprintf("%f", costGB),
		TAG_COST_SO_FAR:           "0",
		TAG_START_TIME:            time.Now().Format(time.RFC3339),
		TAG_CLUSTER_UUID:          clusterUUID,
		TAG_DELETE_ON_TERMINATION: strconv.FormatBool(true),
	}
	if backendSpecificParams.CustomDNS != nil {
		gcpTags[TAG_DNS_NAME] = backendSpecificParams.CustomDNS.Name
		gcpTags[TAG_DNS_REGION] = backendSpecificParams.CustomDNS.Region
		gcpTags[TAG_DNS_DOMAIN_ID] = backendSpecificParams.CustomDNS.DomainID
		gcpTags[TAG_DNS_DOMAIN_NAME] = backendSpecificParams.CustomDNS.DomainName
	}
	for k, v := range input.Tags {
		gcpTags[k] = v
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	// connect
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// resolve SSHKeyName
	sshKeyPath := filepath.Join(s.sshKeysDir, s.project)

	// if key does not exist in gcp, create it
	var publicKeyBytes []byte
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		log.Detail("SSH key %s does not exist, creating it", sshKeyPath)
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
	publicKeyBytes = bytes.Trim(publicKeyBytes, "\n\r\t ")

	// Create instances
	// userdata read from embedded file
	userData, err := scripts.ReadFile("scripts/userdata.sh")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded userdata: %v", err)
	}
	userDataString := fmt.Sprintf(string(userData), string(publicKeyBytes))

	gcpTags["usedby"] = "aerolab"

	onHostMaintenance := "MIGRATE"
	autoRestart := true
	provisioning := "STANDARD"
	if backendSpecificParams.SpotInstance {
		provisioning = "SPOT"
		autoRestart = false
		onHostMaintenance = "TERMINATE"
		gcpTags["isspot"] = "true"
	}
	var serviceAccounts []*computepb.ServiceAccount
	if input.TerminateOnStop || backendSpecificParams.IAMInstanceProfile != "" {
		serviceAccounts = []*computepb.ServiceAccount{
			{
				Scopes: []string{
					"https://www.googleapis.com/auth/compute",
				},
			},
		}
		if !strings.HasSuffix(backendSpecificParams.IAMInstanceProfile, "::nopricing") {
			serviceAccounts[0].Scopes = append(serviceAccounts[0].Scopes, "https://www.googleapis.com/auth/cloud-billing.readonly")
		}
	}

	newNames := []string{}
	log.Detail("Creating %d instances", input.Nodes)
	ops := []*compute.Operation{}
	for i := lastNodeNo; i < lastNodeNo+input.Nodes; i++ {
		// Add node number tag
		nodeTags := make(map[string]string)
		maps.Copy(nodeTags, gcpTags)
		nodeTags[TAG_NODE_NO] = fmt.Sprintf("%d", i+1)
		nodeVolumeTags := make(map[string]string)
		maps.Copy(nodeVolumeTags, gcpTags)
		nodeVolumeTags[TAG_NODE_NO] = fmt.Sprintf("%d", i+1)
		name := input.Name
		if name == "" {
			name = sanitize(fmt.Sprintf("%s-%s-%d", s.project, input.ClusterName, i+1), true)
		}
		newNames = append(newNames, name)
		nodeTags[TAG_NAME] = name
		nodeVolumeTags[TAG_NAME] = name
		// encode tags
		nodeTagsEncoded := encodeToLabels(nodeTags)
		nodeTagsEncoded["usedby"] = "aerolab"
		nodeVolumeTagsEncoded := encodeToLabels(nodeVolumeTags)
		nodeVolumeTagsEncoded["usedby"] = "aerolab"

		disksList, err := getDeviceMappings(backendSpecificParams.Disks, nodeVolumeTagsEncoded, zone, backendSpecificParams)
		if err != nil {
			return nil, err
		}
		var minCpuPlatform *string
		if backendSpecificParams.MinCpuPlatform != "" {
			minCpuPlatform = proto.String(backendSpecificParams.MinCpuPlatform)
		}
		// Create instance
		op, err := client.Insert(context.Background(), &computepb.InsertInstanceRequest{
			Project: s.credentials.Project,
			Zone:    zone,
			InstanceResource: &computepb.Instance{
				Labels: nodeTagsEncoded,
				Metadata: &computepb.Metadata{
					Items: []*computepb.Items{
						{
							Key:   proto.String("startup-script"),
							Value: proto.String(userDataString),
						},
					},
				},
				Name:            &name,
				MachineType:     proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", zone, backendSpecificParams.InstanceType)),
				MinCpuPlatform:  minCpuPlatform,
				ServiceAccounts: serviceAccounts,
				Tags:            &computepb.Tags{Items: securityGroupIds},
				Scheduling: &computepb.Scheduling{
					AutomaticRestart:  proto.Bool(autoRestart),
					OnHostMaintenance: proto.String(onHostMaintenance),
					ProvisioningModel: proto.String(provisioning),
				},
				Disks: disksList,
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						StackType: proto.String("IPV4_ONLY"),
						AccessConfigs: []*computepb.AccessConfig{
							{
								Name:        proto.String("External NAT"),
								NetworkTier: proto.String("PREMIUM"),
							},
						},
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %d: %v", i+1, err)
		}
		ops = append(ops, op)
	}

	// wait for all operations to be completed
	for _, op := range ops {
		err = op.Wait(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to wait for operation %s: %v", op.Name(), err)
		}
	}

	// get final instance details
	log.Detail("Getting final instance details")
	it := client.List(context.Background(), &computepb.ListInstancesRequest{
		Project: s.credentials.Project,
		Zone:    zone,
		Filter:  proto.String(LABEL_FILTER_AEROLAB),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %v", err)
	}
	runResults := []*computepb.Instance{}
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get new instances: %v", err)
		}
		if slices.Contains(newNames, pair.GetName()) {
			runResults = append(runResults, pair)
		}
	}

	// fill output
	output = &backends.CreateInstanceOutput{
		Instances: make(backends.InstanceList, len(runResults)),
	}
	for i, instance := range runResults {
		output.Instances[i] = s.getInstanceDetails(log, instance, s.volumes, s.networks)
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if backendSpecificParams.CustomDNS == nil {
			return
		}
		log.Detail("Creating DNS records start")
		defer log.Detail("Creating DNS records end")
		client, err := dns.NewService(ctx, option.WithHTTPClient(cli))
		if err != nil {
			log.Warn("Failed to get dns client, DNS will not be cleaned up: %s", err)
			return
		}
		// check if the zone is marked as usedby:aerolab, and mark it if not
		zone, err := client.ManagedZones.Get(s.credentials.Project, backendSpecificParams.CustomDNS.DomainID).Do()
		if err != nil {
			log.Warn("Failed to get zone %s: %s", backendSpecificParams.CustomDNS.DomainID, err)
			return
		}
		if !strings.Contains(zone.Description, "usedby:aerolab") {
			log.Warn("Zone %s is not marked as usedby:aerolab, marking it", backendSpecificParams.CustomDNS.DomainID)
			newDesc := zone.Description + " usedby:aerolab"
			if newDesc == " usedby:aerolab" {
				newDesc = "usedby:aerolab"
			}
			_, err = client.ManagedZones.Patch(s.credentials.Project, backendSpecificParams.CustomDNS.DomainID, &dns.ManagedZone{
				Description: newDesc,
			}).Do()
			if err != nil {
				log.Warn("Failed to mark zone %s as usedby:aerolab: %s", backendSpecificParams.CustomDNS.DomainID, err)
				return
			}
		}
		// create DNS records
		for _, instance := range output.Instances {
			if instance.CustomDNS != nil {
				_, err = client.ResourceRecordSets.Create(s.credentials.Project, instance.CustomDNS.DomainID, &dns.ResourceRecordSet{
					Kind:    "dns#resourceRecordSet",
					Name:    strings.TrimSuffix(instance.CustomDNS.GetFQDN(), ".") + ".",
					Rrdatas: []string{instance.IP.Routable()},
					Ttl:     int64(10),
					Type:    "A",
				}).Do()
				if err != nil {
					log.Warn("Failed to create DNS record for instance %s: %s", instance.InstanceID, err)
				}
			}
		}
	}()
	defer wg.Wait()

	// using ssh, wait for the instances to be ready
	log.Detail("Waiting for instances to be ssh-ready")
	for waitDur > 0 {
		now := time.Now()
		success := true
		out := output.Instances.Exec(&backends.ExecInput{
			Username:        backendSpecificParams.Image.Username,
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

// cleanup DNS
func (s *b) CleanupDNS() error {
	regions, err := s.ListEnabledZones()
	if err != nil {
		return fmt.Errorf("failed to list enabled zones: %s", err)
	}
	if len(regions) == 0 {
		s.log.Detail("GCP DNS CLEANUP: No regions enabled, skipping DNS cleanup")
		return nil
	}
	log := s.log.WithPrefix("DNS-CLEANUP: ")
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return fmt.Errorf("failed to get dns client, DNS will not be cleaned up: %s", err)
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := dns.NewService(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return fmt.Errorf("failed to get dns client, DNS will not be cleaned up: %s", err)
	}
	// only cleanup DNS records for zones with the correct description field (has usedby:aerolab)
	// only cleanup DNS records where the record starts with i-
	zones, err := client.ManagedZones.List(s.credentials.Project).Do()
	if err != nil {
		return fmt.Errorf("failed to get zones: %s", err)
	}
	inst := s.instances.WithNotState(backends.LifeCycleStateTerminated).Describe()
	for _, zone := range zones.ManagedZones {
		if strings.Contains(zone.Description, "usedby:aerolab") {
			resp, err := client.ResourceRecordSets.List(s.credentials.Project, zone.Name).Do()
			if err != nil {
				return fmt.Errorf("failed to get record sets: %s", err)
			}
			for _, record := range resp.Rrsets {
				if strings.HasPrefix(record.Name, "i-") {
					// only delete if the instance doesn't exist anymore
					found := false
					for _, i := range inst {
						if i.CustomDNS != nil && i.CustomDNS.GetFQDN() == strings.TrimSuffix(record.Name, ".") {
							found = true
							break
						}
					}
					if !found {
						_, err := client.ResourceRecordSets.Delete(s.credentials.Project, zone.Name, record.Name, record.Type).Do()
						if err != nil {
							return fmt.Errorf("failed to delete record set %s: %s", record.Name, err)
						}
					}
				}
			}
		}
	}
	return nil
}

func (s *b) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	// read update script template
	scriptBytes, err := scripts.ReadFile("scripts/update-hosts-file.sh")
	if err != nil {
		return fmt.Errorf("failed to read update-hosts-file.sh script: %v", err)
	}

	// format script with hosts entries
	script := fmt.Sprintf(string(scriptBytes), strings.Join(hostsEntries, "\n"))

	// upload script to the instances using ssh
	sshConfig, err := instances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get sftp config: %v", err)
	}
	var retErr error
	wait := new(sync.WaitGroup)
	sem := make(chan struct{}, parallelSSHThreads)

	for _, config := range sshConfig {
		config := config
		wait.Add(1)
		sem <- struct{}{}
		go func(config *sshexec.ClientConf) {
			defer wait.Done()
			defer func() { <-sem }()
			cli, err := sshexec.NewSftp(config)
			if err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("failed to create sftp client for host %s: %v", config.Host, err))
				return
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/tmp/update-hosts-file.sh",
				Source:      strings.NewReader(script),
				Permissions: 0755,
			})
			if err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("failed to write update-hosts-file.sh for host %s: %v", config.Host, err))
				return
			}
		}(config)
	}
	wait.Wait()
	if retErr != nil {
		return retErr
	}

	// execute script on all instances
	execInput := &backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:  []string{"bash", "/tmp/update-hosts-file.sh"},
			Terminal: true,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: parallelSSHThreads,
	}

	var errs error
	outputs := instances.Exec(execInput)
	for _, output := range outputs {
		if output.Output.Err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to update hosts file on instance %s: %v", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Err))
		}
	}
	return errs
}
