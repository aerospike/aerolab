package baws

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
)

type imageDetail struct {
	SnapshotID     string `yaml:"snapshotID" json:"snapshotID"`
	RootDeviceName string `yaml:"rootDeviceName" json:"rootDeviceName"`
}

var imageOwners = map[string]string{
	"ubuntu": "099720109477",
	"rocky":  "792107900819",
	"centos": "125523088429",
	"amazon": "137112412989",
	"debian": "136693071363",
}

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

func (s *b) GetImages() (backend.ImageList, error) {
	log := s.log.WithPrefix("GetImages: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backend.ImageList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones) * 2)
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s owned: start", zone)
			defer log.Detail("zone=%s owned: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			paginator := ec2.NewDescribeImagesPaginator(cli, &ec2.DescribeImagesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("tag-key"),
						Values: []string{TAG_AEROLAB_VERSION},
					}, {
						Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
						Values: []string{s.project},
					},
				},
				Owners: []string{"self"},
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, vol := range out.Images {
					tags := make(map[string]string)
					for _, t := range vol.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					state := backend.VolumeStateUnknown
					switch vol.State {
					case types.ImageStateAvailable:
						state = backend.VolumeStateAvailable
					case types.ImageStateDeregistered, types.ImageStateDisabled:
						state = backend.VolumeStateDeleted
					case types.ImageStatePending:
						state = backend.VolumeStateCreating
					case types.ImageStateError, types.ImageStateInvalid, types.ImageStateFailed:
						state = backend.VolumeStateFail
					}
					snap := &types.EbsBlockDevice{}
					if len(vol.BlockDeviceMappings) > 0 {
						snap = vol.BlockDeviceMappings[0].Ebs
					}
					cdstring := *vol.CreationDate
					if len(cdstring) < 19 {
						continue
					}
					cd, _ := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
					arch := backend.ArchitectureX8664
					if vol.Architecture == types.ArchitectureValuesArm64 {
						arch = backend.ArchitectureARM64
					}
					ilock.Lock()
					i = append(i, &backend.Image{
						Name:         tags[TAG_NAME],
						Description:  tags[TAG_DESCRIPTION],
						Owner:        tags[TAG_OWNER],
						OSName:       tags[TAG_OS_NAME],
						OSVersion:    tags[TAG_OS_VERSION],
						ImageId:      aws.ToString(vol.ImageId),
						BackendType:  backend.BackendTypeAWS,
						ZoneName:     zone,
						ZoneID:       zone,
						Architecture: arch,
						Public:       aws.ToBool(vol.Public),
						CreationTime: cd,
						Encrypted:    aws.ToBool(snap.Encrypted),
						InAccount:    true,
						Size:         backend.StorageSize(aws.ToInt32(snap.VolumeSize)) * backend.StorageGiB,
						Tags:         tags,
						State:        state,
						Username:     "root",
						BackendSpecific: &imageDetail{
							SnapshotID:     aws.ToString(snap.SnapshotId),
							RootDeviceName: aws.ToString(vol.RootDeviceName),
						},
					})
					ilock.Unlock()
				}
			}
		}(zone)
	}
	// goroutines to list generic systems which are not custom images
	owners := []string{}
	for _, v := range imageOwners {
		owners = append(owners, v)
	}
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s general: start", zone)
			defer log.Detail("zone=%s general: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			paginator := ec2.NewDescribeImagesPaginator(cli, &ec2.DescribeImagesInput{
				Owners: owners,
			})
			amis := []types.Image{}
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				amis = append(amis, out.Images...)
			}
			images := getImageData(amis)
			for _, image := range images {
				vol := image.Image
				tags := make(map[string]string)
				for _, t := range vol.Tags {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				state := backend.VolumeStateUnknown
				switch vol.State {
				case types.ImageStateAvailable:
					state = backend.VolumeStateAvailable
				case types.ImageStateDeregistered, types.ImageStateDisabled:
					state = backend.VolumeStateDeleted
				case types.ImageStatePending:
					state = backend.VolumeStateCreating
				case types.ImageStateError, types.ImageStateInvalid, types.ImageStateFailed:
					state = backend.VolumeStateFail
				}
				snap := &types.EbsBlockDevice{}
				if len(vol.BlockDeviceMappings) > 0 {
					snap = vol.BlockDeviceMappings[0].Ebs
				}
				arch := backend.ArchitectureARM64
				if image.Architecture == types.ArchitectureTypeX8664 {
					arch = backend.ArchitectureX8664
				}
				ilock.Lock()
				i = append(i, &backend.Image{
					Name:         tags[TAG_NAME],
					Description:  tags[TAG_DESCRIPTION],
					Owner:        aws.ToString(vol.OwnerId),
					ImageId:      aws.ToString(vol.ImageId),
					BackendType:  backend.BackendTypeAWS,
					ZoneName:     zone,
					ZoneID:       zone,
					Architecture: arch,
					Public:       aws.ToBool(vol.Public),
					CreationTime: image.CreateTime,
					Encrypted:    aws.ToBool(snap.Encrypted),
					InAccount:    false,
					Size:         backend.StorageSize(aws.ToInt32(snap.VolumeSize)) * backend.StorageGiB,
					Tags:         tags,
					State:        state,
					OSName:       image.OSName,
					OSVersion:    image.OSVersion,
					Username:     getImageUser(image.OSName, image.OSVersion),
					BackendSpecific: &imageDetail{
						SnapshotID:     aws.ToString(snap.SnapshotId),
						RootDeviceName: aws.ToString(vol.RootDeviceName),
					},
				})
				ilock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	return i, errs
}

func (s *b) ImagesDelete(images backend.ImageList, waitDur time.Duration) error {
	log := s.log.WithPrefix("ImagesDelete: job=" + shortuuid.New() + " ")
	if len(images) == 0 {
		log.Detail("ImageList empty, returning")
		return nil
	}
	volIds := make(map[string]backend.ImageList)
	for _, volume := range images {
		if !volume.InAccount {
			return errors.New("at least one of the provided images is not in the owner's account")
		}
		if _, ok := volIds[volume.ZoneName]; !ok {
			volIds[volume.ZoneName] = backend.ImageList{}
		}
		volIds[volume.ZoneName] = append(volIds[volume.ZoneName], volume)
	}
	log.Detail("Entering goroutines")
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range volIds {
		wg.Add(1)
		go func(zone string, ids backend.ImageList) {
			defer wg.Done()
			log.Detail("Connecting to EC2")
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				golog := log.WithPrefix(zone + "::" + id.ImageId + ": ")
				golog.Detail("Deregistering Image")
				_, err = cli.DeregisterImage(context.TODO(), &ec2.DeregisterImageInput{
					ImageId: aws.String(id.ImageId),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				golog.Detail("Deleting Snapshot")
				_, err = cli.DeleteSnapshot(context.TODO(), &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(id.BackendSpecific.(imageDetail).SnapshotID),
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
					return
				}
				golog.Detail("Done")
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}
