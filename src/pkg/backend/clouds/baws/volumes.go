package baws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/structtags"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	etypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
)

type CreateVolumeParams struct {
	// attached volumes only - volume size
	SizeGiB int `yaml:"sizeGiB" json:"sizeGiB"`
	// vpc: will use first subnet in the vpc, subnet: will use the specified subnet id, zone: will use the default VPC, first subnet in the zone
	Placement string `yaml:"placement" json:"placement" required:"true"`
	// for attached disk only: gp2, gp3, etc
	DiskType string `yaml:"diskType" json:"diskType" required:"true"`
	// optional: attach disk only, provisioned iops
	Iops int `yaml:"iops" json:"iops"`
	// optional: attach disk only, bytes/second
	Throughput int `yaml:"throughput" json:"throughput"`
	// optional: whether the volume uses encryption
	Encrypted bool `yaml:"encrypted" json:"encrypted"`
	// optional: deploy the shared disk in one AZ only - limited availability, lower latency
	SharedDiskOneZone bool `yaml:"sharedDiskOneZone" json:"sharedDiskOneZone"`
}

type VolumeDetail struct {
	FileSystemArn        string `yaml:"fileSystemArn" json:"fileSystemArn"`
	NumberOfMountTargets int    `yaml:"numberOfMountTargets" json:"numberOfMountTargets"`
	PerformanceMode      string `yaml:"performanceMode" json:"performanceMode"`
	ThroughputMode       string `yaml:"throughputMode" json:"throughputMode"`
}

// getVolumeDetail safely extracts *VolumeDetail from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func getVolumeDetail(vol *backends.Volume) *VolumeDetail {
	if vol.BackendSpecific == nil {
		vol.BackendSpecific = &VolumeDetail{}
		return vol.BackendSpecific.(*VolumeDetail)
	}
	if vd, ok := vol.BackendSpecific.(*VolumeDetail); ok {
		return vd
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := vol.BackendSpecific.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var vd VolumeDetail
			if err := json.Unmarshal(jsonBytes, &vd); err == nil {
				vol.BackendSpecific = &vd
				return &vd
			}
		}
	}
	// If conversion failed or it's something else, create a new VolumeDetail
	vol.BackendSpecific = &VolumeDetail{}
	return vol.BackendSpecific.(*VolumeDetail)
}

