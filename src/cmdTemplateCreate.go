package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bestmethod/inslice"
)

type templateCreateCmd struct {
	aerospikeVersionSelectorCmd
	NoVacuumOnFail bool                `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation"`
	Aws            clusterCreateCmdAws `no-flag:"true"`
	Gcp            clusterCreateCmdGcp `no-flag:"true"`
	Help           helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func init() {
	addBackendSwitch("template.create", "aws", &a.opts.Template.Create.Aws)
	addBackendSwitch("template.create", "gcp", &a.opts.Template.Create.Gcp)
}

func (c *templateCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	if err := chDir(string(c.ChDir)); err != nil {
		return logFatal("ChDir failed: %s", err)
	}

	log.Print("Running template.create")

	templates, err := b.ListTemplates()
	if err != nil {
		return logFatal("Could not list templates: %s", err)
	}

	// arm fill
	c.Aws.IsArm, err = b.IsSystemArm(c.Aws.InstanceType)
	if err != nil {
		return fmt.Errorf("IsSystemArm check: %s", err)
	}
	c.Gcp.IsArm = c.Aws.IsArm

	var url string
	isArm := c.Aws.IsArm
	if b.Arch() == TypeArchAmd {
		isArm = false
	}
	if b.Arch() == TypeArchArm {
		isArm = true
	}
	bv := &backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}
	if strings.HasPrefix(c.AerospikeVersion.String(), "latest") || strings.HasSuffix(c.AerospikeVersion.String(), "*") || strings.HasPrefix(c.DistroVersion.String(), "latest") {
		url, err = aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version not found: %s", err)
		}
		c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
		c.DistroName = TypeDistro(bv.distroName)
		c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	}

	log.Printf("Distro = %s:%s ; AerospikeVersion = %s", c.DistroName, c.DistroVersion, c.AerospikeVersion)
	verNoSuffix := strings.TrimSuffix(c.AerospikeVersion.String(), "c")
	verNoSuffix = strings.TrimSuffix(verNoSuffix, "f")
	// check if template exists
	inSlice, err := inslice.Reflect(templates, backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}, 1)
	if err != nil {
		return err
	}
	if len(inSlice) != 0 {
		return errors.New("template already exists")
	}
	if url == "" {
		url, err = aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version URL not found: %s", err)
		}
		c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
		c.DistroName = TypeDistro(bv.distroName)
		c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	}

	var edition string
	if strings.HasSuffix(c.AerospikeVersion.String(), "c") {
		edition = "aerospike-server-community"
	} else if strings.HasSuffix(c.AerospikeVersion.String(), "f") {
		edition = "aerospike-server-federal"
	} else {
		edition = "aerospike-server-enterprise"
	}
	archString := ".x86_64"
	if bv.isArm {
		archString = ".arm64"
	}
	fn := edition + "-" + verNoSuffix + "-" + c.DistroName.String() + c.DistroVersion.String() + archString + ".tgz"
	// download file if not exists
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		log.Println("Downloading installer")
		err = downloadFile(url, fn, c.Username, c.Password)
		if err != nil {
			return err
		}
	}

	// make template here
	log.Println("Creating template image")
	stat, err := os.Stat(fn)
	pfilelen := 0
	if err != nil {
		return err
	}
	pfilelen = int(stat.Size())
	packagefile, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer packagefile.Close()
	nFiles := []fileList{}
	nFiles = append(nFiles, fileList{"/root/installer.tgz", packagefile, pfilelen})
	nscript := aerospikeInstallScript[a.opts.Config.Backend.Type+":"+c.DistroName.String()+":"+c.DistroVersion.String()]
	extra := &backendExtra{
		ami:             c.Aws.AMI,
		instanceType:    c.Aws.InstanceType,
		ebs:             c.Aws.Ebs,
		securityGroupID: c.Aws.SecurityGroupID,
		subnetID:        c.Aws.SubnetID,
		publicIP:        c.Aws.PublicIP,
		tags:            c.Aws.Tags,
	}
	if a.opts.Config.Backend.Type == "gcp" {
		extra = &backendExtra{
			instanceType: c.Gcp.InstanceType,
			ami:          c.Gcp.Image,
			publicIP:     c.Gcp.PublicIP,
			tags:         c.Gcp.Tags,
			ebs:          c.Gcp.Disks,
			zone:         c.Gcp.Zone,
			labels:       c.Gcp.Labels,
		}
	}
	err = b.DeployTemplate(*bv, nscript, nFiles, extra)
	if err != nil {
		if !c.NoVacuumOnFail {
			errA := b.VacuumTemplates()
			if errA != nil {
				log.Printf("Failed to vacuum failed template: %s", errA)
			}
		}
		return err
	}

	log.Print("Done")
	return nil
}
