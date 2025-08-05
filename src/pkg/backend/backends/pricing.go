package backends

import "time"

func (b *backend) GetVolumePrices(backendType BackendType) (output VolumePriceList, err error) {
	start := time.Now()
	defer func() {
		b.log.Detail("GetVolumePrices: err=%v, took=%v", err, time.Since(start))
	}()
	output, err = b.enabledBackends[backendType].GetVolumePrices()
	return
}

func (b *backend) GetInstanceTypes(backendType BackendType) (output InstanceTypeList, err error) {
	start := time.Now()
	defer func() {
		b.log.Detail("GetInstanceTypes: err=%v, took=%v", err, time.Since(start))
	}()
	output, err = b.enabledBackends[backendType].GetInstanceTypes()
	return
}
