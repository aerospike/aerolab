package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
)

func init() {
	addBackendSwitch("volume.create", "aws", &a.opts.Volume.Create.Aws)
	addBackendSwitch("volume.create", "gcp", &a.opts.Volume.Create.Gcp)
	addBackendSwitch("volume.delete", "gcp", &a.opts.Volume.Delete.Gcp)
	addBackendSwitch("volume.mount", "gcp", &a.opts.Volume.Mount.Gcp)
	addBackendSwitch("volume.mount", "aws", &a.opts.Volume.Mount.Aws)
}

type volumeCmd struct {
	Create  volumeCreateCmd    `command:"create" subcommands-optional:"true" description:"Create a volume"`
	List    volumeListCmd      `command:"list" subcommands-optional:"true" description:"List volumes"`
	Mount   volumeMountCmd     `command:"mount" subcommands-optional:"true" description:"Mount a volume on a node"`
	Delete  volumeDeleteCmd    `command:"delete" subcommands-optional:"true" description:"Delete a volume"`
	Resize  volumeResizeCmd    `command:"grow" subcommands-optional:"true" description:"GCP only: grow a volume; if the volume is not attached, the filesystem will not be resized automatically"`
	Detach  volumeDetachCmd    `command:"detach" subcommands-optional:"true" description:"GCP only: detach a volume for an instance"`
	DoMount volumeExecMountCmd `command:"exec-mount" hidden:"true" subcommands-optional:"true" description:"Execute actual mounting operation"`
	Help    helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type volumeListCmd struct {
	SortBy     []string `long:"sort-by" description:"sort by field name; must match exact header name; can be specified multiple times; format: asc:name dsc:name ascnum:name dscnum:name"`
	Json       bool     `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool     `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Owner      string   `long:"owner" description:"filter by owner tag/label"`
	Pager      bool     `long:"pager" description:"set to enable vertical and horizontal pager"`
	RenderType string   `long:"render" description:"different output rendering; supported: text,csv,tsv,html,markdown" default:"text"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.Pager = c.Pager
	a.opts.Inventory.List.SortBy = c.SortBy
	a.opts.Inventory.List.JsonPretty = c.JsonPretty
	a.opts.Inventory.List.RenderType = c.RenderType
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowVolumes)
}

type volumeResizeCmd struct {
	Name string  `short:"n" long:"name" description:"EFS Name" default:"agi"`
	Zone string  `short:"z" long:"zone" description:"Zone name to use"`
	Size int64   `short:"s" long:"size" description:"Volume SizeGB" default:"100"`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeResizeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return nil
	}
	log.Print("Listing volumes, cluster and clients")
	vols, err := b.Inventory("", []int{InventoryItemVolumes})
	if err != nil {
		return fmt.Errorf("cannot list volumes")
	}
	var volume *inventoryVolume
	for _, vol := range vols.Volumes {
		if vol.Name != c.Name {
			continue
		}
		vol := vol
		volume = &vol
		break
	}
	if volume == nil {
		return errors.New("volume not found")
	}
	var ClusterName *string
	var NodeId int
	if volume.GCPAttachedTo != nil {
		node := strings.Split(volume.GCPAttachedTo[0], "-")
		clusterName := strings.Join(node[0:len(node)-1], "-")
		nodeId := node[len(node)-1]
		b.WorkOnServers()
		clusters, err := b.ClusterList()
		if err != nil {
			return fmt.Errorf("could not list clusters")
		}
		b.WorkOnClients()
		clients, err := b.ClusterList()
		if err != nil {
			return fmt.Errorf("could not list clients")
		}
		if inslice.HasString(clusters, clusterName) {
			b.WorkOnServers()
		} else if inslice.HasString(clients, clusterName) {
			b.WorkOnClients()
		} else {
			return fmt.Errorf("could not find attached cluster/client nodes")
		}
		nodes, err := b.NodeListInCluster(clusterName)
		if err != nil {
			return fmt.Errorf("could not list nodes: %s", err)
		}
		nodeIdInt, _ := strconv.Atoi(nodeId)
		if !inslice.HasInt(nodes, nodeIdInt) {
			return fmt.Errorf("node %d not found", nodeIdInt)
		}
		ClusterName = &clusterName
		NodeId = nodeIdInt
	}
	log.Println("Resizing volume")
	err = b.ResizeVolume(c.Name, c.Zone, c.Size)
	if err != nil {
		return err
	}
	if ClusterName != nil {
		log.Println("Resizing filesystem")
		// discover fstype
		outs, err := b.RunCommands(*ClusterName, [][]string{{"cat", "/etc/fstab"}}, []int{NodeId})
		if err != nil {
			return fmt.Errorf("failed to cat fstab: %s: %s", err, string(outs[0]))
		}
		out := string(outs[0])
		fstype := ""
		mountpoint := ""
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "/dev/disk/by-id/google-"+c.Name) {
				continue
			}
			fstab := strings.Split(line, "  ")
			if len(fstab) < 3 {
				return errors.New("could not work out filesystem type from fstab")
			}
			fstype = fstab[2]
			mountpoint = fstab[1]
		}
		if fstype == "" {
			return errors.New("could not work out filesystem location from fstab")
		}
		// run the correct command to resize fs
		switch fstype {
		case "xfs":
			outs, err = b.RunCommands(*ClusterName, [][]string{{"xfs_growfs", mountpoint}}, []int{NodeId})
			if err != nil {
				return fmt.Errorf("resize command failed %s with %s", err, string(outs[0]))
			}
		case "ext3", "ext2", "ext4":
			outs, err = b.RunCommands(*ClusterName, [][]string{{"resize2fs", "/dev/disk/by-id/google-" + c.Name}}, []int{NodeId})
			if err != nil {
				return fmt.Errorf("resize command failed %s with %s", err, string(outs[0]))
			}
		default:
			return fmt.Errorf("fstype %s not supported for automated resizing", fstype)
		}
	}
	log.Println("Done")
	return nil
}

type volumeCreateCmd struct {
	Name    string             `short:"n" long:"name" description:"EFS Name" default:"agi"`
	Tags    []string           `short:"t" long:"tag" description:"tag as key=value; can be specified multiple times"`
	Owner   string             `short:"o" long:"owner" description:"set owner tag to the specified value"`
	Expires time.Duration      `short:"e" long:"expire" description:"expire the volume if 'mount' against the volume has not been executed for this long"`
	Aws     volumeCreateAwsCmd `no-flag:"true"`
	Gcp     volumeCreateGcpCmd `no-flag:"true"`
	Help    helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

type volumeCreateGcpCmd struct {
	Zone        string `short:"z" long:"zone" description:"Zone name to use"`
	Size        int64  `short:"s" long:"size" description:"Volume SizeGB" default:"100"`
	Description string `short:"d" long:"description" description:"set the description field"`
}

type volumeCreateAwsCmd struct {
	Zone string `short:"z" long:"zone" description:"Full Availability Zone name; if provided, will define a one-zone volume; default {REGION}a"`
}

func (c *volumeCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Creating volume")
	if c.Owner != "" {
		c.Tags = append(c.Tags, "aerolab7owner="+c.Owner)
	}
	zone := c.Aws.Zone
	if a.opts.Config.Backend.Type == "gcp" {
		zone = c.Gcp.Zone
	}
	err := b.CreateVolume(c.Name, zone, c.Tags, c.Expires, c.Gcp.Size, c.Gcp.Description)
	if err != nil {
		return err
	}
	log.Println("Done")
	return nil
}

type volumeDetachCmd struct {
	Name        string `short:"n" long:"name" description:"VOL Name" default:"agi"`
	ClusterName string `short:"N" long:"cluster-name" description:"Cluster/Client Name from which to detach" default:"agi"`
	Node        int    `short:"l" long:"node" description:"Node to detach from" default:"1"`
	Zone        string `short:"z" long:"zone" description:"gcp zone"`
	IsClient    bool   `short:"c" long:"is-client" description:"Specify mounting on client instead of cluster"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeDetachCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running volume.detach")
	if c.IsClient {
		b.WorkOnClients()
	} else {
		b.WorkOnServers()
	}
	err := b.DetachVolume(c.Name, c.ClusterName, c.Node, c.Zone)
	if err != nil {
		return err
	}
	log.Println("Done")
	return nil
}

type volumeMountCmd struct {
	Name        string            `short:"n" long:"name" description:"VOL Name" default:"agi"`
	ClusterName string            `short:"N" long:"cluster-name" description:"Cluster/Client Name on which to mount" default:"agi"`
	IsClient    bool              `short:"c" long:"is-client" description:"Specify mounting on client instead of cluster"`
	LocalPath   string            `short:"p" long:"mount-path" description:"Path on the node to mount to" default:"/mnt/{VOL_NAME}"`
	Aws         volumeMountAwsCmd `no-flag:"true"`
	Gcp         volumeMountGcpCmd `no-flag:"true"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type volumeMountAwsCmd struct {
	EfsPath string `short:"P" long:"volume-path" description:"Volume path to mount" default:"/"`
}

type volumeMountGcpCmd struct {
	Node   int    `short:"l" long:"node" description:"Node to attach to" default:"1"`
	FsType string `short:"f" long:"fs-type" description:"Filesystem type to create if one doesn't exist" default:"xfs"`
}

func (c *volumeMountCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running volume.mount")
	log.Println("Gathering volume and cluster data")
	secGroups := []string{}
	subnet := ""
	instanceZone := ""
	if !c.IsClient {
		inv, err := b.Inventory("", []int{InventoryItemClusters})
		if err != nil {
			return err
		}
		for _, cluster := range inv.Clusters {
			if cluster.ClusterName != c.ClusterName {
				continue
			}
			subnet = cluster.awsSubnet
			secGroups = cluster.awsSecGroups
			instanceZone = cluster.Zone
			break
		}
	} else {
		inv, err := b.Inventory("", []int{InventoryItemClients})
		if err != nil {
			return err
		}
		for _, cluster := range inv.Clients {
			if cluster.ClientName != c.ClusterName {
				continue
			}
			subnet = cluster.awsSubnet
			secGroups = cluster.awsSecGroups
			instanceZone = cluster.Zone
			break
		}
	}
	inv, err := b.Inventory("", []int{InventoryItemVolumes})
	if err != nil {
		return err
	}
	var volume *inventoryVolume
	for _, vol := range inv.Volumes {
		if vol.Name != c.Name {
			continue
		}
		volume = &vol
		break
	}
	if volume == nil {
		return errors.New("volume not found")
	}
	if a.opts.Config.Backend.Type == "aws" {
		var mountTarget *inventoryMountTarget
		for _, mt := range volume.MountTargets {
			if mt.SubnetId == subnet {
				mountTarget = &mt
				break
			}
		}
		if mountTarget == nil {
			log.Println("Volume mount target not found, creating")
			_, err := b.CreateMountTarget(volume, subnet, secGroups)
			if err != nil {
				return err
			}
			//mountTarget = &mt
		} else {
			addGroups := mountTarget.SecurityGroups
			needMTFix := false
			for _, sg := range secGroups {
				if !inslice.HasString(addGroups, sg) {
					addGroups = append(addGroups, sg)
					needMTFix = true
				}
			}
			if needMTFix {
				log.Println("Mount Target security group mismatch, fixing")
				err = b.MountTargetAddSecurityGroup(mountTarget, volume, addGroups)
				if err != nil {
					return err
				}
			}
		}
	} else {
		if instanceZone != volume.AvailabilityZoneName {
			return fmt.Errorf("instance zone %s differs from volume zone %s", instanceZone, volume.AvailabilityZoneName)
		}
	}
	usedName := "lastUsed"
	usedTag := time.Now().Format(time.RFC3339)
	if a.opts.Config.Backend.Type == "gcp" {
		usedTag = strings.ReplaceAll(strings.ReplaceAll(usedTag, ":", "_"), "+", "-")
		usedName = "lastused"
	}
	err = b.TagVolume(volume.FileSystemId, usedName, usedTag, volume.AvailabilityZoneName)
	if err != nil {
		return err
	}
	if c.IsClient {
		b.WorkOnClients()
	} else {
		b.WorkOnServers()
	}
	if a.opts.Config.Backend.Type == "gcp" {
		log.Println("Attaching disk to node")
		err = b.AttachVolume(c.Name, instanceZone, c.ClusterName, c.Gcp.Node)
		if err != nil {
			return err
		}
	}
	return c.doMount(volume, c.Gcp.Node)
}

func (c *volumeMountCmd) doMount(volume *inventoryVolume, node int) error {
	var nodes []int
	var err error
	if a.opts.Config.Backend.Type == "aws" {
		log.Println("Listing cluster nodes")
		nodes, err = b.NodeListInCluster(c.ClusterName)
		if err != nil {
			return err
		}
		log.Println("Attempting remote mount on each node")
	} else {
		nodes = []int{node}
		log.Printf("Attempting remote mount on node %d", node)
	}
	returns := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
		isArm, err := b.IsNodeArm(c.ClusterName, node)
		if err != nil {
			return fmt.Errorf("could not identify node architecture: %s", err)
		}
		_, err = b.RunCommands(c.ClusterName, [][]string{{"ls", "/usr/local/bin/aerolab"}}, []int{node})
		if err != nil {
			nLinuxBinary := nLinuxBinaryX64
			if isArm {
				nLinuxBinary = nLinuxBinaryArm64
			}
			if len(nLinuxBinary) == 0 {
				xtail := ""
				if isArm {
					xtail = ".arm"
				} else {
					xtail = ".amd"
				}
				if _, err := os.Stat("/usr/local/bin/aerolab" + xtail); err == nil {
					nLinuxBinary, _ = os.ReadFile("/usr/local/bin/aerolab" + xtail)
				}
			}
			if len(nLinuxBinary) == 0 {
				execName, err := findExec()
				if err != nil {
					return err
				}
				nLinuxBinary, err = os.ReadFile(execName)
				if err != nil {
					return err
				}
			}
			flist := []fileListReader{
				{
					filePath:     "/usr/local/bin/aerolab",
					fileContents: bytes.NewReader(nLinuxBinary),
					fileSize:     len(nLinuxBinary),
				},
			}
			err = b.CopyFilesToClusterReader(c.ClusterName, flist, []int{node})
			if err != nil {
				return fmt.Errorf("could not upload configuration to instance: %s", err)
			}
		}
		c.LocalPath = strings.ReplaceAll(c.LocalPath, "{VOL_NAME}", c.Name)
		out, err := b.RunCommands(c.ClusterName, [][]string{{"/usr/local/bin/aerolab", "config", "backend", "-t", "none"}}, []int{node})
		if err != nil {
			return fmt.Errorf("could not mount: %s: %s", err, string(out[0]))
		}
		if a.opts.Config.Backend.Type == "aws" {
			out, err = b.RunCommands(c.ClusterName, [][]string{{"/usr/local/bin/aerolab", "volume", "exec-mount", "-p", c.LocalPath, "-P", c.Aws.EfsPath, "-n", volume.FileSystemId, "-b", "aws"}}, []int{node})
		} else {
			out, err = b.RunCommands(c.ClusterName, [][]string{{"/usr/local/bin/aerolab", "volume", "exec-mount", "-p", c.LocalPath, "-n", volume.FileSystemId, "-f", c.Gcp.FsType, "-b", "gcp"}}, []int{node})
		}
		if err != nil {
			return fmt.Errorf("could not mount: %s: %s", err, string(out[0]))
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", nodes[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Println("Done")
	return nil
}

type volumeExecMountCmd struct {
	LocalPath string  `short:"p" long:"mount-path" description:"Path on the node to mount to"`
	EfsPath   string  `short:"P" long:"volume-path" description:"Volume path to mount" default:"/"`
	FsId      string  `short:"n" long:"name" description:"FsId" default:"agi"`
	FsType    string  `short:"f" long:"fs-type" description:"type of filesystem" default:"xfs"`
	Backend   string  `short:"b" long:"backend" description:"aws|gcp"`
	Help      helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeExecMountCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	if c.Backend == "aws" {
		err := c.installEFSUtils()
		if err != nil {
			return err
		}
		err = c.installEFSfstab()
		if err != nil {
			return err
		}
		err = c.installMountEFS()
		if err != nil {
			return err
		}
	} else {
		diskPath := "/dev/disk/by-id/google-" + c.FsId
		if _, err := os.Stat(diskPath); err != nil {
			return errors.New("disk not found")
		}
		fstype := c.FsType
		out, err := exec.Command("blkid", diskPath).CombinedOutput()
		if err != nil {
			out, err := exec.Command("mkfs."+c.FsType, diskPath).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s Failed to create filesystem: %s", err, string(out))
			}
		} else {
			outn := strings.Split(string(out), " ")
			for _, outx := range outn {
				if !strings.HasPrefix(outx, "TYPE=") {
					continue
				}
				fstype = strings.Trim(strings.TrimPrefix(outx, "TYPE="), "\r\n\t \"")
			}
		}
		os.MkdirAll(c.LocalPath, 0755)
		fstabstring := diskPath + "  " + c.LocalPath + "  " + fstype + "  discard,auto,nofail  0 1"
		f, err := os.ReadFile("/etc/fstab")
		if err != nil {
			return fmt.Errorf("failed to open fstab: %s", err)
		}
		found := false
		for _, line := range strings.Split(string(f), "\n") {
			if strings.HasPrefix(line, diskPath+"  ") {
				found = true
				break
			}
		}
		if !found {
			fs := string(f) + "\n" + fstabstring + "\n"
			err = os.WriteFile("/etc/fstab", []byte(fs), 0644)
			if err != nil {
				return fmt.Errorf("failed to write fstab: %s", err)
			}
		}
		out, err = exec.Command("mount", "-a").CombinedOutput()
		if err != nil {
			return fmt.Errorf("mount failed: %s: %s", err, string(out))
		}
	}
	return nil
}

type volumeDeleteCmd struct {
	Name string             `short:"n" long:"name" description:"EFS Name" default:"agi"`
	Gcp  volumeDeleteGcpCmd `no-flag:"true"`
	Help helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

type volumeDeleteGcpCmd struct {
	Zone string `short:"z" long:"zone" description:"Zone name to use"`
}

func (c *volumeDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	vols := strings.Split(c.Name, ",")
	for _, vol := range vols {
		log.Printf("Deleting volume %s", vol)
		err := b.DeleteVolume(vol, c.Gcp.Zone)
		if err != nil {
			return err
		}
	}
	log.Println("Done")
	return nil
}

func (c *volumeExecMountCmd) dpkgGrep(name string) (bool, error) {
	out, err := exec.Command("dpkg", "-l").CombinedOutput()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, name) {
			return true, nil
		}
	}
	return false, nil
}

func (c *volumeExecMountCmd) installEFSUtils() error {
	if c.isDeb() {
		return c.installEFSUtilsDeb()
	}
	return c.installEFSUtilsYum()
}

func (c *volumeExecMountCmd) isDeb() bool {
	_, err := exec.Command("/bin/bash", "-c", "which dpkg").CombinedOutput()
	return err == nil
}

func (c *volumeExecMountCmd) installEFSUtilsYum() error {
	// deps
	command := []string{"/bin/bash", "-c", "yum install -y make rpm-build git"}
	out, err := exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	// git clone
	if _, err := os.Stat("efs-utils"); err != nil {
		command := []string{"git", "clone", "https://github.com/aws/efs-utils"}
		out, err := exec.Command(command[0], command[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
		}
	}
	// compile
	command = []string{"/bin/bash", "-c", "cd efs-utils && make rpm"}
	out, err = exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	// install
	command = []string{"/bin/bash", "-c", "yum install -y efs-utils/build/amazon-efs-utils-*.noarch.rpm"}
	out, err = exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func (c *volumeExecMountCmd) installEFSUtilsDeb() error {
	// already installed package?
	alreadyInstalled, err := c.dpkgGrep("amazon-efs-utils")
	if err != nil {
		return err
	}
	if alreadyInstalled {
		return nil
	}

	// git clone
	if _, err := os.Stat("efs-utils"); err != nil {
		command := []string{"git", "clone", "https://github.com/aws/efs-utils"}
		out, err := exec.Command(command[0], command[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
		}
	}

	// compile
	command := []string{"/bin/bash", "-c", "cd efs-utils && ./build-deb.sh"}
	out, err := exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}

	// install
	command = []string{"/bin/bash", "-c", "apt-get update && apt-get -y install ./efs-utils/build/amazon-efs-utils*deb"}
	out, err = exec.Command(command[0], command[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}

	return nil
}

func (c *volumeExecMountCmd) installEFSfstab() error {
	os.MkdirAll(c.LocalPath, 0755)
	// check if entry already exists
	fstab, err := os.ReadFile("/etc/fstab")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(fstab), "\n") {
		line = strings.Trim(line, "\n\t\r ")
		if (strings.Contains(line, " efs ") || strings.Contains(line, " nfs4 ")) && strings.Contains(line, fmt.Sprintf(" %s ", c.LocalPath)) {
			return nil
		}
	}
	// install the entry
	line := fmt.Sprintf("\n%s:%s %s efs defaults,_netdev 0 0\n", c.FsId, c.EfsPath, c.LocalPath)
	//line = fmt.Sprintf("\n%s:/ %s nfs4 nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport,_netdev 0 0\n", w.envconf.EfsIP, w.conf.EfsMountDir)
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return nil
}

func (c *volumeExecMountCmd) installMountEFS() error {
	mountCommand := []string{"mount", "-a"}
	dnsFlushCommand := []string{"systemd-resolve", "--flush-caches"}
	success := false
	var err error
	var out []byte
	for attempt := 1; attempt <= 30; attempt++ {
		out, err = exec.Command(mountCommand[0], mountCommand[1:]...).CombinedOutput()
		if err == nil {
			success = true
			break
		}
		exec.Command(dnsFlushCommand[0], dnsFlushCommand[1:]...).CombinedOutput()
		time.Sleep(20 * time.Second)
	}
	if !success {
		return fmt.Errorf("failed to mount, last error: %s: %s", err, string(out))
	}
	return nil
}

/* could convert the whole exec-mount into this uploadable script:
//// %s fill: c.LocalPath, c.LocalPath, c.FsID, c.EfsPath, c.LocalPath
#localpath
mkdir -p %s
apt update && apt -y install git
dpkg -l |grep amazon-efs-utils
if [ $? -ne 0 ]
then
	ls /opt/efs-utils >/dev/null 2>&1
	if [ $? -ne 0 ]
	then
		set -e
		cd /opt
		git clone https://github.com/aws/efs-utils
		set +e
	fi
	set -e
	cd /opt/efs-utils
	./build-deb.sh
	apt-get -y install ./build/amazon-efs-utils*deb
	set +e
fi

#localpath
cat /etc/fstab |grep efs |grep %s
if [ $? -ne 0 ]
then
	#fsid, efspath, localpath
	echo "\n%s:%s %s efs defaults,_netdev 0 0\n" >> /etc/fstab
fi

SUCCESS=0
while [ $SUCCESS -eq 0 ]
do
	mount -a
	if [ $? -eq 0 ]
	then
		SUCCESS=1
	else
		systemd-resolve --flush-caches
		sleep 10
	fi
done
*/
