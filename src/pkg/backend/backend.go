package backend

type BackendType string

const (
	BackendTypeAWS    = "aws"
	BackendTypeGCP    = "gcp"
	BackendTypeDocker = "docker"
)

func (b *backend) poll() []error {
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	var errs []error

	vols, err := AWSGetVolumes()
	if err != nil {
		errs = append(errs, err)
	} else {
		b.volumes[BackendTypeAWS] = vols
	}

	inst, err := AWSGetInstances()
	if err != nil {
		errs = append(errs, err)
	} else {
		b.instances[BackendTypeAWS] = inst
	}

	// TODO: if caching enabled, store inventory in cache file
	return errs
}

// TODO: placeholders so it will compile
func AWSGetVolumes() (VolumeList, error) {
	return nil, nil
}
func AWSGetInstances() (InstanceList, error) {
	return nil, nil
}
