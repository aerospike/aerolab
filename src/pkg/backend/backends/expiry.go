package backends

import (
	_ "embed"
)

//go:generate bash -c "cd ../../expiry && bash compile.sh"
//go:embed expiry.linux.amd64.zip
var ExpiryBinary []byte

//go:embed expiry.version.txt
var ExpiryVersion string

type ExpiryList struct {
	ExpirySystems []*ExpirySystem `yaml:"expirySystems" json:"expirySystems"`
}

type ExpirySystem struct {
	BackendType         BackendType `yaml:"backendType" json:"backendType"`
	Zone                string      `yaml:"zone" json:"zone"`
	Version             string      `yaml:"version" json:"version"`
	InstallationSuccess bool        `yaml:"installationSuccess" json:"installationSuccess"`
	FrequencyMinutes    int         `yaml:"frequencyMinutes" json:"frequencyMinutes"`
	BackendSpecific     interface{} `yaml:"backendSpecific" json:"backendSpecific"`
}

func (b *backend) ExpiryList() (*ExpiryList, error) {
	ret := &ExpiryList{}
	for _, c := range ListBackendTypes() {
		expirySystems, err := cloudList[c].ExpiryList()
		if err != nil {
			return ret, err
		}
		ret.ExpirySystems = append(ret.ExpirySystems, expirySystems...)
	}
	return ret, nil
}

func (b *backend) ExpiryInstall(backendType BackendType, intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	return cloudList[backendType].ExpiryInstall(intervalMinutes, logLevel, expireEksctl, cleanupDNS, force, onUpdateKeepOriginalSettings, zones...)
}

func (b *backend) ExpiryRemove(backendType BackendType, zones ...string) error {
	return cloudList[backendType].ExpiryRemove(zones...)
}

func (b *backend) ExpiryChangeFrequency(backendType BackendType, intervalMinutes int, zones ...string) error {
	return cloudList[backendType].ExpiryChangeFrequency(intervalMinutes, zones...)
}

func (b *backend) ExpiryChangeConfiguration(backendType BackendType, logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	return cloudList[backendType].ExpiryChangeConfiguration(logLevel, expireEksctl, cleanupDNS, zones...)
}
