package backend

import (
	"slices"
	"sync"
	"time"
)

type Networks interface {
	// volume selector - by backend type
	WithBackendType(types ...BackendType) Networks
	// volume selector - by zone
	WithZoneName(zoneNames ...string) Networks
	WithZoneID(zoneIDs ...string) Networks
	// volume selector - by name
	WithName(names ...string) Networks
	// get volume from ID
	WithNetID(ID ...string) Networks
	// tag filter, if value is "", it will match simply if key exists
	WithTags(tags map[string]string) Networks
	// default/aerolab managed only
	WithDefault(d bool) Networks
	WithAerolabManaged(d bool) Networks
	// number of volumes in selector
	Count() int
	// expose instance details to the caller
	Describe() NetworkList
	// subnets
	Subnets() SubnetList
	// you can also perform action on multiple volumes
	NetworkAction
}

type NetworkAction interface {
	DeleteNetworks(waitDur time.Duration) error
}

// any backend returning this struct, must implement the VolumeAction interface on it
type Network struct {
	BackendType      BackendType       `yaml:"backendType" json:"backendType"`
	Name             string            `yaml:"name" json:"name"`
	Description      string            `yaml:"description" json:"description"` // from description or tags if no description field
	NetworkId        string            `yaml:"networkId" json:"networkId"`
	Cidr             string            `yaml:"cidr" json:"cidr"`
	ZoneName         string            `yaml:"zoneName" json:"zoneName"`
	ZoneID           string            `yaml:"zoneID" json:"zoneID"`
	Owner            string            `yaml:"owner" json:"owner"` // from tags
	Tags             map[string]string `yaml:"tags" json:"tags"`   // all tags
	IsDefault        bool              `yaml:"default" json:"default"`
	IsAerolabManaged bool              `yaml:"aerolabManaged" json:"aerolabManaged"`
	State            NetworkState      `yaml:"networkState" json:"networkState"`
	Subnets          SubnetList        `yaml:"subnets" json:"subnets"`
	BackendSpecific  interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

type Subnet struct {
	BackendType      BackendType       `yaml:"backendType" json:"backendType"`
	Name             string            `yaml:"name" json:"name"`
	Description      string            `yaml:"description" json:"description"` // from description or tags if no description field
	SubnetId         string            `yaml:"subnetId" json:"subnetId"`
	NetworkId        string            `yaml:"networkId" json:"networkId"`
	Cidr             string            `yaml:"cidr" json:"cidr"`
	ZoneName         string            `yaml:"zoneName" json:"zoneName"`
	ZoneID           string            `yaml:"zoneID" json:"zoneID"`
	Owner            string            `yaml:"owner" json:"owner"` // from tags
	Tags             map[string]string `yaml:"tags" json:"tags"`   // all tags
	IsDefault        bool              `yaml:"default" json:"default"`
	IsAerolabManaged bool              `yaml:"aerolabManaged" json:"aerolabManaged"`
	State            NetworkState      `yaml:"networkState" json:"networkState"`
	PublicIP         bool              `yaml:"autoAssignPublicIP" json:"autoAssignPublicIP"`
	Network          *Network          `json:"-" yaml:"-"`
	BackendSpecific  interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

// list of all volumes, for the Inventory interface
type NetworkList []*Network

type SubnetList []*Subnet

func (v NetworkList) WithBackendType(types ...BackendType) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.BackendType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithZoneName(zoneNames ...string) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneNames, volume.ZoneName) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithZoneID(zoneIDs ...string) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneIDs, volume.ZoneID) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithName(names ...string) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(names, volume.Name) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithNetID(id ...string) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(id, volume.NetworkId) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithDefault(d bool) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if d != volume.IsDefault {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithAerolabManaged(d bool) Networks {
	ret := NetworkList{}
	for _, volume := range v {
		volume := volume
		if d != volume.IsAerolabManaged {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v NetworkList) WithTags(tags map[string]string) Networks {
	ret := NetworkList{}
NEXTVOL:
	for _, image := range v {
		image := image
		for k, v := range tags {
			if v == "" {
				if _, ok := image.Tags[k]; !ok {
					continue NEXTVOL
				}
			} else {
				if vv, ok := image.Tags[k]; !ok || v != vv {
					continue NEXTVOL
				}
			}
		}
		ret = append(ret, image)
	}
	return ret
}

func (v NetworkList) Describe() NetworkList {
	return v
}

func (v NetworkList) Count() int {
	return len(v)
}

func (v NetworkList) Subnets() SubnetList {
	ret := SubnetList{}
	for _, net := range v {
		ret = append(ret, net.Subnets...)
	}
	return ret
}

func (v NetworkList) DeleteNetworks(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].NetworksDelete(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v *Network) Delete(waitDur time.Duration) error {
	return NetworkList{v}.DeleteNetworks(waitDur)
}

func (v *SubnetList) WithZoneName(name ...string) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if !slices.Contains(name, i.ZoneName) {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v *SubnetList) WithZoneID(ID ...string) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if !slices.Contains(ID, i.ZoneID) {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v *SubnetList) WithName(name ...string) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if !slices.Contains(name, i.Name) {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v *SubnetList) WithSubnetId(id ...string) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if !slices.Contains(id, i.SubnetId) {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v *SubnetList) WithDefault(d bool) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if i.IsDefault != d {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v *SubnetList) WithAerolabManaged(d bool) SubnetList {
	vv := SubnetList{}
	for _, i := range *v {
		if i.IsAerolabManaged != d {
			continue
		}
		i := i
		vv = append(vv, i)
	}
	return vv
}

func (v SubnetList) WithBackendType(types ...BackendType) SubnetList {
	ret := SubnetList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.BackendType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v *Subnet) Delete(waitDur time.Duration) error {
	a := SubnetList{v}
	return a.Delete(waitDur)
}

func (v *SubnetList) Delete(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].NetworksDeleteSubnets(v.WithBackendType(c), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}
