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
)

type aerospikeUpgradeCmd struct {
	aerospikeStartSelectorCmd
	aerospikeVersionSelectorCmd
	RestartAerospike TypeYesNo `short:"s" long:"restart" description:"Restart aerospike after upgrade (y/n)" default:"y" webchoice:"y,n"`
	parallelThreadsCmd
	Aws aerospikeUpgradeCmdAws `no-flag:"true"`
	Gcp aerospikeUpgradeCmdAws `no-flag:"true"`
}

type aerospikeUpgradeCmdAws struct {
	IsArm bool `long:"arm" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("aerospike.upgrade", "aws", &a.opts.Aerospike.Upgrade.Aws)
	addBackendSwitch("aerospike.upgrade", "gcp", &a.opts.Aerospike.Upgrade.Gcp)
}

func (c *aerospikeUpgradeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running aerospike.upgrade")
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
	err := chDir(string(c.ChDir))
	if err != nil {
		return err
	}

	var edition string
	if strings.HasSuffix(c.AerospikeVersion.String(), "c") {
		edition = "aerospike-server-community"
	} else if strings.HasSuffix(c.AerospikeVersion.String(), "f") {
		edition = "aerospike-server-federal"
	} else {
		edition = "aerospike-server-enterprise"
	}

	// check cluster name
	if len(string(c.ClusterName)) == 0 || len(string(c.ClusterName)) > 20 {
		return errors.New("max size for clusterName is 20 characters")
	}

	if !inslice.HasString([]string{"YES", "NO", "Y", "N"}, strings.ToUpper(c.RestartAerospike.String())) {
		return errors.New("value for restartAerospike should be one of: y|n")
	}

	// download aerospike
	bv := &backendVersion{
		distroName:       c.DistroName.String(),
		distroVersion:    c.DistroVersion.String(),
		aerospikeVersion: c.AerospikeVersion.String(),
		isArm:            isArm,
	}
	url, err := aerospikeGetUrl(bv, c.Username, c.Password)
	if err != nil {
		return err
	}
	c.DistroName = TypeDistro(bv.distroName)
	c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
	verNoSuffix := strings.TrimSuffix(c.AerospikeVersion.String(), "c")
	verNoSuffix = strings.TrimSuffix(verNoSuffix, "f")
	archString := ".x86_64"
	if bv.isArm {
		archString = ".arm64"
	}
	fn := edition + "-" + verNoSuffix + "-" + c.DistroName.String() + c.DistroVersion.String() + archString + ".tgz"
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		log.Println("Downloading installer")
		downloadFile(url, fn, c.Username, c.Password)
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("error, cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	// make a node list
	nodes, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	nodeList := []int{}
	if c.Nodes == "" {
		nodeList = nodes
	} else {
		err = c.Nodes.ExpandNodes(string(c.ClusterName))
		if err != nil {
			return err
		}
		nNodes := strings.Split(c.Nodes.String(), ",")
		for _, nNode := range nNodes {
			nNodeInt, err := strconv.Atoi(nNode)
			if err != nil {
				return err
			}
			if !inslice.HasInt(nodes, nNodeInt) {
				return fmt.Errorf("node %d does not exist in cluster", nNodeInt)
			}
			nodeList = append(nodeList, nNodeInt)
		}
	}

	// copy installer to destination nodes
	stat, err := os.Stat(fn)
	pfilelen := 0
	if err != nil {
		return err
	}
	pfilelen = int(stat.Size())
	fnContents, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer fnContents.Close()
	log.Print("Uploading installer to nodes")
	err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{"/root/upgrade.tgz", fnContents, pfilelen}}, nodeList)
	if err != nil {
		return err
	}

	// stop aerospike
	a.opts.Aerospike.Stop.ClusterName = c.ClusterName
	a.opts.Aerospike.Stop.Nodes = c.Nodes
	a.opts.Aerospike.Stop.ParallelThreads = c.ParallelThreads
	err = a.opts.Aerospike.Stop.Execute(nil)
	if err != nil {
		return err
	}

	log.Print("Upgrading Aerospike")
	// upgrade
	ntime := strconv.Itoa(int(time.Now().Unix()))
	returns := parallelize.MapLimit(nodeList, c.ParallelThreads, func(i int) error {
		// backup aerospike.conf
		nret, err := b.RunCommands(string(c.ClusterName), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}, {"mkdir", "-p", "/tmp/" + ntime}}, []int{i})
		if err != nil {
			return err
		}
		nfile := nret[0]
		out, err := b.RunCommands(string(c.ClusterName), [][]string{{"tar", "-zxvf", "/root/upgrade.tgz", "-C", "/tmp/" + ntime}}, []int{i})
		if err != nil {
			return fmt.Errorf("%s : %s", string(out[0]), err)
		}
		// upgrade
		out, err = b.RunCommands(string(c.ClusterName), [][]string{{"/bin/bash", "-c", fmt.Sprintf("export DEBIAN_FRONTEND=noninteractive; cd /tmp/%s/aerospike* && ./asinstall", ntime)}}, []int{i})
		if err != nil {
			return fmt.Errorf("%s : %s", string(out[0]), err)
		}
		// recover aerospike.conf backup
		err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{{"/etc/aerospike/aerospike.conf", string(nfile), len(nfile)}}, []int{i})
		if err != nil {
			return err
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

	// start aerospike if selected
	if inslice.HasString([]string{"YES", "Y"}, strings.ToUpper(c.RestartAerospike.String())) {
		a.opts.Aerospike.Start.ClusterName = c.ClusterName
		a.opts.Aerospike.Start.Nodes = c.Nodes
		a.opts.Aerospike.Start.ParallelThreads = c.ParallelThreads
		err = a.opts.Aerospike.Start.Execute(nil)
		if err != nil {
			return err
		}
	}

	log.Print("Done")
	return nil
}
