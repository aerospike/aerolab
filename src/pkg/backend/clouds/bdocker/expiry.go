package bdocker

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
)

type ExpiryDetail struct {
	LogLevel int `json:"logLevel" yaml:"logLevel"`
}

func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeConfiguration: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// TODO: implement
	return nil
}

// force true means remove previous expiry systems and install new ones
// force false means install only if previous installation was failed or version is different
// onUpdateKeepOriginalSettings true means keep original settings on update, and only apply specified settings on reinstall
func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryInstall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	for _, zone := range zones {
		if !slices.Contains(s.regions, zone) {
			return fmt.Errorf("zone %s is not enabled", zone)
		}
	}

	log.Detail("Getting expiry list")
	esys, err := s.ExpiryList()
	if err != nil {
		return err
	}

	expiryVersion, err := strconv.Atoi(strings.Trim(backends.ExpiryVersion, "\n \t\r"))
	if err != nil {
		return err
	}

	delZones := []string{}
	if force {
		for _, zone := range zones {
			for _, esys := range esys {
				if esys.Zone == zone {
					delZones = append(delZones, zone)
					break
				}
			}
		}
	} else {
		newZones := []string{}
		for _, zone := range zones {
			found := false
			for _, esys := range esys {
				if esys.Zone == zone {
					found = true
					esysVersion, _ := strconv.Atoi(strings.Trim(esys.Version, "\n \t\r"))
					if !esys.InstallationSuccess || esysVersion < expiryVersion {
						delZones = append(delZones, zone)
						newZones = append(newZones, zone)
					}
					break
				}
			}
			if !found {
				newZones = append(newZones, zone)
			}
		}
		zones = newZones
	}

	if len(delZones) > 0 {
		log.Detail("Removing previous expiry systems from zones: " + strings.Join(delZones, ", "))
		err := s.ExpiryRemove(delZones...)
		if err != nil {
			return err
		}
	}
	log.Detail("Installing new expiry systems in zones: " + strings.Join(zones, ", "))

	wg := new(sync.WaitGroup)
	wg.Add(len(zones))
	var reterr error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log := log.WithPrefix("zone=" + zone + " ")
			log.Detail("Start")
			defer log.Detail("End")
			err := s.expiryInstall(zone, log, intervalMinutes, expireEksctl, cleanupDNS, logLevel, onUpdateKeepOriginalSettings, esys, slices.Contains(delZones, zone))
			if err != nil {
				reterr = errors.Join(reterr, err)
			}
		}(zone)
	}
	wg.Wait()
	if reterr != nil {
		return reterr
	}
	return nil
}

func (s *b) expiryInstall(zone string, log *logger.Logger, intervalMinutes int, expireEksctl bool, cleanupDNS bool, logLevel int, onUpdateKeepOriginalSettings bool, esys []*backends.ExpirySystem, isUpdate bool) error {
	_ = zone
	_ = log
	_ = intervalMinutes
	_ = expireEksctl
	_ = cleanupDNS
	_ = logLevel
	_ = onUpdateKeepOriginalSettings
	_ = esys
	_ = isUpdate
	// TODO: implement
	return nil
}

func (s *b) ExpiryRemove(zones ...string) error {
	log := s.log.WithPrefix("ExpiryRemove: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// TODO: implement
	return nil
}

func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	log := s.log.WithPrefix("ExpiryList: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// TODO: implement
	return []*backends.ExpirySystem{}, nil
}

func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeFrequency: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// TODO: implement
	return nil
}

func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	log := s.log.WithPrefix("InstancesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// If expiry is zero, remove the tag to indicate no expiry
	if expiry.IsZero() {
		return instances.RemoveTags([]string{TAG_EXPIRES})
	}
	return instances.AddTags(map[string]string{TAG_EXPIRES: expiry.Format(time.RFC3339)})
}

func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	log := s.log.WithPrefix("VolumesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// If expiry is zero, remove the tag to indicate no expiry
	if expiry.IsZero() {
		return volumes.RemoveTags([]string{TAG_EXPIRES}, 0)
	}
	return volumes.AddTags(map[string]string{TAG_EXPIRES: expiry.Format(time.RFC3339)}, 0)
}

// ExpiryV7Check always returns false for Docker as there was no v7 expiry system for Docker.
func (s *b) ExpiryV7Check() (bool, []string, error) {
	return false, nil, nil
}

// TODO: for docker, the expiry system will also keep updated expiry information as a file either in the container or on the volume itself as /opt/aerolab/expires.json
// TODO: updates instances.go GetInstances() to read the expiry information from the file
// TODO: updates volumes.go GetVolumes() to read the expiry information from the file
