//go:build nogcp

package cmd

import "github.com/aerospike/aerolab/pkg/backend/backends"

func buildGCPInstanceParams(_ *InstancesCreateCmd, _ string, _ string) any {
	return nil
}

func resolveGCPInstanceImage(_ any, _ *backends.Inventory, _ string, _ backends.Architecture) error {
	return nil
}

func getGCPInstanceImage(_ any) *backends.Image {
	return nil
}
