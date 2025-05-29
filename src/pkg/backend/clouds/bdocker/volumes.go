package bdocker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/lithammer/shortuuid"
)

type VolumeDetail struct {
	Docker *volume.Volume `json:"docker" yaml:"docker"`
}

func (s *b) GetVolumes() (backends.VolumeList, error) {
	log := s.log.WithPrefix("GetVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.VolumeList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			f := filters.NewArgs()
			if !s.listAllProjects {
				f.Add("label", TAG_AEROLAB_PROJECT+"="+s.project)
			}
			out, err := cli.VolumeList(context.Background(), volume.ListOptions{
				Filters: f,
			})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for _, vol := range out.Volumes {
				if vol.Labels[TAG_AEROLAB_VERSION] == "" {
					continue
				}
				ilock.Lock()
				size := int64(0)
				if vol.UsageData != nil {
					size = vol.UsageData.Size
				}
				createTime, _ := time.Parse(time.RFC3339, vol.CreatedAt)
				owner := ""
				if val, ok := vol.Labels["owner"]; ok {
					owner = val
				}
				expires := time.Time{}
				if val, ok := vol.Labels[TAG_EXPIRES]; ok {
					expires, _ = time.Parse(time.RFC3339, val)
				}
				i = append(i, &backends.Volume{
					BackendType:         backends.BackendTypeDocker,
					VolumeType:          backends.VolumeTypeSharedDisk,
					Name:                vol.Name,
					Description:         vol.Labels[TAG_DESCRIPTION],
					Size:                backends.StorageSize(size),
					FileSystemId:        vol.Name,
					ZoneName:            zone,
					ZoneID:              zone,
					CreationTime:        createTime,
					Iops:                0,
					Throughput:          0,
					Owner:               owner,
					Tags:                vol.Labels,
					Encrypted:           false,
					Expires:             expires,
					DiskType:            vol.Driver + "-" + vol.Scope,
					State:               backends.VolumeStateAvailable,
					DeleteOnTermination: false,
					AttachedTo:          []string{},
					EstimatedCostUSD: backends.CostVolume{
						PricePerGBHour: 0,
						SizeGB:         0,
						CreateTime:     createTime,
					},
					BackendSpecific: &VolumeDetail{
						Docker: vol,
					},
				})
				ilock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.volumes = i
	}
	return i, errs
}

func (s *b) VolumesAddTags(volumes backends.VolumeList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("DeleteVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backends.VolumeTypeSharedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	ctx := context.Background()
	var cancel context.CancelFunc
	if waitDur > 0 {
		ctx, cancel = context.WithTimeout(ctx, waitDur)
		defer cancel()
	}
	for zone, ids := range ec2VolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				err = cli.VolumeRemove(ctx, id, true)
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

// resize the volume - only on Attached volume type; this does not run resize2fs or any such action on the instance itself, just the AWS APIs
func (s *b) ResizeVolumes(volumes backends.VolumeList, newSizeGiB backends.StorageSize, waitDur time.Duration) error {
	log := s.log.WithPrefix("ResizeVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) AttachVolumes(volumes backends.VolumeList, instance *backends.Instance, sharedMountData *backends.VolumeAttachShared, waitDur time.Duration) error {
	log := s.log.WithPrefix("AttachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) DetachVolumes(volumes backends.VolumeList, instance *backends.Instance, waitDur time.Duration) error {
	log := s.log.WithPrefix("DetachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	log := s.log.WithPrefix("CreateVolumeGetPrice: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return 0, nil
}

func (s *b) CreateVolume(input *backends.CreateVolumeInput) (output *backends.CreateVolumeOutput, err error) {
	log := s.log.WithPrefix("CreateVolume: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if input.VolumeType != backends.VolumeTypeSharedDisk {
		return nil, errors.New("volume type not supported")
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)

	cli, err := s.getDockerClient(input.Placement)
	if err != nil {
		return nil, err
	}
	tagsIn := make(map[string]string)
	for k, v := range input.Tags {
		tagsIn[k] = v
	}
	tagsIn[TAG_NAME] = input.Name
	tagsIn[TAG_OWNER] = input.Owner
	tagsIn[TAG_DESCRIPTION] = input.Description
	tagsIn[TAG_EXPIRES] = input.Expires.Format(time.RFC3339)
	tagsIn[TAG_AEROLAB_PROJECT] = s.project
	tagsIn[TAG_AEROLAB_VERSION] = s.aerolabVersion
	driver := "local"
	if input.DiskType != "" {
		driver = input.DiskType
	}
	out, err := cli.VolumeCreate(context.Background(), volume.CreateOptions{
		Driver:     driver,
		DriverOpts: map[string]string{},
		Labels:     tagsIn,
		Name:       input.Name,
	})
	if err != nil {
		return nil, err
	}
	return &backends.CreateVolumeOutput{
		Volume: backends.Volume{
			BackendType:         backends.BackendTypeDocker,
			VolumeType:          backends.VolumeTypeSharedDisk,
			Name:                input.Name,
			Description:         input.Description,
			Size:                0,
			FileSystemId:        input.Name,
			ZoneName:            input.Placement,
			ZoneID:              input.Placement,
			CreationTime:        time.Now(),
			Iops:                0,
			Throughput:          0,
			Owner:               input.Owner,
			Tags:                tagsIn,
			Encrypted:           false,
			Expires:             input.Expires,
			DiskType:            driver + "-" + out.Scope,
			State:               backends.VolumeStateAvailable,
			DeleteOnTermination: false,
			AttachedTo:          []string{},
			EstimatedCostUSD: backends.CostVolume{
				PricePerGBHour: 0,
				SizeGB:         0,
				CreateTime:     time.Now(),
			},
			BackendSpecific: &VolumeDetail{
				Docker: &out,
			},
		},
	}, nil
}
