package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type XdrCreateClustersCmd struct {
	ClusterCreateCmd
	DestinationClusterNames TypeClusterName `short:"N" long:"destinations" description:"Comma-separated list of destination cluster names" default:"destdc"`
	DestinationNodeCount    int             `short:"C" long:"destination-count" description:"Number of nodes per destination cluster" default:"1"`
	XdrVersion              TypeXDRVersion  `short:"V" long:"xdr-version" description:"Specify aerospike xdr configuration version (4|5|auto)" default:"auto" webchoice:"auto,5,4"`
	XdrRestart              TypeYesNo       `short:"T" long:"restart-source" description:"Restart source nodes after connecting (y/n)" default:"y" webchoice:"y,n"`
	XdrNamespaces           string          `short:"M" long:"namespaces" description:"Comma-separated list of namespaces to connect" default:"test"`
	CustomDestinationPort   int             `short:"P" long:"destination-port" description:"Optionally specify a custom destination port for the xdr connection"`
}

func (c *XdrCreateClustersCmd) Execute(args []string) error {
	cmd := []string{"xdr", "create-clusters"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.createClusters(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *XdrCreateClustersCmd) createClusters(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	destinations := strings.Split(c.DestinationClusterNames.String(), ",")

	// Check if any clusters already exist
	for _, dest := range destinations {
		existing := inventory.Instances.WithClusterName(dest)
		if existing != nil && existing.Count() > 0 {
			return fmt.Errorf("cluster %s already exists", dest)
		}
	}

	// Check if source cluster exists
	srcCluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if srcCluster != nil && srcCluster.Count() > 0 {
		return fmt.Errorf("source cluster %s already exists", c.ClusterName.String())
	}

	// Create source cluster
	logger.Info("Creating source cluster %s with %d nodes", c.ClusterName.String(), c.NodeCount)
	_, err := c.ClusterCreateCmd.CreateCluster(system, inventory, logger, args, "create")
	if err != nil {
		return fmt.Errorf("failed to create source cluster: %w", err)
	}

	// Refresh inventory after creating source cluster
	inventory = system.Backend.GetInventory()

	// Save source cluster settings
	srcClusterName := c.ClusterName
	srcNodeCount := c.NodeCount

	// Create destination clusters
	c.NodeCount = c.DestinationNodeCount
	for _, dest := range destinations {
		logger.Info("Creating destination cluster %s with %d nodes", dest, c.NodeCount)
		c.ClusterName = TypeClusterName(dest)
		_, err := c.ClusterCreateCmd.CreateCluster(system, inventory, logger, args, "create")
		if err != nil {
			return fmt.Errorf("failed to create destination cluster %s: %w", dest, err)
		}
		// Refresh inventory after each cluster creation
		inventory = system.Backend.GetInventory()
	}

	// Restore source cluster settings
	c.ClusterName = srcClusterName
	c.NodeCount = srcNodeCount

	// Refresh inventory cache before connecting to ensure we have the latest instance states
	logger.Info("Refreshing inventory cache")
	err = system.Backend.RefreshChangedInventory()
	if err != nil {
		return fmt.Errorf("failed to refresh inventory: %w", err)
	}
	inventory = system.Backend.GetInventory()

	// Now connect clusters via XDR
	logger.Info("Connecting clusters via XDR")
	xdrConnect := &XdrConnectCmd{
		SourceClusterName:       srcClusterName,
		DestinationClusterNames: c.DestinationClusterNames,
		IsConnector:             false, // we are creating clusters, not connectors
		Version:                 c.XdrVersion,
		Restart:                 c.XdrRestart,
		Namespaces:              c.XdrNamespaces,
		CustomDestinationPort:   c.CustomDestinationPort,
		ParallelThreads:         c.ParallelThreads,
	}

	err = xdrConnect.connect(system, inventory, logger, args)
	if err != nil {
		return fmt.Errorf("failed to connect clusters via XDR: %w", err)
	}

	return nil
}
