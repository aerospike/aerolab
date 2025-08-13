package cmd

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryAnsibleCmd struct {
	IPType      string  `short:"i" long:"ip-type" description:"IP type to use (private, public)" default:"private"`
	OnlyRunning bool    `short:"r" long:"only-running" description:"Only include running instances"`
	Help        HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryAnsibleCmd) Execute(args []string) error {
	cmd := []string{"inventory", "ansible"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.InventoryAnsible(system, cmd, args, system.Backend.GetInventory(), os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryAnsibleCmd) InventoryAnsible(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).Describe()
	if c.OnlyRunning {
		instances = instances.WithState(backends.LifeCycleStateRunning).Describe()
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].ClusterName != instances[j].ClusterName {
			return instances[i].ClusterName < instances[j].ClusterName
		}
		return instances[i].NodeNo < instances[j].NodeNo
	})

	type ansibleHost struct {
		Cluster    string `json:"aerolab_cluster"`
		Node       string `json:"aerolab_node"`
		Host       string `json:"ansible_host"`
		User       string `json:"ansible_user"`
		InstanceId string `json:"instance_id"`
		NodeName   string `json:"node_name"`
		SSHKey     string `json:"ansible_ssh_private_key_file"`
	}
	ansible := make(map[string]interface{})
	ansible["_meta"] = make(map[string]interface{})
	ansible["_meta"].(map[string]interface{})["hostvars"] = make(map[string]ansibleHost)
	for _, inst := range instances.Describe() {
		ip := inst.IP.Private
		if c.IPType == "public" {
			ip = inst.IP.Public
		}
		// get ssh key path
		sshKeyPath := inst.GetSSHKeyPath()
		// add instance to hostvars
		ansible["_meta"].(map[string]interface{})["hostvars"].(map[string]ansibleHost)[ip] = ansibleHost{
			Cluster:    inst.ClusterName,
			Node:       strconv.Itoa(inst.NodeNo),
			Host:       ip,
			User:       "root",
			InstanceId: inst.InstanceID,
			NodeName:   inst.ClusterName + "-" + strconv.Itoa(inst.NodeNo),
			SSHKey:     sshKeyPath,
		}
		// add instance to type lists
		ntype := inst.Tags["aerolab.type"]
		if ntype == "" {
			ntype = "none"
		}
		if _, ok := ansible[ntype]; !ok {
			ansible[ntype] = map[string][]string{
				"hosts": {},
			}
		}
		ansible[ntype].(map[string][]string)["hosts"] = append(ansible[ntype].(map[string][]string)["hosts"], ip)
	}
	return json.NewEncoder(out).Encode(ansible)
}
