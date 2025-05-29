package backends

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type CreateVolumeInput struct {
	BackendType       BackendType       `yaml:"backendType" json:"backendType"`
	VolumeType        VolumeType        `yaml:"volumeType" json:"volumeType"`
	Name              string            `yaml:"name" json:"name"`
	Description       string            `yaml:"description" json:"description"` // from description or tags if no description field
	SizeGiB           int               `yaml:"sizeGiB" json:"sizeGiB"`
	Placement         string            `yaml:"placement" json:"placement"` // vpc: will use first subnet in the vpc, subnet: will use the specified subnet id, zone: will use the default VPC, first subnet in the zone
	Iops              int               `yaml:"iops" json:"iops"`
	Throughput        int               `yaml:"throughput" json:"throughput"` // bytes/second
	Owner             string            `yaml:"owner" json:"owner"`           // from tags
	Tags              map[string]string `yaml:"tags" json:"tags"`             // all tags
	Encrypted         bool              `yaml:"encrypted" json:"encrypted"`
	Expires           time.Time         `yaml:"expires" json:"expires"` // from tags
	DiskType          string            `yaml:"diskType" json:"diskType"`
	SharedDiskOneZone bool              `yaml:"sharedDiskOneZone" json:"sharedDiskOneZone"`
}

type CreateVolumeOutput struct {
	Volume Volume
}

type VolumeType int

const (
	VolumeTypeAttachedDisk VolumeType = iota // example: pd-ssd,ebs
	VolumeTypeSharedDisk   VolumeType = iota // example: EFS
)

func (a VolumeType) String() string {
	switch a {
	case VolumeTypeAttachedDisk:
		return "AttachedDisk"
	case VolumeTypeSharedDisk:
		return "SharedDisk"
	default:
		return "unknown"
	}
}

func (a VolumeType) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a VolumeType) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

func (a *VolumeType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	switch s {
	case "AttachedDisk":
		*a = VolumeTypeAttachedDisk
	case "SharedDisk":
		*a = VolumeTypeSharedDisk
	default:
		return fmt.Errorf("unknown volume type: %s", s)
	}
	return nil
}

func (a *VolumeType) UnmarshalYAML(value *yaml.Node) error {
	var s string
	err := value.Decode(&s)
	if err != nil {
		return err
	}
	switch s {
	case "AttachedDisk":
		*a = VolumeTypeAttachedDisk
	case "SharedDisk":
		*a = VolumeTypeSharedDisk
	default:
		return fmt.Errorf("unknown volume type: %s", s)
	}
	return nil
}

type Volumes interface {
	// volume selector - by backend type
	WithBackendType(types ...BackendType) Volumes
	// volume selector - by volume type
	WithType(types ...VolumeType) Volumes
	// volume selector - by zone
	WithZoneName(zoneNames ...string) Volumes
	// volume selector - by zone
	WithZoneID(zoneIDs ...string) Volumes
	// volume selector - by name
	WithName(names ...string) Volumes
	// get volume from ID
	WithVolumeID(ID ...string) Volumes
	// tag filter: if value is "", it will only check if tag key exists, not it's value
	WithTags(tags map[string]string) Volumes
	// filter by expiry date
	WithExpired(expired bool) Volumes
	// filter by delete on termination (volumes created with instances)
	WithDeleteOnTermination(deleteOnTermination bool) Volumes
	// number of volumes in selector
	Count() int
	// expose instance details to the caller
	Describe() VolumeList
	// you can also perform action on multiple volumes
	VolumeAction
}

type VolumeAction interface {
	// add/override tags for volumes
	AddTags(tags map[string]string, waitDur time.Duration) error
	// remove tag(s) from volumes
	RemoveTags(tagKeys []string, waitDur time.Duration) error
	// delete selected volumes
	// to delete shared volumes, must receive a full firewall list
	DeleteVolumes(fw FirewallList, waitDur time.Duration) error
	// for pd-ssd/ebs - attach volume to instance; if mountTargetDirectory is specified, also ssh in to the instance and perform mount
	// for EFS,docker - create mount targets if needed, sort out security groups, ssh to the instances and mount+fstab
	// for share volumes - must get a full list of Networks and Firewalls
	Attach(instance *Instance, sharedMountData *VolumeAttachShared, waitDur time.Duration) error
	// umount if required, and for pd-ssd/ebs, also detach device
	// for shared volume, we need a full firewall list
	Detach(instance *Instance, waitDur time.Duration) error
	// resize a non-EFS device
	Resize(newSizeGiB StorageSize, waitDur time.Duration) error
	// expiry
	ChangeExpiry(expiry time.Time) error
}

type VolumeAttachShared struct {
	MountTargetDirectory string
	FIPS                 bool
}

