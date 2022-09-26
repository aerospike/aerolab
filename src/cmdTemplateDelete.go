package main

import (
	"errors"
	"log"

	"github.com/bestmethod/inslice"
)

type templateDeleteCmd struct {
	AerospikeVersion TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Aerospike server version (or 'all')"`
	DistroName       TypeDistro           `short:"d" long:"distro" description:"Linux distro, one of: ubuntu|centos|amazon (or 'all')"`
	DistroVersion    TypeDistroVersion    `short:"i" long:"distro-version" description:"ubuntu:22.04|20.04|18.04 centos:8|7 amazon:2 (or 'all')"`
	Help             helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running template.destroy")
	// check template exists
	versions, err := b.ListTemplates()
	if err != nil {
		return err
	}

	if c.DistroName != "all" && c.DistroVersion != "all" && c.AerospikeVersion != "all" {
		v := backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String()}

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
