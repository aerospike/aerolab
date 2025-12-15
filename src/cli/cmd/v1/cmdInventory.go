package cmd

import (
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/expiry/expire"
)

type InventoryCmd struct {
	List                   InventoryListCmd                   `command:"list" subcommands-optional:"true" description:"List clusters, clients and images" webicon:"fas fa-list"`
	RefreshDiskCache       InventoryRefreshDiskCacheCmd       `command:"refresh-disk-cache" subcommands-optional:"true" description:"Refresh the disk cache" webicon:"fas fa-sync"`
	Ansible                InventoryAnsibleCmd                `command:"ansible" subcommands-optional:"true" description:"Export inventory as ansible inventory" webicon:"fas fa-list"`
	Genders                InventoryGendersCmd                `command:"genders" subcommands-optional:"true" description:"Export inventory as genders file" webicon:"fas fa-list"`
	Hostfile               InventoryHostfileCmd               `command:"hostfile" subcommands-optional:"true" description:"Export inventory as hosts file" webicon:"fas fa-list"`
	InstanceTypes          InventoryInstanceTypesCmd          `command:"instance-types" subcommands-optional:"true" description:"Lookup GCP|AWS available instance types" webicon:"fas fa-table-list"`
	Expire                 InventoryExpireCmd                 `command:"expire" subcommands-optional:"true" description:"Expire resources in the current aerolab project" webicon:"fas fa-trash"`
	DeleteProjectResources InventoryDeleteProjectResourcesCmd `command:"delete-project-resources" subcommands-optional:"true" description:"Delete all resources in the current aerolab project" webicon:"fas fa-trash"`
	InventoryMigrate       InventoryMigrateCmd                `command:"migrate" subcommands-optional:"true" description:"Migrate the inventory to the new AeroLab directory" webicon:"fas fa-arrow-right-to-city"`
	Help                   HelpCmd                            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

type InventoryExpireCmd struct {
	ExpireEksctl bool `long:"aws-expire-eksctl" description:"enable eksctl expiry; AWS only"`
	CleanupDNS   bool `long:"cleanup-dns" description:"enable dns cleanup"`
}

func (c *InventoryExpireCmd) Execute(args []string) error {
	cmd := []string{"inventory", "expire"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)
	err = c.Expire(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryExpireCmd) Expire(system *System, inventory *backends.Inventory, args []string) error {
	h := expire.ExpiryHandler{
		Backend:      system.Backend,
		ExpireEksctl: c.ExpireEksctl,
		CleanupDNS:   c.CleanupDNS,
		Credentials:  system.Backend.GetCredentials(),
	}
	return h.Expire()
}
