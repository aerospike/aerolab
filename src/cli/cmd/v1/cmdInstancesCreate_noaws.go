//go:build noaws

package cmd

import "github.com/aerospike/aerolab/pkg/backend/backends"

func buildAWSInstanceParams(_ *InstancesCreateCmd, _ string, _ string) any {
	return nil
}

func resolveAWSInstanceImage(_ any, _ *backends.Inventory, _ string, _ backends.Architecture) error {
	return nil
}

func getAWSInstanceImage(_ any) *backends.Image {
	return nil
}
