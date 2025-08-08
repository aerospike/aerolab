package bdocker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	"github.com/lithammer/shortuuid"
	"gopkg.in/yaml.v3"

	_ "embed"
)

//go:embed distros.yaml
var distros string
var distrosMap map[string][]string

func init() {
	yaml.Unmarshal([]byte(distros), &distrosMap)
}

type ImageDetail struct {
	Docker *image.Summary `json:"docker" yaml:"docker"`
}

func imageNaming(distroName string, distroVersion string, arch string) (templName string) {
	switch distroName {
	case "rocky":
		switch arch {
		case "amd64":
			return "amd64/rockylinux:" + distroVersion
		case "arm64":
			return "arm64v8/rockylinux:" + distroVersion
		default:
			return "rockylinux:" + distroVersion
		}
	case "centos":
		switch distroVersion {
		case "6":
			return "quay.io/centos/centos:6"
		case "7":
			return "quay.io/centos/centos:7"
		default:
			switch arch {
			case "amd64":
				return "quay.io/centos/amd64:stream" + distroVersion // centos stream is dev branch of RHEL; it gets abandoned as soon as RHEL reaches maintenance phase
			case "arm64":
				return "quay.io/centos/arm64v8:stream" + distroVersion // centos stream is dev branch of RHEL; it gets abandoned as soon as RHEL reaches maintenance phase
			default:
				return "quay.io/centos/centos:stream" + distroVersion
			}
		}
	case "ubuntu", "debian":
		switch arch {
		case "amd64":
			return fmt.Sprintf("amd64/%s:%s", distroName, distroVersion)
		case "arm64":
			return fmt.Sprintf("arm64v8/%s:%s", distroName, distroVersion)
		}
		fallthrough
	default:
		return fmt.Sprintf("%s:%s", distroName, distroVersion)
	}
}

