package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type clientStartCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client names, comma separated OR 'all' to affect all clusters" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
	clientStartStopDestroyCmd
}

type clientStopCmd struct {
	clientStartCmd
}

type clientDestroyCmd struct {
	clientStartCmd
	Docker clusterDestroyCmdDocker `no-flag:"true"`
}

func (c *clientStartCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.runStart(args)
}

func (c *clientStartCmd) runStart(args []string) error {
	log.Println("Running client.start")
	b.WorkOnClients()
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
	if err != nil {
		return err
	}
	var nerr error
	scriptErr := false
	for _, ClusterName := range cList {
		err = b.ClusterStart(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
		if err == nil {
			// generic startup scripts
			out, err := b.RunCommands(ClusterName, [][]string{[]string{"/bin/bash", "-c", "[ ! -d /opt/autoload ] && exit 0; ls /opt/autoload |sort -n |while read f; do /bin/bash /opt/autoload/${f}; done"}}, nodes[ClusterName])
			if err != nil {
				scriptErr = true
				prt := ""
				for i, o := range out {
					prt = prt + "\n ---- " + strconv.Itoa(i) + " ----\n" + string(o)
				}
				log.Printf("Some startup sripts returned an error (%s). Outputs:%s", err, prt)
			}
			// custom startup script
			out, err = b.RunCommands(ClusterName, [][]string{[]string{"/bin/bash", "/usr/local/bin/start.sh"}}, nodes[ClusterName])
			if err != nil {
				scriptErr = true
				prt := ""
				for i, o := range out {
					prt = prt + "\n ---- " + strconv.Itoa(i) + " ----\n" + string(o)
				}
				log.Printf("Some startup sripts returned an error (%s). Outputs:%s", err, prt)
			}
		}
	}
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	if scriptErr {
		return errors.New("SOME SCRIPTS RETURNED ERRORS")
	}
	return nil
}

func (c *clientDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running client.destroy")
	b.WorkOnClients()
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		if c.Docker.Force && a.opts.Config.Backend.Type == "docker" {
			b.ClusterStop(ClusterName, nodes[ClusterName])
		}
		err = b.ClusterDestroy(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
	}
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	return nil
}

func (c *clientStopCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.runStop(args)
}

func (c *clientStopCmd) runStop(args []string) error {
	b.WorkOnClients()
	log.Println("Running client.stop")
	err := c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClientName), c.Machines.String())
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		err = b.ClusterStop(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
	}
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	return nil
}

type clientStartStopDestroyCmd struct {
}

func (c *clientStartStopDestroyCmd) getBasicData(clusterName string, Nodes string) (cList []string, nodes map[string][]int, err error) {
	// check cluster exists
	clusterList, err := b.ClusterList()
	if err != nil {
		return nil, nil, err
	}
	if clusterName != "all" && clusterName != "ALL" {
		cList = strings.Split(clusterName, ",")
	} else {
		cList = clusterList
	}
	for _, clusterName = range cList {
		if !inslice.HasString(clusterList, clusterName) {
			err = fmt.Errorf("client group does not exist: %s", clusterName)
			return nil, nil, err
		}
	}
	nodes = make(map[string][]int)
	var nodesC []int
	if Nodes == "" || Nodes == "all" || Nodes == "ALL" {
		for _, clusterName = range cList {
			nodesC, err = b.NodeListInCluster(clusterName)
			if err != nil {
				return nil, nil, err
			}
			nodes[clusterName] = nodesC
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return nil, nil, err
			}
			nodesC = append(nodesC, nodeInt)
		}
		for _, clusterName = range cList {
			nodes[clusterName] = nodesC
		}
	}
	for _, clusterName = range cList {
		if len(nodes[clusterName]) == 0 {
			err = errors.New("found 0 machines in client group")
			return nil, nil, err
		}
	}
	return
}
