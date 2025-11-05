# Attach Commands

Attach commands allow you to connect to cluster nodes and run commands or access Aerospike tools.

## Commands Overview

- `attach shell` - Open shell or run commands on nodes
- `attach aql` - Access Aerospike Query Language (AQL)
- `attach asinfo` - Access Aerospike info command
- `attach asadm` - Access Aerospike Admin (asadm)
- `attach client` - Attach to client tools
- `attach agi` - Attach to AGI (Aerospike Grafana Integration)
- `attach trino` - Attach to Trino

## Common Options

All attach commands support these common options:

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name (comma-separated for multiple) |
| `-l, --nodes` | Node list (`all` for all nodes, ranges like `1-3`) |
| `--parallel` | Execute in parallel on all nodes |

## Attach Shell

Open an interactive shell or run commands on cluster nodes.

### Basic Usage

```bash
aerolab attach shell -n mydc -l 1
```

### Run Commands

Run a command on nodes:

```bash
aerolab attach shell -n mydc -l 1 -- ls /tmp
```

### Run on All Nodes

Run command on all nodes:

```bash
aerolab attach shell -n mydc -l all -- ls /tmp
```

### Run in Parallel

Run command in parallel on all nodes:

```bash
aerolab attach shell -n mydc -l all --parallel -- ls /tmp
```

### Examples

**Interactive shell:**
```bash
aerolab attach shell -n mydc -l 1
```

**Run single command:**
```bash
aerolab attach shell -n mydc -l 1 -- cat /etc/aerospike/aerospike.conf
```

**Run on multiple nodes:**
```bash
aerolab attach shell -n mydc -l 1-3 -- df -h
```

**Run on all nodes:**
```bash
aerolab attach shell -n mydc -l all -- systemctl status aerospike
```

**Run in parallel:**
```bash
aerolab attach shell -n mydc -l all --parallel -- uptime
```

## Attach AQL

Access Aerospike Query Language (AQL) to interact with namespaces and data.

### Basic Usage

```bash
aerolab attach aql -n mydc
```

### Run AQL Commands

Run AQL commands:

```bash
aerolab attach aql -n mydc -- -c "show namespaces"
```

### Common AQL Commands

**Show namespaces:**
```bash
aerolab attach aql -n mydc -- -c "show namespaces"
```

**Show sets:**
```bash
aerolab attach aql -n mydc -- -c "show sets"
```

**Insert data:**
```bash
aerolab attach aql -n mydc -- -c "INSERT INTO test.demoset (PK, data) VALUES ('key1', 'value1')"
```

**Query data:**
```bash
aerolab attach aql -n mydc -- -c "SELECT * FROM test.demoset"
```

**Interactive AQL:**
```bash
aerolab attach aql -n mydc
```

### Examples

**Show namespaces:**
```bash
aerolab attach aql -n mydc -- -c "show namespaces"
```

**Query data:**
```bash
aerolab attach aql -n mydc -- -c "SELECT * FROM test.demoset WHERE PK='key1'"
```

**Insert data:**
```bash
aerolab attach aql -n mydc -- -c "INSERT INTO test.demoset (PK, data, bin1) VALUES ('key1', 'value1', 123)"
```

## Attach Asinfo

Access Aerospike info command for cluster and node information.

### Basic Usage

```bash
aerolab attach asinfo -n mydc
```

### Run Asinfo Commands

Run asinfo commands:

```bash
aerolab attach asinfo -n mydc -- -v "cluster"
```

### Common Asinfo Commands

**Cluster information:**
```bash
aerolab attach asinfo -n mydc -- -v "cluster"
```

**Namespace statistics:**
```bash
aerolab attach asinfo -n mydc -- -v "namespaces"
```

**Cluster stability:**
```bash
aerolab attach asinfo -n mydc -- -v "cluster-stable"
```

**Network information:**
```bash
aerolab attach asinfo -n mydc -- -v "network"
```

**Version information:**
```bash
aerolab attach asinfo -n mydc -- -v "version"
```

**Migration status:**
```bash
aerolab attach asinfo -n mydc -- -v "migrate"
```

### Examples

**Check cluster state:**
```bash
aerolab attach asinfo -n mydc -- -v "cluster"
```

