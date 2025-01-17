package backend

import (
	"fmt"
	"slices"
	"time"
)

type Instances interface {
	// instance selector - by backend type
	WithBackendType(types []BackendType) Instances
	// instance selector - by volume type
	WithType(types []string) Instances
	// instance selector - by zone
	WithZoneName(zoneNames []string) Instances
	// instance selector - by zone
	WithZoneID(zoneIDs []string) Instances
	// instance selector - by name
	WithName(names []string) Instances
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
	Terminate() error
	// stop selected instances
	Stop() error
	// start selected instances
	Start() error
}

// any backend returning this struct, must implement the InstanceAction interface on it
type Instance struct {
	Action InstanceAction
	Data   struct {
		// instance details, yaml/json tags included
		BackendType     BackendType       `yaml:"backendType" json:"backendType"`
		InstanceType    string            `yaml:"instanceType" json:"instanceType"`
		Name            string            `yaml:"name" json:"name"`
		RootDiskSize    StorageSize       `yaml:"rootDiskSize" json:"rootDiskSize"`
		ZoneName        string            `yaml:"zoneName" json:"zoneName"`
		ZoneID          string            `yaml:"zoneID" json:"zoneID"`
		CreationTime    time.Time         `yaml:"creationTime" json:"creationTime"`
		Owner           string            `yaml:"owner" json:"owner"`                   // from tags
		LifeCycleState  string            `yaml:"lifeCycleState" json:"lifeCycleState"` // TODO: define states as iota
		Tags            map[string]string `yaml:"tags" json:"tags"`                     // all tags
		Expires         time.Time         `yaml:"expires" json:"expires"`               // from tags
		Description     string            `yaml:"description" json:"description"`       // from description or tags if no description field
		Encrypted       bool              `yaml:"encrypted" json:"encrypted"`
		BackendSpecific interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
	}
}

// list of all Instances, for the Inventory interface
type InstanceList []*Instance

func (v InstanceList) WithBackendType(types []BackendType) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(types, instance.Data.BackendType) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithType(types []string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(types, instance.Data.InstanceType) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithZoneName(zoneNames []string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(zoneNames, instance.Data.ZoneName) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithZoneID(zoneIDs []string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(zoneIDs, instance.Data.ZoneID) {
			continue
		}
		ret = append(ret, instance)
	}
	return ret
}

func (v InstanceList) WithName(names []string) Instances {
	ret := InstanceList{}
	for _, instance := range v {
		instance := instance
		if !slices.Contains(names, instance.Data.Name) {
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
	for _, instance := range v {
		err := instance.Action.AddTags(tags)
		if err != nil {
			return fmt.Errorf("%s: %s", instance.Data.Name, err)
		}
	}
	return nil
}

func (v InstanceList) RemoveTags(tagKeys []string) error {
	for _, instance := range v {
		err := instance.Action.RemoveTags(tagKeys)
		if err != nil {
			return fmt.Errorf("%s: %s", instance.Data.Name, err)
		}
	}
	return nil
}

func (v InstanceList) Terminate() error {
	for _, instance := range v {
		err := instance.Action.Terminate()
		if err != nil {
			return fmt.Errorf("%s: %s", instance.Data.Name, err)
		}
	}
	return nil
}

func (v InstanceList) Stop() error {
	for _, instance := range v {
		err := instance.Action.Stop()
		if err != nil {
			return fmt.Errorf("%s: %s", instance.Data.Name, err)
		}
	}
	return nil
}

func (v InstanceList) Start() error {
	for _, instance := range v {
		err := instance.Action.Start()
		if err != nil {
			return fmt.Errorf("%s: %s", instance.Data.Name, err)
		}
	}
	return nil
}
