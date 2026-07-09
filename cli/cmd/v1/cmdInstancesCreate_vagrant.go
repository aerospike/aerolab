//go:build !novagrant

package cmd

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bvagrant"
)

func buildVagrantInstanceParams(c *InstancesCreateCmd, image *backends.Image) any {
	return &bvagrant.CreateInstanceParams{
		Image:    image,
		Box:      c.Vagrant.Box,
		Provider: c.Vagrant.Provider,
		CPUs:     c.Vagrant.CPUs,
		RAMMB:    c.Vagrant.RAMMB,
		Disks:    c.Vagrant.Disks,
	}
}