**Check namespace statistics:**
```bash
aerolab attach asinfo -n mydc -- -v "namespace/test"
```

**Check cluster stability:**
```bash
aerolab attach asinfo -n mydc -- -v "cluster-stable:size=5"
```

**Check network:**
```bash
aerolab attach asinfo -n mydc -- -v "network"
```

## Attach Asadm

Access Aerospike Admin (asadm) for cluster management.

### Basic Usage

```bash
aerolab attach asadm -n mydc
```

### Run Asadm Commands

Run asadm commands:

```bash
aerolab attach asadm -n mydc -- -e info
```

### Common Asadm Commands

**Show info:**
```bash
aerolab attach asadm -n mydc -- -e info
```

**Show statistics:**
```bash
aerolab attach asadm -n mydc -- -e "show statistics"
```

**Show configuration:**
```bash
aerolab attach asadm -n mydc -- -e "show config"
```

**Show namespaces:**
```bash
aerolab attach asadm -n mydc -- -e "show namespaces"
```

**Interactive asadm:**
```bash
aerolab attach asadm -n mydc
```

### Examples

**Show cluster info:**
```bash
aerolab attach asadm -n mydc -- -e info
```

**Show statistics:**
```bash
aerolab attach asadm -n mydc -- -e "show statistics namespace test"
```

**Show configuration:**
```bash
aerolab attach asadm -n mydc -- -e "show config"
```

## Attach Client

Attach to client tools on cluster nodes.

### Basic Usage

```bash
aerolab attach client -n mydc -l 1
```

Useful for running client applications or tests.

## Attach AGI

Attach to AGI (Aerospike Grafana Integration) instances.

### Basic Usage

```bash
aerolab attach agi -n mydc
```

Useful for accessing AGI tools and utilities.

## Attach Trino

Attach to Trino instances.

### Basic Usage

```bash
aerolab attach trino -n mydc
```

Useful for accessing Trino SQL interface.

## Cluster Attach

Shorthand for `attach shell` via cluster command.

### Basic Usage

```bash
aerolab cluster attach -n mydc -l 1
```

### Examples

**Interactive shell:**
```bash
aerolab cluster attach -n mydc -l 1
```

**Run command:**
```bash
aerolab cluster attach -n mydc -l all -- ls /tmp
```

**Run in parallel:**
```bash
aerolab cluster attach -n mydc -l all --parallel -- uptime
```

## Common Workflows

### Check Cluster Status

```bash
# Check cluster state
aerolab attach asinfo -n mydc -- -v "cluster"

# Check cluster stability
aerolab attach asinfo -n mydc -- -v "cluster-stable"

# Check namespace statistics
aerolab attach asinfo -n mydc -- -v "namespace/test"
```

### Query and Manipulate Data

```bash
# Show namespaces
aerolab attach aql -n mydc -- -c "show namespaces"

# Insert data
aerolab attach aql -n mydc -- -c "INSERT INTO test.demoset (PK, data) VALUES ('key1', 'value1')"

# Query data
aerolab attach aql -n mydc -- -c "SELECT * FROM test.demoset"
```

### System Administration

```bash
# Check disk usage
aerolab attach shell -n mydc -l all -- df -h

# Check service status
aerolab attach shell -n mydc -l all -- systemctl status aerospike

# Check configuration
aerolab attach shell -n mydc -- cat /etc/aerospike/aerospike.conf

# Check logs
aerolab attach shell -n mydc -- tail -f /var/log/aerospike/aerospike.log
```

### Run Commands on All Nodes

```bash
# Run in parallel
aerolab attach shell -n mydc -l all --parallel -- systemctl status aerospike

# Run sequentially
aerolab attach shell -n mydc -l all -- systemctl status aerospike
```

## Tips

1. **Use `--` separator**: Always use `--` to separate Aerolab options from command options
2. **Parallel execution**: Use `--parallel` for faster execution on multiple nodes
3. **Node filtering**: Use `-l` to target specific nodes or `all` for all nodes
4. **Interactive mode**: Omit command after `--` to enter interactive mode
5. **Multiple clusters**: Use comma-separated cluster names to work with multiple clusters

