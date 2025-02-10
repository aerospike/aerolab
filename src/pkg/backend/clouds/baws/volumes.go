package baws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	etypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/lithammer/shortuuid"
)

type volumeDetail struct {
	FileSystemArn        string `yaml:"fileSystemArn" json:"fileSystemArn"`
	NumberOfMountTargets int    `yaml:"numberOfMountTargets" json:"numberOfMountTargets"`
	PerformanceMode      string `yaml:"performanceMode" json:"performanceMode"`
	ThroughputMode       string `yaml:"throughputMode" json:"throughputMode"`
}

func (s *b) GetVolumes() (backend.VolumeList, error) {
	log := s.log.WithPrefix("GetVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backend.VolumeList
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
			paginator := ec2.NewDescribeVolumesPaginator(cli, &ec2.DescribeVolumesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("tag-key"),
						Values: []string{TAG_AEROLAB_VERSION},
					}, {
						Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
						Values: []string{s.project},
					},
				},
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
					state := backend.VolumeStateUnknown
					switch vol.State {
					case types.VolumeStateCreating:
						state = backend.VolumeStateCreating
					case types.VolumeStateAvailable:
						state = backend.VolumeStateAvailable
					case types.VolumeStateDeleting:
						state = backend.VolumeStateDeleting
					case types.VolumeStateDeleted:
						state = backend.VolumeStateDeleted
					case types.VolumeStateInUse:
						state = backend.VolumeStateInUse
					case types.VolumeStateError:
						state = backend.VolumeStateFail
					}
					deleteOnTermination := false
					var attachedTo []string
					if len(vol.Attachments) > 0 {
						deleteOnTermination = aws.ToBool(vol.Attachments[0].DeleteOnTermination)
						attachedTo = append(attachedTo, aws.ToString(vol.Attachments[0].InstanceId))
						switch vol.Attachments[0].State {
						case types.VolumeAttachmentStateAttaching:
							state = backend.VolumeStateAttaching
						case types.VolumeAttachmentStateDetaching:
							state = backend.VolumeStateDetaching
						}
					}
					cpg, _ := strconv.ParseFloat(tags[TAG_COST_GB], 64)
					ilock.Lock()
					i = append(i, &backend.Volume{
						Name:                tags[TAG_NAME],
						Description:         tags[TAG_DESCRIPTION],
						Owner:               tags[TAG_OWNER],
						BackendType:         backend.BackendTypeAWS,
						ZoneName:            aws.ToString(vol.AvailabilityZone),
						ZoneID:              aws.ToString(vol.AvailabilityZone),
						CreationTime:        aws.ToTime(vol.CreateTime),
						Encrypted:           aws.ToBool(vol.Encrypted),
						Iops:                int(aws.ToInt32(vol.Iops)),
						Throughput:          backend.StorageSize(aws.ToInt32(vol.Throughput)) * backend.StorageMiB,
						Size:                backend.StorageSize(aws.ToInt32(vol.Size)) * backend.StorageGiB,
						Tags:                tags,
						FileSystemId:        aws.ToString(vol.VolumeId),
						VolumeType:          backend.VolumeTypeAttachedDisk,
						Expires:             expires,
						DiskType:            string(vol.VolumeType),
						State:               state,
						DeleteOnTermination: deleteOnTermination,
						AttachedTo:          attachedTo,
						EstimatedCostUSD: backend.CostVolume{
							PricePerGBHour: cpg,
							SizeGB:         int64(backend.StorageSize(aws.ToInt32(vol.Size)) * backend.StorageGiB / backend.StorageGB),
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
					if project, ok := tags[TAG_AEROLAB_PROJECT]; !ok || project != s.project {
						continue
					}
					expires, _ := time.Parse(time.RFC3339, tags[TAG_EXPIRES])
					cpg, _ := strconv.ParseFloat(tags[TAG_COST_GB], 64)
					state := backend.VolumeStateUnknown
					switch fs.LifeCycleState {
					case etypes.LifeCycleStateCreating:
						state = backend.VolumeStateCreating
					case etypes.LifeCycleStateAvailable:
						state = backend.VolumeStateAvailable
					case etypes.LifeCycleStateDeleted:
						state = backend.VolumeStateDeleted
					case etypes.LifeCycleStateDeleting:
						state = backend.VolumeStateDeleting
					case etypes.LifeCycleStateError:
						state = backend.VolumeStateFail
					case etypes.LifeCycleStateUpdating:
						state = backend.VolumeStateConfiguring
					}
					ilock.Lock()
					i = append(i, &backend.Volume{
						Name:        tags[TAG_NAME],
						Description: tags[TAG_DESCRIPTION],
						Owner:       tags[TAG_OWNER],
						BackendType: backend.BackendTypeAWS,
						Tags:        tags,
						VolumeType:  backend.VolumeTypeSharedDisk,
						Expires:     expires,
						EstimatedCostUSD: backend.CostVolume{
							PricePerGBHour: cpg,
							SizeGB:         int64(backend.StorageSize(int(fs.SizeInBytes.Value)) / backend.StorageGB),
							CreateTime:     aws.ToTime(fs.CreationTime),
						},
						CreationTime: aws.ToTime(fs.CreationTime),
						Encrypted:    aws.ToBool(fs.Encrypted),
						Throughput:   backend.StorageSize(aws.ToFloat64(fs.ProvisionedThroughputInMibps)) * backend.StorageMiB / 8,
						FileSystemId: aws.ToString(fs.FileSystemId),
						ZoneName:     aws.ToString(fs.AvailabilityZoneName),
						ZoneID:       aws.ToString(fs.AvailabilityZoneId),
						Size:         backend.StorageSize(int(fs.SizeInBytes.Value)),
						State:        state,
						BackendSpecific: &volumeDetail{
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

func (s *b) VolumesAddTags(volumes backend.VolumeList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	efsVolumeIds := make(map[string][]string)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backend.VolumeTypeSharedDisk:
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
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
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
			log.Detail("zone=%s shared: start")
			defer log.Detail("zone=%s shared: end")
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

func (s *b) VolumesRemoveTags(volumes backend.VolumeList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	efsVolumeIds := make(map[string][]string)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backend.VolumeTypeSharedDisk:
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
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
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
			log.Detail("zone=%s shared: start")
			defer log.Detail("zone=%s shared: end")
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

func (s *b) DeleteVolumes(volumes backend.VolumeList, fw backend.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("DeleteVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	efsVolumeIds := make(map[string]backend.VolumeList)
	ec2VolumeIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := ec2VolumeIds[volume.ZoneName]; !ok {
				ec2VolumeIds[volume.ZoneName] = []string{}
			}
			ec2VolumeIds[volume.ZoneName] = append(ec2VolumeIds[volume.ZoneName], volume.FileSystemId)
		case backend.VolumeTypeSharedDisk:
			if _, ok := efsVolumeIds[volume.ZoneName]; !ok {
				efsVolumeIds[volume.ZoneName] = backend.VolumeList{}
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
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
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
		}(zone, ids)
	}
	for zone, ids := range efsVolumeIds {
		wg.Add(1)
		go func(zone string, ids backend.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s shared: start")
			defer log.Detail("zone=%s shared: end")
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				secGroups := []string{}
				// delete mount targets and get security group names
				switch detail := id.BackendSpecific.(type) {
				case *volumeDetail:
					if detail != nil && detail.NumberOfMountTargets > 0 {
						mts, err := cli.DescribeMountTargets(context.TODO(), &efs.DescribeMountTargetsInput{
							FileSystemId: aws.String(id.FileSystemId),
							MaxItems:     aws.Int32(100),
						})
						if err != nil {
							reterr = errors.Join(reterr, err)
							return
						}
						for _, mt := range mts.MountTargets {
							secGroups = append(secGroups, fmt.Sprintf("%s-%s", aws.ToString(mt.FileSystemId), aws.ToString(mt.VpcId)))
							_, err := cli.DeleteMountTarget(context.TODO(), &efs.DeleteMountTargetInput{
								MountTargetId: mt.MountTargetId,
							})
							if err != nil {
								reterr = errors.Join(reterr, err)
								return
							}
						}
					}
				}
				// delete filesystem
				_, err = cli.DeleteFileSystem(context.TODO(), &efs.DeleteFileSystemInput{
					FileSystemId: aws.String(id.FileSystemId),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				// delete security groups (name is fsid-*)
				err = fw.WithName(secGroups...).Delete(waitDur)
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
func (s *b) ResizeVolumes(volumes backend.VolumeList, newSizeGiB backend.StorageSize) error {
	log := s.log.WithPrefix("ResizeVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	for _, volume := range volumes {
		if volume.Size >= newSizeGiB*backend.StorageGiB {
			return fmt.Errorf("volume %s must be smaller than new requested size", volume.FileSystemId)
		}
	}
	volIds := make(map[string][]string)
	for _, volume := range volumes {
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := volIds[volume.ZoneName]; !ok {
				volIds[volume.ZoneName] = []string{}
			}
			volIds[volume.ZoneName] = append(volIds[volume.ZoneName], volume.FileSystemId)
		case backend.VolumeTypeSharedDisk:
			return errors.New("volume is type shared, not attached")
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range volIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s start")
			defer log.Detail("zone=%s end")
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.ModifyVolume(context.TODO(), &ec2.ModifyVolumeInput{
					VolumeId: aws.String(id),
					Size:     aws.Int32(int32(newSizeGiB)),
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

type deviceName struct {
	names []string
	lock  sync.Mutex
}

func (d *deviceName) next() string {
	return d.doNext("")
}

func (d *deviceName) doNext(start string) string {
	d.lock.Lock()
	defer d.lock.Unlock()
	for i := 'a'; i <= 'z'; i++ {
		for _, n := range d.names {
			if !strings.HasPrefix(n, "xvd") && !strings.HasPrefix(n, "/dev/xvd") {
				continue
			}
			x := start + string(i)
			if !strings.HasSuffix(n, x) {
				d.names = append(d.names, "xvd"+x)
				return "xvd" + x
			}
		}
	}
	if start == "" {
		return d.doNext("a")
	}
	return d.doNext(string(start[0] + 1))
}

// for Shared volume type, this will attach those volumes to the instance by modifying fstab on the instance itself and running mount -a; it will also create mount targets and assign security groups as required
// for Attached volume type, this will just attach the volumes to the instance using AWS API, no mounting will be performed
func (s *b) AttachVolumes(volumes backend.VolumeList, instance *backend.Instance, sharedMountData *backend.VolumeAttachShared, waitDur time.Duration) error {
	log := s.log.WithPrefix("AttachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	d := &deviceName{}
	for _, dv := range instance.BackendSpecific.(instanceDetail).Volumes {
		d.names = append(d.names, dv.Device)
	}
	attached := make(map[string]backend.VolumeList)
	shared := make(map[string]backend.VolumeList)
	for _, volume := range volumes {
		volume := volume
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := attached[volume.ZoneName]; !ok {
				attached[volume.ZoneName] = backend.VolumeList{}
			}
			attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
		case backend.VolumeTypeSharedDisk:
			if _, ok := shared[volume.ZoneName]; !ok {
				shared[volume.ZoneName] = backend.VolumeList{}
			}
			shared[volume.ZoneName] = append(shared[volume.ZoneName], volume)
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backend.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.AttachVolume(context.TODO(), &ec2.AttachVolumeInput{
					VolumeId:   aws.String(id.FileSystemId),
					InstanceId: aws.String(instance.InstanceID),
					Device:     aws.String(d.next()),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, ids)
	}
	// another set of goroutines for shared: create mountpoints if required, ssh to instance and execute efs_install.sh && efs_mount.sh
	for zone, ids := range shared {
		wg.Add(1)
		go func(zone string, ids backend.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
			cli, err := getEfsClient(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				mountTargetExists := false
				if id.BackendSpecific.(volumeDetail).NumberOfMountTargets > 0 {
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
								break PAGINATOR
							}
						}
					}
				}
				// check if security group for volume-network(vpc) pair exists and get the ID or create new security group if needed
				secGroupName := fmt.Sprintf("%s-%s", id.FileSystemId, instance.BackendSpecific.(instanceDetail).Network.NetworkId)
				secGroupId := ""
				fw := s.firewalls.WithName(secGroupName)
				if fw.Count() == 0 {
					out, err := s.CreateFirewall(&backend.CreateFirewallInput{
						BackendType: backend.BackendTypeAWS,
						Name:        secGroupName,
						Description: "Automatically created by aerolab volume mount",
						Owner:       id.Owner,
						Tags:        make(map[string]string),
						Ports: []*backend.Port{
							{
								FromPort:   -1,
								ToPort:     -1,
								SourceCidr: "",
								SourceId:   "self",
								Protocol:   backend.ProtocolAll,
							},
						},
						Network: instance.BackendSpecific.(instanceDetail).Network,
					}, waitDur)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
					err = s.FirewallsUpdate(backend.FirewallList{out.Firewall}, backend.PortsIn{
						{
							Port: backend.Port{
								FromPort:   -1,
								ToPort:     -1,
								SourceCidr: "",
								SourceId:   out.Firewall.FirewallID,
								Protocol:   backend.ProtocolAll,
							},
							Action: backend.PortActionAdd,
						},
					}, 0)
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
					secGroupId = out.Firewall.FirewallID
				} else {
					secGroupId = fw.Describe()[0].FirewallID
				}
				if !mountTargetExists {
					// create mount target
					_, err = cli.CreateMountTarget(context.TODO(), &efs.CreateMountTargetInput{
						FileSystemId:   aws.String(id.FileSystemId),
						SubnetId:       aws.String(instance.BackendSpecific.(instanceDetail).Subnet.SubnetId),
						SecurityGroups: []string{secGroupId},
					})
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
				}
				// check if the security group is assigned to instance; if not, assign it
				if instance.BackendSpecific.(instanceDetail).FirewallList.WithFirewallID(secGroupId).Count() == 0 {
					err = instance.AssignFirewalls(backend.FirewallList{{
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
				// RUN scripts: attempts: 15 (150 seconds total), fsid, mount target dir, [on] for iam, [profilename] for iam profile
				installparam := ""
				if sharedMountData.FIPS {
					installparam = "fips"
				}
				execOut := instance.Exec(&backend.ExecInput{
					ExecDetail: sshexec.ExecDetail{
						Command:  []string{"/bin/bash", "-c", fmt.Sprintf("bash /opt/aerolab/scripts/efs_install.sh %s && bash /opt/aerolab/scripts/efs_mount.sh 15 %s %s", installparam, id.FileSystemId, sharedMountData.MountTargetDirectory)},
						Terminal: true,
					},
					Username: "root",
				})
				if execOut.Output.Err != nil {
					reterr = errors.Join(reterr, fmt.Errorf("=== err ===\n%s\n=== warns ===\n%s\n=== output ===\n%s", err, execOut.Output.Warn, string(execOut.Output.Stdout)))
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

// for Shared volume type, this will umount and remove the volume from fstab
// for Attached volume type, this will just run AWS Detach API command, no umount is performed, it us up to the caller to do so
func (s *b) DetachVolumes(volumes backend.VolumeList, instance *backend.Instance, waitDur time.Duration) error {
	log := s.log.WithPrefix("DetachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(volumes) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc()
	attached := make(map[string]backend.VolumeList)
	shared := backend.VolumeList{}
	for _, volume := range volumes {
		volume := volume
		switch volume.VolumeType {
		case backend.VolumeTypeAttachedDisk:
			if _, ok := attached[volume.ZoneName]; !ok {
				attached[volume.ZoneName] = backend.VolumeList{}
			}
			attached[volume.ZoneName] = append(attached[volume.ZoneName], volume)
		case backend.VolumeTypeSharedDisk:
			shared = append(shared, volume)
		}
	}
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range attached {
		wg.Add(1)
		go func(zone string, ids backend.VolumeList) {
			defer wg.Done()
			log.Detail("zone=%s attached: start")
			defer log.Detail("zone=%s attached: end")
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				_, err = cli.DetachVolume(context.TODO(), &ec2.DetachVolumeInput{
					VolumeId:   aws.String(id.FileSystemId),
					InstanceId: aws.String(instance.InstanceID),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
			}
		}(zone, ids)
	}
	// shared: umount and remove the volume from fstab
	if len(shared) > 0 {
		err := func() error {
			log.Detail("zone=%s shared: start")
			defer log.Detail("zone=%s shared: end")
			// upload scripts
			sftpConf, err := s.InstancesGetSftpConfig(backend.InstanceList{instance}, "root")
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
			out := s.InstancesExec(backend.InstanceList{instance}, &backend.ExecInput{
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
			fws := backend.FirewallList{}
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

func (s *b) CreateVolume(input *backend.CreateVolumeInput) (output *backend.CreateVolumeOutput, err error) {

}
