package bgcp

import (
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

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
	return nil, nil
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
	return instances.AddTags(map[string]string{TAG_AEROLAB_EXPIRES: expiry.Format(time.RFC3339)})
}

func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	log := s.log.WithPrefix("VolumesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return volumes.AddTags(map[string]string{TAG_AEROLAB_EXPIRES: expiry.Format(time.RFC3339)}, 2*time.Minute)
}
