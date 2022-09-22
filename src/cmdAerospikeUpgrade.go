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
)

type aerospikeUpgradeCmd struct {
	aerospikeStartCmd
	aerospikeVersionSelectorCmd
	RestartAerospike string `short:"s" long:"restart" description:"Restart aerospike after upgrade (y/n)" default:"y"`
}

func (c *aerospikeUpgradeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running aerospike.upgrade")

	err := chDir(c.ChDir)
	if err != nil {
		return err
	}

	var edition string
	if strings.HasSuffix(c.AerospikeVersion, "c") {
		edition = "aerospike-server-community"
	} else {
		edition = "aerospike-server-enterprise"
	}

	// check cluster name
	if len(c.ClusterName) == 0 || len(c.ClusterName) > 20 {
		return errors.New("max size for clusterName is 20 characters")
	}

	if !inslice.HasString([]string{"YES", "NO", "Y", "N"}, strings.ToUpper(c.RestartAerospike)) {
		return errors.New("value for restartAerospike should be one of: y|n")
	}

	// download aerospike
	bv := &backendVersion{
		distroName:       c.DistroName,
		distroVersion:    c.DistroVersion,
		aerospikeVersion: c.AerospikeVersion,
	}
	url, err := aerospikeGetUrl(bv, c.Username, c.Password)
	if err != nil {
		return err
	}
	c.DistroName = bv.distroName
	c.DistroVersion = bv.distroVersion
	c.AerospikeVersion = bv.aerospikeVersion
	verNoSuffix := strings.TrimSuffix(c.AerospikeVersion, "c")
	fn := edition + "-" + verNoSuffix + "-" + c.DistroName + c.DistroVersion + ".tgz"
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		log.Println("Downloading installer")
		downloadFile(url, fn, c.Username, c.Password)
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, c.ClusterName) {
		err = fmt.Errorf("error, cluster does not exist: %s", c.ClusterName)
		return err
	}

	// make a node list
	nodes, err := b.NodeListInCluster(c.ClusterName)
	if err != nil {
		return err
	}

	nodeList := []int{}
	if c.Nodes == "" {
		nodeList = nodes
	} else {
		nNodes := strings.Split(c.Nodes, ",")
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
	err = b.CopyFilesToCluster(c.ClusterName, []fileList{fileList{"/root/upgrade.tgz", fnContents, pfilelen}}, nodeList)
	if err != nil {
		return err
	}

	// stop aerospike
	a.opts.Aerospike.Stop.ClusterName = c.ClusterName
	a.opts.Aerospike.Stop.Nodes = c.Nodes
	err = a.opts.Aerospike.Stop.Execute(nil)
	if err != nil {
		return err
	}

	log.Print("Upgrading Aerospike")
	// upgrade
	for _, i := range nodeList {
		// backup aerospike.conf
		nret, err := b.RunCommands(c.ClusterName, [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, []int{i})
		if err != nil {
			return err
		}
		nfile := nret[0]
		out, err := b.RunCommands(c.ClusterName, [][]string{[]string{"tar", "-zxvf", "/root/upgrade.tgz"}}, []int{i})
		if err != nil {
			return fmt.Errorf("%s : %s", string(out[0]), err)
		}
		// upgrade
		out, err = b.RunCommands(c.ClusterName, [][]string{[]string{"/bin/bash", "-c", fmt.Sprintf("export DEBIAN_FRONTEND=noninteractive; cd %s && ./asinstall", strings.TrimSuffix(fn, ".tgz"))}}, []int{i})
		if err != nil {
			return fmt.Errorf("%s : %s", string(out[0]), err)
		}
		// recover aerospike.conf backup
		err = b.CopyFilesToCluster(c.ClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", bytes.NewReader(nfile), len(nfile)}}, []int{i})
		if err != nil {
			return err
		}
	}

	// start aerospike if selected
	if inslice.HasString([]string{"YES", "Y"}, strings.ToUpper(c.RestartAerospike)) {
		a.opts.Aerospike.Start.ClusterName = c.ClusterName
		a.opts.Aerospike.Start.Nodes = c.Nodes
		err = a.opts.Aerospike.Start.Execute(nil)
		if err != nil {
			return err
		}
	}

	log.Print("Done")
	return nil
}
