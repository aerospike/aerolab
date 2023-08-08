package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateBaseCmd struct {
	ClientName    TypeClientName         `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount   int                    `short:"c" long:"count" description:"Number of clients" default:"1"`
	NoSetHostname bool                   `short:"H" long:"no-set-hostname" description:"by default, hostname of each machine will be set, use this to prevent hostname change"`
	StartScript   flags.Filename         `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Aws           clusterCreateCmdAws    `no-flag:"true"`
	Gcp           clusterCreateCmdGcp    `no-flag:"true"`
	Docker        clusterCreateCmdDocker `no-flag:"true"`
	osSelectorCmd
	ParallelThreads int    `long:"threads" description:"Run on this many nodes in parallel" default:"50"`
	PriceOnly       bool   `long:"price" description:"Only display price of ownership; do not actually create the cluster"`
	Owner           string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
}

func (c *clientCreateBaseCmd) isGrow() bool {
	if len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow" {
		return true
	}
	return false
}

func (c *clientCreateBaseCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	if inslice.HasString(args, "help") {
		if a.opts.Config.Backend.Type == "docker" {
			printHelp("The aerolab command can be optionally followed by '--' and then extra switches that will be passed directory to Docker. Ex: aerolab cluster create -c 2 -n bob -- -v local:remote --device-read-bps=...\n\n")
		} else {
			printHelp("")
		}
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
	if c.PriceOnly && a.opts.Config.Backend.Type == "docker" {
		return nil, logFatal("Docker backend does not support pricing")
	}
	iType := c.Aws.InstanceType
	if a.opts.Config.Backend.Type == "gcp" {
		iType = c.Gcp.InstanceType
	}
	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "gcp" {
		isArm = c.Gcp.IsArm
	}
	if a.opts.Config.Backend.Type != "docker" {
		printPrice(isArm, c.Gcp.Zone, iType, c.ClientCount)
	}
	if c.PriceOnly {
		return nil, nil
	}

	var startScriptSize os.FileInfo
	if string(c.StartScript) != "" {
		startScriptSize, err = os.Stat(string(c.StartScript))
		if err != nil {
			return nil, logFatal("Early Script does not exist: %s", err)
		}
	}

	if len(string(c.ClientName)) == 0 || len(string(c.ClientName)) > 20 {
		return nil, logFatal("Client name must be up to 20 characters long")
	}

	if !isLegalName(c.ClientName.String()) {
		return nil, logFatal("Client name is not legal, only use a-zA-Z0-9_-")
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
			return nil, logFatal(err)
		}
		totalNodes += len(nlic)
	}

	if totalNodes > 255 || totalNodes < 1 {
		return nil, logFatal("Max node count is 255")
	}

	if totalNodes > 1 && c.Docker.ExposePortsToHost != "" && strings.Contains(c.Docker.ExposePortsToHost, ":") {
		return nil, logFatal("Cannot use docker export-ports feature with more than 1 node")
	}

	if err := checkDistroVersion(c.DistroName.String(), c.DistroVersion.String()); err != nil {
		return nil, logFatal(err)
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
		network:         c.Docker.NetworkName,
		exposePorts:     ep,
		switches:        args,
		dockerHostname:  !c.NoSetHostname,
		ami:             c.Aws.AMI,
		instanceType:    c.Aws.InstanceType,
		ebs:             c.Aws.Ebs,
		securityGroupID: c.Aws.SecurityGroupID,
		subnetID:        c.Aws.SubnetID,
		publicIP:        c.Aws.PublicIP,
		tags:            c.Aws.Tags,
		clientType:      strings.ToLower(nt),
	}
	if a.opts.Config.Backend.Type == "gcp" {
		extra = &backendExtra{
			instanceType: c.Gcp.InstanceType,
			ami:          c.Gcp.Image,
			publicIP:     c.Gcp.PublicIP,
			tags:         c.Gcp.Tags,
			disks:        c.Gcp.Disks,
			zone:         c.Gcp.Zone,
			labels:       c.Gcp.Labels,
			clientType:   strings.ToLower(nt),
		}
	}

	// arm fill
	c.Aws.IsArm, err = b.IsSystemArm(c.Aws.InstanceType)
	if err != nil {
		return nil, fmt.Errorf("IsSystemArm check: %s", err)
	}
	c.Gcp.IsArm = c.Aws.IsArm

	isArm = c.Aws.IsArm
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
	if a.opts.Config.Backend.Type == "gcp" {
		extra.firewallNamePrefix = c.Gcp.NamePrefix
		extra.labels = append(extra.labels, "owner="+c.Owner)
	} else {
		extra.firewallNamePrefix = c.Aws.NamePrefix
		extra.tags = append(extra.tags, "owner="+c.Owner)
	}
	if a.opts.Config.Backend.Type == "aws" {
		if c.Aws.Expires == 0 {
			extra.expiresTime = time.Time{}
		} else {
			extra.expiresTime = time.Now().Add(c.Aws.Expires)
		}
	} else if a.opts.Config.Backend.Type == "gcp" {
		if c.Gcp.Expires == 0 {
			extra.expiresTime = time.Time{}
		} else {
			extra.expiresTime = time.Now().Add(c.Gcp.Expires)
		}
	}
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
	var nip map[int]string
	if a.opts.Config.Backend.Type != "docker" && !c.NoSetHostname {
		nip, err = b.GetNodeIpMap(string(c.ClientName), false)
		if err != nil {
			return nil, err
		}
		log.Printf("Node IP map: %v", nip)
	}
	returns := parallelize.MapLimit(nodeListNew, c.ParallelThreads, func(nnode int) error {
		repl := "cd aerospike-server-* ; ./asinstall || exit 1"
		repl2 := "cd /root && tar -zxf installer.tgz || exit 1"
		repl3 := "cd /root && tar -zxvf installer.tgz || exit 1"
		installer := aerospikeInstallScript[a.opts.Config.Backend.Type+":"+string(c.DistroName)+":"+string(c.DistroVersion)]
		installer = strings.ReplaceAll(installer, repl, "")
		installer = strings.ReplaceAll(installer, repl2, "")
		installer = strings.ReplaceAll(installer, repl3, "")
		err = b.CopyFilesToCluster(c.ClientName.String(), []fileList{{"/opt/install-base.sh", installer, len(installer)}}, []int{nnode})
		if err != nil {
			return fmt.Errorf("could not copy install script to nodes: %s", err)
		}
		out, err := b.RunCommands(string(c.ClientName), [][]string{{"/bin/bash", "/opt/install-base.sh"}}, []int{nnode})
		if err != nil {
			nout := ""
			for i, o := range out {
				nout = nout + "\n---- " + strconv.Itoa(i) + " ----\n" + string(o)
			}
			return fmt.Errorf("some installers failed: %s%s", err, out)
		}

		// set hostnames for cloud
		if a.opts.Config.Backend.Type != "docker" && !c.NoSetHostname {
			newHostname := fmt.Sprintf("%s-%d", string(c.ClientName), nnode)
			newHostname = strings.ReplaceAll(newHostname, "_", "-")
			hComm := [][]string{
				{"hostname", newHostname},
			}
			nr, err := b.RunCommands(string(c.ClientName), hComm, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr, err = b.RunCommands(string(c.ClientName), [][]string{{"sed", "s/" + nip[nnode] + ".*//g", "/etc/hosts"}}, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr[0] = append(nr[0], []byte(fmt.Sprintf("\n%s %s-%d\n", nip[nnode], string(c.ClientName), nnode))...)
			hst := fmt.Sprintf("%s-%d\n", string(c.ClientName), nnode)
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/hostname", hst, len(hst)}}, []int{nnode})
			if err != nil {
				return err
			}
			err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/hosts", string(nr[0]), len(nr[0])}}, []int{nnode})
			if err != nil {
				return err
			}
		}

		// install early/late scripts
		if string(c.StartScript) != "" {
			StartScriptFile, err := os.Open(string(c.StartScript))
			if err != nil {
				log.Printf("ERROR: could not install early script: %s", err)
			} else {
				err = b.CopyFilesToClusterReader(string(c.ClientName), []fileListReader{{"/usr/local/bin/start.sh", StartScriptFile, int(startScriptSize.Size())}}, []int{nnode})
				if err != nil {
					log.Printf("ERROR: could not install early script: %s", err)
				}
				StartScriptFile.Close()
			}
		} else {
			emptyStart := "#!/bin/bash\ndate"
			StartScriptFile := strings.NewReader(emptyStart)
			b.CopyFilesToClusterReader(string(c.ClientName), []fileListReader{{"/usr/local/bin/start.sh", StartScriptFile, len(emptyStart)}}, []int{nnode})
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", nodeListNew[i], ret)
			isError = true
		}
	}
	if isError {
		return nil, errors.New("some nodes returned errors")
	}
	log.Println("Done")
	log.Println("WARN: Deprecation notice: the way clients are created and deployed is changing. A new way will be published in AeroLab 7.2 and the current client creation methods will be removed in AeroLab 8.0")
	return nodeListNew, nil
}
