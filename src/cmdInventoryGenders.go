package main

import (
	"fmt"
	"os"
)

type inventoryGendersCmd struct{}

func (c *inventoryGendersCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run()
}

func (c *inventoryGendersCmd) run() error {
	projectName := os.Getenv("PROJECT_NAME")

	inventoryItems := []int{}
	inventoryItems = append(inventoryItems, InventoryItemClusters)
	inventoryItems = append(inventoryItems, InventoryItemClients)

	inventory, err := b.Inventory("", inventoryItems)
	if err != nil {
		return fmt.Errorf("b.Inventory: %s", err)
	}

	var processedInventory []string

	processCluster := func(
		clusterName string,
		nodeNo string,
		groupName string,
		project string,
	) {
		nodeName := fmt.Sprintf("%s-%s", clusterName, nodeNo)
		entry := fmt.Sprintf("%s\t%s,group=%s,project=%s,all,pdsh_rcmd_type=ssh", nodeName, clusterName, groupName, project)
		processedInventory = append(processedInventory, entry)
	}

	for _, cluster := range inventory.Clusters {
		project := searchField("project", cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)

		if projectName != "" {
			if project != projectName {
				continue
			}
		}

		groupName := "aerospike"
		if cluster.Features&ClusterFeatureAGI > 0 {
			groupName = "agi"
		}

		processCluster(
			cluster.ClusterName,
			cluster.NodeNo,
			groupName,
			project,
		)
	}

	for _, cluster := range inventory.Clients {
		project := searchField("project", cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)

		if projectName != "" {
			if project != projectName {
				continue
			}
		}
		processCluster(
			cluster.ClientName,
			cluster.NodeNo,
			cluster.ClientType,
			project,
		)
	}

	for _, entry := range processedInventory {
		fmt.Println(entry)
	}

	return nil
}
