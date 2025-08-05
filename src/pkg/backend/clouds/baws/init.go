package baws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/utils/counters"
	"github.com/aerospike/aerolab/pkg/utils/file"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/rglonek/logger"
)

type b struct {
	configDir           string
	credentials         *clouds.AWS
	regions             []string
	project             string
	sshKeysDir          string
	log                 *logger.Logger
	aerolabVersion      string
	networks            backends.NetworkList
	firewalls           backends.FirewallList
	instances           backends.InstanceList
	volumes             backends.VolumeList
	images              backends.ImageList
	workDir             string
	invalidateCacheFunc func(names ...string) error
	listAllProjects     bool
	createInstanceCount *counters.Int
}

func init() {
	backends.RegisterBackend(backends.BackendTypeAWS, &b{})
}

func (s *b) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	s.networks = networks
	s.firewalls = firewalls
	s.instances = instances
	s.volumes = volumes
	s.images = images
}

func (s *b) SetConfig(dir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error {
	s.configDir = dir
	if credentials != nil {
		s.credentials = &credentials.AWS
	}
	s.project = project
	s.sshKeysDir = sshKeyDir
	s.log = log
	s.aerolabVersion = aerolabVersion
	s.workDir = workDir
	s.invalidateCacheFunc = invalidateCacheFunc
	s.listAllProjects = listAllProjects
	s.createInstanceCount = counters.NewInt(0)
	// read regions
	err := s.setConfigRegions()
	if err != nil {
		return err
	}
	return nil
}

func (s *b) setConfigRegions() error {
	regionsFile := path.Join(s.configDir, "regions.json")
	s.log.Detail("setConfigRegions: looking for %s", regionsFile)
	_, err := os.Stat(regionsFile)
	if err != nil && !os.IsNotExist(err) {
		// error reading
		return err
	}
	if err != nil {
		// file does not exist
		s.log.Detail("setConfigRegions: %s does not exist, not parsing", regionsFile)
		return nil
	}
	// read
	f, err := os.Open(regionsFile)
	if err != nil {
		return err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&s.regions)
	if err != nil {
		return err
	}
	s.log.Detail("setConfigRegions: result=%v", s.regions)
	return nil
}

func (s *b) ListEnabledZones() ([]string, error) {
	return s.regions, nil
}

func (s *b) EnableZones(names ...string) error {
	if len(names) == 0 {
		return nil
	}

	// check cache for valid regions
	regionCacheFile := path.Join(s.configDir, "region-cache.json")
	type regionCache struct {
		Regions     []string  `json:"regions"`
		LastUpdated time.Time `json:"last_updated"`
	}
	rrList := []string{}
	if _, err := os.Stat(regionCacheFile); err == nil {
		f, err := os.Open(regionCacheFile)
		if err != nil {
			return err
		}
		var cache regionCache
		err = json.NewDecoder(f).Decode(&cache)
		f.Close()
		if err != nil {
			return err
		}
		if cache.LastUpdated.Add(24 * time.Hour).After(time.Now()) {
			rrList = cache.Regions
		} else {
			os.Remove(regionCacheFile)
		}
	}

	// get region list from provider
	if len(rrList) == 0 {
		cli, err := getEc2Client(s.credentials, aws.String(names[0]))
		if err != nil {
			if strings.Contains(err.Error(), "no such host") {
				return fmt.Errorf("region %s not found in AWS", names[0])
			}
			return err
		}
		rr, err := cli.DescribeRegions(context.Background(), &ec2.DescribeRegionsInput{
			AllRegions: aws.Bool(true),
		})
		if err != nil {
			return err
		}
		for _, r := range rr.Regions {
			rrList = append(rrList, *r.RegionName)
		}
		// store cache
		err = file.StoreJSON(regionCacheFile, ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regionCache{Regions: rrList, LastUpdated: time.Now()})
		if err != nil {
			return err
		}
	}

	// check if the regions are valid
	for _, name := range names {
		if !slices.Contains(rrList, name) {
			return fmt.Errorf("region %s not found in AWS", name)
		}
	}

	// add missing regions to the list
	regions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	added := false
	for _, r := range names {
		if slices.Contains(regions, r) {
			continue
		}
		regions = append(regions, r)
		added = true
	}
	if added {
		s.instanceTypeCacheInvalidate()
		s.volumePriceCacheInvalidate()
	}
	s.regions = regions
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}

func (s *b) DisableZones(names ...string) error {
	currentRegions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	regions := []string{}
	for _, r := range currentRegions {
		if slices.Contains(names, r) {
			continue
		}
		regions = append(regions, r)
	}
	s.regions = regions
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}

func toInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
