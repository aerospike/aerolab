# AeroLab

AeroLab deploys and manages Aerospike clusters on Docker, AWS, or GCP — for local dev,
testing, and performance benchmarking.

## Quick Start

```bash
# Install (macOS via Homebrew; see Getting Started for other platforms)
brew install aerospike/tools/aerolab

# Configure backend (Docker)
aerolab config backend -t docker

# Create a cluster
aerolab cluster create -n mycluster -c 3

# List clusters
aerolab cluster list
```

Want AWS or GCP instead of Docker? Both need extra setup (credentials, and gotchas like
private-only VPCs) — see [Getting Started](docs/getting-started/README.md).

## Documentation

- [Getting Started](docs/getting-started/README.md) - install, configure a backend, create your first cluster
- [Commands Reference](docs/commands/)
- [Cloud Configuration](docs/cloud/)
- [MCP Server for AI agents](docs/mcp.md) - Drive AeroLab from Claude, Cursor, Codex, etc. via `aerolab mcp`
- [Migration Guide](docs/migration-guide.md) - Upgrading from AeroLab 7.x to 8.x

## Migration from v7.x

If upgrading from AeroLab 7.x, see the [Migration Guide](docs/migration-guide.md) for details on migrating your configuration and inventory.
