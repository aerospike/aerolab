package main

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func (d *backendAws) lookupAmi(region string, v backendVersion) (ami string, err error) {
	owner := ""
	switch v.distroName {
	case "ubuntu":
		owner = "099720109477"
	case "centos":
		owner = "125523088429"
	case "amazon":
		owner = "137112412989"
	case "debian":
		owner = "136693071363"
	default:
		return "", errors.New("distro name lookup unsupporter")
	}
	arch := "x86_64"
	if v.isArm {
		arch = "arm64"
	}
	out, err := d.ec2svc.DescribeImages(&ec2.DescribeImagesInput{
		IncludeDeprecated: aws.Bool(false),
		Owners:            aws.StringSlice([]string{owner}),
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("architecture"),
				Values: aws.StringSlice([]string{arch}),
			},
			{
				Name:   aws.String("state"),
				Values: aws.StringSlice([]string{"available"}),
			},
		},
	})
	if err != nil {
		return "", err
	}

	imageId := ""
	var creationDate time.Time
	for _, ami := range out.Images {
		if ami.VirtualizationType == nil || *ami.VirtualizationType != "hvm" || ami.Name == nil || ami.ImageId == nil || ami.CreationDate == nil {
			continue
		}
		name := *ami.Name
		switch v.distroName {
		case "debian":
			if !strings.HasPrefix(name, "debian-") {
				continue
			}
			if strings.Contains(name, "-backports-") {
				continue
			}
			vals := strings.Split(name, "-")
			if len(vals) < 3 {
				continue
			}
			if v.distroVersion != vals[1] {
				continue
			}
			if vals[2] != "amd64" && vals[2] != "arm64" {
				continue
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err == nil {
				if cd.After(creationDate) {
					creationDate = cd
					imageId = *ami.ImageId
				}
			}
		case "ubuntu":
			if !strings.HasPrefix(name, "ubuntu/images/hvm-ssd/") {
				continue
			}
			vals := strings.Split(name, "/")
			if len(vals) < 4 {
				continue
			}
			val := vals[3]
			vals = strings.Split(val, "-")
			if len(vals) < 5 {
				continue
			}
			ver := vals[2]
			if v.distroVersion == ver {
				cdstring := *ami.CreationDate
				if len(cdstring) < 19 {
					continue
				}
				cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
				if err == nil {
					if cd.After(creationDate) {
						creationDate = cd
						imageId = *ami.ImageId
					}
				}
			}
		case "centos":
			if !strings.HasPrefix(name, "CentOS ") {
				continue
			}
			vals := strings.Split(name, " ")
			if len(vals) < 3 {
				continue
			}
			if v.distroVersion == "7" && vals[1] != "Linux" {
				continue
			} else if v.distroVersion != "7" && vals[1] != "Stream" {
				continue
			}
			val := vals[2]
			ver := val
			if v.distroVersion == ver {
				cdstring := *ami.CreationDate
				if len(cdstring) < 19 {
					continue
				}
				cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
				if err == nil {
					if cd.After(creationDate) {
						creationDate = cd
						imageId = *ami.ImageId
					}
				}
			}
		case "amazon":
			if v.distroVersion == "2" && ((!(strings.HasPrefix(name, "amzn2-ami-kernel-") || strings.HasPrefix(name, "amzn2-ami-hvm-")) || !strings.Contains(name, "-hvm-") || !strings.HasSuffix(name, "-gp2")) || (!strings.HasSuffix(name, "-x86_64-gp2") && !strings.HasSuffix(name, "-arm64-gp2"))) {
				continue
			} else if v.distroVersion == "2023" && (!strings.HasPrefix(name, "al2023-ami-") || strings.HasPrefix(name, "al2023-ami-minimal-") || (!strings.HasSuffix(name, "-x86_64") && !strings.HasSuffix(name, "-arm64"))) {
				continue
			}
			cdstring := *ami.CreationDate
			if len(cdstring) < 19 {
				continue
			}
			cd, err := time.Parse("2006-01-02T15:04:05", cdstring[0:19])
			if err == nil {
				if cd.After(creationDate) {
					creationDate = cd
					imageId = *ami.ImageId
				}
			}
		}
	}
	if imageId != "" {
		return imageId, nil
	}
	return "", errors.New("not found")
}

func (d *backendAws) getUser(v backendVersion) string {
	switch v.distroName {
	case "debian":
		return "admin"
	case "ubuntu":
		return "ubuntu"
	case "centos":
		switch v.distroVersion {
		case "8", "7":
			return "centos"
		}
		return "ec2-user"
	case "amazon":
		return "ec2-user"
	}
	return "root"
}

