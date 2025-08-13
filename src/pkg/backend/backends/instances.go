package backends

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/sshexec"
)

type CreateInstanceInput struct {
	// a user-friendly name for the cluster of instances(nodes)
	ClusterName string `yaml:"clusterName" json:"clusterName" required:"true"`
	// number of instances(nodes) to create
	Nodes int `yaml:"nodes" json:"nodes" required:"true"`
	// backend type
	BackendType BackendType `yaml:"backendType" json:"backendType" required:"true"`
	// backend-specific parameters; use ex: bdocker.CreateInstanceParams, baws.CreateInstanceParams, bgcp.CreateInstanceParams, etc
	BackendSpecificParams map[BackendType]interface{} `yaml:"backendSpecificParams" json:"backendSpecificParams" required:"true"`
	// optional: the name of the ssh key to use for the instances(nodes); if not set, the default ssh key for the project will be used
	SSHKeyName string `yaml:"sshKeyName" json:"sshKeyName"`
	// optional: the name of the instance(node); if not set, the default name will be used (project-clusterName-nodeNo)
	Name string `yaml:"name" json:"name"`
	// optional: the owner of the instance(node); this will create an owner tag on the instance(node)
	Owner string `yaml:"owner" json:"owner"`
	// optional: extra tags to assign to the instance(node)
	Tags map[string]string `yaml:"tags" json:"tags"`
	// optional: the expiry date of the instance(node); if not set, will not expire
	Expires time.Time `yaml:"expires" json:"expires"`
	// optional: the description of the instance(node); if not set, will not create a description tag
	Description string `yaml:"description" json:"description"`
	// optional: if true, will terminate the instance(node) when it is stopped from the instance itself (poweroff or shutdown)
	TerminateOnStop bool `yaml:"terminateOnStop" json:"terminateOnStop"`
	// optional: the number of parallel SSH threads to use for the instance(node); if not set, will use the number of Nodes being created
	ParallelSSHThreads int `yaml:"parallelSSHThreads" json:"parallelSSHThreads"`
	// optional: just so it can be marshalled/printed, it is ignored by the backend
	ImageName string `yaml:"imageName" json:"imageName"`
}

type InstanceDNS struct {
	DomainID   string `yaml:"domainID" json:"domainID"`     // the ID of the domain, as defined for DomainID
	DomainName string `yaml:"domainName" json:"domainName"` // the name of the domain, as defined for DomainID
	Name       string `yaml:"name" json:"name"`             // the name to assign the instance, if not set, the instance ID will be used
	Region     string `yaml:"region" json:"region"`         // the region to use for the assignment
}

func (i *InstanceDNS) GetFQDN() string {
	return fmt.Sprintf("%s.%s", i.Name, i.DomainName)
}

type CreateInstanceOutput struct {
	Instances InstanceList `yaml:"instances" json:"instances"`
}

type Instances interface {
	// instance selector - by backend type
	WithBackendType(types ...BackendType) Instances
	// instance selector - by volume type
	WithType(types ...string) Instances
	// instance selector - by zone
	WithZoneName(zoneNames ...string) Instances
	// instance selector - by zone
	WithZoneID(zoneIDs ...string) Instances
	// instance selector - by name
	WithName(names ...string) Instances
	// instance selector - by owner
	WithOwner(owners ...string) Instances
	// filter by cluster name(s)
	WithClusterName(names ...string) Instances
	// filter by node number(s)
	WithNodeNo(number ...int) Instances
	// if a tag only has key, instances that contain the tag will be returned, if it has a value, also the value will be matched against
	WithTags(tags map[string]string) Instances
	// filter by expiry date
	WithExpired(expired bool) Instances
	// filter by state
	WithState(states ...LifeCycleState) Instances
	// filter by not state
	WithNotState(states ...LifeCycleState) Instances
	// filter by instance ID
	WithInstanceID(instanceIDs ...string) Instances
	// filter by architecture
	WithArchitecture(architecture Architecture) Instances
	// filter by OS name
	WithOSName(osName string) Instances
	// filter by OS version
	WithOSVersion(osVersion string) Instances
	// number of instances in selector
	Count() int
	// expose instance details to the caller
	Describe() InstanceList
	// you can also perform action on multiple instances
	InstanceAction
}

