package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryDeleteProjectResourcesCmd struct {
	Expiry bool    `long:"expiry" description:"Also remove the expiry system; WARN: expiry system is NOT project-bound but global"`
	Force  bool    `short:"f" long:"force" description:"Force deletion without confirmation"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryDeleteProjectResourcesCmd) Execute(args []string) error {
	cmd := []string{"inventory", "delete-project-resources"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)()
	err = c.DeleteProjectResources(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryDeleteProjectResourcesCmd) DeleteProjectResources(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"inventory", "delete-project-resources"}, c, args...)
		if err != nil {
			return err
		}
	}

	projectName := os.Getenv("AEROLAB_PROJECT")
	if projectName == "" {
		projectName = "default"
	}

	// Get inventory and display what will be deleted
	inv := system.Backend.GetInventory()
	backendType := backends.BackendType(system.Opts.Config.Backend.Type)
	instances := inv.Instances.WithBackendType(backendType).Describe()
	volumes := inv.Volumes.WithBackendType(backendType).Describe()
	firewalls := inv.Firewalls.WithBackendType(backendType).Describe()
	images := inv.Images.WithBackendType(backendType).WithInAccount(true).Describe()

	totalResources := len(instances) + len(volumes) + len(firewalls) + len(images)

	if totalResources == 0 {
		system.Logger.Info("No resources found for project '%s'", projectName)
		return nil
	}

	// Print summary
	system.Logger.Warn("=== Resources to be deleted for project '%s' ===", projectName)
	fmt.Println()

	// Print instances
	if len(instances) > 0 {
		system.Logger.Warn("INSTANCES (%d):", len(instances))
		for _, inst := range instances {
			fmt.Printf("  - %s (name: %s, cluster: %s, node: %d, zone: %s, state: %s)\n",
				inst.InstanceID, inst.Name, inst.ClusterName, inst.NodeNo, inst.ZoneName, inst.InstanceState.String())
		}
		fmt.Println()
	}

	// Print volumes
	if len(volumes) > 0 {
		system.Logger.Warn("VOLUMES (%d):", len(volumes))
		for _, vol := range volumes {
			attached := ""
			if len(vol.AttachedTo) > 0 {
				attached = fmt.Sprintf(", attached to: %s", strings.Join(vol.AttachedTo, ", "))
			}
			sizeStr := formatStorageSize(vol.Size)
			fmt.Printf("  - %s (size: %s, zone: %s%s)\n",
				vol.Name, sizeStr, vol.ZoneName, attached)
		}
		fmt.Println()
	}

	// Print firewalls
	if len(firewalls) > 0 {
		system.Logger.Warn("FIREWALLS (%d):", len(firewalls))
		for _, fw := range firewalls {
			fmt.Printf("  - %s (network: %s)\n", fw.Name, fw.Network.NetworkId)
		}
		fmt.Println()
	}

	// Print images
	if len(images) > 0 {
		system.Logger.Warn("IMAGES (%d):", len(images))
		for _, img := range images {
			fmt.Printf("  - %s (%s)\n", img.ImageId, img.Name)
		}
		fmt.Println()
	}

	// Print expiry system warning if flag is set
	if c.Expiry {
		system.Logger.Warn("EXPIRY SYSTEM:")
		fmt.Printf("  - The global expiry system will be removed from all enabled regions\n")
		fmt.Printf("  - WARNING: This affects ALL projects, not just '%s'\n", projectName)
		fmt.Println()
	}

	// Print summary line
	summaryParts := []string{
		fmt.Sprintf("%d instances", len(instances)),
		fmt.Sprintf("%d volumes", len(volumes)),
		fmt.Sprintf("%d firewalls", len(firewalls)),
		fmt.Sprintf("%d images", len(images)),
	}
	if c.Expiry {
		summaryParts = append(summaryParts, "expiry system")
	}
	system.Logger.Warn("TOTAL: %s", strings.Join(summaryParts, ", "))
	fmt.Println()

	system.Logger.Warn("WARNING: This action cannot be undone!")

	if !c.Force && IsInteractive() {
		var input string
		fmt.Printf("Enter project name '%s' to confirm deletion: ", projectName)
		_, err := fmt.Scanln(&input)
		if err != nil {
			return err
		}

		if strings.TrimSuffix(input, "\n") != projectName {
			return errors.New("project name does not match")
		}
	}

	system.Logger.Info("Deleting resources...")
	err := system.Backend.DeleteProjectResources(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}
	if c.Expiry {
		system.Logger.Info("Removing expiry system...")
		zones, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
		if err != nil {
			return err
		}
		err = system.Backend.ExpiryRemove(backends.BackendType(system.Opts.Config.Backend.Type), zones...)
		if err != nil {
			return err
		}
	}
	return nil
}

// formatStorageSize converts a StorageSize to a human-readable string
func formatStorageSize(size backends.StorageSize) string {
	if size >= backends.StorageTiB {
		return fmt.Sprintf("%.1f TiB", float64(size)/float64(backends.StorageTiB))
	}
	if size >= backends.StorageGiB {
		return fmt.Sprintf("%.1f GiB", float64(size)/float64(backends.StorageGiB))
	}
	if size >= backends.StorageMiB {
		return fmt.Sprintf("%.1f MiB", float64(size)/float64(backends.StorageMiB))
	}
	if size >= backends.StorageKiB {
		return fmt.Sprintf("%.1f KiB", float64(size)/float64(backends.StorageKiB))
	}
	return fmt.Sprintf("%d bytes", size)
}
