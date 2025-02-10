package backend

import (
	"slices"
	"sync"
	"time"
)

type CreateImageInput struct {
	BackendType BackendType       `yaml:"backendType" json:"backendType"`
	Instance    *Instance         `yaml:"instance" json:"instance"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"` // from description or tags if no description field
	SizeGiB     StorageSize       `yaml:"size" json:"size"`               // override size of volume if > 0
	Owner       string            `yaml:"owner" json:"owner"`             // from tags
	Tags        map[string]string `yaml:"tags" json:"tags"`               // all tags
	Encrypted   bool              `yaml:"encrypted" json:"encrypted"`
	OSName      string            `yaml:"osName" json:"osName"`       // optional, if not set, will use instance.OperatingSystem.Name
	OSVersion   string            `yaml:"osVersion" json:"osVersion"` // optional, if not set, will use instance.OperatingSystem.Version
}

type CreateImageOutput struct {
	Image *Image `yaml:"image" json:"image"`
}

type Images interface {
	// volume selector - by backend type
	WithBackendType(types ...BackendType) Images
	// volume selector - by volume type
	WithOSName(names ...string) Images
	// volume selector - by zone
	WithOSVersion(version ...string) Images
	// volume selector - by zone
	WithZoneName(zoneNames ...string) Images
	WithZoneID(zoneIDs ...string) Images
	// volume selector - by name
	WithName(names ...string) Images
	// get volume from ID
	WithImageID(ID ...string) Images
	// tag filter, if value is "", it will match simply if key exists
	WithTags(tags map[string]string) Images
	// only get images matching that particular architecture
	WithArchitecture(arch Architecture) Images
	// only get images that are(not) owned by the account we are working in (created by aerolab that is)
	WithInAccount(inOwnerAccount bool) Images
	// number of volumes in selector
	Count() int
	// expose instance details to the caller
	Describe() ImageList
	// you can also perform action on multiple volumes
	ImageAction
}

type ImageAction interface {
	DeleteImages(waitDur time.Duration) error
}

// any backend returning this struct, must implement the VolumeAction interface on it
type Image struct {
	BackendType     BackendType       `yaml:"backendType" json:"backendType"`
	Name            string            `yaml:"name" json:"name"`
	Description     string            `yaml:"description" json:"description"` // from description or tags if no description field
	Size            StorageSize       `yaml:"size" json:"size"`
	ImageId         string            `yaml:"imageId" json:"imageId"`
	ZoneName        string            `yaml:"zoneName" json:"zoneName"`
	ZoneID          string            `yaml:"zoneID" json:"zoneID"`
	CreationTime    time.Time         `yaml:"creationTime" json:"creationTime"`
	Owner           string            `yaml:"owner" json:"owner"` // from tags
	Tags            map[string]string `yaml:"tags" json:"tags"`   // all tags
	Encrypted       bool              `yaml:"encrypted" json:"encrypted"`
	Architecture    Architecture      `yaml:"architecture" json:"architecture"`
	Public          bool              `yaml:"public" json:"public"`
	State           VolumeState       `yaml:"state" json:"state"` // states, cloud or custom
	OSName          string            `yaml:"osName" json:"osName"`
	OSVersion       string            `yaml:"osVersion" json:"osVersion"`
	InAccount       bool              `yaml:"inAccount" json:"inAccount"`             // whether this AMI can be deleted as it's owned by this account
	Username        string            `yaml:"username" json:"username"`               // default username to use for SSH
	BackendSpecific interface{}       `yaml:"backendSpecific" json:"backendSpecific"` // each backend can use this for their own specific needs not relating to the overall Volume definition, like mountatarget IDs, FileSystemArn, etc
}

// list of all volumes, for the Inventory interface
type ImageList []*Image

func (v ImageList) WithBackendType(types ...BackendType) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(types, volume.BackendType) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithOSName(names ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(names, volume.OSName) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithOSVersion(version ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(version, volume.OSVersion) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithZoneName(zoneNames ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneNames, volume.ZoneName) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithZoneID(zoneIDs ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(zoneIDs, volume.ZoneID) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithName(names ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(names, volume.Name) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithImageID(id ...string) Images {
	ret := ImageList{}
	for _, volume := range v {
		volume := volume
		if !slices.Contains(id, volume.ImageId) {
			continue
		}
		ret = append(ret, volume)
	}
	return ret
}

func (v ImageList) WithTags(tags map[string]string) Images {
	ret := ImageList{}
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

func (v ImageList) WithArchitecture(arch Architecture) Images {
	ret := ImageList{}
	for _, image := range v {
		image := image
		if image.Architecture != arch {
			continue
		}
		ret = append(ret, image)
	}
	return ret
}

func (v ImageList) WithInAccount(inOwnerAccount bool) Images {
	ret := ImageList{}
	for _, image := range v {
		image := image
		if image.InAccount != inOwnerAccount {
			continue
		}
		ret = append(ret, image)
	}
	return ret
}

func (v ImageList) Describe() ImageList {
	return v
}

func (v ImageList) Count() int {
	return len(v)
}

func (v ImageList) DeleteImages(waitDur time.Duration) error {
	var retErr error
	wait := new(sync.WaitGroup)
	for _, c := range ListBackendTypes() {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cloudList[c].ImagesDelete(v.WithBackendType(c).Describe(), waitDur)
			if err != nil {
				retErr = err
			}
		}()
	}
	wait.Wait()
	return retErr
}

func (v *Image) Delete(waitDur time.Duration) error {
	return ImageList{v}.DeleteImages(waitDur)
}
