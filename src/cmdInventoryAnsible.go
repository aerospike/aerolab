package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type HostVars map[string]interface{}
type Hosts map[string]HostVars

type Group struct {
	Hosts []string               `json:"hosts,omitempty"`
	Vars  map[string]interface{} `json:"vars,omitempty"`
}

type Groups map[string]Group

type Meta struct {
	Hostvars Hosts `json:"hostvars"`
}

type AnsibleInventory struct {
	Groups Groups `json:"groups"`
	Meta   Meta   `json:"_meta"`
}

func (a AnsibleInventory) MarshalJSON() ([]byte, error) {
	aux := make(map[string]interface{})

	for key, value := range a.Groups {
		aux[key] = value
	}

	aux["_meta"] = a.Meta

	return json.Marshal(aux)
}

type inventoryAnsibleCmd struct{}

func (c *inventoryAnsibleCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run()
}

func (c *inventoryAnsibleCmd) run() error {
	projectName := os.Getenv("PROJECT_NAME")

	inventoryItems := []int{}
	inventoryItems = append(inventoryItems, InventoryItemClusters)
	inventoryItems = append(inventoryItems, InventoryItemClients)

	inventory, err := b.Inventory("", inventoryItems)
	if err != nil {
		return fmt.Errorf("b.Inventory: %s", err)
	}

	inv := AnsibleInventory{
		Groups: Groups{},
		Meta: Meta{
			Hostvars: Hosts{},
		},
	}

	processCluster := func(clusterName, groupName, project, nodeNo, privateIP, instanceId, sshKeyPath string, awsTags, gcpLabels, dockerLabels map[string]string) {
		inv.Meta.Hostvars[privateIP] = HostVars{
			"ansible_host":    privateIP,
			"instance_id":     instanceId,
			"node_name":       fmt.Sprintf("%s-%s", clusterName, nodeNo),
			"ansible_user":    "root",
			"aerolab_cluster": clusterName,
		}

		if project != "" {
			inv.Meta.Hostvars[privateIP]["project"] = project
		}

		if sshKeyPath != "" {
			inv.Meta.Hostvars[privateIP]["ansible_ssh_private_key_file"] = sshKeyPath
		}

		// Merge all tags/labels into hostvars
		if awsTags != nil {
			for key, value := range awsTags {
				inv.Meta.Hostvars[privateIP][key] = value
			}
		}
		if gcpLabels != nil {
			for key, value := range gcpLabels {
				inv.Meta.Hostvars[privateIP][key] = value
			}
		}
		if dockerLabels != nil {
			for key, value := range dockerLabels {
				inv.Meta.Hostvars[privateIP][key] = value
			}
		}

		if _, exists := inv.Groups[groupName]; !exists {
			inv.Groups[groupName] = Group{Hosts: []string{}, Vars: map[string]interface{}{}}
		}
		group := inv.Groups[groupName]
		group.Hosts = append(group.Hosts, privateIP)
		inv.Groups[groupName] = group
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

		processCluster(cluster.ClusterName, groupName, project, cluster.NodeNo, cluster.PrivateIp, cluster.InstanceId, cluster.SSHKeyPath, cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)
	}

	for _, cluster := range inventory.Clients {
		project := searchField("project", cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)

		if projectName != "" {
			if project != projectName {
				continue
			}
		}

		processCluster(cluster.ClientName, cluster.ClientType, project, cluster.NodeNo, cluster.PrivateIp, cluster.InstanceId, cluster.SSHKeyPath, cluster.AwsTags, cluster.GcpLabels, cluster.DockerLabels)
	}

	inventoryJson, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println(string(inventoryJson))

	return nil
}

func searchField(field string, maps ...map[string]string) string {
	for _, m := range maps {
		if value, ok := m[field]; ok {
			return value
		}
	}

	return ""
}
