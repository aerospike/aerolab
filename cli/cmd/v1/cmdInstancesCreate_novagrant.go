//go:build novagrant

package cmd

import "github.com/aerospike/aerolab/pkg/backend/backends"

func buildVagrantInstanceParams(_ *InstancesCreateCmd, _ *backends.Image) any {
	return nil
}
