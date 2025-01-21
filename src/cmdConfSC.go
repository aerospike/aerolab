package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	"github.com/aerospike/aerolab/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
)

type confSCCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Namespace   string          `short:"m" long:"namespace" description:"Namespace to change" default:"test"`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf" default:"/etc/aerospike/aerospike.conf"`
	Force       bool            `short:"f" long:"force" description:"If set, will zero out the devices even if strong-consistency was already configured"`
	Racks       int             `short:"r" long:"racks" description:"If rack-aware feature is required, set this to the number of racks you want to divide the cluster into"`
	WithDisks   bool            `short:"d" long:"with-disks" description:"If set, will attempt to configure device storage engine for the namespace, using all available devices"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confSCCmd) Execute(args []string) error {
	if earlyProcessV2(nil, true) {
		return nil
	}
	log.Println("Running conf.sc")
	// stop aerospike
	log.Println("conf.sc: Stopping aerospike")
	a.opts.Aerospike.Stop.ClusterName = c.ClusterName
	a.opts.Aerospike.Stop.ParallelThreads = c.ParallelThreads
	err := a.opts.Aerospike.Stop.run(nil, "stop", os.Stdout)
	if err != nil {
		return err
	}
	// use partitioner
	if c.WithDisks {
		log.Println("conf.sc: Partitioning all available devices")
		a.opts.Cluster.Partition.Create.ClusterName = c.ClusterName
		a.opts.Cluster.Partition.Create.ParallelThreads = c.ParallelThreads
		a.opts.Cluster.Partition.Create.Partitions = "24,24,24,24"
		err = a.opts.Cluster.Partition.Create.Execute(nil)
		if err != nil {
			return err
		}
		a.opts.Cluster.Partition.Conf.ClusterName = c.ClusterName
		a.opts.Cluster.Partition.Conf.ParallelThreads = c.ParallelThreads
		a.opts.Cluster.Partition.Conf.FilterPartitions = "1-4"
		a.opts.Cluster.Partition.Conf.ConfDest = "device"
		a.opts.Cluster.Partition.Conf.Namespace = c.Namespace
		err = a.opts.Cluster.Partition.Conf.Execute(nil)
		if err != nil {
			return err
		}
	}
	// get node count
	log.Println("conf.sc: Getting cluster size")
	nodes, err := b.NodeListInCluster(c.ClusterName.String())
	if err != nil {
		return err
	}
	// patch aerospike.conf
	log.Println("conf.sc: Patching aerospike.conf")
	returns := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
		// read config file
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"cat", c.Path}}, []int{node})
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", c.ClusterName, nout, err)
		}
		fileContents := bytes.NewReader(out[0])
		// edit actual file contents
		s, err := aeroconf.Parse(fileContents)
		if err != nil {
			return err
		}
		if s.Type("namespace "+c.Namespace) != aeroconf.ValueStanza {
			return errors.New("namespace not found")
		}
		changes := false
		x := s.Stanza("namespace " + c.Namespace)
		// check RF
		if x.Type("replication-factor") == aeroconf.ValueString {
			vals, err := x.GetValues("replication-factor")
			if err != nil {
				return err
			}
			if len(vals) != 1 {
				return errors.New("replication-factor parameter error")
			}
			rf, err := strconv.Atoi(*vals[0])
			if err != nil {
				return errors.New("replication-factor parameter invalid value found")
			}
			if rf > len(nodes) {
				x.SetValue("replication-factor", strconv.Itoa(len(nodes)))
				changes = true
			}
		} else if len(nodes) == 1 {
			x.SetValue("replication-factor", "1")
			changes = true
		}
		// get SC
		rmFiles := false
		if x.Type("strong-consistency") != aeroconf.ValueString {
			x.SetValue("strong-consistency", "true")
			changes = true
			rmFiles = true
		} else {
			vals, err := x.GetValues("strong-consistency")
			if err != nil {
				return err
			}
			if len(vals) != 1 {
				return errors.New("strong-consistency parameter error")
			}
			if *vals[0] != "true" {
				x.SetValue("strong-consistency", "true")
				changes = true
				rmFiles = true
			}
		}
		// remove storage files
		if rmFiles || c.Force {
			if x.Type("storage-engine device") == aeroconf.ValueStanza {
				if x.Stanza("storage-engine device").Type("file") == aeroconf.ValueString {
					files, err := x.Stanza("storage-engine device").GetValues("file")
					if err != nil {
						return err
					}
					cmd := []string{"rm", "-f"}
					for _, file := range files {
						cmd = append(cmd, *file)
					}
					data, err := b.RunCommands(string(c.ClusterName), [][]string{cmd}, []int{node})
					if len(data) == 0 {
						data = [][]byte{{'-'}}
					}
					if err != nil {
						return fmt.Errorf("%s: %s", err, string(data[0]))
					}
				}
			}
		}
		// configure rackid
		if c.Racks > 0 {
			nodesPerRack := int(math.Ceil(float64(len(nodes)) / float64(c.Racks)))
			nodeRack := ((node - 1) / nodesPerRack) + 1
			if x.Type("rack-id") != aeroconf.ValueString {
				x.SetValue("rack-id", strconv.Itoa(nodeRack))
				changes = true
			} else {
				vals, err := x.GetValues("rack-id")
				if err != nil {
					return err
				}
				if *vals[0] != strconv.Itoa(nodeRack) {
					x.SetValue("rack-id", strconv.Itoa(nodeRack))
					changes = true
				}
			}
		}
		// store changes back
		if changes {
			var buf bytes.Buffer
			err = s.Write(&buf, "", "    ", true)
			if err != nil {
				return err
			}
			contents := buf.Bytes()
			fileContents = bytes.NewReader(contents)
			// edit end
			err = b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{filePath: c.Path, fileContents: fileContents, fileSize: len(contents)}}, []int{node})
			if err != nil {
				return err
			}
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
	// restart aerospike
	log.Println("conf.sc: Cold-Starting aerospike")
	a.opts.Aerospike.ColdStart.ClusterName = c.ClusterName
	a.opts.Aerospike.ColdStart.ParallelThreads = c.ParallelThreads
	err = a.opts.Aerospike.ColdStart.run(nil, "cold-start", os.Stdout)
	if err != nil {
		return err
	}
	// wait for cluster to be stable
	log.Println("conf.sc: Waiting for cluster to be stable")
	a.opts.Aerospike.IsStable.ClusterName = c.ClusterName
	a.opts.Aerospike.IsStable.ParallelThreads = c.ParallelThreads
	a.opts.Aerospike.IsStable.Wait = true
	a.opts.Aerospike.IsStable.IgnoreMigrations = true
	a.opts.Aerospike.IsStable.Namespace = c.Namespace
	err = a.opts.Aerospike.IsStable.Execute(nil)
	if err != nil {
		return err
	}
	// apply roster
	log.Println("conf.sc: Applying roster")
	a.opts.Roster.Apply.ClusterName = c.ClusterName
	a.opts.Roster.Apply.Namespace = c.Namespace
	a.opts.Roster.Apply.ParallelThreads = c.ParallelThreads
	a.opts.Roster.Apply.Quiet = true
	err = a.opts.Roster.Apply.Execute(nil)
	if err != nil {
		return err
	}
	log.Println("conf.sc: Done")
	return nil
}
