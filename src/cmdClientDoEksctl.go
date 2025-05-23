package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/aerospike/aerolab/scripts"
	flags "github.com/rglonek/jeddevdk-goflags"
)

//go:embed eks/*
var eksYamls embed.FS

type clientCreateEksCtlCmd struct {
	clientCreateNoneCmd
	EksAwsRegion          string         `short:"r" long:"eks-aws-region" description:"AWS region to install expiries system too and configure as default region"`
	EksAwsKeyId           string         `short:"k" long:"eks-aws-keyid" description:"AWS Key ID to use for auth when performing eksctl tasks and expiries"`
	EksAwsSecretKey       string         `short:"s" long:"eks-aws-secretkey" description:"AWS Secret Key to use for auth when performing eksctl tasks and expiries"`
	EksAwsInstanceProfile string         `short:"x" long:"eks-aws-profile" description:"AWS instance profile to use instead of KEYID/SecretKey for authentication"`
	FeaturesFilePath      flags.Filename `short:"f" long:"eks-asd-features" description:"Aerospike Features File to copy to the EKSCTL client machine; destination: /root/features.conf"`
	JustDoIt              bool           `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	InstallYamls          bool           `long:"install-yamls" hidden:"true"`
	UpdateBootstrap       bool           `long:"update-bootstrap" hidden:"true"`
	chDirCmd
}

func (c *clientCreateEksCtlCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "24.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || c.DistroVersion != TypeDistroVersion("24.04") {
		return fmt.Errorf("eksctl is only supported on ubuntu:24.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	if strings.HasPrefix(c.EksAwsKeyId, "ENV::") {
		c.EksAwsKeyId = os.Getenv(strings.Split(c.EksAwsKeyId, "::")[1])
	}
	if strings.HasPrefix(c.EksAwsSecretKey, "ENV::") {
		c.EksAwsSecretKey = os.Getenv(strings.Split(c.EksAwsSecretKey, "::")[1])
	}
	script := scripts.GetEksctlBootstrapScript()
	if c.InstallYamls {
		os.Mkdir("/root/eks", 0755)
		err := fs.WalkDir(eksYamls, ".", func(npath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			contents, err := eksYamls.ReadFile(npath)
			if err != nil {
				return err
			}
			contents = bytes.ReplaceAll(contents, []byte("{AWS-REGION}"), []byte(c.EksAwsRegion))
			err = os.WriteFile(path.Join("/root/eks", d.Name()), contents, 0644)
			return err
		})
		if err != nil {
			return err
		}
	}
	if c.UpdateBootstrap {
		if err := os.WriteFile("/usr/local/bin/bootstrap", script, 0755); err != nil {
			return err
		}
	}
	if c.InstallYamls || c.UpdateBootstrap {
		return nil
	}
	//basic checks
	if c.FeaturesFilePath == "" {
		return errors.New("features file must be specified using -f /path/to/features.conf")
	}
	if (c.EksAwsKeyId == "" || c.EksAwsSecretKey == "") && c.EksAwsInstanceProfile == "" {
		return errors.New("either KeyID+SecretKey OR InstanceProfile must be specified; for help see: aerolab client create eksctl help")
	}
	if c.EksAwsRegion == "" {
		return errors.New("AWS region must be specified (use -r AWSREGION)")
	}
	features, err := os.ReadFile(string(c.FeaturesFilePath))
	if err != nil {
		return fmt.Errorf("could not read features file: %s", err)
	}
	//continue
	b.WorkOnClients()
	c.instanceRole = c.EksAwsInstanceProfile
	machines, err := c.createBase(args, "eksctl")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}
	log.Println("Continuing eksctl installation...")
	a.opts.Client.Configure.AeroLab.ClusterName = c.ClientName
	a.opts.Client.Configure.AeroLab.ParallelThreads = c.ParallelThreads
	err = a.opts.Client.Configure.AeroLab.Execute(nil)
	if err != nil {
		return err
	}
	returns := parallelize.MapLimit(machines, c.ParallelThreads, func(node int) error {
		// bootstrap script
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/usr/local/bin/bootstrap", string(script), len(script)}}, []int{node})
		if err != nil {
			return err
		}
		defer backendRestoreTerminal()
		bootstrapParams := []string{"-r", c.EksAwsRegion}
		if c.EksAwsKeyId != "" && c.EksAwsSecretKey != "" {
			bootstrapParams = append(bootstrapParams, "-k", c.EksAwsKeyId, "-s", c.EksAwsSecretKey)
		}
		err = b.AttachAndRun(c.ClientName.String(), node, []string{"/bin/bash", "-c", "chmod 755 /usr/local/bin/bootstrap && /bin/bash /usr/local/bin/bootstrap " + strings.Join(bootstrapParams, " ")}, false)
		if err != nil {
			return err
		}
		// features file
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/root/features.conf", string(features), len(features)}}, []int{node})
		if err != nil {
			return err
		}
		// done
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", machines[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Println("Done")
	log.Println("To configure timezone inside the machine, run: aerolab attach client -n " + c.ClientName + " -- dpkg-reconfigure tzdata")
	log.Println("Attach command: aerolab attach client -n " + c.ClientName)
	log.Println("Usage instructions: https://github.com/aerospike/aerolab/blob/master/docs/eks/README.md")
	return nil
}
