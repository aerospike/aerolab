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
	Aws  clusterCreateCmdAws `no-flag:"true"`
	Help helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func init() {
	addBackendSwitch("template.create", "aws", &a.opts.Template.Create.Aws)
}

func (c *templateCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	if err := chDir(string(c.ChDir)); err != nil {
		logFatal("ChDir failed: %s", err)
	}

	log.Print("Running template.create")

	if a.opts.Config.Backend.Type == "aws" {
		if c.Aws.SecurityGroupID == "" || c.Aws.SubnetID == "" {
			logFatal("AWS backend requires SecurityGroupID and SubnetID to be specified")
		}
	}

	templates, err := b.ListTemplates()
	if err != nil {
		logFatal("Could not list templates: %s", err)
	}

	var url string
	bv := &backendVersion{c.DistroName, c.DistroVersion, c.AerospikeVersion}
	if strings.HasPrefix(c.AerospikeVersion, "latest") || strings.HasSuffix(c.AerospikeVersion, "*") || strings.HasPrefix(c.DistroVersion, "latest") {
		url, err = aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version not found: %s", err)
		}
		c.AerospikeVersion = bv.aerospikeVersion
		c.DistroName = bv.distroName
		c.DistroVersion = bv.distroVersion
	}

	log.Printf("Distro = %s:%s ; AerospikeVersion = %s", c.DistroName, c.DistroVersion, c.AerospikeVersion)
	verNoSuffix := strings.TrimSuffix(c.AerospikeVersion, "c")
	// check if template exists
	inSlice, err := inslice.Reflect(templates, backendVersion{c.DistroName, c.DistroVersion, c.AerospikeVersion}, 1)
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
		c.AerospikeVersion = bv.aerospikeVersion
		c.DistroName = bv.distroName
		c.DistroVersion = bv.distroVersion
	}

	var edition string
	if strings.HasSuffix(c.AerospikeVersion, "c") {
		edition = "aerospike-server-community"
	} else {
		edition = "aerospike-server-enterprise"
	}
	fn := edition + "-" + verNoSuffix + "-" + c.DistroName + c.DistroVersion + ".tgz"
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
	nscript := aerospikeInstallScript[a.opts.Config.Backend.Type+":"+c.DistroName+":"+c.DistroVersion]
	extra := &backendExtra{
		ami:             c.Aws.AMI,
		instanceType:    c.Aws.InstanceType,
		ebs:             c.Aws.Ebs,
		securityGroupID: c.Aws.SecurityGroupID,
		subnetID:        c.Aws.SubnetID,
		publicIP:        c.Aws.PublicIP,
	}
	err = b.DeployTemplate(*bv, nscript, nFiles, extra)
	if err != nil {
		return err
	}

	log.Print("Done")
	return nil
}