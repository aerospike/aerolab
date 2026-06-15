//go:build !nogcp

package cmd

import (
	"errors"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
)

func buildGCPInstanceParams(c *InstancesCreateCmd, itype string, gcpCustomImageID string) any {
	return &bgcp.CreateInstanceParams{
		Image:              nil,
		NetworkPlacement:   string(c.GCP.Zone),
		VPCName:            string(c.GCP.VPC),
		SubnetName:         c.GCP.Subnet,
		InstanceType:       itype,
		Disks:              c.GCP.Disks,
		Firewalls:          c.GCP.Firewalls,
		SpotInstance:       c.GCP.SpotInstance,
		DisablePublicIP:    c.GCP.DisablePublicIP,
		IAMInstanceProfile: c.GCP.IAMInstanceProfile,
		CustomDNS:          c.GCP.CustomDNS.makeInstanceDNS(),
		MinCpuPlatform:     c.GCP.MinCPUPlatform,
		GVNIC:              c.GCP.GVNIC,
		CustomImageID:      gcpCustomImageID,
		OnHostMaintenance:  c.GCP.OnHostMaintenance,
	}
}

func resolveGCPInstanceImage(params any, inventory *backends.Inventory, imageName string, narch backends.Architecture) error {
	gcpParams := params.(*bgcp.CreateInstanceParams)
	img := inventory.Images.WithName(imageName).WithArchitecture(narch).Describe()
	if img.Count() == 0 {
		return errors.New("gcp: image " + imageName + " not found with architecture " + narch.String())
	}
	gcpParams.Image = img[0]
	return nil
}

func getGCPInstanceImage(params any) *backends.Image {
	if gcpParams, ok := params.(*bgcp.CreateInstanceParams); ok && gcpParams.Image != nil {
		return gcpParams.Image
	}
	return nil
}