type InstanceAction interface {
	// add/override tags for instances
	AddTags(tags map[string]string) error
	// remove tag(s) from instances
	RemoveTags(tagKeys []string) error
	// delete selected instances
	Terminate(waitDur time.Duration) error
	// stop selected instances
	Stop(force bool, waitDur time.Duration) error
	// start selected instances
	Start(waitDur time.Duration) error
	// run command
	Exec(*ExecInput) []*ExecOutput
	// get sftp config
	GetSftpConfig(username string) ([]*sshexec.ClientConf, error)
	// get ssh key path
	GetSSHKeyPath() []string
	// firewalls
	AssignFirewalls(fw FirewallList) error
	RemoveFirewalls(fw FirewallList) error
	// expiry
	ChangeExpiry(expiry time.Time) error
	// update hosts file
	UpdateHostsFile(withList InstanceList, parallelSSHThreads int) error
}

type ExecInput struct {
	sshexec.ExecDetail
	Username        string
	ConnectTimeout  time.Duration
	ParallelThreads int
}

type ExecOutput struct {
	Output   *sshexec.ExecOutput
	Instance *Instance
}

type IP struct {
	Public  string `yaml:"public" json:"public"`
	Private string `yaml:"private" json:"private"`
}

func (i *IP) Routable() string {
	if i.Public != "" {
		return i.Public
	}
	return i.Private
}

