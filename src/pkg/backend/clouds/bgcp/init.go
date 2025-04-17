package bgcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"slices"
	"strconv"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/aerospike/aerolab/pkg/file"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type b struct {
	configDir           string
	credentials         *clouds.GCP
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
	allZones            []string
}

func init() {
	backends.RegisterBackend(backends.BackendTypeGCP, &b{})
}

func (s *b) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	s.networks = networks
	s.firewalls = firewalls
	s.instances = instances
	s.volumes = volumes
	s.images = images
}

func (s *b) SetConfig(dir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error {
	if credentials == nil {
		return errors.New("credentials are nil")
	}
	if credentials.GCP.Project == "" {
		return errors.New("project is nil")
	}
	if credentials.GCP.AuthMethod == "" {
		return errors.New("auth method is nil")
	}
	s.configDir = dir
	s.credentials = &credentials.GCP
	s.project = project
	s.sshKeysDir = sshKeyDir
	s.log = log
	s.aerolabVersion = aerolabVersion
	s.workDir = workDir
	s.invalidateCacheFunc = invalidateCacheFunc
	s.listAllProjects = listAllProjects
	// read regions
	err := s.setConfigRegions()
	if err != nil {
		return err
	}
	// try to get client - early auth if required
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	zones, err := s.listAllZones()
	if err != nil {
		return err
	}
	s.allZones = zones
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

func (s *b) listAllZones() ([]string, error) {
	log := s.log.WithPrefix("ListAllZones: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()

	ctx := context.Background()
	client, err := compute.NewZonesRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var zones []string
	it := client.List(ctx, &computepb.ListZonesRequest{
		Project: s.credentials.Project,
	})
	for {
		zone, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		zones = append(zones, zone.GetName())
	}
	return zones, nil
}
