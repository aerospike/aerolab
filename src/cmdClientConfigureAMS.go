package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
)

type clientConfigureAMSCmd struct {
	ClientName      TypeClientName  `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines        TypeMachines    `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	ConnectClusters TypeClusterName `short:"s" long:"clusters" description:"comma-separated list of clusters to configure as source for this AMS"`
	ConnectClients  TypeClientName  `short:"S" long:"clients" description:"comma-separated list of clients to configure as source for this AMS"`
	Help            helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureAMSCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.ams")
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	if c.ConnectClients == "" && c.ConnectClusters == "" {
		return errors.New("either clients or clusters must be specified")
	}
	var nodeList map[string][]string
	var clientList map[string][]string
	var err error
	if c.ConnectClusters != "" {
		nodeList, err = c.checkClustersExist(c.ConnectClusters.String())
		if err != nil {
			return err
		}
	}
	if c.ConnectClients != "" {
		b.WorkOnClients()
		clientList, err = c.checkClustersExist(c.ConnectClients.String())
		if err != nil {
			return err
		}
	}
	a.opts.Attach.Client.Machine = c.Machines
	b.WorkOnClients()
	allnodes := []string{}
	allnodeExp := []string{}
	allClients := []string{}
	for _, nodes := range nodeList {
		for _, node := range nodes {
			allnodes = append(allnodes, node+":9145")
			allnodeExp = append(allnodeExp, node+":9100")
		}
	}
	for _, nodes := range clientList {
		for _, node := range nodes {
			allClients = append(allClients, node+":9090")
		}
	}
	ips := "'" + strings.Join(allnodes, "','") + "'"
	nips := "'" + strings.Join(allnodeExp, "','") + "'"
	cips := "'" + strings.Join(allClients, "','") + "'"
	defer backendRestoreTerminal()
	if len(allnodes) != 0 || len(allnodeExp) != 0 {
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakX", "-E", "s/.*TODO_ASD_TARGETS/      - targets: [" + ips + "] #TODO_ASD_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed): %s", err)
		}
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakY", "-E", "s/.*TODO_ASDN_TARGETS/      - targets: [" + nips + "] #TODO_ASDN_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed.1): %s", err)
		}
	}
	if len(allClients) != 0 {
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakY", "-E", "s/.*TODO_CLIENT_TARGETS/      - targets: [" + cips + "] #TODO_CLIENT_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed.1): %s", err)
		}
	}
	// (re)start prometheus
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "kill -HUP $(pidof prometheus)"})
	if err != nil {
		return fmt.Errorf("failed to restart prometheus: %s", err)
	}
	backendRestoreTerminal()
	log.Printf("To access grafana, visit the client IP on port 3000 from your browser. Do `aerolab client list` to get IPs. Username:Password is admin:admin")
	log.Print("Done")
	return nil
}