// any backend returning this struct, must implement the VolumeAction interface on it
type Volume struct {
	BackendType         BackendType       `yaml:"backendType" json:"backendType"`
	VolumeType          VolumeType        `yaml:"volumeType" json:"volumeType"`
	Name                string            `yaml:"name" json:"name"`
	Description         string            `yaml:"description" json:"description"` // from description or tags if no description field
	Size                StorageSize       `yaml:"size" json:"size"`
	FileSystemId        string            `yaml:"fsID" json:"fsID"`
	ZoneName            string            `yaml:"zoneName" json:"zoneName"`
	ZoneID              string            `yaml:"zoneID" json:"zoneID"`
	CreationTime        time.Time         `yaml:"creationTime" json:"creationTime"`
	Iops                int               `yaml:"iops" json:"iops"`
	Throughput          StorageSize       `yaml:"throughput" json:"throughput"` // bytes/second
	Owner               string            `yaml:"owner" json:"owner"`           // from tags
	Tags                map[string]string `yaml:"tags" json:"tags"`             // all tags
	Encrypted           bool              `yaml:"encrypted" json:"encrypted"`
	Expires             time.Time         `yaml:"expires" json:"expires"` // from tags
	DiskType            string            `yaml:"diskType" json:"diskType"`
	State               VolumeState       `yaml:"state" json:"state"` // states, cloud or custom
	DeleteOnTermination bool              `yaml:"deleteOnTermination" json:"deleteOnTermination"`
	AttachedTo          []string          `yaml:"attachedTo" json:"attachedTo"` // for non-efs
	EstimatedCostUSD    CostVolume        `yaml:"estimatedCost" json:"estimatedCost"`
	BackendSpecific     interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

// list of all volumes, for the Inventory interface
type VolumeList []*Volume

func (v VolumeList) WithBackendType(types ...BackendType) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.BackendType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithType(types ...VolumeType) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.VolumeType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithZoneName(zoneNames ...string) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneNames, volume.ZoneName) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithZoneID(zoneIDs ...string) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneIDs, volume.ZoneID) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithName(names ...string) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(names, volume.Name) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithVolumeID(id ...string) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(id, volume.FileSystemId) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithTags(tags map[string]string) Volumes {
	ret := VolumeList{}
NEXTVOL:
	for _, volume := range v {
		volume := volume
		for k, v := range tags {
			if v == "" {
				if _, ok := volume.Tags[k]; !ok {
					continue NEXTVOL
				}
			} else {
				if vv, ok := volume.Tags[k]; !ok || v != vv {
					continue NEXTVOL
				}
			}
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithExpired(expired bool) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !expired && volume.Expires.Before(time.Now()) {
			continue
		}
		if expired && volume.Expires.After(time.Now()) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) WithDeleteOnTermination(deleteOnTermination bool) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if deleteOnTermination != volume.DeleteOnTermination {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v VolumeList) Describe() VolumeList {
	return v
}

func (v VolumeList) Count() int {
	return len(v)
}

func (v VolumeList) AddTags(tags map[string]string, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].VolumesAddTags(v.WithBackendType(c).Describe(), tags, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) RemoveTags(tagKeys []string, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].VolumesRemoveTags(v.WithBackendType(c).Describe(), tagKeys, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) DeleteVolumes(fw FirewallList, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].DeleteVolumes(v.WithBackendType(c).Describe(), fw, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) Attach(instance *Instance, sharedMountData *VolumeAttachShared, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].AttachVolumes(v.WithBackendType(c).Describe(), instance, sharedMountData, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) Detach(instance *Instance, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].DetachVolumes(v.WithBackendType(c).Describe(), instance, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) Resize(newSizeGiB StorageSize, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].ResizeVolumes(v.WithBackendType(c).Describe(), newSizeGiB, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) ChangeExpiry(expiry time.Time) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if v.WithBackendType(c).Count() == 0 {
				return
			}
			err := cloudList[c].VolumesChangeExpiry(v.WithBackendType(c).Describe(), expiry)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v *Volume) ChangeExpiry(expiry time.Time) error {
	return VolumeList{v}.ChangeExpiry(expiry)
}

func (v *Volume) AddTags(tags map[string]string, waitDur time.Duration) error {
	return VolumeList{v}.AddTags(tags, waitDur)
}

func (v *Volume) RemoveTags(tagKeys []string, waitDur time.Duration) error {
	return VolumeList{v}.RemoveTags(tagKeys, waitDur)
}

func (v *Volume) DeleteVolumes(fw FirewallList, waitDur time.Duration) error {
	return VolumeList{v}.DeleteVolumes(fw, waitDur)
}

func (v *Volume) Attach(instance *Instance, sharedMountData *VolumeAttachShared, waitDur time.Duration) error {
	return VolumeList{v}.Attach(instance, sharedMountData, waitDur)
}

func (v *Volume) Detach(instance *Instance, waitDur time.Duration) error {
	return VolumeList{v}.Detach(instance, waitDur)
}

func (v *Volume) Resize(newSizeGiB StorageSize, waitDur time.Duration) error {
	return VolumeList{v}.Resize(newSizeGiB, waitDur)
}