// any backend returning this struct, must implement the InstanceAction interface on it
type Instance struct {
	ClusterName      string            `yaml:"clusterName" json:"clusterName"`
	ClusterUUID      string            `yaml:"clusterUUID" json:"clusterUUID"`
	NodeNo           int               `yaml:"nodeNo" json:"nodeNo"`
	IP               IP                `yaml:"IP" json:"IP"`
	ImageID          string            `yaml:"imageID" json:"imageID"`
	SubnetID         string            `yaml:"subnetID" json:"subnetID"`
	NetworkID        string            `yaml:"networkID" json:"networkID"`
	Architecture     Architecture      `yaml:"architecture" json:"architecture"`
	OperatingSystem  OS                `yaml:"operatingSystem" json:"operatingSystem"`
	Firewalls        []string          `yaml:"firewalls" json:"firewalls"`
	InstanceID       string            `yaml:"instanceId" json:"instanceId"`
	BackendType      BackendType       `yaml:"backendType" json:"backendType"`
	InstanceType     string            `yaml:"instanceType" json:"instanceType"`
	SpotInstance     bool              `yaml:"spotInstance" json:"spotInstance"`
	Name             string            `yaml:"name" json:"name"`
	ZoneName         string            `yaml:"zoneName" json:"zoneName"`
	ZoneID           string            `yaml:"zoneID" json:"zoneID"`
	CreationTime     time.Time         `yaml:"creationTime" json:"creationTime"`
	EstimatedCostUSD Cost              `yaml:"estimateCost" json:"estimatedCost"`
	AttachedVolumes  Volumes           `yaml:"attachedVolumes" json:"attachedVolumes"`
	Owner            string            `yaml:"owner" json:"owner"`                   // from tags
	InstanceState    LifeCycleState    `yaml:"lifeCycleState" json:"lifeCycleState"` // states, cloud or custom
	Tags             map[string]string `yaml:"tags" json:"tags"`                     // all tags
	Expires          time.Time         `yaml:"expires" json:"expires"`               // from tags
	Description      string            `yaml:"description" json:"description"`       // from description or tags if no description field
	CustomDNS        *InstanceDNS      `yaml:"customDNS" json:"customDNS"`
	BackendSpecific  interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

type OS struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

type Cost struct {
	Instance        CostInstance `yaml:"instance" json:"instance"`
	DeployedVolumes CostVolumes  `yaml:"deployedVolumes" json:"deployedVolumes"`
	AttachedVolumes CostVolumes  `yaml:"attachedVolumes" json:"attachedVolumes"`
}

func (c *Cost) AccruedCost() float64 {
	return c.Instance.AccruedCost() + c.DeployedVolumes.AccruedCost() + c.AttachedVolumes.AccruedCost()
}

type CostInstance struct {
	RunningPricePerHour float64   `yaml:"runningPricePerHour" json:"runningPricePerHour"`
	CostUntilLastStop   float64   `yaml:"costUntilLastStop" json:"costUntilLastStop"`
	LastStartTime       time.Time `yaml:"lastStartTime" json:"lastStartTime"`
}

func (c *CostInstance) AccruedCost() float64 {
	if c.LastStartTime.IsZero() {
		return c.CostUntilLastStop
	}
	return c.CostUntilLastStop + (c.RunningPricePerHour * time.Since(c.LastStartTime).Hours())
}

type CostVolume struct {
	PricePerGBHour float64   `yaml:"pricePerGBHour" json:"pricePerGBHour"`
	SizeGB         int64     `yaml:"sizeGB" json:"sizeGB"`
	CreateTime     time.Time `yaml:"createTime" json:"createTime"`
}

func (c *CostVolume) AccruedCost() float64 {
	return c.PricePerGBHour * float64(c.SizeGB) * time.Since(c.CreateTime).Hours()
}

type CostVolumes []CostVolume

func (v *CostVolumes) AccruedCost() float64 {
	var e float64
	for _, c := range *v {
		e += c.AccruedCost()
	}
	return e
}

func (i *Instance) Stop(force bool, waitDur time.Duration) error {
	return InstanceList{i}.Stop(force, waitDur)
}

func (i *Instance) Start(force bool, waitDur time.Duration) error {
	return InstanceList{i}.Start(waitDur)
}

func (i *Instance) Terminate(waitDur time.Duration) error {
	return InstanceList{i}.Terminate(waitDur)
}

func (i *Instance) AddTags(tags map[string]string) error {
	return InstanceList{i}.AddTags(tags)
}

func (i *Instance) RemoveTags(tagKeys []string) error {
	return InstanceList{i}.RemoveTags(tagKeys)
}

func (i *Instance) Exec(e *ExecInput) *ExecOutput {
	return InstanceList{i}.Exec(e)[0]
}

func (i *Instance) AssignFirewalls(fw FirewallList) error {
	return InstanceList{i}.AssignFirewalls(fw)
}

func (i *Instance) RemoveFirewalls(fw FirewallList) error {
	return InstanceList{i}.RemoveFirewalls(fw)
}

func (i *Instance) GetSftpConfig(username string) (*sshexec.ClientConf, error) {
	out, err := InstanceList{i}.GetSftpConfig(username)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func (i *Instance) GetSSHKeyPath() string {
	return InstanceList{i}.GetSSHKeyPath()[0]
}

// list of all Instances, for the Inventory interface
type InstanceList []*Instance

func (v InstanceList) WithBackendType(types ...BackendType) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(types, instance.BackendType) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithArchitecture(architecture Architecture) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if instance.Architecture != architecture {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithOSName(osName string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if instance.OperatingSystem.Name != osName {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithOSVersion(osVersion string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if instance.OperatingSystem.Version != osVersion {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithType(types ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(types, instance.InstanceType) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithOwner(owners ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(owners, instance.Owner) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithZoneName(zoneNames ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(zoneNames, instance.ZoneName) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithZoneID(zoneIDs ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(zoneIDs, instance.ZoneID) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithName(names ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(names, instance.Name) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithClusterName(names ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(names, instance.ClusterName) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithNodeNo(number ...int) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(number, instance.NodeNo) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithTags(tags map[string]string) Instances {
	ret := InstanceList{}
NEXTINST:
	for _, instance := range v {
		instance := instance
		for k, v := range tags {
			if v == "" {
				if _, ok := instance.Tags[k]; !ok {
					continue NEXTINST
				}
			} else {
				if vv, ok := instance.Tags[k]; !ok || v != vv {
					continue NEXTINST
				}
			}
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithExpired(expired bool) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !expired && instance.Expires.Before(time.Now()) {
			continue
		}
		if expired && instance.Expires.After(time.Now()) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithState(states ...LifeCycleState) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(states, instance.InstanceState) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithNotState(states ...LifeCycleState) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if slices.Contains(states, instance.InstanceState) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) Describe() InstanceList {
	return v
}

func (v InstanceList) Count() int {
	return len(v)
}

func (v InstanceList) AddTags(tags map[string]string) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesAddTags(v.WithBackendType(c).Describe(), tags)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) RemoveTags(tagKeys []string) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesRemoveTags(v.WithBackendType(c).Describe(), tagKeys)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) Terminate(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesTerminate(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) Stop(force bool, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesStop(v.WithBackendType(c).Describe(), force, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) Start(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesStart(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) Exec(e *ExecInput) []*ExecOutput {
	var outs []*ExecOutput
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			out := cloudList[c].InstancesExec(v.WithBackendType(c).Describe(), e)
			outs = append(outs, out...)
		}()
	}
	wait.Wait()
	return outs
}

func (v InstanceList) GetSftpConfig(username string) ([]*sshexec.ClientConf, error) {
	var outs []*sshexec.ClientConf
	wait := new(sync.WaitGroup)
	var nerr error
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			out, err := cloudList[c].InstancesGetSftpConfig(v.WithBackendType(c).Describe(), username)
			if err != nil {
				nerr = errors.Join(nerr, err)
				return
			}
			outs = append(outs, out...)
		}()
	}
	wait.Wait()
	return outs, nerr
}

func (v InstanceList) GetSSHKeyPath() []string {
	var outs []string
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			out := cloudList[c].InstancesGetSSHKeyPath(v.WithBackendType(c).Describe())
			outs = append(outs, out...)
		}()
	}
	wait.Wait()
	return outs
}

func (v InstanceList) AssignFirewalls(fw FirewallList) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesAssignFirewalls(v.WithBackendType(c).Describe(), fw)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) RemoveFirewalls(fw FirewallList) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].InstancesRemoveFirewalls(v.WithBackendType(c).Describe(), fw)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (b *backend) CleanupDNS() error {
	var retErr error
	wait := new(sync.WaitGroup)
	for cname := range b.enabledBackends {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := b.enabledBackends[cname].CleanupDNS()
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v InstanceList) WithInstanceID(instanceIDs ...string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(instanceIDs, instance.InstanceID) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) ChangeExpiry(expiry time.Time) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesChangeExpiry(v.WithBackendType(c).Describe(), expiry)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v *Instance) ChangeExpiry(expiry time.Time) error {
	return InstanceList{v}.ChangeExpiry(expiry)
}

func (v *Instance) UpdateHostsFile(withList InstanceList, parallelSSHThreads int) error {
	return InstanceList{v}.UpdateHostsFile(withList, parallelSSHThreads)
}

// if withList is empty, will use the object from which the v.UpdateHostsFile method is called
func (v InstanceList) UpdateHostsFile(withList InstanceList, parallelSSHThreads int) error {
	if withList.Count() == 0 {
		withList = v
	}
	var hostsEntries []string
	var entries []struct {
		hostname string
		entry    string
	}
	for _, instance := range v.WithNotState(LifeCycleStateTerminated).Describe() {
		hostname := instance.ClusterName + "-" + strconv.Itoa(instance.NodeNo)
		// remove any characters not allowed in hostname
		hostname = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return '-'
		}, hostname)
		// replace multiple dashes with single dash
		hostname = strings.ReplaceAll(hostname, "--", "-")
		// trim dashes from start/end
		hostname = strings.Trim(hostname, "-")
		// fill in hostname entries
		if instance.IP.Public != "" {
			entries = append(entries, struct {
				hostname string
				entry    string
			}{
				hostname: "zzzz-" + hostname,
				entry:    fmt.Sprintf("%-15s %-30s # aerolab-managed", instance.IP.Public, hostname+"-pub"),
			})
		}
		if instance.IP.Private != "" {
			entries = append(entries, struct {
				hostname string
				entry    string
			}{
				hostname: hostname,
				entry:    fmt.Sprintf("%-15s %-30s # aerolab-managed", instance.IP.Private, hostname),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].hostname < entries[j].hostname
	})

	for _, entry := range entries {
		hostsEntries = append(hostsEntries, entry.entry)
	}

	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].InstancesUpdateHostsFile(v.WithBackendType(c).Describe(), hostsEntries, parallelSSHThreads)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (b *backend) ResolveNetworkPlacement(backendType BackendType, placement string) (vpc *Network, subnet *Subnet, zone string, err error) {
	return b.enabledBackends[backendType].ResolveNetworkPlacement(placement)
}
