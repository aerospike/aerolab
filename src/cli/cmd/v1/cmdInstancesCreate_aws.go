//go:build !noaws

package cmd

import (
	"errors"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
)

func buildAWSInstanceParams(c *InstancesCreateCmd, itype string, awsCustomImageID string) any {
	return &baws.CreateInstanceParams{
		Image:              nil,
		NetworkPlacement:   c.AWS.NetworkPlacement,
		InstanceType:       itype,
		Disks:              c.AWS.Disks,
		Firewalls:          c.AWS.Firewalls,
		SpotInstance:       c.AWS.SpotInstance,
		DisablePublicIP:    c.AWS.DisablePublicIP,
		IAMInstanceProfile: c.AWS.IAMInstanceProfile,
		CustomDNS:          c.AWS.CustomDNS.makeInstanceDNS(),
		CustomImageID:      awsCustomImageID,
	}
}

func resolveAWSInstanceImage(params any, inventory *backends.Inventory, imageID string, narch backends.Architecture) error {
	awsParams := params.(*baws.CreateInstanceParams)
	if strings.HasPrefix(imageID, "ami-") {
		awsParams.Image = inventory.Images.WithImageID(imageID).Describe()[0]
	} else {
		img := inventory.Images.WithName(imageID).WithArchitecture(narch).Describe()
		if img.Count() == 0 {
			return errors.New("aws: image Name " + imageID + " not found with architecture " + narch.String())
		}
		awsParams.Image = img[0]
	}
	return nil
}

func getAWSInstanceImage(params any) *backends.Image {
	if awsParams, ok := params.(*baws.CreateInstanceParams); ok && awsParams.Image != nil {
		return awsParams.Image
	}
	return nil
}
