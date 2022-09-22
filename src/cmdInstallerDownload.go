package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type installerDownloadCmd struct {
	aerospikeVersionSelectorCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *installerDownloadCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	log.Print("Running installer.download")
	if err := chDir(c.ChDir); err != nil {
		logFatal("ChDir failed: %s", err)
	}
	var url string
	var err error
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
	log.Print("Done")
	return nil
}
