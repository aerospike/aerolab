# Using AeroLab as a Library

AeroLab can be used as a Go library to programmatically create, manage, and operate Aerospike clusters across multiple cloud providers (AWS, GCP) and Docker.

## Import

```go
import (
    cmd "github.com/aerospike/aerolab/cli/cmd/v1"
    "github.com/aerospike/aerolab/pkg/backend/backends"
    "github.com/aerospike/aerolab/pkg/backend/clouds"  // for GCP auth methods
)
```

## Key Types

### System

The `System` struct is the main entry point after initialization:

| Field | Type | Description |
|-------|------|-------------|
| `Logger` | `*logger.Logger` | Logger instance for your application |
| `Opts` | `*Commands` | All available commands and their configuration |
| `Backend` | `backends.Backend` | Backend interface for cloud/Docker operations |
| `InitOptions` | `*Init` | The init options used during initialization |

### Init

Controls the initialization behavior:

| Field | Type | Description |
|-------|------|-------------|
| `InitBackend` | `bool` | Whether to initialize the backend during `Initialize()` |
| `UpgradeCheck` | `bool` | Check for available AeroLab upgrades |
| `Backend` | `*InitBackend` | Backend-specific configuration overrides |
| `ExistingInventory` | `*backends.Inventory` | Pass existing inventory to avoid re-fetching |

### InitBackend

Backend-specific configuration options:

| Field | Type | Description |
|-------|------|-------------|
| `PollInventoryHourly` | `bool` | Auto-refresh inventory in background (for long-running services) |
| `UseCache` | `bool` | Use local disk cache for inventory |
| `LogMillisecond` | `bool` | Include milliseconds in log timestamps |
| `ListAllProjects` | `bool` | GCP: list all projects in inventory |
| `GCPAuthMethod` | `clouds.GCPAuthMethod` | GCP authentication method |
| `GCPBrowser` | `bool` | GCP: open browser for login authentication |

## Backend Configuration Requirements

| Backend | Required Fields |
|---------|-----------------|
| Docker | `Type = "docker"` |
| AWS | `Type = "aws"`, `Region` |
| GCP | `Type = "gcp"`, `Region`, `Project` |

## Complete Example

```go
package main

import (
    "fmt"
    "os"

    cmd "github.com/aerospike/aerolab/cli/cmd/v1"
    "github.com/aerospike/aerolab/pkg/backend/backends"
)

func main() {
    // 1. Create the aerolab home directory if needed
    ahome, err := cmd.AerolabRootDir()
    if err != nil {
        panic(err)
    }
    os.MkdirAll(ahome, 0700)

    // 2. Initialize WITHOUT the backend first (to read existing config)
    system, err := cmd.Initialize(&cmd.Init{
        InitBackend:  false,  // Important: false here
        UpgradeCheck: false,
    }, nil, nil)
    if err != nil {
        panic(err)
    }

    // 3. Check if backend is already configured
    if system.Opts.Config.Backend.Type == "" || system.Opts.Config.Backend.Type == "none" {
        // Backend not configured - set it up
        fmt.Println("Backend not configured, setting up...")

        // Configure the backend settings programmatically
        // Option A: Docker (simplest, no cloud credentials needed)
        system.Opts.Config.Backend.Type = "docker"

        // Option B: AWS
        // system.Opts.Config.Backend.Type = "aws"
        // system.Opts.Config.Backend.Region = "us-east-1"  // required
        // system.Opts.Config.Backend.AWSProfile = ""       // optional, uses default if empty

        // Option C: GCP
        // system.Opts.Config.Backend.Type = "gcp"
        // system.Opts.Config.Backend.Project = "my-gcp-project"  // required
        // system.Opts.Config.Backend.Region = "us-central1"      // required

        // 4. Initialize the backend
        err = system.GetBackend(false)  // false = don't poll inventory hourly
        if err != nil {
            panic(fmt.Errorf("failed to initialize backend: %w", err))
        }

        // 5. Save the config file for future runs
        err = system.WriteConfigFile()
        if err != nil {
            fmt.Println("Warning: could not save config file:", err)
        }
        fmt.Println("Backend configured and saved")
    } else {
        // Backend already configured, just initialize it
        fmt.Printf("Using existing backend: %s\n", system.Opts.Config.Backend.Type)
        err = system.GetBackend(false)
        if err != nil {
            panic(fmt.Errorf("failed to initialize backend: %w", err))
        }
    }

    // 6. Now you can use the backend
    inventory := system.Backend.GetInventory()

    // Example: List all running instances
    instances := inventory.Instances.WithState(backends.LifeCycleStateRunning)
    fmt.Printf("Found %d running instances\n", instances.Count())

    for _, inst := range instances.Describe() {
        fmt.Printf("  Cluster: %s, Node: %d, State: %s, IP: %s\n",
            inst.ClusterName, inst.NodeNo, inst.State, inst.PublicIP)
    }

    // Example: Call a command function directly
    listCmd := &cmd.InventoryListCmd{
        Output: "json",
    }
    err = listCmd.InventoryList(system, []string{"inventory", "list"},
        []string{}, inventory, os.Stdout)
    if err != nil {
        fmt.Println("Error listing inventory:", err)
    }
}
```

## Calling Command Functions

Each CLI command has two methods:

- `Execute(args []string) error` - CLI entry point (re-initializes system, not for library use)
- Internal function (e.g., `ClusterCreate()`, `InventoryList()`) - for library use

Always call the internal function when using AeroLab as a library:

```go
// Create a command struct with parameters
createCmd := &cmd.ClusterCreateCmd{
    // Set fields matching CLI flags
}

// Call the internal function, NOT Execute()
err = createCmd.ClusterCreate(system, []string{"cluster", "create"}, []string{}, inventory)
```

## Key Methods

| Method | Description |
|--------|-------------|
| `cmd.AerolabRootDir()` | Get the AeroLab home directory path |
| `cmd.Initialize(init, cmd, params, args...)` | Initialize the system |
| `system.GetBackend(pollHourly)` | Initialize or reinitialize the backend |
| `system.WriteConfigFile()` | Save current config to disk |
| `system.Backend.GetInventory()` | Get the current inventory |
| `system.Backend.RefreshChangedInventory()` | Refresh cache after making changes |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AEROLAB_HOME` | Custom AeroLab home directory |
| `AEROLAB_CONFIG_FILE` | Custom configuration file path |
| `AEROLAB_LOG_LEVEL` | Log level: DEBUG, INFO, DETAIL, WARNING, ERROR, CRITICAL |
