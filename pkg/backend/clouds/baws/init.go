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
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds"
	"github.com/citrusleaf/aerolab/pkg/sshexec"
	"github.com/citrusleaf/aerolab/pkg/utils/counters"
	"github.com/citrusleaf/aerolab/pkg/utils/file"
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
	hostKeys            *sshexec.HostKeyStore
	hostKeysStrict      bool
	identity            *backends.Identity
	// Serialize the ensure-then-create of per-user default security groups so
	// concurrent callers do not race each other into a duplicate group.
	defaultFWCreateLock sync.Mutex
	// Serialize caller-access checks, and remember the VPCs whose ingress has
	// already been reconciled: the caller's address is resolved once per
	// process, so it cannot drift underneath us.
	callerAccessLock  sync.Mutex
	callerAccessReady map[string]bool
	// Guard the refresh of the on-disk pricing caches. The Pricing API is a
	// single low-TPS global endpoint, so concurrent callers that all miss the
	// cache must not each start their own full pagination.
	instanceTypesFetch sync.Mutex
	volumePricesFetch  sync.Mutex
}

func init() {
	backends.RegisterBackend(backends.BackendTypeAWS, &b{})
}

func (s *b) SetHostKeyPolicy(store *sshexec.HostKeyStore, strict bool) {
	s.hostKeys = store
	s.hostKeysStrict = strict
}

func (s *b) SetIdentity(identity *backends.Identity) {
	s.identity = identity
}

// applyHostKeyPolicy points an SSH client config at the host key store so the
// connection is verified against the key previously learned for this instance.
func (s *b) applyHostKeyPolicy(clientConf *sshexec.ClientConf, i *backends.Instance) {
	if s.hostKeys == nil {
		return
	}
	id := i.HostKeyID()
	if id == "" {
		return
	}
	clientConf.HostKeyStore = s.hostKeys
	clientConf.HostKeyID = id
	clientConf.HostKeyStrict = s.hostKeysStrict
	if s.log != nil {
		clientConf.HostKeyLogf = s.log.Warn
	}
}

// forgetHostKeys drops remembered host keys for the given instances. Called
// when instances are created or terminated, so a node number that comes back
// on fresh hardware is learned again instead of tripping a mismatch.
func (s *b) forgetHostKeys(ids ...string) {
	if s.hostKeys == nil || len(ids) == 0 {
		return
	}
	if err := s.hostKeys.Forget(ids...); err != nil && s.log != nil {
		s.log.Warn("Could not update the SSH host key store: %s", err)
	}
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
		// No file means nothing is enabled. This backend is a process-global
		// singleton, so leaving s.regions as-is would carry regions over from a
		// previously configured root dir. EnableZones writes the file
		// synchronously, so the file is the source of truth.
		s.log.Detail("setConfigRegions: %s does not exist, clearing enabled regions", regionsFile)
		s.regions = nil
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

func (s *b) ListAvailableZones() ([]string, error) {
	if len(s.regions) == 0 {
		return nil, nil
	}
	return s.getAllRegions(s.regions[0])
}

func (s *b) EnableZones(names ...string) error {
	if len(names) == 0 {
		return nil
	}

	rrList, err := s.getAllRegions(names[0])
	if err != nil {
		return err
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

func (s *b) getAllRegions(name string) ([]string, error) {
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
			return nil, err
		}
		var cache regionCache
		err = json.NewDecoder(f).Decode(&cache)
		f.Close()
		if err != nil {
			return nil, err
		}
		if cache.LastUpdated.Add(24 * time.Hour).After(time.Now()) {
			rrList = cache.Regions
		} else {
			os.Remove(regionCacheFile)
		}
	}

	// get region list from provider
	if len(rrList) == 0 {
		cli, err := getEc2Client(s.credentials, aws.String(name))
		if err != nil {
			if strings.Contains(err.Error(), "no such host") {
				return nil, fmt.Errorf("region %s not found in AWS", name)
			}
			return nil, err
		}
		rr, err := cli.DescribeRegions(context.Background(), &ec2.DescribeRegionsInput{
			AllRegions: aws.Bool(true),
		})
		if err != nil {
			return nil, err
		}
		for _, r := range rr.Regions {
			rrList = append(rrList, *r.RegionName)
		}
		// store cache
		err = file.StoreJSON(regionCacheFile, ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regionCache{Regions: rrList, LastUpdated: time.Now()})
		if err != nil {
			return nil, err
		}
	}
	return rrList, nil
}
