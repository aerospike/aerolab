package main

import "errors"

/*
func (d *backendDocker) getAmi(filter string) (ami string, err error) {
	out, err := exec.Command("aws", "ec2", "describe-images", "--owners", "099720109477", "--filters", filter, "Name=state,Values=available", "--query", "reverse(sort_by(Images, &CreationDate))[:1].ImageId", "--output", "text").CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return strings.Trim(string(out), " \r\n"), nil
}
*/

func (d *backendAws) getUser(v backendVersion) string {
	switch v.distroName {
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
	switch region {
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