func (s *b) GetVolumes() (backends.VolumeList, error) {
	log := s.log.WithPrefix("GetVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.VolumeList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones) * 2)
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			listFilters := []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{TAG_AEROLAB_VERSION},
				},
			}
			if !s.listAllProjects {
				listFilters = append(listFilters, types.Filter{
					Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
					Values: []string{s.project},
				})
			}
			paginator := ec2.NewDescribeVolumesPaginator(cli, &ec2.DescribeVolumesInput{
				Filters: listFilters,
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, vol := range out.Volumes {
					tags := make(map[string]string)
					for _, t := range vol.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					expires, _ := time.Parse(time.RFC3339, tags[TAG_EXPIRES])
					state := backends.VolumeStateUnknown
					switch vol.State {
					case types.VolumeStateCreating:
						state = backends.VolumeStateCreating
					case types.VolumeStateAvailable:
						state = backends.VolumeStateAvailable
					case types.VolumeStateDeleting:
						state = backends.VolumeStateDeleting
					case types.VolumeStateDeleted:
						state = backends.VolumeStateDeleted
					case types.VolumeStateInUse:
						state = backends.VolumeStateInUse
					case types.VolumeStateError:
						state = backends.VolumeStateFail
					}
					deleteOnTermination := false
					var attachedTo []string
					if len(vol.Attachments) > 0 {
						deleteOnTermination = aws.ToBool(vol.Attachments[0].DeleteOnTermination)
						attachedTo = append(attachedTo, aws.ToString(vol.Attachments[0].InstanceId))
						switch vol.Attachments[0].State {
						case types.VolumeAttachmentStateAttaching:
							state = backends.VolumeStateAttaching
						case types.VolumeAttachmentStateDetaching:
							state = backends.VolumeStateDetaching
						}
					}
					cpg, _ := strconv.ParseFloat(tags[TAG_COST_GB], 64)
					ilock.Lock()
					i = append(i, &backends.Volume{
						Name:                tags[TAG_NAME],
						Description:         tags[TAG_DESCRIPTION],
						Owner:               tags[TAG_OWNER],
						BackendType:         backends.BackendTypeAWS,
						ZoneName:            zone,
						ZoneID:              aws.ToString(vol.AvailabilityZone),
						CreationTime:        aws.ToTime(vol.CreateTime),
						Encrypted:           aws.ToBool(vol.Encrypted),
						Iops:                int(aws.ToInt32(vol.Iops)),
						Throughput:          backends.StorageSize(aws.ToInt32(vol.Throughput)) * backends.StorageMiB,
						Size:                backends.StorageSize(aws.ToInt32(vol.Size)) * backends.StorageGiB,
						Tags:                tags,
						FileSystemId:        aws.ToString(vol.VolumeId),
						VolumeType:          backends.VolumeTypeAttachedDisk,
						Expires:             expires,
						DiskType:            string(vol.VolumeType),
						State:               state,
						DeleteOnTermination: deleteOnTermination,
						AttachedTo:          attachedTo,
						EstimatedCostUSD: backends.CostVolume{
							PricePerGBHour: cpg,
							SizeGB:         int64(backends.StorageSize(aws.ToInt32(vol.Size)) * backends.StorageGiB / backends.StorageGB),
							CreateTime:     aws.ToTime(vol.CreateTime),
						},
					})
					ilock.Unlock()
				}
			}
		}(zone)
		// another set of goroutines to do EFS
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s shared: start", zone)
			defer log.Detail("zone=%s shared: end", zone)
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			paginator := efs.NewDescribeFileSystemsPaginator(cli, &efs.DescribeFileSystemsInput{})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, fs := range out.FileSystems {
					tags := make(map[string]string)
					for _, t := range fs.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					if _, ok := tags[TAG_AEROLAB_VERSION]; !ok {
						continue
					}
					if project, ok := tags[TAG_AEROLAB_PROJECT]; !ok || project != s.project && !s.listAllProjects {
						continue
					}
					expires, _ := time.Parse(time.RFC3339, tags[TAG_EXPIRES])
					cpg, _ := strconv.ParseFloat(tags[TAG_COST_GB], 64)
					state := backends.VolumeStateUnknown
					switch fs.LifeCycleState {
					case etypes.LifeCycleStateCreating:
						state = backends.VolumeStateCreating
					case etypes.LifeCycleStateAvailable:
						state = backends.VolumeStateAvailable
					case etypes.LifeCycleStateDeleted:
						state = backends.VolumeStateDeleted
					case etypes.LifeCycleStateDeleting:
						state = backends.VolumeStateDeleting
					case etypes.LifeCycleStateError:
						state = backends.VolumeStateFail
					case etypes.LifeCycleStateUpdating:
						state = backends.VolumeStateConfiguring
					}
					ilock.Lock()
					i = append(i, &backends.Volume{
						Name:        tags[TAG_NAME],
						Description: tags[TAG_DESCRIPTION],
						Owner:       tags[TAG_OWNER],
						BackendType: backends.BackendTypeAWS,
						Tags:        tags,
						VolumeType:  backends.VolumeTypeSharedDisk,
						Expires:     expires,
						EstimatedCostUSD: backends.CostVolume{
							PricePerGBHour: cpg,
							SizeGB:         int64(backends.StorageSize(int(fs.SizeInBytes.Value)) / backends.StorageGB),
							CreateTime:     aws.ToTime(fs.CreationTime),
						},
						CreationTime: aws.ToTime(fs.CreationTime),
						Encrypted:    aws.ToBool(fs.Encrypted),
						Throughput:   backends.StorageSize(aws.ToFloat64(fs.ProvisionedThroughputInMibps)) * backends.StorageMiB / 8,
						FileSystemId: aws.ToString(fs.FileSystemId),
						ZoneName:     zone,
						ZoneID:       aws.ToString(fs.AvailabilityZoneName),
						Size:         backends.StorageSize(int(fs.SizeInBytes.Value)),
						State:        state,
						BackendSpecific: &VolumeDetail{
							NumberOfMountTargets: int(fs.NumberOfMountTargets),
							FileSystemArn:        aws.ToString(fs.FileSystemArn),
							PerformanceMode:      string(fs.PerformanceMode),
							ThroughputMode:       string(fs.ThroughputMode),
						},
					})
					ilock.Unlock()
				}
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
	efsVolumeIds := make(map[string][]string)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backends.VolumeTypeSharedDisk:
			if _, ok := efsVolumeIds[volume.ZoneName]; !ok {
				efsVolumeIds[volume.ZoneName] = []string{}
			}
			efsVolumeIds[volume.ZoneName] = append(efsVolumeIds[volume.ZoneName], volume.FileSystemId)
		}
	}
	ec2Tags := []types.Tag{}
	for k, v := range tags {
		ec2Tags = append(ec2Tags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	efsTags := []etypes.Tag{}
	for k, v := range tags {
		efsTags = append(efsTags, etypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range ec2VolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
				Resources: ids,
				Tags:      ec2Tags,
			})
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
		}(zone, ids)
	}
	for zone, ids := range efsVolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s shared: start", zone)
			defer log.Detail("zone=%s shared: end", zone)
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.TagResource(context.TODO(), &efs.TagResourceInput{
					ResourceId: aws.String(id),
					Tags:       efsTags,
				})
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

func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	efsVolumeIds := make(map[string][]string)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backends.VolumeTypeSharedDisk:
			if _, ok := efsVolumeIds[volume.ZoneName]; !ok {
				efsVolumeIds[volume.ZoneName] = []string{}
			}
			efsVolumeIds[volume.ZoneName] = append(efsVolumeIds[volume.ZoneName], volume.FileSystemId)
		}
	}
	ec2Tags := []types.Tag{}
	for _, k := range tagKeys {
		ec2Tags = append(ec2Tags, types.Tag{
			Key: aws.String(k),
		})
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range ec2VolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			_, err = cli.DeleteTags(context.TODO(), &ec2.DeleteTagsInput{
				Resources: ids,
				Tags:      ec2Tags,
			})
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
		}(zone, ids)
	}
	for zone, ids := range efsVolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s shared: start", zone)
			defer log.Detail("zone=%s shared: end", zone)
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.UntagResource(context.TODO(), &efs.UntagResourceInput{
					ResourceId: aws.String(id),
					TagKeys:    tagKeys,
				})
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

