# AeroLab

AeroLab is a tool for deploying and managing Aerospike clusters across multiple cloud providers and Docker.

## Documentation

- [Getting Started](docs/getting-started/)
- [Commands Reference](docs/commands/)
- [Cloud Configuration](docs/cloud/)
- [Migration Guide](docs/migration-guide.md) - Upgrading from AeroLab 7.x to 8.x

## Quick Start

```bash
# Configure backend (Docker)
aerolab config backend -t docker

# Create a cluster
aerolab cluster create -n mycluster -c 3

# List instances
aerolab instances list
```

## Migration from v7.x

If upgrading from AeroLab 7.x, see the [Migration Guide](docs/migration-guide.md) for details on migrating your configuration and inventory.
