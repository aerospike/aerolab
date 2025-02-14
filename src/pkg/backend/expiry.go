package backend

type ExpiryList struct {
	ExpirySystems []*ExpirySystem
}

type ExpirySystem struct {
	BackendType     BackendType
	Zone            string
	Version         string
	FunctionName    string
	SchedulerName   string
	BackendSpecific interface{}
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

func (b *backend) ExpiryInstall(backendType BackendType, zones ...string) error {
	return cloudList[backendType].ExpiryInstall(zones...)
}

func (b *backend) ExpiryRemove(backendType BackendType, zones ...string) error {
	return cloudList[backendType].ExpiryRemove(zones...)
}