func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("DeleteVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	efsVolumeIds := make(map[string]backends.VolumeList)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backends.VolumeTypeSharedDisk:
			if _, ok := efsVolumeIds[volume.ZoneName]; !ok {
				efsVolumeIds[volume.ZoneName] = backends.VolumeList{}
			}
			volume := volume
			efsVolumeIds[volume.ZoneName] = append(efsVolumeIds[volume.ZoneName], volume)
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range ec2VolumeIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.DeleteVolume(context.TODO(), &ec2.DeleteVolumeInput{
					VolumeId: aws.String(id),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
			waiter := ec2.NewVolumeDeletedWaiter(cli, func(o *ec2.VolumeDeletedWaiterOptions) {
				o.MinDelay = 1 * time.Second
				o.MaxDelay = 5 * time.Second
			})
			err = waiter.Wait(context.TODO(), &ec2.DescribeVolumesInput{
				VolumeIds: ids,
			}, waitDur)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
		}(zone, ids)
	}
	for zone, ids := range efsVolumeIds {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s shared: start", zone)
			defer log.Detail("zone=%s shared: end", zone)
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				// delete mount targets and get security group names
				detail := getVolumeDetail(id)
				if detail != nil && detail.NumberOfMountTargets > 0 {
					log.Detail("zone=%s shared: deleting mount targets for %s", zone, id.FileSystemId)
					mts, err := cli.DescribeMountTargets(context.TODO(), &efs.DescribeMountTargetsInput{
						FileSystemId: aws.String(id.FileSystemId),
						MaxItems:     aws.Int32(100),
					})
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
					for _, mt := range mts.MountTargets {
						_, err := cli.DeleteMountTarget(context.TODO(), &efs.DeleteMountTargetInput{
							MountTargetId: mt.MountTargetId,
						})
						if err != nil {
							reterr = errors.Join(reterr, err)
							return
						}
					}
					log.Detail("zone=%s shared: waiting for mount targets to be deleted for %s", zone, id.FileSystemId)
					waitTimer := time.Now()
					for {
						time.Sleep(1 * time.Second)
						mts, err := cli.DescribeMountTargets(context.TODO(), &efs.DescribeMountTargetsInput{
							FileSystemId: aws.String(id.FileSystemId),
							MaxItems:     aws.Int32(100),
						})
						if err != nil {
							reterr = errors.Join(reterr, err)
							return
						}
						if len(mts.MountTargets) == 0 {
							break
						}
						if time.Since(waitTimer) > waitDur {
							reterr = errors.Join(reterr, fmt.Errorf("timeout waiting for mount targets to be deleted"))
							return
						}
					}
				}
				// delete filesystem
				log.Detail("zone=%s shared: deleting filesystem for %s", zone, id.FileSystemId)
				_, err = cli.DeleteFileSystem(context.TODO(), &efs.DeleteFileSystemInput{
					FileSystemId: aws.String(id.FileSystemId),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				// wait for filesystem to be deleted
				log.Detail("zone=%s shared: waiting for filesystem to be deleted for %s", zone, id.FileSystemId)
				waitTimer := time.Now()
				for {
					time.Sleep(1 * time.Second)
					fs, err := cli.DescribeFileSystems(context.TODO(), &efs.DescribeFileSystemsInput{
						FileSystemId: aws.String(id.FileSystemId),
					})
					if err != nil {
						break
					}
					if len(fs.FileSystems) == 0 {
						break
					}
					if time.Since(waitTimer) > waitDur {
						reterr = errors.Join(reterr, fmt.Errorf("timeout waiting for filesystem to be deleted"))
						return
					}
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
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	for _, volume := range volumes {
		if volume.Size >= newSizeGiB*backends.StorageGiB {
			return fmt.Errorf("volume %s must be smaller than new requested size", volume.FileSystemId)
		}
	}
	volIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := volIds[volume.ZoneName]; !ok {
				volIds[volume.ZoneName] = []string{}
			}
			volIds[volume.ZoneName] = append(volIds[volume.ZoneName], volume.FileSystemId)
		case backends.VolumeTypeSharedDisk:
			return errors.New("volume is type shared, not attached")
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range volIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				log.Detail("zone=%s resize volume %s to %dGiB", zone, id, newSizeGiB)
				_, err = cli.ModifyVolume(context.TODO(), &ec2.ModifyVolumeInput{
					VolumeId: aws.String(id),
					Size:     aws.Int32(int32(newSizeGiB)),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
			for _, id := range ids {
				log.Detail("zone=%s wait for volume %s to resize", zone, id)
				err = waitForVolumeModification(context.TODO(), cli, id, waitDur)
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

type deviceName struct {
	names []string
	lock  sync.Mutex
}

func (d *deviceName) next() string {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.doNext("")
}

func (d *deviceName) doNext(start string) string {
	for i := 'b'; i <= 'z'; i++ {
		x := start + string(i)
		deviceName := "xvdb" + x
		inUse := false
		for _, n := range d.names {
			if !strings.HasPrefix(n, "xvdb") && !strings.HasPrefix(n, "/dev/xvdb") {
				continue
			}
			if strings.HasSuffix(n, x) {
				inUse = true
				break
			}
		}
		if !inUse {
			d.names = append(d.names, deviceName)
			return deviceName
		}
	}
	if start == "" {
		return d.doNext("a")
	}
	return d.doNext(string(start[0] + 1))
}

// for Shared volume type, this will attach those volumes to the instance by modifying fstab on the instance itself and running mount -a; it will also create mount targets and assign security groups as required
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
	d := &deviceName{}
	instanceDetail := getInstanceDetail(instance)
	for _, dv := range instanceDetail.Volumes {
		d.names = append(d.names, dv.Device)
	}
	attached := make(map[string]backends.VolumeList)
	shared := make(map[string]backends.VolumeList)
	for _, volume := range volumes {
		volume := volume
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := attached[volume.ZoneName]; !ok {
				attached[volume.ZoneName] = backends.VolumeList{}
			}
			attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
		case backends.VolumeTypeSharedDisk:
			if _, ok := shared[volume.ZoneName]; !ok {
				shared[volume.ZoneName] = backends.VolumeList{}
			}
			shared[volume.ZoneName] = append(shared[volume.ZoneName], volume)
		}
	}
	if len(shared) > 0 {
		defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				nextDev := d.next()
				log.Detail("zone=%s attach volume %s to instance %s as %s", zone, id.FileSystemId, instance.InstanceID, nextDev)
				_, err = cli.AttachVolume(context.TODO(), &ec2.AttachVolumeInput{
					VolumeId:   aws.String(id.FileSystemId),
					InstanceId: aws.String(instance.InstanceID),
					Device:     aws.String("/dev/" + nextDev),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
			log.Detail("zone=%s wait for volumes to be in in-use state", zone)
			waiter := ec2.NewVolumeInUseWaiter(cli, func(o *ec2.VolumeInUseWaiterOptions) {
				o.MinDelay = 1 * time.Second
				o.MaxDelay = 5 * time.Second
			})
			volIds := make([]string, len(ids))
			for i, id := range ids {
				volIds[i] = id.FileSystemId
			}
			err = waiter.Wait(context.TODO(), &ec2.DescribeVolumesInput{
				VolumeIds: volIds,
			}, waitDur)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
		}(zone, ids)
	}
	// another set of goroutines for shared: create mountpoints if required, ssh to instance and execute efs_install.sh && efs_mount.sh
	for zone, ids := range shared {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				log.Detail("zone=%s attach volume %s to instance %s", zone, id.FileSystemId, instance.InstanceID)
				mountTargetExists := false
				var mountTargetIP string
				var mountTargetID string
				volumeDetail := getVolumeDetail(id)
				if volumeDetail.NumberOfMountTargets > 0 {
					// see if we can find a working mount target
					paginator := efs.NewDescribeMountTargetsPaginator(cli, &efs.DescribeMountTargetsInput{
						FileSystemId: aws.String(id.FileSystemId),
					})
				PAGINATOR:
					for paginator.HasMorePages() {
						mts, err := paginator.NextPage(context.TODO())
						if err != nil {
							reterr = errors.Join(reterr, err)
							return
						}
						for _, mt := range mts.MountTargets {
							if aws.ToString(mt.SubnetId) == instance.SubnetID {
								mountTargetExists = true
								mountTargetID = aws.ToString(mt.MountTargetId)
								// Get the IP address from the mount target
								if mt.IpAddress != nil {
									mountTargetIP = aws.ToString(mt.IpAddress)
								}
								break PAGINATOR
							}
						}
					}
				}
				// resolve default firewall that instances use in the given VPC
				instanceDetail := getInstanceDetail(instance)
				defaultFwName := TAG_FIREWALL_NAME_PREFIX + s.project + "_" + instanceDetail.NetworkID
				fw := s.firewalls.WithName(defaultFwName).Describe()
				if len(fw) == 0 {
					reterr = errors.Join(reterr, fmt.Errorf("default security group for volume-network(vpc) %s not found", defaultFwName))
					return
				}
				if len(fw) > 1 {
					reterr = errors.Join(reterr, fmt.Errorf("multiple default security groups found for volume-network(vpc) %s", defaultFwName))
					return
				}
				secGroupId := fw[0].FirewallID
				if !mountTargetExists {
					// Wait for filesystem to be in Available state before creating mount target
					log.Detail("zone=%s checking filesystem %s lifecycle state before creating mount target", zone, id.FileSystemId)
					fsWaitStart := time.Now()
					for {
						fsOut, err := cli.DescribeFileSystems(context.TODO(), &efs.DescribeFileSystemsInput{
							FileSystemId: aws.String(id.FileSystemId),
						})
						if err != nil {
							reterr = errors.Join(reterr, fmt.Errorf("failed to describe filesystem %s: %w", id.FileSystemId, err))
							return
						}
						if len(fsOut.FileSystems) == 0 {
							reterr = errors.Join(reterr, fmt.Errorf("filesystem %s not found", id.FileSystemId))
							return
						}
						lifecycleState := fsOut.FileSystems[0].LifeCycleState
						if lifecycleState == etypes.LifeCycleStateAvailable {
							log.Detail("zone=%s filesystem %s is available", zone, id.FileSystemId)
							break
						}
						if lifecycleState == etypes.LifeCycleStateDeleted || lifecycleState == etypes.LifeCycleStateDeleting {
							reterr = errors.Join(reterr, fmt.Errorf("filesystem %s is in %s state, cannot create mount target", id.FileSystemId, lifecycleState))
							return
						}
						if lifecycleState == etypes.LifeCycleStateError {
							reterr = errors.Join(reterr, fmt.Errorf("filesystem %s is in error state", id.FileSystemId))
							return
						}
						if time.Since(fsWaitStart) > waitDur {
							reterr = errors.Join(reterr, fmt.Errorf("timeout waiting for filesystem %s to be available (current state: %s)", id.FileSystemId, lifecycleState))
							return
						}
						log.Detail("zone=%s filesystem %s is in %s state, waiting for Available state", zone, id.FileSystemId, lifecycleState)
						time.Sleep(5 * time.Second)
					}
					// create mount target
					log.Detail("zone=%s creating mount target for filesystem %s", zone, id.FileSystemId)
					createResult, err := cli.CreateMountTarget(context.TODO(), &efs.CreateMountTargetInput{
						FileSystemId:   aws.String(id.FileSystemId),
						SubnetId:       aws.String(instanceDetail.SubnetID),
						SecurityGroups: []string{secGroupId},
					})
					if err != nil {
						reterr = errors.Join(reterr, fmt.Errorf("failed to create mount target for filesystem %s: %w", id.FileSystemId, err))
						return
					}
					mountTargetID = aws.ToString(createResult.MountTargetId)
					if createResult.IpAddress != nil {
						mountTargetIP = aws.ToString(createResult.IpAddress)
					}
					// Wait for mount target to be available
					log.Detail("zone=%s waiting for mount target %s to be available", zone, mountTargetID)
					mtWaitStart := time.Now()
					for {
						mtOut, err := cli.DescribeMountTargets(context.TODO(), &efs.DescribeMountTargetsInput{
							MountTargetId: aws.String(mountTargetID),
						})
						if err != nil {
							reterr = errors.Join(reterr, fmt.Errorf("failed to describe mount target %s: %w", mountTargetID, err))
							return
						}
						if len(mtOut.MountTargets) == 0 {
							reterr = errors.Join(reterr, fmt.Errorf("mount target %s not found", mountTargetID))
							return
						}
						mtState := mtOut.MountTargets[0].LifeCycleState
						if mtState == etypes.LifeCycleStateAvailable {
							log.Detail("zone=%s mount target %s is available", zone, mountTargetID)
							if mtOut.MountTargets[0].IpAddress != nil {
								mountTargetIP = aws.ToString(mtOut.MountTargets[0].IpAddress)
							}
							break
						}
						if mtState == etypes.LifeCycleStateDeleted || mtState == etypes.LifeCycleStateDeleting {
							reterr = errors.Join(reterr, fmt.Errorf("mount target %s is in %s state", mountTargetID, mtState))
							return
						}
						if mtState == etypes.LifeCycleStateError {
							reterr = errors.Join(reterr, fmt.Errorf("mount target %s is in error state", mountTargetID))
							return
						}
						if time.Since(mtWaitStart) > waitDur {
							reterr = errors.Join(reterr, fmt.Errorf("timeout waiting for mount target %s to be available (current state: %s)", mountTargetID, mtState))
							return
						}
						log.Detail("zone=%s mount target %s is in %s state, waiting for Available state", zone, mountTargetID, mtState)
						time.Sleep(5 * time.Second)
					}
				}
				// If we don't have the IP yet (e.g., mount target existed but IP wasn't retrieved), query it
				if mountTargetIP == "" && mountTargetID != "" {
					// Query the mount target to get its IP
					mtOut, err := cli.DescribeMountTargets(context.TODO(), &efs.DescribeMountTargetsInput{
						MountTargetId: aws.String(mountTargetID),
					})
					if err == nil && len(mtOut.MountTargets) > 0 {
						if mtOut.MountTargets[0].IpAddress != nil {
							mountTargetIP = aws.ToString(mtOut.MountTargets[0].IpAddress)
						}
					}
				}
				// check if the security group is assigned to instance; if not, assign it
				if !slices.Contains(instanceDetail.FirewallIDs, secGroupId) {
					err = instance.AssignFirewalls(backends.FirewallList{{
						FirewallID: secGroupId,
					}})
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
				// upload scripts
				err = func() error {
					sshconf, err := instance.GetSftpConfig("root")
					if err != nil {
						return err
					}
					sftp, err := sshexec.NewSftp(sshconf)
					if err != nil {
						return err
					}
					defer sftp.Close()
					data, err := scripts.ReadFile("scripts/efs_install.sh")
					if err != nil {
						return err
					}
					err = sftp.WriteFile(true, &sshexec.FileWriter{
						DestPath:    "/opt/aerolab/scripts/efs_install.sh",
						Permissions: 0755,
						Source:      bytes.NewReader(data),
					})
					if err != nil {
						return err
					}
					data, err = scripts.ReadFile("scripts/efs_mount.sh")
					if err != nil {
						return err
					}
					err = sftp.WriteFile(true, &sshexec.FileWriter{
						DestPath:    "/opt/aerolab/scripts/efs_mount.sh",
						Permissions: 0755,
						Source:      bytes.NewReader(data),
					})
					if err != nil {
						return err
					}
					return nil
				}()
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				// RUN scripts: attempts: 15 (150 seconds total), mount target IP, filesystem ID, region, mount target dir, [on] for iam, [profilename] for iam profile
				installparam := ""
				if sharedMountData.FIPS {
					installparam = "fips"
				}
				// Derive region from zone (zone format is like "us-east-1a", region is "us-east-1")
				// Only derive if zone ends with a letter (a-z), otherwise use zone as-is
				region := zone
				if len(zone) > 0 {
					lastChar := zone[len(zone)-1]
					if lastChar >= 'a' && lastChar <= 'z' {
						region = zone[:len(zone)-1]
					}
				}
				// Use mount target IP if available, otherwise fall back to filesystem ID
				// If IP is not available, we still need to pass something, but the script will fail gracefully
				mountTarget := mountTargetIP
				if mountTarget == "" {
					log.Detail("Mount target IP not available, using filesystem ID %s as fallback", id.FileSystemId)
					mountTarget = id.FileSystemId
				} else {
					log.Detail("Using mount target IP %s for filesystem ID %s", mountTarget, id.FileSystemId)
				}
				execOut := instance.Exec(&backends.ExecInput{
					ExecDetail: sshexec.ExecDetail{
						Command:  []string{"/bin/bash", "-c", fmt.Sprintf("bash /opt/aerolab/scripts/efs_install.sh %s && bash /opt/aerolab/scripts/efs_mount.sh 15 %s %s %s %s", installparam, mountTarget, id.FileSystemId, region, sharedMountData.MountTargetDirectory)},
						Terminal: true,
					},
					Username: "root",
				})
				if execOut.Output.Err != nil {
					reterr = errors.Join(reterr, fmt.Errorf("=== err ===\n%s\n=== warns ===\n%s\n=== output ===\n%s", execOut.Output.Err, execOut.Output.Warn, string(execOut.Output.Stdout)))
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
	shared := backends.VolumeList{}
	for _, volume := range volumes {
		volume := volume
		switch volume.VolumeType {
		case backends.VolumeTypeAttachedDisk:
			if _, ok := attached[volume.ZoneName]; !ok {
				attached[volume.ZoneName] = backends.VolumeList{}
			}
			attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
		case backends.VolumeTypeSharedDisk:
			shared = append(shared, volume)
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backends.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start", zone)
			defer log.Detail("zone=%s attached: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				log.Detail("zone=%s detach volume %s from instance %s", zone, id.FileSystemId, instance.InstanceID)
				_, err = cli.DetachVolume(context.TODO(), &ec2.DetachVolumeInput{
					VolumeId:   aws.String(id.FileSystemId),
					InstanceId: aws.String(instance.InstanceID),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
			log.Detail("zone=%s wait for volumes to be in detached state", zone)
			waiter := ec2.NewVolumeAvailableWaiter(cli, func(o *ec2.VolumeAvailableWaiterOptions) {
				o.MinDelay = 1 * time.Second
				o.MaxDelay = 5 * time.Second
			})
			volIds := make([]string, len(ids))
			for i, id := range ids {
				volIds[i] = id.FileSystemId
			}
			err = waiter.Wait(context.TODO(), &ec2.DescribeVolumesInput{
				VolumeIds: volIds,
			}, waitDur)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
		}(zone, ids)
	}
	// shared: umount and remove the volume from fstab
	if len(shared) > 0 {
		err := func() error {
			log.Detail("zone=%s shared: start", instance.ZoneName)
			defer log.Detail("zone=%s shared: end", instance.ZoneName)
			// upload scripts
			sftpConf, err := s.InstancesGetSftpConfig(backends.InstanceList{instance}, "root")
			if err != nil {
				return err
			}
			if len(sftpConf) == 0 {
				return errors.New("shared: could not get config for instance - not found")
			}
			sftp, err := sshexec.NewSftp(sftpConf[0])
			if err != nil {
				return err
			}
			// upload umount script
			data, err := scripts.ReadFile("scripts/efs_umount.sh")
			if err != nil {
				return err
			}
			err = sftp.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/aerolab/scripts/efs_umount.sh",
				Permissions: 0755,
				Source:      bytes.NewReader(data),
			})
			if err != nil {
				return err
			}
			// run the unmount script
			params := []string{"/bin/bash", "/opt/aerolab/scripts/efs_umount.sh"}
			for _, vol := range shared {
				params = append(params, vol.FileSystemId)
			}
			out := s.InstancesExec(backends.InstanceList{instance}, &backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command: params,
				},
				Username:        "root",
				ParallelThreads: 1,
			})
			if len(out) == 0 {
				return errors.New("fatal bug: could not get script command result")
			}
			if out[0].Output.Err != nil {
				return fmt.Errorf("ERR:%s\nSTDOUT:%s\nSTDERR:%s", out[0].Output.Err, string(out[0].Output.Stdout), string(out[0].Output.Stderr))
			}
			fws := backends.FirewallList{}
			for _, vol := range shared {
				fw := s.firewalls.WithName(fmt.Sprintf("%s-%s", vol.FileSystemId, instance.NetworkID)).Describe()
				if len(fw) > 0 {
					fws = append(fws, fw...)
				}
			}
			if fws.Count() > 0 {
				err = instance.RemoveFirewalls(fws)
				if err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			reterr = errors.Join(reterr, err)
		}
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
		if _, ok := input.BackendSpecificParams[backends.BackendTypeAWS]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeAWS].(type) {
			case *CreateVolumeParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeAWS].(*CreateVolumeParams)
			case CreateVolumeParams:
				item := input.BackendSpecificParams[backends.BackendTypeAWS].(CreateVolumeParams)
				backendSpecificParams = &item
			default:
				return 0, fmt.Errorf("invalid backend-specific parameters for aws")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return 0, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	if backendSpecificParams.SizeGiB == 0 && input.VolumeType == backends.VolumeTypeAttachedDisk {
		return 0, errors.New("sizeGiB is required for attached disk")
	}
	_, _, zone, err := s.ResolveNetworkPlacement(backendSpecificParams.Placement)
	if err != nil {
		return 0, err
	}
	region := zone[:len(zone)-1]

	switch input.VolumeType {
	case backends.VolumeTypeAttachedDisk:
		price, err := s.GetVolumePrice(region, backendSpecificParams.DiskType)
		if err != nil {
			return 0, err
		}
		costGB = price.PricePerGBHour
	case backends.VolumeTypeSharedDisk:
		zoneType := "GeneralPurpose"
		if backendSpecificParams.SharedDiskOneZone {
			zoneType = "OneZone"
		}
		price, err := s.GetVolumePrice(region, fmt.Sprintf("SharedDisk_%s", zoneType))
		if err != nil {
			return 0, err
		}
		costGB = price.PricePerGBHour
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
		if _, ok := input.BackendSpecificParams[backends.BackendTypeAWS]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeAWS].(type) {
			case *CreateVolumeParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeAWS].(*CreateVolumeParams)
			case CreateVolumeParams:
				item := input.BackendSpecificParams[backends.BackendTypeAWS].(CreateVolumeParams)
				backendSpecificParams = &item
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for aws")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	if backendSpecificParams.SizeGiB == 0 && input.VolumeType == backends.VolumeTypeAttachedDisk {
		return nil, errors.New("sizeGiB is required for attached disk")
	}

	_, _, zone, err := s.ResolveNetworkPlacement(backendSpecificParams.Placement)
	if err != nil {
		return nil, err
	}
	region := zone[:len(zone)-1]
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
		cli, err := getEc2Client(s.credentials, aws.String(region))
		if err != nil {
			return nil, err
		}
		var iops *int32
		if backendSpecificParams.Iops > 0 {
			iops = aws.Int32(int32(backendSpecificParams.Iops))
		}
		var throughput *int32
		if backendSpecificParams.Throughput > 0 {
			throughput = aws.Int32(int32(backendSpecificParams.Throughput))
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
		tagsIn[TAG_COST_GB] = fmt.Sprintf("%f", ppgb)
		tagsIn[TAG_START_TIME] = time.Now().Format(time.RFC3339)
		tagsOut := []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVolume,
			},
		}
		for k, v := range tagsIn {
			tagsOut[0].Tags = append(tagsOut[0].Tags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		out, err := cli.CreateVolume(context.TODO(), &ec2.CreateVolumeInput{
			AvailabilityZone:  aws.String(zone),
			Encrypted:         aws.Bool(backendSpecificParams.Encrypted),
			Iops:              iops,
			Throughput:        throughput,
			VolumeType:        types.VolumeType(backendSpecificParams.DiskType),
			Size:              aws.Int32(int32(backendSpecificParams.SizeGiB)),
			TagSpecifications: tagsOut,
		})
		if err != nil {
			return nil, err
		}
		return &backends.CreateVolumeOutput{
			Volume: backends.Volume{
				BackendType:         backends.BackendTypeAWS,
				VolumeType:          backends.VolumeTypeAttachedDisk,
				Name:                input.Name,
				Description:         input.Description,
				Size:                backends.StorageSize(backendSpecificParams.SizeGiB) * backends.StorageGiB,
				FileSystemId:        "",
				ZoneName:            region,
				ZoneID:              zone,
				CreationTime:        aws.ToTime(out.CreateTime),
				Iops:                backendSpecificParams.Iops,
				Throughput:          backends.StorageSize(backendSpecificParams.Throughput),
				Owner:               input.Owner,
				Tags:                tagsIn,
				Encrypted:           backendSpecificParams.Encrypted,
				Expires:             input.Expires,
				DiskType:            backendSpecificParams.DiskType,
				State:               backends.VolumeStateAvailable,
				DeleteOnTermination: false,
				AttachedTo:          nil,
				EstimatedCostUSD: backends.CostVolume{
					PricePerGBHour: price.PricePerGBHour,
					SizeGB:         int64(backendSpecificParams.SizeGiB),
					CreateTime:     aws.ToTime(out.CreateTime),
				},
				BackendSpecific: nil,
			},
		}, nil
	case backends.VolumeTypeSharedDisk:
		zoneType := "GeneralPurpose"
		if backendSpecificParams.SharedDiskOneZone {
			zoneType = "OneZone"
		}
		price, err := s.GetVolumePrice(region, fmt.Sprintf("SharedDisk_%s", zoneType))
		if err != nil {
			log.Detail("error getting volume price: %s", err)
		} else {
			ppgb = price.PricePerGBHour
		}
		cli, err := getEfsClient(s.credentials, aws.String(region))
		if err != nil {
			return nil, err
		}
		var throughputMode etypes.ThroughputMode
		var throughput *float64
		if backendSpecificParams.Throughput > 0 {
			throughputMode = etypes.ThroughputModeProvisioned
			throughput = aws.Float64(float64(backendSpecificParams.Throughput) * 8 / 1024 / 1024)
		} else {
			throughputMode = etypes.ThroughputModeBursting
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
		tagsIn[TAG_COST_GB] = fmt.Sprintf("%f", ppgb)
		tagsIn[TAG_START_TIME] = time.Now().Format(time.RFC3339)
		tagsOut := []etypes.Tag{}
		for k, v := range tagsIn {
			tagsOut = append(tagsOut, etypes.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		var oneZone *string
		zoneId := region
		if backendSpecificParams.SharedDiskOneZone {
			oneZone = aws.String(zone)
			zoneId = zone
		}
		out, err := cli.CreateFileSystem(context.TODO(), &efs.CreateFileSystemInput{
			CreationToken:                aws.String(uuid.New().String() + fmt.Sprintf("%d", time.Now().UnixMicro())),
			Encrypted:                    aws.Bool(backendSpecificParams.Encrypted),
			ThroughputMode:               throughputMode,
			PerformanceMode:              etypes.PerformanceModeGeneralPurpose,
			ProvisionedThroughputInMibps: throughput,
			Tags:                         tagsOut,
			AvailabilityZoneName:         oneZone,
		})
		if err != nil {
			return nil, err
		}
		return &backends.CreateVolumeOutput{
			Volume: backends.Volume{
				BackendType:         backends.BackendTypeAWS,
				VolumeType:          backends.VolumeTypeSharedDisk,
				Name:                input.Name,
				Description:         input.Description,
				Size:                backends.StorageSize(backendSpecificParams.SizeGiB) * backends.StorageGiB,
				FileSystemId:        aws.ToString(out.FileSystemId),
				ZoneName:            region,
				ZoneID:              zoneId,
				CreationTime:        aws.ToTime(out.CreationTime),
				Iops:                backendSpecificParams.Iops,
				Throughput:          backends.StorageSize(backendSpecificParams.Throughput),
				Owner:               input.Owner,
				Tags:                tagsIn,
				Encrypted:           backendSpecificParams.Encrypted,
				Expires:             input.Expires,
				DiskType:            backendSpecificParams.DiskType,
				State:               backends.VolumeStateAvailable,
				DeleteOnTermination: false,
				AttachedTo:          nil,
				EstimatedCostUSD: backends.CostVolume{
					PricePerGBHour: price.PricePerGBHour,
					SizeGB:         int64(backendSpecificParams.SizeGiB),
					CreateTime:     aws.ToTime(out.CreationTime),
				},
				BackendSpecific: &VolumeDetail{
					FileSystemArn:        aws.ToString(out.FileSystemArn),
					NumberOfMountTargets: int(out.NumberOfMountTargets),
					PerformanceMode:      string(out.PerformanceMode),
					ThroughputMode:       string(out.ThroughputMode),
				},
			},
		}, nil
	default:
		return nil, errors.New("volume type invalid")
	}
}

func waitForVolumeModification(ctx context.Context, client *ec2.Client, volumeID string, timeout time.Duration) error {
	// Default waiter settings similar to other AWS waiters
	minDelay := 1 * time.Second
	maxDelay := 5 * time.Second

	waiter := newExponentialBackoff(timeout, minDelay, maxDelay)

	for {
		output, err := client.DescribeVolumesModifications(ctx, &ec2.DescribeVolumesModificationsInput{
			VolumeIds: []string{volumeID},
		})
		if err != nil {
			return err
		}

		if len(output.VolumesModifications) == 0 {
			return nil
		}

		mod := output.VolumesModifications[0]
		switch mod.ModificationState {
		case types.VolumeModificationStateCompleted,
			types.VolumeModificationStateOptimizing:
			return nil
		case types.VolumeModificationStateFailed:
			return fmt.Errorf("volume modification failed: %s", *mod.StatusMessage)
		}

		if err := waiter.Wait(ctx); err != nil {
			return fmt.Errorf("volume modification timed out after %v", timeout)
		}
	}
}

type exponentialBackoff struct {
	timeout  time.Duration
	minDelay time.Duration
	maxDelay time.Duration
	attempt  int
	start    time.Time
}

func newExponentialBackoff(timeout, minDelay, maxDelay time.Duration) *exponentialBackoff {
	return &exponentialBackoff{
		timeout:  timeout,
		minDelay: minDelay,
		maxDelay: maxDelay,
		start:    time.Now(),
	}
}

func (b *exponentialBackoff) Wait(ctx context.Context) error {
	if time.Since(b.start) >= b.timeout {
		return fmt.Errorf("exceeded timeout of %v", b.timeout)
	}

	delay := b.minDelay * time.Duration(math.Pow(2, float64(b.attempt)))
	if delay > b.maxDelay {
		delay = b.maxDelay
	}

	b.attempt++

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
