package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	"github.com/jessevdk/go-flags"
)

type clientCreateBaseCmd struct {
	ClientName    TypeClientName         `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount   int                    `short:"c" long:"count" description:"Number of clients" default:"1"`
	NoSetHostname bool                   `short:"H" long:"no-set-hostname" description:"by default, hostname of each machine will be set, use this to prevent hostname change"`
	StartScript   flags.Filename         `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Aws           clusterCreateCmdAws    `no-flag:"true"`
	Docker        clusterCreateCmdDocker `no-flag:"true"`
	osSelectorCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateBaseCmd) isGrow() bool {
	if len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow" {
		return true
	}
	return false
}

func (c *clientCreateBaseCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	_, err := c.createBase(args, "base")
	return err
}

func (c *clientCreateBaseCmd) createBase(args []string, nt string) (machines []int, err error) {
	if !c.isGrow() {
		fmt.Println("Running client.create." + nt)
	} else {
		fmt.Println("Running client.grow." + nt)
	}

	var startScriptSize os.FileInfo
	if string(c.StartScript) != "" {
		startScriptSize, err = os.Stat(string(c.StartScript))
		if err != nil {
			logFatal("Early Script does not exist: %s", err)
		}
	}

	if len(string(c.ClientName)) == 0 || len(string(c.ClientName)) > 20 {
		logFatal("Client name must be up to 20 characters long")
	}

	b.WorkOnClients()
	clist, err := b.ClusterList()
	if err != nil {
		return nil, err
	}

	if inslice.HasString(clist, c.ClientName.String()) && !c.isGrow() {
		return nil, errors.New("cluster already exists, did you mean 'grow'?")
	}

	if !inslice.HasString(clist, c.ClientName.String()) && c.isGrow() {
		return nil, errors.New("cluster doesn't exist, did you mean 'create'?")
	}

	totalNodes := c.ClientCount
	var nlic []int
	if c.isGrow() {
		nlic, err = b.NodeListInCluster(string(c.ClientName))
		if err != nil {
			logFatal(err)
		}
		totalNodes += len(nlic)
	}

	if totalNodes > 255 || totalNodes < 1 {
		logFatal("Max node count is 255")
	}

	if totalNodes > 1 && c.Docker.ExposePortsToHost != "" {
		logFatal("Cannot use docker export-ports feature with more than 1 node")
	}

	if err := checkDistroVersion(c.DistroName.String(), c.DistroVersion.String()); err != nil {
		logFatal(err)
	}
	if string(c.DistroVersion) == "latest" {
		c.DistroVersion = TypeDistroVersion(getLatestVersionForDistro(c.DistroName.String()))
	}

	// build extra
	var ep []string
	if c.Docker.ExposePortsToHost != "" {
		ep = strings.Split(c.Docker.ExposePortsToHost, ",")
	}
	extra := &backendExtra{
		cpuLimit:        c.Docker.CpuLimit,
		ramLimit:        c.Docker.RamLimit,
		swapLimit:       c.Docker.SwapLimit,
		privileged:      c.Docker.Privileged,
		exposePorts:     ep,
		switches:        c.Docker.ExtraFlags,
		dockerHostname:  !c.NoSetHostname,
		ami:             c.Aws.AMI,
		instanceType:    c.Aws.InstanceType,
		ebs:             c.Aws.Ebs,
		securityGroupID: c.Aws.SecurityGroupID,
		subnetID:        c.Aws.SubnetID,
		publicIP:        c.Aws.PublicIP,
	}

	isArm := c.Aws.IsArm
	if b.Arch() == TypeArchAmd {
		isArm = false
	}
	if b.Arch() == TypeArchArm {
		isArm = true
	}
	bv := &backendVersion{
		distroName:       string(c.DistroName),
		distroVersion:    string(c.DistroVersion),
		aerospikeVersion: "client",
		isArm:            isArm,
	}
	log.Printf("Distro: %s Version: %s", string(c.DistroName), string(c.DistroVersion))

	err = b.DeployCluster(*bv, string(c.ClientName), c.ClientCount, extra)
	if err != nil {
		return nil, err
	}

	err = b.ClusterStart(string(c.ClientName), nil)
	if err != nil {
		return nil, err
	}

	nodeList, err := b.NodeListInCluster(string(c.ClientName))
	if err != nil {
		return nil, err
	}

	nodeListNew := []int{}
	for _, i := range nodeList {
		if !inslice.HasInt(nlic, i) {
			nodeListNew = append(nodeListNew, i)
		}
	}

	repl := "cd aerospike-server-* && ./asinstall || exit 1"
	repl2 := "cd /root && tar -zxf installer.tgz || exit 1"
	installer := aerospikeInstallScript["aws:"+string(c.DistroName)+":"+string(c.DistroVersion)]
	installer = strings.ReplaceAll(installer, repl, "")
	installer = strings.ReplaceAll(installer, repl2, "")
	err = b.CopyFilesToCluster(c.ClientName.String(), []fileList{fileList{"/opt/install-base.sh", strings.NewReader(installer), len(installer)}}, nodeListNew)
	if err != nil {
		return nil, fmt.Errorf("could not copy install script to nodes: %s", err)
	}
	out, err := b.RunCommands(string(c.ClientName), [][]string{[]string{"/bin/bash", "/opt/install-base.sh"}}, nodeListNew)
	if err != nil {
		nout := ""
		for i, o := range out {
			nout = nout + "\n---- " + strconv.Itoa(i) + " ----\n" + string(o)
		}
		return nil, fmt.Errorf("some installers failed: %s%s", err, out)
	}

	// set hostnames for aws
	if a.opts.Config.Backend.Type == "aws" && !c.NoSetHostname {
		nip, err := b.GetNodeIpMap(string(c.ClientName), false)
		if err != nil {
			return nil, err
		}
		fmt.Println(nip)
		for _, nnode := range nodeListNew {
			hComm := [][]string{
				[]string{"hostname", fmt.Sprintf("%s-%d", string(c.ClientName), nnode)},
			}
			nr, err := b.RunCommands(string(c.ClientName), hComm, []int{nnode})
			if err != nil {
				return nil, fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr, err = b.RunCommands(string(c.ClientName), [][]string{[]string{"sed", "s/" + nip[nnode] + ".*//g", "/etc/hosts"}}, []int{nnode})
			if err != nil {
				return nil, fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr[0] = append(nr[0], []byte(fmt.Sprintf("\n%s %s-%d\n", nip[nnode], string(c.ClientName), nnode))...)
			hst := fmt.Sprintf("%s-%d\n", string(c.ClientName), nnode)
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{fileList{"/etc/hostname", strings.NewReader(hst), len(hst)}}, []int{nnode})
			if err != nil {
				return nil, err
			}
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{fileList{"/etc/hosts", bytes.NewReader(nr[0]), len(nr[0])}}, []int{nnode})
			if err != nil {
				return nil, err
			}
		}
	}

	// install early/late scripts
	if string(c.StartScript) != "" {
		StartScriptFile, err := os.Open(string(c.StartScript))
		if err != nil {
			log.Printf("ERROR: could not install early script: %s", err)
		} else {
			defer StartScriptFile.Close()
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{fileList{"/usr/local/bin/start.sh", StartScriptFile, int(startScriptSize.Size())}}, nodeListNew)
			if err != nil {
				log.Printf("ERROR: could not install early script: %s", err)
			}
		}
	} else {
		emptyStart := "#!/bin/bash\ndate"
		StartScriptFile := strings.NewReader(emptyStart)
		b.CopyFilesToCluster(string(c.ClientName), []fileList{fileList{"/usr/local/bin/start.sh", StartScriptFile, len(emptyStart)}}, nodeListNew)
	}

	log.Println("Done")
	return nodeListNew, nil
}
