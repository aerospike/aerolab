package backend

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	_ "github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
)

type Config backends.Config

func New(project string, c *Config, pollInventoryHourly bool, enabledBackends []backends.BackendType, setInventory *backends.Inventory) (backends.Backend, error) {
	return backends.InternalNew(project, (*backends.Config)(c), pollInventoryHourly, enabledBackends, setInventory)
}
