package bgcp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/aerospike/aerolab/pkg/structtags"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

type CreateVolumeParams struct {
	// volume size; as GCP uses GB instead, the system will translate GiB to nearest GB and use that in GCP calls
	SizeGiB int `yaml:"sizeGiB" json:"sizeGiB"`
	// specify placement as zone
	Placement string `yaml:"placement" json:"placement" required:"true"`
	// pd-ssd, etc
	DiskType string `yaml:"diskType" json:"diskType" required:"true"`
	// optional: provisioned iops
	Iops int `yaml:"iops" json:"iops"`
	// optional: mb/second
	Throughput int `yaml:"throughput" json:"throughput"`
}

type VolumeDetail struct {
	LabelFingerprint string   `json:"labelFingerprint" yaml:"labelFingerprint"`
	AttachedTo       []string `json:"attachedTo" yaml:"attachedTo"`
}

func (s *b) GetVolumes() (backends.VolumeList, error) {
	log := s.log.WithPrefix("GetVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.VolumeList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	enabledZones, _ := s.ListEnabledZones()
	zones := []string{}
	for _, zone := range s.allZones {
		for _, enabledZone := range enabledZones {
			if strings.HasPrefix(zone, enabledZone) {
				zones = append(zones, zone)
				break
			}
		}
	}
	wg.Add(len(zones))
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			defer client.Close()
			it := client.List(ctx, &computepb.ListDisksRequest{
				Project: s.credentials.Project,
				Filter:  proto.String(LABEL_FILTER_AEROLAB),
				Zone:    zone,
			})
			for {
				pair, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				meta, err := decodeFromLabels(pair.Labels)
				if err != nil {
					log.Warn("failed to decode metadata for volume %s: %s", pair.Name, err)
					continue
				}
				if !s.listAllProjects {
					if meta[TAG_AEROLAB_PROJECT] != s.project {
						continue
					}
				}
				expires, _ := time.Parse(time.RFC3339, meta[TAG_AEROLAB_EXPIRES])
				state := backends.VolumeStateUnknown
				switch stringValue(pair.Status) {
				case computepb.Disk_CREATING.String():
					state = backends.VolumeStateCreating
				case computepb.Disk_READY.String():
					state = backends.VolumeStateAvailable
				case computepb.Disk_DELETING.String():
					state = backends.VolumeStateDeleting
				case computepb.Disk_FAILED.String():
					state = backends.VolumeStateFail
				case computepb.Disk_UNAVAILABLE.String():
					state = backends.VolumeStateUnknown
				case computepb.Disk_RESTORING.String():
					state = backends.VolumeStateConfiguring
				}
				attachedTo := pair.GetUsers()
				attachedToShort := []string{}
				for _, user := range attachedTo {
					attachedToShort = append(attachedToShort, getValueFromURL(user))
				}
				deleteOnTermination := meta[TAG_DELETE_ON_TERMINATION] == "true"
				cpg, _ := strconv.ParseFloat(meta[TAG_COST_PER_GB], 64)
				createTime, _ := time.Parse(time.RFC3339, pair.GetCreationTimestamp())
				ilock.Lock()
				i = append(i, &backends.Volume{
					Name:                pair.GetName(),
					Description:         pair.GetDescription(),
					Owner:               meta[TAG_AEROLAB_OWNER],
					BackendType:         backends.BackendTypeGCP,
					ZoneName:            zone,
					ZoneID:              getValueFromURL(pair.GetZone()),
					CreationTime:        createTime,
					Encrypted:           true,
					Iops:                int(pair.GetProvisionedIops()),
					Throughput:          backends.StorageSize(pair.GetProvisionedThroughput()) * backends.StorageMB,
					Size:                backends.StorageSize(pair.GetSizeGb()) * backends.StorageGB,
					Tags:                meta,
					FileSystemId:        pair.GetName(),
					VolumeType:          backends.VolumeTypeAttachedDisk,
					Expires:             expires,
					DiskType:            getValueFromURL(pair.GetType()),
					State:               state,
					DeleteOnTermination: deleteOnTermination,
					AttachedTo:          attachedToShort,
					EstimatedCostUSD: backends.CostVolume{
						PricePerGBHour: cpg,
						SizeGB:         int64(backends.StorageSize(pair.GetSizeGb()) * backends.StorageGB),
						CreateTime:     createTime,
					},
					BackendSpecific: &VolumeDetail{
						LabelFingerprint: pair.GetLabelFingerprint(),
						AttachedTo:       attachedTo,
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
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	vols := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := vols[volume.ZoneName]; !ok {
			vols[volume.ZoneName] = backends.VolumeList{}
		}
		vols[volume.ZoneName] = append(vols[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, vols := range vols {
		wg.Add(1)
		go func(zone string, vols backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			for _, vol := range vols {
				data := vol.Tags
				for k, v := range tags {
					data[k] = v
				}
				labels := encodeToLabels(data)
				labels["usedby"] = "aerolab"
				_, err = client.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
					Project:  s.credentials.Project,
					Zone:     zone,
					Resource: vol.Name,
					ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
						LabelFingerprint: proto.String(vol.BackendSpecific.(*VolumeDetail).LabelFingerprint),
						Labels:           labels,
					},
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, vols)
	}
	wg.Wait()
	return reterr
}

func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	vols := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := vols[volume.ZoneName]; !ok {
			vols[volume.ZoneName] = backends.VolumeList{}
		}
		vols[volume.ZoneName] = append(vols[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, vols := range vols {
		wg.Add(1)
		go func(zone string, vols backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			for _, vol := range vols {
				data := vol.Tags
				if data == nil {
					continue
				}
				for _, tag := range tagKeys {
					delete(data, tag)
				}
				labels := encodeToLabels(data)
				labels["usedby"] = "aerolab"
				_, err = client.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
					Project:  s.credentials.Project,
					Zone:     zone,
					Resource: vol.Name,
					ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
						LabelFingerprint: proto.String(vol.BackendSpecific.(*VolumeDetail).LabelFingerprint),
						Labels:           labels,
					},
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, vols)
	}
	wg.Wait()
	return reterr
}

func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("DeleteVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	vols := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := vols[volume.ZoneName]; !ok {
			vols[volume.ZoneName] = backends.VolumeList{}
		}
		vols[volume.ZoneName] = append(vols[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, vols := range vols {
		wg.Add(1)
		go func(zone string, vols backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			var ops []*compute.Operation
			for _, vol := range vols {
				op, err := client.Delete(ctx, &computepb.DeleteDiskRequest{
					Project: s.credentials.Project,
					Zone:    zone,
					Disk:    vol.Name,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				ops = append(ops, op)
			}
			if waitDur > 0 {
				ctx, cancel := context.WithTimeout(ctx, waitDur)
				defer cancel()
				for _, op := range ops {
					err = op.Wait(ctx)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
			}
		}(zone, vols)
	}
	wg.Wait()
	return reterr
}

// resize the volume - only on Attached volume type; this does not run resize2fs or any such action on the instance itself, just the AWS APIs
func (s *b) ResizeVolumes(volumes backends.VolumeList, newSizeGiB backends.StorageSize, waitDur time.Duration) error {
	log := s.log.WithPrefix("ResizeVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	for _, volume := range volumes {
		if volume.Size >= newSizeGiB*backends.StorageGiB {
			return fmt.Errorf("volume %s must be smaller than new requested size", volume.FileSystemId)
		}
	}
	volIds := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := volIds[volume.ZoneName]; !ok {
			volIds[volume.ZoneName] = backends.VolumeList{}
		}
		volIds[volume.ZoneName] = append(volIds[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range volIds {
		wg.Add(1)
		go func(zone string, vols backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			ctx := context.Background()
			client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			var ops []*compute.Operation
			for _, vol := range vols {
				log.Detail("zone=%s resize volume %s to %dGiB", zone, vol.FileSystemId, newSizeGiB)
				op, err := client.Resize(ctx, &computepb.ResizeDiskRequest{
					Project: s.credentials.Project,
					Zone:    zone,
					Disk:    vol.Name,
					DisksResizeRequestResource: &computepb.DisksResizeRequest{
						SizeGb: proto.Int64(int64(newSizeGiB * backends.StorageGiB / backends.StorageGB)),
					},
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				ops = append(ops, op)
			}
			if waitDur > 0 {
				ctx, cancel := context.WithTimeout(ctx, waitDur)
				defer cancel()
				for _, op := range ops {
					err = op.Wait(ctx)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

// for Attached volume type, this will just attach the volumes to the instance using AWS API, no mounting will be performed
func (s *b) AttachVolumes(volumes backends.VolumeList, instance *backends.Instance, sharedMountData *backends.VolumeAttachShared, waitDur time.Duration) error {
	log := s.log.WithPrefix("AttachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	attached := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := attached[volume.ZoneName]; !ok {
			attached[volume.ZoneName] = backends.VolumeList{}
		}
		attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			ctx := context.Background()
			client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			var ops []*compute.Operation
			for _, id := range ids {
				nextDev := shortuuid.New()
				diskLink := fmt.Sprintf("projects/%s/zones/%s/disks/%s", s.credentials.Project, zone, id.FileSystemId)
				log.Detail("zone=%s attach volume %s to instance %s as /dev/disk/by-id/google-%s", zone, id.FileSystemId, instance.InstanceID, nextDev)
				op, err := client.AttachDisk(ctx, &computepb.AttachDiskInstanceRequest{
					Project:  s.credentials.Project,
					Zone:     zone,
					Instance: instance.Name,
					AttachedDiskResource: &computepb.AttachedDisk{
						AutoDelete: proto.Bool(false),
						Boot:       proto.Bool(false),
						DeviceName: proto.String(nextDev),
						Mode:       proto.String("READ_WRITE"),
						Source:     proto.String(diskLink),
						Type:       proto.String("PERSISTENT"),
					},
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				ops = append(ops, op)
			}
			log.Detail("zone=%s wait for volumes to be in in-use state", zone)
			if waitDur > 0 {
				ctx, cancel := context.WithTimeout(ctx, waitDur)
				defer cancel()
				for _, op := range ops {
					err = op.Wait(ctx)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

// for Shared volume type, this will umount and remove the volume from fstab
// for Attached volume type, this will just run AWS Detach API command, no umount is performed, it us up to the caller to do so
func (s *b) DetachVolumes(volumes backends.VolumeList, instance *backends.Instance, waitDur time.Duration) error {
	log := s.log.WithPrefix("DetachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	attached := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		if _, ok := attached[volume.ZoneName]; !ok {
			attached[volume.ZoneName] = backends.VolumeList{}
		}
		attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
	}
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			ctx := context.Background()
			client, err := compute.NewInstancesRESTClient(ctx, option.WithHTTPClient(cli))
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			defer client.Close()
			var ops []*compute.Operation
			for _, id := range ids {
				volName := ""
				vols := instance.BackendSpecific.(*InstanceDetail).Volumes
				for _, vol := range vols {
					if vol.VolumeID == id.FileSystemId || getValueFromURL(vol.VolumeID) == id.FileSystemId {
						volName = vol.Device
						break
					}
				}
				if volName == "" {
					reterr = errors.Join(reterr, fmt.Errorf("volume %s not found on instance %s", id.FileSystemId, instance.InstanceID))
					return
				}
				log.Detail("zone=%s detach volume %s from instance %s", zone, id.FileSystemId, instance.InstanceID)
				op, err := client.DetachDisk(ctx, &computepb.DetachDiskInstanceRequest{
					Project:    s.credentials.Project,
					Zone:       zone,
					Instance:   instance.Name,
					DeviceName: volName,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				ops = append(ops, op)
			}
			log.Detail("zone=%s wait for volumes to be in detached state", zone)
			if waitDur > 0 {
				ctx, cancel := context.WithTimeout(ctx, waitDur)
				defer cancel()
				for _, op := range ops {
					err = op.Wait(ctx)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

func (s *b) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	log := s.log.WithPrefix("CreateVolumeGetPrice: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// resolve backend-specific parameters
	backendSpecificParams := &CreateVolumeParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeGCP]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeGCP].(type) {
			case *CreateVolumeParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeGCP].(*CreateVolumeParams)
			case CreateVolumeParams:
				item := input.BackendSpecificParams[backends.BackendTypeGCP].(CreateVolumeParams)
				backendSpecificParams = &item
			default:
				return 0, fmt.Errorf("invalid backend-specific parameters for gcp")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return 0, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	if backendSpecificParams.SizeGiB == 0 && input.VolumeType == backends.VolumeTypeAttachedDisk {
		return 0, errors.New("sizeGiB is required for attached disk")
	}

	_, _, _, err = s.resolveNetworkPlacement(backendSpecificParams.Placement)
	if err != nil {
		return 0, err
	}
	region := backendSpecificParams.Placement
	if strings.Count(region, "-") == 2 {
		parts := strings.Split(region, "-")
		region = parts[0] + "-" + parts[1]
	}

	switch input.VolumeType {
	case backends.VolumeTypeAttachedDisk:
		price, err := s.GetVolumePrice(region, backendSpecificParams.DiskType)
		if err != nil {
			return 0, err
		}
		costGB = price.PricePerGBHour
	case backends.VolumeTypeSharedDisk:
		return 0, errors.New("shared disk not supported on GCP")
	default:
		return 0, errors.New("volume type invalid")
	}
	return costGB, nil
}

func (s *b) CreateVolume(input *backends.CreateVolumeInput) (output *backends.CreateVolumeOutput, err error) {
	log := s.log.WithPrefix("CreateVolume: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// resolve backend-specific parameters
	backendSpecificParams := &CreateVolumeParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeGCP]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeGCP].(type) {
			case *CreateVolumeParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeGCP].(*CreateVolumeParams)
			case CreateVolumeParams:
				item := input.BackendSpecificParams[backends.BackendTypeGCP].(CreateVolumeParams)
				backendSpecificParams = &item
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for gcp")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	if backendSpecificParams.SizeGiB == 0 && input.VolumeType == backends.VolumeTypeAttachedDisk {
		return nil, errors.New("sizeGiB is required for attached disk")
	}

	_, _, _, err = s.resolveNetworkPlacement(backendSpecificParams.Placement)
	if err != nil {
		return nil, err
	}
	region := backendSpecificParams.Placement
	if strings.Count(region, "-") == 2 {
		parts := strings.Split(region, "-")
		region = parts[0] + "-" + parts[1]
	}
	zone := backendSpecificParams.Placement
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)

	ppgb := 0.0
	switch input.VolumeType {
	case backends.VolumeTypeAttachedDisk:
		price, err := s.GetVolumePrice(region, backendSpecificParams.DiskType)
		if err != nil {
			log.Detail("error getting volume price: %s", err)
		} else {
			ppgb = price.PricePerGBHour
		}

		cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
		if err != nil {
			return nil, err
		}
		defer cli.CloseIdleConnections()
		ctx := context.Background()
		client, err := compute.NewDisksRESTClient(ctx, option.WithHTTPClient(cli))
		if err != nil {
			return nil, err
		}
		defer client.Close()

		var iops *int64
		if backendSpecificParams.Iops > 0 {
			iops = proto.Int64(int64(backendSpecificParams.Iops))
		}
		var throughput *int64
		if backendSpecificParams.Throughput > 0 {
			throughput = proto.Int64(int64(backendSpecificParams.Throughput))
		}
		tagsIn := make(map[string]string)
		for k, v := range input.Tags {
			tagsIn[k] = v
		}
		tagsIn[TAG_NAME] = input.Name
		tagsIn[TAG_AEROLAB_OWNER] = input.Owner
		tagsIn[TAG_AEROLAB_DESCRIPTION] = input.Description
		tagsIn[TAG_AEROLAB_EXPIRES] = input.Expires.Format(time.RFC3339)
		tagsIn[TAG_AEROLAB_PROJECT] = s.project
		tagsIn[TAG_AEROLAB_VERSION] = s.aerolabVersion
		tagsIn[TAG_COST_PER_GB] = fmt.Sprintf("%f", ppgb)
		tagsIn[TAG_START_TIME] = time.Now().Format(time.RFC3339)
		labels := encodeToLabels(tagsIn)
		labels["usedby"] = "aerolab"
		diskTypeFull := fmt.Sprintf("zones/%s/diskTypes/%s", zone, backendSpecificParams.DiskType)
		op, err := client.Insert(ctx, &computepb.InsertDiskRequest{
			Project: s.credentials.Project,
			Zone:    zone,
			DiskResource: &computepb.Disk{
				Name:                  proto.String(input.Name),
				Labels:                labels,
				SizeGb:                proto.Int64(int64(backends.StorageSize(backendSpecificParams.SizeGiB) * backends.StorageGiB / backends.StorageGB)),
				Zone:                  proto.String(zone),
				ProvisionedIops:       iops,
				ProvisionedThroughput: throughput,
				Type:                  proto.String(diskTypeFull),
			},
		})
		if err != nil {
			return nil, err
		}
		err = op.Wait(ctx)
		if err != nil {
			return nil, err
		}
		return &backends.CreateVolumeOutput{
			Volume: backends.Volume{
				BackendType:         backends.BackendTypeGCP,
				VolumeType:          backends.VolumeTypeAttachedDisk,
				Name:                input.Name,
				Description:         input.Description,
				Size:                backends.StorageSize(backendSpecificParams.SizeGiB) * backends.StorageGiB,
				FileSystemId:        "",
				ZoneName:            region,
				ZoneID:              zone,
				CreationTime:        time.Now(),
				Iops:                backendSpecificParams.Iops,
				Throughput:          backends.StorageSize(backendSpecificParams.Throughput),
				Owner:               input.Owner,
				Tags:                input.Tags,
				Encrypted:           true,
				Expires:             input.Expires,
				DiskType:            backendSpecificParams.DiskType,
				State:               backends.VolumeStateAvailable,
				DeleteOnTermination: false,
				AttachedTo:          nil,
				EstimatedCostUSD: backends.CostVolume{
					PricePerGBHour: ppgb,
					SizeGB:         int64(backendSpecificParams.SizeGiB),
					CreateTime:     time.Now(),
				},
				BackendSpecific: nil,
			},
		}, nil
	case backends.VolumeTypeSharedDisk:
		return nil, errors.New("shared disk not supported on GCP")
	default:
		return nil, errors.New("volume type invalid")
	}
}
