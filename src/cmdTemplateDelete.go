package main

import (
	"errors"
	"log"

	"github.com/bestmethod/inslice"
)

type templateDeleteCmd struct {
	AerospikeVersion TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Aerospike server version (or 'all')"`
	DistroName       TypeDistro           `short:"d" long:"distro" description:"Linux distro, one of: debian|ubuntu|centos|rocky|amazon (or 'all')"`
	DistroVersion    TypeDistroVersion    `short:"i" long:"distro-version" description:"ubuntu:24.04|22.04|20.04|18.04 rocky:9|8 centos:9|7 amazon:2|2023 debian:12|11|10|9|8 (or 'all')"`
	Aws              templateDeleteCmdAws `no-flag:"true"`
	Gcp              templateDeleteCmdAws `no-flag:"true"`
	Help             helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type templateDeleteCmdAws struct {
	IsArm bool `long:"arm" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("template.destroy", "aws", &a.opts.Template.Delete.Aws)
	addBackendSwitch("template.destroy", "gcp", &a.opts.Template.Delete.Gcp)
}

func (c *templateDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	if c.AerospikeVersion == "" || c.DistroName == "" || c.DistroVersion == "" {
		return errors.New("--aerospike-version, --distro, --distro-version must all be specified; set any parameter to 'all' for all inclusive")
	}

	log.Print("Running template.destroy")
	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "gcp" {
		isArm = c.Gcp.IsArm
	}
	if b.Arch() == TypeArchAmd {
		isArm = false
	}
	if b.Arch() == TypeArchArm {
		isArm = true
	}
	// check template exists
	versions, err := b.ListTemplates()
	if err != nil {
		return err
	}

	if c.DistroName != "all" && c.DistroVersion != "all" && c.AerospikeVersion != "all" {
		v := backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}

		inSlice, err := inslice.Reflect(versions, v, 1)
		if err != nil {
			return err
		}
		if len(inSlice) == 0 {
			err = errors.New("template does not exist; run: template list")
			return err
		}

		log.Printf("Destroying %s on %s:%s", v.aerospikeVersion, v.distroName, v.distroVersion)
		err = b.TemplateDestroy(v)
		if err != nil {
			return err
		}
		log.Print("Done")
		return nil
	}

	var nerr error
	for _, v := range versions {
		if v.isArm != isArm {
			continue
		}
		if c.DistroName == "all" || c.DistroName.String() == v.distroName {
			if c.DistroVersion == "all" || c.DistroVersion.String() == v.distroVersion {
				if c.AerospikeVersion == "all" || c.AerospikeVersion.String() == v.aerospikeVersion {
					log.Printf("Destroying %s on %s:%s", v.aerospikeVersion, v.distroName, v.distroVersion)
					err = b.TemplateDestroy(v)
					if err != nil {
						log.Printf("ERROR: %s", err)
						nerr = err
					}
				}
			}
		}
	}
	if nerr != nil {
		return errors.New("encountered errors")
	}
	log.Print("Done")
	return nil
}