func (d *backendAws) getAmi(region string, v backendVersion) (ami string, err error) {
	ret, err := d.lookupAmi(region, v)
	if err == nil {
		return ret, nil
	}
	log.Printf("AMI lookup failed, using prebuilt lists (%s)", err)
	if v.isArm {
		return d.getAmiArm(region, v)
	}
	return d.getAmiAmd(region, v)
}

func (d *backendAws) getAmiAmd(region string, v backendVersion) (ami string, err error) {
	switch region {
	case "eu-central-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-0b584e86060d6e0f6", nil
			case "20.04":
				return "ami-093b7895c0f11f648", nil
			case "18.04":
				return "ami-0d3a4a1a50fc3e2b4", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-0a2dc38dc30ba417e", nil
			case "7":
				return "ami-08b6d44b4f6f7b279", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-08658d5197becde34", nil
			}
		}
	case "eu-west-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-096800910c1b781ba", nil
			case "20.04":
				return "ami-03caf24deed650e2c", nil
			case "18.04":
				return "ami-0c259a97cbf621daf", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-05beabd0fc875ce04", nil
			case "7":
				return "ami-04f5641b0d178a27a", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0f89681a05a3a9de7", nil
			}
		}
	case "us-west-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-02ea247e531eb3ce6", nil
			case "20.04":
				return "ami-04b61997e51f6d5c7", nil
			case "18.04":
				return "ami-0558dde970ca91ee5", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-01c1168b9f7398306", nil
			case "7":
				return "ami-08d2d8b00f270d03b", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-02f24ad9a1d24a799", nil
			}
		}
	case "us-east-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-08c40ec9ead489470", nil
			case "20.04":
				return "ami-04505e74c0741db8d", nil
			case "18.04":
				return "ami-0e472ba40eb589f49", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-05d7cb15bfbf13b6d", nil
			case "7":
				return "ami-00e87074e52e6c9f9", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0ab4d1e9cf9a1215a", nil
			}
		}
	case "ap-south-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-062df10d14676e201", nil
			case "20.04":
				return "ami-0b9e641f013a385af", nil
			case "18.04":
				return "ami-0bd1a64868721e9ef", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-0c8ad4b0ff2d20c79", nil
			case "7":
				return "ami-0ffc7af9c06de0077", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-011c99152163a87ae", nil
			}
		}
	}
	return "", errors.New("distro/version has no known AMI in any of eu-west-1, us-west-1, us-east-1, ap-south-1; specify region and AMI manually if needed")
}

func (d *backendAws) getAmiArm(region string, v backendVersion) (ami string, err error) {
	switch region {
	case "eu-central-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-00a6c4e9199920ae2", nil
			case "20.04":
				return "ami-0ea4bef4b2f572a6a", nil
			case "18.04":
				return "ami-0763d702f397ed271", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-06ccb76c8c2697a57", nil
			case "7":
				return "ami-0a305a7534a53874c", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0aac4d36ec040d2c1", nil
			}
		}
	case "eu-west-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-0a359adcdbc93673e", nil
			case "20.04":
				return "ami-090f0680110154823", nil
			case "18.04":
				return "ami-04be55e3249d3b5b0", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-07d54ca4e98347364", nil
			case "7":
				return "ami-0e2a2f48fffbaa4fa", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-05223a36a9748498d", nil
			}
		}
	case "us-west-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-0f544c46a0f2fa3e2", nil
			case "20.04":
				return "ami-00e55c45cafaec8b3", nil
			case "18.04":
				return "ami-0c84dbe6ecd4a16ea", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-04352074d490ba9b5", nil
			case "7":
				return "ami-09748e99ee14e3823", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0f36e12deb25112d9", nil
			}
		}
	case "us-east-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-015633ed1298a9ba9", nil
			case "20.04":
				return "ami-082babd6b8e20852c", nil
			case "18.04":
				return "ami-0351643488963af72", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-007473542501fb8fa", nil
			case "7":
				return "ami-0b802bd2b502aa382", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0ff3578f8df132330", nil
			}
		}
	case "ap-south-1":
		switch v.distroName {
		case "ubuntu":
			switch v.distroVersion {
			case "22.04":
				return "ami-01ab9f7634483fe77", nil
			case "20.04":
				return "ami-01afd0d5d005c25b5", nil
			case "18.04":
				return "ami-012c804720e756e6f", nil
			}
		case "centos":
			switch v.distroVersion {
			case "8":
				return "ami-070237a0a64c58642", nil
			case "7":
				return "ami-0b5c298137e260867", nil
			}
		case "amazon":
			switch v.distroVersion {
			case "2":
				return "ami-0fc81ccb6d411c58b", nil
			}
		}
	}
	return "", errors.New("distro/version has no known AMI in any of eu-west-1, eu-cental-1, us-west-1, us-east-1, ap-south-1; specify region and AMI manually if needed")
}