func (s *b) GetImages() (backends.ImageList, error) {
	log := s.log.WithPrefix("GetImages: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.ImageList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	var errs error
	wg.Add(len(zones) * 2)

	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s owned: start", zone)
			defer log.Detail("zone=%s owned: end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			f := filters.NewArgs()
			if !s.listAllProjects {
				f.Add("label", TAG_PUBLIC_TEMPLATE+"=true")
			}
			out, err := cli.ImageList(context.Background(), image.ListOptions{
				SharedSize:     true,
				Manifests:      true,
				ContainerCount: true,
				Filters:        f,
			})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for distro, versions := range distrosMap {
				for _, version := range versions {
					for _, arch := range []string{"amd64", "arm64", "default"} {
						archString := arch
						architecture := backends.ArchitectureX8664
						if arch == "arm64" {
							architecture = backends.ArchitectureARM64
						} else if arch == "default" {
							architecture = backends.ArchitectureNative
							if runtime.GOARCH == "amd64" {
								archString = "amd64"
							} else {
								archString = "arm64"
							}
						}
						imageName := imageNaming(distro, version, arch)
						var sdImg *image.Summary
						for _, img := range out {
							// if arch, distro, version match, set sdImg
							if img.Labels[TAG_ARCHITECTURE] == archString && img.Labels[TAG_OS_NAME] == distro && img.Labels[TAG_OS_VERSION] == version && img.Labels[TAG_PUBLIC_NAME] == imageName {
								sdImg = &img
							}
						}
						size := 0.0
						if sdImg != nil {
							size = math.Ceil(float64(sdImg.Size)/1024/1024/1024) * 1024 * 1024 * 1024
						}
						i = append(i, &backends.Image{
							BackendType:  backends.BackendTypeDocker,
							Name:         imageName,
							Description:  "Default image for " + distro + " " + version + " " + arch,
							Size:         backends.StorageSize(size),
							ImageId:      imageName,
							ZoneName:     zone,
							ZoneID:       zone,
							CreationTime: time.Time{},
							Owner:        "",
							Tags:         map[string]string{},
							Encrypted:    false,
							Architecture: architecture,
							Public:       true,
							State:        backends.VolumeStateAvailable,
							OSName:       distro,
							OSVersion:    version,
							InAccount:    sdImg != nil,
							Username:     "root",
							BackendSpecific: &ImageDetail{
								Docker: sdImg,
							},
						})
					}
				}
			}
		}(zone)
	}

	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s owned: start", zone)
			defer log.Detail("zone=%s owned: end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			f := filters.NewArgs()
			if !s.listAllProjects {
				f.Add("label", TAG_AEROLAB_PROJECT+"="+s.project)
			}
			out, err := cli.ImageList(context.Background(), image.ListOptions{
				SharedSize:     true,
				Manifests:      true,
				ContainerCount: true,
				Filters:        f,
			})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for _, image := range out {
				if image.Labels[TAG_AEROLAB_VERSION] == "" {
					continue
				}
				name := ""
				if len(image.RepoTags) > 0 {
					name = image.RepoTags[0]
					if s.isPodman[zone] {
						name = strings.Replace(name, "localhost/", "", 1)
					}
				} else {
					name = image.ID
				}
				arch := backends.ArchitectureX8664
				if image.Labels[TAG_ARCHITECTURE] == "arm64" {
					arch = backends.ArchitectureARM64
				}
				img := image
				ilock.Lock()
				i = append(i, &backends.Image{
					BackendType:  backends.BackendTypeDocker,
					Name:         name,
					Description:  image.Labels[TAG_DESCRIPTION],
					Size:         backends.StorageSize(image.SharedSize),
					ImageId:      image.ID,
					ZoneName:     zone,
					ZoneID:       zone,
					CreationTime: time.Unix(image.Created, 0),
					Owner:        image.Labels[TAG_OWNER],
					Tags:         image.Labels,
					Encrypted:    false,
					Architecture: arch,
					Public:       false,
					State:        backends.VolumeStateAvailable,
					OSName:       image.Labels[TAG_OS_NAME],
					OSVersion:    image.Labels[TAG_OS_VERSION],
					InAccount:    true,
					Username:     "root",
					BackendSpecific: &ImageDetail{
						Docker: &img,
					},
				})
				ilock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.images = i
		s.builderMutex.Lock()
		for _, builders := range s.builders {
			for _, builder := range builders {
				builder.wg.Wait()
			}
		}
		s.builders = make(map[string]map[string]*dockerBuilder)
		s.builderMutex.Unlock()
	}
	return i, errs
}

func (s *b) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	log := s.log.WithPrefix("ImagesDelete: job=" + shortuuid.New() + " ")
	if len(images) == 0 {
		log.Detail("ImageList empty, returning")
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)
	volIds := make(map[string]backends.ImageList)
	for _, volume := range images {
		volume := volume
		if _, ok := volIds[volume.ZoneName]; !ok {
			volIds[volume.ZoneName] = backends.ImageList{}
		}
		volIds[volume.ZoneName] = append(volIds[volume.ZoneName], volume)
	}
	log.Detail("Entering goroutines")
	wg := new(sync.WaitGroup)
	var reterr error
	for zone, ids := range volIds {
		wg.Add(1)
		go func(zone string, ids backends.ImageList) {
			defer wg.Done()
			log.Detail("Connecting to Docker")
			cli, err := s.getDockerClient(zone)
			if err != nil {
				reterr = errors.Join(reterr, err)
				return
			}
			for _, id := range ids {
				if id.InAccount && id.Public && id.BackendSpecific != nil && id.BackendSpecific.(*ImageDetail).Docker != nil {
					customId := id.BackendSpecific.(*ImageDetail).Docker.ID
					golog := log.WithPrefix(zone + "::" + customId + ": ")
					golog.Detail("Deregistering Custom Root Image")
					s.builderMutex.Lock()
					if _, ok := s.builders[zone]; ok {
						delete(s.builders[zone], id.Name)
					}
					s.builderMutex.Unlock()
					_, err = cli.ImageRemove(context.Background(), customId, image.RemoveOptions{
						Force:         true,
						PruneChildren: true,
					})
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
					golog.Detail("Done")
				} else if !id.Public {
					golog := log.WithPrefix(zone + "::" + id.ImageId + ": ")
					golog.Detail("Deregistering Image")
					_, err = cli.ImageRemove(context.Background(), id.ImageId, image.RemoveOptions{
						Force:         true,
						PruneChildren: true,
					})
					if err != nil {
						reterr = errors.Join(reterr, err)
						return
					}
					golog.Detail("Done")
				}
			}
		}(zone, ids)
	}
	wg.Wait()
	return reterr
}

