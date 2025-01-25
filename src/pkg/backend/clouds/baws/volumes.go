package baws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func (s *b) GetVolumes() (backend.VolumeList, error) {
	var i backend.VolumeList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errors.Join(errs, err)
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
					errors.Join(errs, err)
					return
				}
				for _, vol := range out.Volumes {
					tags := make(map[string]string)
					for _, t := range vol.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					ilock.Lock()
					i = append(i, &backend.Volume{
						// TODO
					})
					ilock.Unlock()
				}
			}
		}(zone)
		// TODO another set of goroutines to do EFS
		// TODO do not forget to change wg.Add to have 2x the amount of entries
	}
	wg.Wait()
	return i, errs
}

func (s *b) VolumesAddTags(volumes backend.VolumeList, tags map[string]string, waitDur time.Duration) error {
	return nil
}

func (s *b) VolumesRemoveTags(volumes backend.VolumeList, tagKeys []string, waitDur time.Duration) error {
	return nil
}

func (s *b) DeleteVolumes(volumes backend.VolumeList, waitDur time.Duration) error {
	return nil
}

func (s *b) AttachVolumes(volumes backend.VolumeList, instance *backend.Instance, mountTargetDirectory *string) error {
	return nil
}

func (s *b) DetachVolumes(volumes backend.VolumeList, instance *backend.Instance) error {
	return nil
}

func (s *b) ResizeVolumes(volumes backend.VolumeList, newSize backend.StorageSize) error {
	return nil
}
