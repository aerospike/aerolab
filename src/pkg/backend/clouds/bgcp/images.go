package bgcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

type ImageDetail struct {
	LabelFingerprint string `yaml:"labelFingerprint" json:"labelFingerprint"`
}

var imageOwners = map[string]string{
	"ubuntu": "099720109477",
	"rocky":  "792107900819",
	"centos": "125523088429",
	"amazon": "137112412989",
	"debian": "136693071363",
}

/*
func getImageUser(osName string, osVersion string) string {
	switch osName {
	case "debian":
		return "admin"
	case "ubuntu":
		return "ubuntu"
	case "centos":
		switch osVersion {
		case "7":
			return "centos"
		}
		return "ec2-user"
	case "rocky":
		return "rocky"
	case "amazon":
		return "ec2-user"
	}
	return "root"
}
*/

type imageData struct {
	OSName       string
	OSVersion    string
	Architecture types.ArchitectureType
	Image        *types.Image
	CreateTime   time.Time
}

func getImageDataMerge(data []*imageData, ami *types.Image, osName string, osVersion string, arch types.ArchitectureType, cd time.Time) []*imageData {
	for _, id := range data {
		if id.Architecture != arch {
			continue
		}
		if id.OSName != osName {
			continue
		}
		if id.OSVersion != osVersion {
			continue
		}
		if cd.After(id.CreateTime) {
			id.CreateTime = cd
			id.Image = ami
		}
		return data
	}
	return append(data, &imageData{
		Architecture: arch,
		OSName:       osName,
		OSVersion:    osVersion,
		CreateTime:   cd,
		Image:        ami,
	})
}

