package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"

	"github.com/aerospike/aerolab/parallelize"
)

type clientCreateNoneCmd struct {
	ClientName    TypeClientName         `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount   int                    `short:"c" long:"count" description:"Number of clients" default:"1"`
	NoSetHostname bool                   `short:"H" long:"no-set-hostname" description:"by default, hostname of each machine will be set, use this to prevent hostname change"`
	NoSetDNS      bool                   `long:"no-set-dns" description:"set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	StartScript   flags.Filename         `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Aws           clusterCreateCmdAws    `no-flag:"true"`
	Gcp           clusterCreateCmdGcp    `no-flag:"true"`
	Docker        clusterCreateCmdDocker `no-flag:"true"`
	PriceOnly     bool                   `long:"price" description:"Only display price of ownership; do not actually create the cluster"`
	Owner         string                 `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
	osSelectorCmd
	parallelThreadsCmd

	TypeOverride string `long:"type-override" description:"Override the client type label"`
	instanceRole string
}

func (c *clientCreateNoneCmd) isGrow() bool {
	if len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow" {
		return true
	}
	return false
}

func (c *clientCreateNoneCmd) Execute(args []string) error {
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
	_, err := c.createBase(args, "none")
	return err
}

func (c *clientCreateNoneCmd) createBase(args []string, nt string) (machines []int, err error) {
	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}
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
	if a.opts.Config.Backend.Type == "gcp" {
		printPrice(c.Gcp.Zone.String(), iType.String(), c.ClientCount, false)
	} else if a.opts.Config.Backend.Type == "aws" {
		printPrice(c.Gcp.Zone.String(), iType.String(), c.ClientCount, c.Aws.SpotInstance)
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

	if len(string(c.ClientName)) == 0 || len(string(c.ClientName)) > 24 {
		return nil, logFatal("Client name must be up to 24 characters long")
	}

	if !isLegalName(c.ClientName.String()) {
		return nil, logFatal("Client name is not legal, only use a-zA-Z0-9_-")
	}

	if c.TypeOverride != "" {
		fmt.Println("Overriding client type:", c.TypeOverride)
		nt = c.TypeOverride
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

	if totalNodes > 1 && c.Docker.ExposePortsToHost != "" {
		return nil, logFatal("Cannot use docker export-ports feature with more than 1 node")
	}

	if err := checkDistroVersion(c.DistroName.String(), c.DistroVersion.String()); err != nil {
		return nil, logFatal(err)
	}
	if string(c.DistroVersion) == "latest" {
		c.DistroVersion = TypeDistroVersion(getLatestVersionForDistro(c.DistroName.String()))
	}

	// efs mounts
	var foundVol *inventoryVolume
	var efsName, efsLocalPath, efsPath string
	if a.opts.Config.Backend.Type == "aws" && c.Aws.EFSMount != "" {
		if len(strings.Split(c.Aws.EFSMount, ":")) < 2 {
			return nil, logFatal("EFS Mount format incorrect")
		}
		mountDetail := strings.Split(c.Aws.EFSMount, ":")
		efsName = mountDetail[0]
		efsLocalPath = mountDetail[1]
		efsPath = "/"
		if len(mountDetail) > 2 {
			efsPath = mountDetail[1]
			efsLocalPath = mountDetail[2]
		}
		inv, err := b.Inventory("", []int{InventoryItemVolumes})
		if err != nil {
			return nil, err
		}
		for _, vol := range inv.Volumes {
			if vol.Name != efsName {
				continue
			}
			foundVol = &vol
			break
		}
		if foundVol == nil && !c.Aws.EFSCreate {
			return nil, logFatal("EFS Volume not found, and is not set to be created")
		} else if foundVol == nil {
			a.opts.Volume.Create.Name = efsName
			if c.Aws.EFSOneZone {
				a.opts.Volume.Create.Aws.Zone, err = b.GetAZName(c.Aws.SubnetID)
				if err != nil {
					return nil, err
				}
			}
			a.opts.Volume.Create.Owner = c.Owner
			a.opts.Volume.Create.Tags = c.Aws.Tags
			err = a.opts.Volume.Create.Execute(nil)
			if err != nil {
				return nil, err
			}
		} else {
			err = b.TagVolume(foundVol.FileSystemId, "expireDuration", c.Aws.EFSExpires.String(), foundVol.AvailabilityZoneName)
			if err != nil {
				return nil, err
			}
		}
	} else if a.opts.Config.Backend.Type == "gcp" && c.Gcp.VolMount != "" {
		if c.Gcp.VolMount != "" && len(strings.Split(c.Gcp.VolMount, ":")) < 2 {
			return nil, logFatal("Mount format incorrect")
		}
		if c.Gcp.VolMount != "" {
			mountDetail := strings.Split(c.Gcp.VolMount, ":")
			efsName = mountDetail[0]
			efsLocalPath = mountDetail[1]
			inv, err := b.Inventory("", []int{InventoryItemVolumes})
			if err != nil {
				return nil, err
			}
			for _, vol := range inv.Volumes {
				if vol.Name != efsName {
					continue
				}
				foundVol = &vol
				break
			}
			if foundVol == nil && !c.Gcp.VolCreate {
				return nil, logFatal("Volume not found, and is not set to be created")
			} else if foundVol == nil {
				a.opts.Volume.Create.Name = efsName
				a.opts.Volume.Create.Tags = c.Gcp.Labels
				a.opts.Volume.Create.Owner = c.Owner
				a.opts.Volume.Create.Expires = c.Gcp.VolExpires
				a.opts.Volume.Create.Gcp.Zone = c.Gcp.Zone.String()
				err = a.opts.Volume.Create.Execute(nil)
				if err != nil {
					return nil, err
				}
			} else {
				err = b.TagVolume(foundVol.FileSystemId, "expireduration", strings.ToLower(strings.ReplaceAll(c.Gcp.VolExpires.String(), ".", "_")), foundVol.AvailabilityZoneName)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	b.WorkOnClients()

	// build extra
	var ep []string
	if c.Docker.ExposePortsToHost != "" {
		ep = strings.Split(c.Docker.ExposePortsToHost, ",")
	}
	cloudDisks, err := disk2backend(c.Aws.Disk)
	if err != nil {
		return nil, err
	}
	extra := &backendExtra{
		cpuLimit:          c.Docker.CpuLimit,
		ramLimit:          c.Docker.RamLimit,
		swapLimit:         c.Docker.SwapLimit,
		privileged:        c.Docker.Privileged,
		network:           c.Docker.NetworkName,
		customDockerImage: c.Docker.clientCustomDockerImage,
		exposePorts:       ep,
		switches:          args,
		dockerHostname:    !c.NoSetHostname,
		ami:               c.Aws.AMI,
		instanceType:      c.Aws.InstanceType.String(),
		ebs:               c.Aws.Ebs,
		securityGroupID:   c.Aws.SecurityGroupID,
		subnetID:          c.Aws.SubnetID,
		publicIP:          c.Aws.PublicIP,
		tags:              c.Aws.Tags,
		clientType:        strings.ToLower(nt),
		instanceRole:      c.instanceRole,
		cloudDisks:        cloudDisks,
	}
	if a.opts.Config.Backend.Type == "gcp" {
		cloudDisks, err := disk2backend(c.Gcp.Disk)
		if err != nil {
			return nil, err
		}
		extra = &backendExtra{
			instanceType: c.Gcp.InstanceType.String(),
			ami:          c.Gcp.Image,
			publicIP:     c.Gcp.PublicIP,
			tags:         c.Gcp.Tags,
			disks:        c.Gcp.Disks,
			zone:         c.Gcp.Zone.String(),
			labels:       c.Gcp.Labels,
			instanceRole: c.instanceRole,
			clientType:   strings.ToLower(nt),
			cloudDisks:   cloudDisks,
		}
	}

	// arm fill
	c.Aws.IsArm, err = b.IsSystemArm(c.Aws.InstanceType.String())
	if err != nil {
		return nil, fmt.Errorf("IsSystemArm check: %s", err)
	}
	c.Gcp.IsArm = c.Aws.IsArm

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
	if c.Docker.clientCustomDockerImage == "" {
		log.Printf("Distro: %s Version: %s", string(c.DistroName), string(c.DistroVersion))
	} else {
		log.Printf("Custom image: %s", c.Docker.clientCustomDockerImage)
	}
	if a.opts.Config.Backend.Type != "aws" {
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
	expirySet := false
	for _, aaa := range os.Args {
		if strings.HasPrefix(aaa, "--aws-expire") || strings.HasPrefix(aaa, "--gcp-expire") {
			expirySet = true
		}
	}

	var ij inventoryJson
	if c.isGrow() {
		ij, err = b.Inventory("", []int{InventoryItemClients})
		b.WorkOnClients()
		if err != nil {
			return nil, err
		}
	}

	// Cluster Expiry
	if c.isGrow() && !expirySet {
		extra.expiresTime = time.Time{}

		for _, item := range ij.Clients {
			if item.ClientName != string(c.ClientName) {
				continue
			}
			if item.Expires == "" || item.Expires == "0001-01-01T00:00:00Z" {
				extra.expiresTime = time.Time{}
				break
			}
			expiry, err := time.Parse(time.RFC3339, item.Expires)
			if err != nil {
				return nil, err
			}
			if extra.expiresTime.IsZero() || expiry.After(extra.expiresTime) {
				extra.expiresTime = expiry
			}
		}
	} else if c.isGrow() && expirySet {
		log.Println("WARNING: you are setting a different expiry to these nodes than the existing ones. To change expiry for all nodes, use: aerolab client configure expiry")
	}

	// Client Type Override
	if c.isGrow() {
		for _, item := range ij.Clients {
			if item.ClientName != string(c.ClientName) {
				continue
			}

			if item.ClientType != extra.clientType {
				extra.clientType = item.ClientType
				break
			}
		}

		if c.TypeOverride != "" {
			if extra.clientType != strings.ToLower(c.TypeOverride) {
				return nil, logFatal("cluster client type does not match type-override")
			}
		}
	}

	extra.spotInstance = c.Aws.SpotInstance
	if a.opts.Config.Backend.Type == "gcp" {
		extra.spotInstance = c.Gcp.SpotInstance
	}
	extra.onHostMaintenance = c.Gcp.OnHostMaintenance
	if c.Gcp.MinCPUPlatform != "" {
		extra.gcpMinCpuPlatform = &c.Gcp.MinCPUPlatform
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
		if a.opts.Config.Backend.Type != "docker" && !c.NoSetDNS {
			dnsScript := `mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' > /etc/systemd/resolved.conf.d/aerolab.conf
[Resolve]
DNS=1.1.1.1
FallbackDNS=8.8.8.8
EOF
systemctl restart systemd-resolved
systemctl restart systemd-resolved || echo "No systemctl"
if [ -d /etc/NetworkManager/system-connections ]
then
ls /etc/NetworkManager/system-connections |sed 's/.nmconnection//g' |while read file; do nmcli conn modify "$file" ipv4.dns "1.1.1.1 8.8.8.8"; done
systemctl restart NetworkManager
fi
`
			if err = b.CopyFilesToClusterReader(string(c.ClientName), []fileListReader{{filePath: "/tmp/fix-dns.sh", fileContents: strings.NewReader(dnsScript), fileSize: len(dnsScript)}}, []int{nnode}); err == nil {
				if _, err = b.RunCommands(string(c.ClientName), [][]string{{"/bin/bash", "-c", "chmod 755 /tmp/fix-dns.sh; bash /tmp/fix-dns.sh"}}, []int{nnode}); err != nil {
					log.Print("Failed to set DNS resolvers by running /tmp/fix-dns.sh")
				}
			} else {
				log.Printf("Failed to upload DNS resolver script to /tmp/fix-dns.sh: %s", err)
			}
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

	// efs mounts
	if a.opts.Config.Backend.Type == "aws" && c.Aws.EFSMount != "" {
		a.opts.Volume.Mount.ClusterName = string(c.ClientName)
		a.opts.Volume.Mount.Aws.EfsPath = efsPath
		a.opts.Volume.Mount.IsClient = true
		a.opts.Volume.Mount.LocalPath = efsLocalPath
		a.opts.Volume.Mount.Name = efsName
		a.opts.Volume.Mount.ParallelThreads = c.ParallelThreads
		err = a.opts.Volume.Mount.Execute(nil)
		if err != nil {
			return nil, err
		}
	} else if a.opts.Config.Backend.Type == "gcp" && c.Gcp.VolMount != "" {
		a.opts.Volume.Mount.ClusterName = string(c.ClientName)
		a.opts.Volume.Mount.Aws.EfsPath = efsPath
		a.opts.Volume.Mount.IsClient = true
		a.opts.Volume.Mount.LocalPath = efsLocalPath
		a.opts.Volume.Mount.Name = efsName
		a.opts.Volume.Mount.ParallelThreads = c.ParallelThreads
		err = a.opts.Volume.Mount.Execute(nil)
		if err != nil {
			return nil, err
		}
	}
	b.WorkOnClients()

	if a.opts.Config.Backend.Type != "docker" && !extra.expiresTime.IsZero() {
		log.Printf("CLUSTER EXPIRES: %s (in: %s); to extend, use: aerolab client configure expiry", extra.expiresTime.Format(time.RFC850), time.Until(extra.expiresTime).Truncate(time.Second).String())
	}
	log.Println("Done")
	log.Println("WARN: Deprecation notice: the way clients are created and deployed is changing. A new design will be explored during AeroLab's version 7's lifecycle and the current client creation methods will be removed in AeroLab 8.0")
	return nodeListNew, nil
}
