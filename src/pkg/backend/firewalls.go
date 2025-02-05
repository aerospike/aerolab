package backend

import (
	"slices"
	"sync"
	"time"
)

type Firewalls interface {
	// volume selector - by backend type
	WithBackendType(types ...BackendType) Firewalls
	// volume selector - by zone
	WithZoneName(zoneNames ...string) Firewalls
	// volume selector - by zone
	WithZoneID(zoneIDs ...string) Firewalls
	// volume selector - by name
	WithName(names ...string) Firewalls
	// get volume from ID
	WithFirewallID(ID ...string) Firewalls
	// tag filter: if value is "", it will only check if tag key exists, not it's value
	WithTags(tags map[string]string) Firewalls
	// number of volumes in selector
	Count() int
	// expose instance details to the caller
	Describe() FirewallList
	// you can also perform action on multiple volumes
	FirewallAction
}

type FirewallAction interface {
	// add/override tags for volumes
	AddTags(tags map[string]string, waitDur time.Duration) error
	// remove tag(s) from volumes
	RemoveTags(tagKeys []string, waitDur time.Duration) error
	// delete selected volumes
	Delete(waitDur time.Duration) error
	// for pd-ssd/ebs - attach volume to instance; if mountTargetDirectory is specified, also ssh in to the instance and perform mount
	// for EFS,docker - create mount targets if needed, sort out security groups, ssh to the instances and mount+fstab
	Update(ports PortsIn, waitDur time.Duration) error
}

type PortsIn []*PortIn
type PortsOut []*PortOut

type Port struct {
	FromPort   int    `yaml:"fromPort" json:"fromPort"` // port or ICMP type or ICMPTypeAll
	ToPort     int    `yaml:"toPort" json:"toPort"`     // port or ICMP type or ICMPTypeAll
	SourceCidr string `yaml:"sourceCidr" json:"sourceCidr"`
	SourceId   string `yaml:"sourceId" json:"sourceId"`
	Protocol   string `yaml:"protocol" json:"protocol"`
}

const (
	ProtocolTCP  = "tcp"
	ProtocolUDP  = "udp"
	ProtocolICMP = "icmp"
	ProtocolAll  = "-1"
	ICMPTypeAll  = "-1" // use for port
)

type PortIn struct {
	Port
	Action PortAction `yaml:"action" json:"action"`
}

type PortOut struct {
	Port
	BackendSpecific interface{} `yaml:"backendSpecific" json:"backendSpecific"`
}

type PortAction string

const (
	PortActionAdd    PortAction = "Add"
	PortActionDelete PortAction = "Delete"
)

// any backend returning this struct, must implement the VolumeAction interface on it
type Firewall struct {
	BackendType     BackendType       `yaml:"backendType" json:"backendType"`
	Name            string            `yaml:"name" json:"name"`
	Description     string            `yaml:"description" json:"description"` // from description or tags if no description field
	FirewallID      string            `yaml:"firewallID" json:"firewallID"`
	ZoneName        string            `yaml:"zoneName" json:"zoneName"`
	ZoneID          string            `yaml:"zoneID" json:"zoneID"`
	Owner           string            `yaml:"owner" json:"owner"` // from tags
	Tags            map[string]string `yaml:"tags" json:"tags"`   // all tags
	Ports           PortsOut          `yaml:"ports" json:"ports"`
	Network         *Network          `yaml:"network" json:"network"`
	BackendSpecific interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

// list of all volumes, for the Inventory interface
type FirewallList []*Firewall

func (v FirewallList) WithBackendType(types ...BackendType) Firewalls {
	ret := FirewallList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.BackendType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v FirewallList) WithZoneName(zoneNames ...string) Firewalls {
	ret := FirewallList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneNames, volume.ZoneName) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v FirewallList) WithZoneID(zoneIDs ...string) Firewalls {
	ret := FirewallList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneIDs, volume.ZoneID) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v FirewallList) WithName(names ...string) Firewalls {
	ret := FirewallList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(names, volume.Name) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v FirewallList) WithFirewallID(id ...string) Firewalls {
	ret := FirewallList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(id, volume.FirewallID) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v FirewallList) WithTags(tags map[string]string) Firewalls {
	ret := FirewallList{}
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

func (v FirewallList) Describe() FirewallList {
	return v
}

func (v FirewallList) Count() int {
	return len(v)
}

func (v FirewallList) AddTags(tags map[string]string, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].FirewallsAddTags(v.WithBackendType(c).Describe(), tags, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v FirewallList) RemoveTags(tagKeys []string, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].FirewallsRemoveTags(v.WithBackendType(c).Describe(), tagKeys, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v FirewallList) Delete(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].FirewallsDelete(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v FirewallList) Update(ports PortsIn, waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].FirewallsUpdate(v.WithBackendType(c).Describe(), ports, waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v *Firewall) AddTags(tags map[string]string, waitDur time.Duration) error {
	return FirewallList{v}.AddTags(tags, waitDur)
}

func (v *Firewall) RemoveTags(tagKeys []string, waitDur time.Duration) error {
	return FirewallList{v}.RemoveTags(tagKeys, waitDur)
}

func (v *Firewall) DeleteVolumes(waitDur time.Duration) error {
	return FirewallList{v}.Delete(waitDur)
}

func (v *Firewall) Update(ports PortsIn, waitDur time.Duration) error {
	return FirewallList{v}.Update(ports, waitDur)
}
