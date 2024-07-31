package main

import (
	"fmt"
	"os"
)

type inventoryHostfileCmd struct{}

func (c *inventoryHostfileCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run()
}

func (c *inventoryHostfileCmd) run() error {
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
		privateIP string,
		instanceId string,
		clusterName string,
		nodeNo string,
	) {
		nodeName := fmt.Sprintf("%s-%s", clusterName, nodeNo)
		entry := fmt.Sprintf("%s\t\t%s\t\t%s", privateIP, instanceId, nodeName)
		processedInventory = append(processedInventory, entry)
	}

	for _, cluster := range inventory.Clusters {
		project := searchField("project", cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)

		if projectName != "" {
			if project != projectName {
				continue
			}
		}

		processCluster(
			cluster.PrivateIp,
			cluster.InstanceId,
			cluster.ClusterName,
			cluster.NodeNo,
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
			cluster.PrivateIp,
			cluster.InstanceId,
			cluster.ClientName,
			cluster.NodeNo,
		)
	}

	for _, entry := range processedInventory {
		fmt.Println(entry)
	}

	return nil
}
