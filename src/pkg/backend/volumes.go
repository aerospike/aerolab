package backend

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"
)

type VolumeType int

const (
	VolumeTypeAttachedDisk = iota // example: pd-ssd,ebs
	VolumeTypeSharedDisk   = iota // example: EFS
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
	// number of volumes in selector
	Count() int
	// expose instance details to the caller
	Describe() VolumeList
	// you can also perform action on multiple volumes
	VolumeAction
}

type VolumeAction interface {
	// add/override tags for volumes
	AddTags(tags map[string]string, waitDur int) error
	// remove tag(s) from volumes
	RemoveTags(tagKeys []string, waitDur int) error
	// delete selected volumes
	DeleteVolumes(waitDur int) error
	// for pd-ssd/ebs - attach volume to instance; if mountTargetDirectory is specified, also ssh in to the instance and perform mount
	// for EFS,docker - create mount targets if needed, sort out security groups, ssh to the instances and mount+fstab
	Attach(instance *Instance, mountTargetDirectory *string) error
	// umount if required, and for pd-ssd/ebs, also detach device
	Detach(instance *Instance) error
	// resize a non-EFS device
	Resize(newSize StorageSize) error
}

// any backend returning this struct, must implement the VolumeAction interface on it
type Volume struct {
	Action VolumeAction
	Data   struct { // volume details, yaml/json tags included
		BackendType     BackendType       `yaml:"backendType" json:"backendType"`
		VolumeType      VolumeType        `yaml:"volumeType" json:"volumeType"`
		Name            string            `yaml:"name" json:"name"`
		Size            StorageSize       `yaml:"size" json:"size"`
		FileSystemId    string            `yaml:"fsID" json:"fsID"`
		ZoneName        string            `yaml:"zoneName" json:"zoneName"`
		ZoneID          string            `yaml:"zoneID" json:"zoneID"`
		CreationTime    time.Time         `yaml:"creationTime" json:"creationTime"`
		Owner           string            `yaml:"owner" json:"owner"`                   // from tags
		LifeCycleState  LifeCycleState    `yaml:"lifeCycleState" json:"lifeCycleState"` // states, cloud or custom
		Tags            map[string]string `yaml:"tags" json:"tags"`                     // all tags
		Expires         time.Time         `yaml:"expires" json:"expires"`               // from tags
		AttachedTo      []string          `yaml:"attachedTo" json:"attachedTo"`         // for non-efs
		Description     string            `yaml:"description" json:"description"`       // from description or tags if no description field
		Encrypted       bool              `yaml:"encrypted" json:"encrypted"`
		BackendSpecific interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
	}
}

// list of all volumes, for the Inventory interface
type VolumeList []*Volume

func (v VolumeList) WithBackendType(types ...BackendType) Volumes {
	ret := VolumeList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.Data.BackendType) {
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
		if !slices.Contains(types, volume.Data.VolumeType) {
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
		if !slices.Contains(zoneNames, volume.Data.ZoneName) {
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
		if !slices.Contains(zoneIDs, volume.Data.ZoneID) {
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
		if !slices.Contains(names, volume.Data.Name) {
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

func (v VolumeList) AddTags(tags map[string]string, waitDur int) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].VolumesAddTags(v.WithBackendType(c).Describe(), tags, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) RemoveTags(tagKeys []string, waitDur int) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].VolumesRemoveTags(v.WithBackendType(c).Describe(), tagKeys, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) DeleteVolumes(waitDur int) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].DeleteVolumes(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v VolumeList) Attach(instance *Instance, mountTargetDirectory *string) error {
	for _, volume := range v {
		err := volume.Action.Attach(instance, mountTargetDirectory)
		if err != nil {
			return fmt.Errorf("%s: %s", volume.Data.Name, err)
		}
	}
	return nil
}

func (v VolumeList) Detach(instance *Instance) error {
	for _, volume := range v {
		err := volume.Action.Detach(instance)
		if err != nil {
			return fmt.Errorf("%s: %s", volume.Data.Name, err)
		}
	}
	return nil
}

func (v VolumeList) Resize(newSize StorageSize) error {
	for _, volume := range v {
		err := volume.Action.Resize(newSize)
		if err != nil {
			return fmt.Errorf("%s: %s", volume.Data.Name, err)
		}
	}
	return nil
}