func getImageData(amis []types.Image) (data []*imageData) {
	for _, ami := range amis {
		ami := ami
		if ami.VirtualizationType != "hvm" || ami.Name == nil || ami.ImageId == nil || ami.CreationDate == nil {
			continue
		}
		name := *ami.Name
		switch aws.ToString(ami.OwnerId) {
		case imageOwners["debian"]:
			if !strings.HasPrefix(name, "debian-") {
				continue
			}
			if strings.Contains(name, "-backports-") {
				continue
			}
			vals := strings.Split(name, "-")
			if len(vals) < 3 {
				continue
			}
			osVersion := vals[1]
			var arch types.ArchitectureType
			if vals[2] == "amd64" {
				arch = types.ArchitectureTypeX8664
			} else if vals[2] == "arm64" {
				arch = types.ArchitectureTypeArm64
			} else {
				continue
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err != nil {
				continue
			}
			data = getImageDataMerge(data, &ami, "debian", osVersion, arch, cd)
		case imageOwners["ubuntu"]:
			if !strings.HasPrefix(name, "ubuntu/images/hvm-ssd/") && !strings.HasPrefix(name, "ubuntu/images/hvm-ssd-gp3/") {
				continue
			}
			vals := strings.Split(name, "/")
			if len(vals) < 4 {
				continue
			}
			val := vals[3]
			vals = strings.Split(val, "-")
			if len(vals) < 5 {
				continue
			}
			arch := types.ArchitectureTypeX8664
			if strings.Contains(name, "-arm64-") {
				arch = types.ArchitectureTypeArm64
			}
			osVer := vals[2]
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err != nil {
				continue
			}
			data = getImageDataMerge(data, &ami, "ubuntu", osVer, arch, cd)
		case imageOwners["centos"]:
			if !strings.HasPrefix(name, "CentOS ") {
				continue
			}
			vals := strings.Split(name, " ")
			if len(vals) < 3 {
				continue
			}
			osVer := vals[2]
			if osVer == "7" && vals[1] != "Linux" {
				continue
			} else if osVer != "7" && vals[1] != "Stream" {
				continue
			}
			arch := types.ArchitectureTypeX8664
			if strings.Contains(name, " aarch64 ") {
				arch = types.ArchitectureTypeArm64
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err != nil {
				continue
			}
			data = getImageDataMerge(data, &ami, "centos", osVer, arch, cd)
		case imageOwners["rocky"]:
			if !strings.HasPrefix(name, "Rocky-") {
				continue
			}
			vals := strings.Split(name, "-")
			if len(vals) < 6 {
				continue
			}
			if vals[2] != "EC2" || vals[3] != "Base" {
				continue
			}
			osVer := vals[1]
			arch := types.ArchitectureTypeX8664
			if strings.HasSuffix(name, ".aarch64") {
				arch = types.ArchitectureTypeArm64
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err != nil {
				continue
			}
			data = getImageDataMerge(data, &ami, "rocky", osVer, arch, cd)
		case imageOwners["amazon"]:
			osVer := ""
			var arch types.ArchitectureType
			if strings.HasPrefix(name, "al2023-ami-") && !strings.HasPrefix(name, "al2023-ami-minimal-") && (strings.HasSuffix(name, "-x86_64") || strings.HasSuffix(name, "-arm64")) {
				osVer = "2023"
				if strings.HasSuffix(name, "-x86_64") {
					arch = types.ArchitectureTypeX8664
				} else {
					arch = types.ArchitectureTypeArm64
				}
			} else if (strings.HasPrefix(name, "amzn2-ami-kernel-") || strings.HasPrefix(name, "amzn2-ami-hvm-")) && strings.Contains(name, "-hvm-") && (strings.HasSuffix(name, "-x86_64-gp2") || strings.HasSuffix(name, "-arm64-gp2")) {
				osVer = "2"
				if strings.HasSuffix(name, "-x86_64-gp2") {
					arch = types.ArchitectureTypeX8664
				} else {
					arch = types.ArchitectureTypeArm64
				}
			} else {
				continue
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err != nil {
				continue
			}
			data = getImageDataMerge(data, &ami, "amazon", osVer, arch, cd)
		}
	}
	return data
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
			tags := &metadata{}
			err = tags.decodeFromLabels(image.Labels)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			arch := backends.ArchitectureX8664
			if *image.Architecture == "ARM64" {
				arch = backends.ArchitectureARM64
			}
			im := &backends.Image{
				BackendType:  backends.BackendTypeGCP,
				Name:         *image.Name,
				Description:  *image.Description,
				Size:         backends.StorageSize(*image.DiskSizeGb) * backends.StorageGB,
				ImageId:      *image.SelfLink,
				ZoneName:     strings.Join(image.StorageLocations, ","),
				ZoneID:       strings.Join(image.StorageLocations, ","),
				CreationTime: ctime,
				Owner:        tags.Owner,
				Tags:         tags.Custom,
				Encrypted:    image.ImageEncryptionKey != nil,
				Architecture: arch,
				Public:       false,
				State:        backends.VolumeStateAvailable,
				OSName:       tags.OsName,
				OSVersion:    tags.OsVersion,
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, project := range []string{"ubuntu-os-cloud", "debian-cloud", "centos-cloud", "rocky-linux-cloud"} {
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
				arch := backends.ArchitectureX8664
				if *image.Architecture == "ARM64" {
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
					Name:         *image.Name,
					Description:  *image.Description,
					Size:         backends.StorageSize(*image.DiskSizeGb) * backends.StorageGB,
					ImageId:      *image.SelfLink,
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
				i = append(i, im)
				ilock.Unlock()
			}
		}
	}()

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
	log.Detail("Waiting for operations to complete")
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
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
		op, err := client.Patch(ctx, &computepb.PatchImageRequest{
			Image: image.Name,
			ImageResource: &computepb.Image{
				LabelFingerprint: proto.String(image.BackendSpecific.(*ImageDetail).LabelFingerprint),
				Labels:           newTags,
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
		op, err := client.Patch(ctx, &computepb.PatchImageRequest{
			Image: image.Name,
			ImageResource: &computepb.Image{
				LabelFingerprint: proto.String(image.BackendSpecific.(*ImageDetail).LabelFingerprint),
				Labels:           newTags,
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

/*
func (s *b) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (output *backends.CreateImageOutput, err error) {
	log := s.log.WithPrefix("CreateImage: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	tags := make(map[string]string)
	maps.Copy(tags, input.Tags)
	tags[TAG_NAME] = input.Name
	tags[TAG_DESCRIPTION] = input.Description
	tags[TAG_OS_NAME] = input.OSName
	tags[TAG_OS_VERSION] = input.OSVersion
	tags[TAG_OWNER] = input.Owner
	tags[TAG_AEROLAB_PROJECT] = s.project
	tags[TAG_AEROLAB_VERSION] = s.aerolabVersion
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
			Tags:         tags,
			BackendSpecific: &ImageDetail{
				SnapshotID:       "", // will be set later
				RootDeviceName:   "", // will be set later
				LabelFingerprint: "", // will be set later
			},
			ImageId: "", // will be set later
		},
	}
	if input.OSName == "" || input.OSVersion == "" {
		output.Image.OSName = input.Instance.OperatingSystem.Name
		output.Image.OSVersion = input.Instance.OperatingSystem.Version
	}

	cli, err := getEc2Client(s.credentials, &input.Instance.ZoneName)
	if err != nil {
		return nil, err
	}

	// Convert tags map to AWS tags
	tagsOut := []types.Tag{}
	for k, v := range tags {
		tagsOut = append(tagsOut, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	// Create the image
	log.Detail("Creating image")
	bdm := types.BlockDeviceMapping{
		DeviceName: aws.String(input.Instance.BackendSpecific.(*InstanceDetail).Volumes[0].Device),
		Ebs: &types.EbsBlockDevice{
			DeleteOnTermination: aws.Bool(true),
			Encrypted:           aws.Bool(input.Encrypted),
		},
	}
	if input.SizeGiB > 0 {
		bdm.Ebs.VolumeSize = aws.Int32(int32(input.SizeGiB))
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)
	resp, err := cli.CreateImage(context.TODO(), &ec2.CreateImageInput{
		Name:        aws.String(input.Name),
		InstanceId:  aws.String(input.Instance.InstanceID),
		Description: aws.String(input.Description),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeImage,
				Tags:         tagsOut,
			},
		},
		BlockDeviceMappings: []types.BlockDeviceMapping{bdm},
	})
	if err != nil {
		return output, err
	}

	output.Image.ImageId = aws.ToString(resp.ImageId)
	output.Image.BackendSpecific = &ImageDetail{
		SnapshotID:     "",
		RootDeviceName: input.Instance.BackendSpecific.(*InstanceDetail).Volumes[0].Device,
	}

	// Wait for the image to be created
	if waitDur > 0 {
		log.Detail("Waiting for image to be created")
		waiter := ec2.NewImageAvailableWaiter(cli, func(o *ec2.ImageAvailableWaiterOptions) {
			o.MinDelay = 5 * time.Second
			o.MaxDelay = 5 * time.Second
		})
		err = waiter.Wait(context.TODO(), &ec2.DescribeImagesInput{
			ImageIds: []string{aws.ToString(resp.ImageId)},
		}, waitDur)
		if err != nil {
			return output, err
		}
	}

	return output, nil
}
*/
