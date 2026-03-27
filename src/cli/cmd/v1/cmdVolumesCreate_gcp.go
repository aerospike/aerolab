//go:build !nogcp

package cmd

import "github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"

func buildGCPVolumeParams(c *VolumesCreateCmd) any {
	return &bgcp.CreateVolumeParams{
		SizeGiB:    c.GCP.SizeGiB,
		Placement:  string(c.GCP.Zone),
		DiskType:   c.GCP.DiskType,
		Iops:       c.GCP.Iops,
		Throughput: c.GCP.Throughput,
	}
}
