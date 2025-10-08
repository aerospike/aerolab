package backend

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
)

// Config is an alias for backends.Config, providing a simplified interface for backend configuration.
// It contains all necessary settings for initializing cloud backends including credentials,
// logging, caching, and project settings.
type Config backends.Config

// New creates and initializes a new multi-cloud backend instance with the specified configuration.
// This is the main entry point for creating backend instances that can manage resources across
// multiple cloud providers simultaneously.
//
// The function automatically registers and initializes all available cloud backends (AWS, GCP, Docker)
// based on the enabledBackends parameter. It sets up inventory polling, credential management,
// and caching as specified in the configuration.
//
// Parameters:
//   - project: The project name/identifier for resource organization and tagging
//   - c: Backend configuration containing credentials, settings, and options
//   - pollInventoryHourly: Whether to automatically refresh inventory every hour
//   - enabledBackends: List of backend types to enable (AWS, GCP, Docker, etc.)
//   - setInventory: Optional pre-populated inventory to use instead of discovering resources
//
// Returns:
//   - backends.Backend: The initialized backend interface for managing cloud resources
//   - error: nil on success, or an error if initialization fails
//
// Usage:
//
//	config := &backend.Config{
//	    RootDir:         "/path/to/aerolab",
//	    Cache:           true,
//	    LogLevel:        4,
//	    AerolabVersion:  "8.0.0",
//	}
//
//	backend, err := backend.New("my-project", config, true,
//	    []backends.BackendType{backends.BackendTypeAWS, backends.BackendTypeGCP}, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
func New(project string, c *Config, pollInventoryHourly bool, enabledBackends []backends.BackendType, setInventory *backends.Inventory) (backends.Backend, error) {
	return backends.InternalNew(project, (*backends.Config)(c), pollInventoryHourly, enabledBackends, setInventory)
}