func (s *b) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	log := s.log.WithPrefix("ImagesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(images) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

func (s *b) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	log := s.log.WithPrefix("ImagesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(images) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

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
	tags[TAG_ARCHITECTURE] = input.Instance.Architecture.String()
	output = &backends.CreateImageOutput{
		Image: &backends.Image{
			BackendType:     input.BackendType,
			Name:            input.Name,
			Description:     input.Description,
			Size:            input.SizeGiB * backends.StorageGiB,
			ZoneName:        input.Instance.ZoneName,
			ZoneID:          input.Instance.ZoneID,
			Architecture:    input.Instance.Architecture,
			Public:          false,
			OSName:          input.OSName,
			OSVersion:       input.OSVersion,
			Username:        "root",
			Encrypted:       input.Encrypted,
			State:           backends.VolumeStateAvailable,
			CreationTime:    time.Now(),
			Owner:           input.Owner,
			Tags:            tags,
			BackendSpecific: &ImageDetail{},
			ImageId:         input.Name,
		},
	}
	if input.OSName == "" || input.OSVersion == "" {
		output.Image.OSName = input.Instance.OperatingSystem.Name
		output.Image.OSVersion = input.Instance.OperatingSystem.Version
	}

	cli, err := s.getDockerClient(input.Instance.ZoneName)
	if err != nil {
		return nil, err
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateImage)

	// Create the image
	log.Detail("Creating image")
	nname := input.Name
	if s.isPodman[input.Instance.ZoneName] {
		nname = "localhost/" + nname
	}
	cr, err := cli.ContainerCommit(context.Background(), input.Instance.InstanceID, container.CommitOptions{
		Reference: nname,             // resulting full-name, as in name:tag
		Comment:   input.Description, // could be used as description
		Author:    "aerolab",         // should be owner or aerolab
		Changes:   []string{},        // dockerfile style tweaks, like ENV, CMD, EXPOSE
		Pause:     true,              // whether to pause the container before committing, should be true, always
		Config: &container.Config{
			User: "root", // ex 1001 or nobody; need more explanation
			Env: []string{
				"AEROLAB_OS_NAME=" + input.OSName,
				"AEROLAB_OS_VERSION=" + input.OSVersion,
				"AEROLAB_ARCHITECTURE=" + input.Instance.Architecture.String(),
				"AEROLAB_PROJECT=" + s.project,
				"AEROLAB_VERSION=" + s.aerolabVersion,
				"AEROLAB_OWNER=" + input.Owner,
				"AEROLAB_NAME=" + input.Name,
			}, // KEY=value list of env vars to bake into the image
			Cmd:             nil,                                                     // dockerfile CMD
			WorkingDir:      "/root",                                                 // WORKDIR
			Entrypoint:      strslice.StrSlice{"/usr/local/bin/init-docker-systemd"}, // dockerfile ENTRYPOINT
			NetworkDisabled: false,                                                   // if true, zero network access
			Labels:          tags,                                                    // all our tags and labels
			StopSignal:      "SIGTERM",                                               // custom stop signal, default SIGTERM
			StopTimeout:     nil,                                                     // how long to wait on stop before deciding to forcefully kill the container processes
			Shell:           nil,                                                     // default shell, default is /bin/sh
		},
	})
	if err != nil {
		return nil, err
	}
	output.Image.ImageId = cr.ID
	return output, nil
}
