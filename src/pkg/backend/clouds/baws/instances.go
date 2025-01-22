package baws

import "github.com/aerospike/aerolab/pkg/backend"

func (s *b) GetInstances() (backend.InstanceList, error) {
	// TODO: actually get instances from AWS
	return nil, nil
}

func (s *b) InstancesAddTags(instances backend.InstanceList, tags map[string]string, waitDur int) error {
	return nil
}

func (s *b) InstancesRemoveTags(instances backend.InstanceList, tagKeys []string, waitDur int) error {
	return nil
}

func (s *b) InstancesTerminate(instances backend.InstanceList, waitDur int) error {
	return nil
}

func (s *b) InstancesStop(instances backend.InstanceList, waitDur int) error {
	return nil
}

func (s *b) InstancesStart(instances backend.InstanceList, waitDur int) error {
	return nil
}

// TODO: each action must update the underlying instance state and save cache
