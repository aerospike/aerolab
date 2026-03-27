//go:build !noaws

package cmd

import "github.com/aerospike/aerolab/pkg/backend/clouds/baws"

func buildAWSVolumeParams(c *VolumesCreateCmd) any {
	return &baws.CreateVolumeParams{
		SizeGiB:           c.AWS.SizeGiB,
		Placement:         c.AWS.Placement,
		DiskType:          c.AWS.DiskType,
		Iops:              c.AWS.Iops,
		Throughput:        c.AWS.Throughput,
		Encrypted:         c.AWS.Encrypted,
		SharedDiskOneZone: c.AWS.SharedDiskOneZone,
	}
}

func updateAWSVolumePlacement(params any, region string) {
	if awsParams, ok := params.(*baws.CreateVolumeParams); ok {
		awsParams.Placement = region
	}
}
