package bgcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

type ImageDetail struct {
	LabelFingerprint string `yaml:"labelFingerprint" json:"labelFingerprint"`
}

// getImageDetail safely extracts *ImageDetail from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func getImageDetail(image *backends.Image) *ImageDetail {
	if image.BackendSpecific == nil {
		image.BackendSpecific = &ImageDetail{}
		return image.BackendSpecific.(*ImageDetail)
	}
	if id, ok := image.BackendSpecific.(*ImageDetail); ok {
		return id
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := image.BackendSpecific.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var id ImageDetail
			if err := json.Unmarshal(jsonBytes, &id); err == nil {
				image.BackendSpecific = &id
				return &id
			}
		}
	}
	// If conversion failed or it's something else, create a new ImageDetail
	image.BackendSpecific = &ImageDetail{}
	return image.BackendSpecific.(*ImageDetail)
}

func (s *b) GetImages() (backends.ImageList, error) {
	log := s.log.WithPrefix("GetImages: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.ImageList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var errs error

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Detail("Listing owned images: start")
		defer log.Detail("Listing owned images: end")
		iter := client.List(ctx, &computepb.ListImagesRequest{
			Project: s.credentials.Project,
			Filter:  proto.String(LABEL_FILTER_AEROLAB),
		})
		for {
			image, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			ctime := time.Time{}
			if image.CreationTimestamp != nil {
				ctime, err = time.Parse(time.RFC3339, *image.CreationTimestamp)
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
			}
			tags, err := decodeFromLabels(image.Labels)
			if err != nil {
				log.Detail("failed to decode metadata for image %s: %s", image.GetName(), err)
				continue
			}
			if !s.listAllProjects {
				if tags[TAG_AEROLAB_PROJECT] != s.project {
					continue
				}
			}
			arch := backends.ArchitectureX8664
			if *image.Architecture == "ARM64" {
				arch = backends.ArchitectureARM64
			}
			im := &backends.Image{
				BackendType:  backends.BackendTypeGCP,
				Name:         image.GetName(),
				Description:  image.GetDescription(),
				Size:         backends.StorageSize(*image.DiskSizeGb) * backends.StorageGB,
				ImageId:      image.GetSelfLink(),
				ZoneName:     strings.Join(image.StorageLocations, ","),
				ZoneID:       strings.Join(image.StorageLocations, ","),
				CreationTime: ctime,
				Owner:        tags[TAG_AEROLAB_OWNER],
				Tags:         tags,
				Encrypted:    image.ImageEncryptionKey != nil,
				Architecture: arch,
				Public:       false,
				State:        backends.VolumeStateAvailable,
				OSName:       tags[TAG_OS_NAME],
				OSVersion:    tags[TAG_OS_VERSION],
				InAccount:    true,
				Username:     "root",
				BackendSpecific: &ImageDetail{
					LabelFingerprint: *image.LabelFingerprint,
				},
			}
			ilock.Lock()
			i = append(i, im)
			ilock.Unlock()
		}
	}()

	wg.Add(4)
	for _, project := range []string{"ubuntu-os-cloud", "debian-cloud", "centos-cloud", "rocky-linux-cloud"} {
		go func(project string) {
			defer wg.Done()
			log.Detail("Listing generic images for %s: start", project)
			defer log.Detail("Listing generic images for %s: end", project)
			iter := client.List(ctx, &computepb.ListImagesRequest{
				Project: project,
			})
			for {
				image, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				ctime := time.Time{}
				if image.CreationTimestamp != nil {
					ctime, err = time.Parse(time.RFC3339, *image.CreationTimestamp)
					if err != nil {
						errs = errors.Join(errs, err)
						return
					}
				}
				if image.Architecture == nil {
					continue
				}
				arch := backends.ArchitectureX8664
				if image.GetArchitecture() == "ARM64" {
					arch = backends.ArchitectureARM64
				}
				osName := ""
				osVersion := ""
				if image.Family == nil {
					continue
				}
				family := *image.Family
				switch project {
				case "ubuntu-os-cloud":
					osName = "ubuntu"
					osVersion = ""
					d := strings.Split(family, "-")
					if len(d) >= 3 && len(d) <= 4 && d[0] == "ubuntu" && d[2] == "lts" && len(d[1]) == 4 {
						osVersion = d[1][0:2] + "." + d[1][2:4]
					}
				case "debian-cloud":
					osName = "debian"
					osVersion = ""
					if strings.HasPrefix(family, "debian-") {
						osVersion = strings.Split(strings.TrimPrefix(family, "debian-"), "-")[0]
					}
				case "centos-cloud":
					osName = "centos"
					osVersion = ""
					if strings.HasPrefix(family, "centos-stream-") {
						osVersion = strings.Split(strings.TrimPrefix(family, "centos-stream-"), "-")[0]
					}
				case "rocky-linux-cloud":
					osName = "rocky"
					osVersion = ""
					if strings.HasPrefix(family, "rocky-linux-") && !strings.Contains(family, "optimized") {
						osVersion = strings.Split(strings.TrimPrefix(family, "rocky-linux-"), "-")[0]
					}
				}
				if osName == "" || osVersion == "" {
					continue
				}
				im := &backends.Image{
					BackendType:  backends.BackendTypeGCP,
					Name:         image.GetName(),
					Description:  image.GetDescription(),
					Size:         backends.StorageSize(*image.DiskSizeGb) * backends.StorageGB,
					ImageId:      image.GetSelfLink(),
					ZoneName:     strings.Join(image.StorageLocations, ","),
					ZoneID:       strings.Join(image.StorageLocations, ","),
					CreationTime: ctime,
					Owner:        project,
					Tags:         map[string]string{},
					Encrypted:    image.ImageEncryptionKey != nil,
					Architecture: arch,
					Public:       true,
					State:        backends.VolumeStateAvailable,
					OSName:       osName,
					OSVersion:    osVersion,
					InAccount:    false,
					Username:     "root",
					BackendSpecific: &ImageDetail{
						LabelFingerprint: *image.LabelFingerprint,
					},
				}
				ilock.Lock()
				found := false
				for imei, ime := range i {
					if ime.InAccount {
						continue
					}
					if ime.OSName != im.OSName || ime.OSVersion != im.OSVersion {
						continue
					}
					if ime.Architecture != im.Architecture {
						continue
					}
					if ime.CreationTime.Before(im.CreationTime) {
						i[imei] = im
					}
					found = true
					break
				}
				if !found {
					i = append(i, im)
				}
				ilock.Unlock()
			}
		}(project)
	}

	wg.Wait()
	if errs != nil {
		return i, errs
	}
	return i, nil
}

