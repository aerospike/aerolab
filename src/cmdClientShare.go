package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/aerospike/aerolab/parallelize"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientShareCmd struct {
	ClusterName TypeClientName `short:"n" long:"name" description:"Client name" default:"client"`
	KeyFile     flags.Filename `short:"f" long:"pubkey" description:"Path to a pubkey to import to cluster nodes"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientShareCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.share")
	isComm, err := exec.LookPath("ssh-copy-id")
	if (err != nil && !errors.Is(err, exec.ErrDot)) || isComm == "" {
		return errors.New("command `ssh-copy-id` not found; this command relies on existance of `ssh-copy-id`, part of the ssh-client")
	}
	if _, err := os.Stat(string(c.KeyFile)); err != nil {
		return fmt.Errorf("could not access the provided key file %s: %s", string(c.KeyFile), err)
	}
	b.WorkOnClients()
	nodeIpMap, err := b.GetNodeIpMap(c.ClusterName.String(), false)
	if err != nil {
		return fmt.Errorf("could not get cluster node IPs: %s", err)
	}
	nodeIps := []string{}
	for _, ip := range nodeIpMap {
		nodeIps = append(nodeIps, ip)
	}
	myKey, err := b.GetKeyPath(c.ClusterName.String())
	if err != nil {
		return err
	}
	returns := parallelize.MapLimit(nodeIps, c.ParallelThreads, func(ip string) error {
		params := []string{
			"-f",
			"-i",
			string(c.KeyFile),
			"-o",
			"IdentityFile=" + myKey,
			"-o",
			"PreferredAuthentications=publickey",
			"-o",
			"StrictHostKeyChecking=no",
			"root@" + ip,
		}
		out, err := exec.Command("ssh-copy-id", params...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not copy id: %s: %s", err, string(out))
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node IP %s returned %s", nodeIps[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Println("Done")
	return nil
}