func (s *b) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	log := s.log.WithPrefix("ImagesDelete: job=" + shortuuid.New() + " ")
	if len(images) == 0 {
		log.Detail("ImageList empty, returning")
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()

	ops := []*compute.Operation{}
	log.Detail("Deleting images")
	for _, image := range images {
		op, err := client.Delete(ctx, &computepb.DeleteImageRequest{
			Project: s.credentials.Project,
			Image:   image.Name,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		log.Detail("Waiting for operations to complete")
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *b) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	log := s.log.WithPrefix("ImagesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(images) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	log.Detail("Adding tags to images")
	for _, image := range images {
		newTags := make(map[string]string)
		for k, v := range image.Tags {
			newTags[k] = v
		}
		for k, v := range tags {
			newTags[k] = v
		}
		labels := encodeToLabels(newTags)
		labels["usedby"] = "aerolab"
		id := getImageDetail(image)
		op, err := client.Patch(ctx, &computepb.PatchImageRequest{
			Image: image.Name,
			ImageResource: &computepb.Image{
				LabelFingerprint: proto.String(id.LabelFingerprint),
				Labels:           labels,
			},
			Project: s.credentials.Project,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	log.Detail("Waiting for operations to complete")
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	log := s.log.WithPrefix("ImagesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(images) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	log.Detail("Adding tags to images")
	for _, image := range images {
		newTags := make(map[string]string)
		for k, v := range image.Tags {
			if slices.Contains(tagKeys, k) {
				continue
			}
			newTags[k] = v
		}
		labels := encodeToLabels(newTags)
		labels["usedby"] = "aerolab"
		id := getImageDetail(image)
		op, err := client.Patch(ctx, &computepb.PatchImageRequest{
			Image: image.Name,
			ImageResource: &computepb.Image{
				LabelFingerprint: proto.String(id.LabelFingerprint),
				Labels:           labels,
			},
			Project: s.credentials.Project,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	log.Detail("Waiting for operations to complete")
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (output *backends.CreateImageOutput, err error) {
	log := s.log.WithPrefix("CreateImage: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	m := make(map[string]string)
	for k, v := range input.Tags {
		m[k] = v
	}
	m[TAG_AEROLAB_OWNER] = input.Owner
	m[TAG_AEROLAB_PROJECT] = s.project
	m[TAG_AEROLAB_VERSION] = s.aerolabVersion
	m[TAG_AEROLAB_DESCRIPTION] = input.Description
	m[TAG_OS_NAME] = input.OSName
	m[TAG_OS_VERSION] = input.OSVersion
	m[TAG_NAME] = input.Name
	labels := encodeToLabels(m)
	labels["usedby"] = "aerolab"
	output = &backends.CreateImageOutput{
		Image: &backends.Image{
			BackendType:  input.BackendType,
			Name:         input.Name,
			Description:  input.Description,
			Size:         input.SizeGiB * backends.StorageGiB,
			ZoneName:     input.Instance.ZoneName,
			ZoneID:       input.Instance.ZoneID,
			Architecture: input.Instance.Architecture,
			Public:       false,
			OSName:       input.OSName,
			OSVersion:    input.OSVersion,
			Username:     "root",
			Encrypted:    input.Encrypted,
			State:        backends.VolumeStateAvailable,
			CreationTime: time.Now(),
			Owner:        input.Owner,
			Tags:         input.Tags,
			BackendSpecific: &ImageDetail{
				LabelFingerprint: "", // will be set later
			},
			ImageId: "", // will be set later
		},
	}
	if input.OSName == "" || input.OSVersion == "" {
		output.Image.OSName = input.Instance.OperatingSystem.Name
		output.Image.OSVersion = input.Instance.OperatingSystem.Version
	}

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewImagesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)
	idetail := getInstanceDetail(input.Instance)
	op, err := client.Insert(ctx, &computepb.InsertImageRequest{
		Project: s.credentials.Project,
		ImageResource: &computepb.Image{
			Labels:     labels,
			Name:       proto.String(input.Name),
			SourceDisk: proto.String(idetail.Volumes[0].VolumeID),
			DiskSizeGb: proto.Int64(int64(input.SizeGiB * backends.StorageGiB / backends.StorageGB)),
		},
	})
	if err != nil {
		return nil, err
	}
	err = op.Wait(ctx)
	if err != nil {
		return nil, err
	}
	// fill output imageId and labelFingerprint
	iter := client.List(ctx, &computepb.ListImagesRequest{
		Project: s.credentials.Project,
		Filter:  proto.String(LABEL_FILTER_AEROLAB),
	})
	for {
		image, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if *image.Name == input.Name {
			output.Image.ImageId = *image.SelfLink
			id := getImageDetail(output.Image)
			id.LabelFingerprint = *image.LabelFingerprint
			break
		}
	}
	return output, nil
}
