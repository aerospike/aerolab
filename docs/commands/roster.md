# Roster Management Commands

Roster commands help manage Aerospike's Strong Consistency (SC) roster, which defines the set of nodes that participate in strong consistency operations.

## Commands Overview

- `roster show` - Show current roster configuration
- `roster apply` - Apply roster configuration to the cluster
- `roster cheat` - Display Strong Consistency quick reference guide

## What is a Roster?

In Aerospike Strong Consistency mode, the **roster** is the list of nodes that participate in consensus for data operations. The roster must be explicitly managed when:

- Adding nodes to a Strong Consistency cluster
- Removing nodes from a Strong Consistency cluster
- Recovering from node failures
- Changing cluster topology

## Prerequisites

- Cluster must be configured with Strong Consistency
- Use `conf sc` command to enable Strong Consistency first
- Requires Aerospike 4.0+ for Strong Consistency support

## Roster Show

Display the current roster configuration for a namespace.

### Basic Usage

```bash
# Show roster for default namespace
aerolab roster show -n mycluster

# Show roster for specific namespace
aerolab roster show -n mycluster --namespace test
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Cluster name | `mydc` |
| `-l, --nodes` | Nodes to query (comma-separated) | All nodes |
| `--namespace` | Namespace to check | `test` |

### Output Example

```
Node 1 (172.17.0.2):
  Namespace: test
  Roster:
    - BB9070016AE4202 (172.17.0.2:3000)
    - BB9080016AE4203 (172.17.0.3:3000)
    - BB9090016AE4204 (172.17.0.4:3000)
  Observed Nodes: 3
  Roster Generation: 1

Node 2 (172.17.0.3):
  Namespace: test
  Roster:
    - BB9070016AE4202 (172.17.0.2:3000)
    - BB9080016AE4203 (172.17.0.3:3000)
    - BB9090016AE4204 (172.17.0.4:3000)
  Observed Nodes: 3
  Roster Generation: 1
```

### What to Look For

- **Roster consistency** - All nodes should show the same roster
- **Node count** - Roster should include expected number of nodes
- **Generation number** - Increases with each roster change
- **Missing nodes** - Indicates nodes that have left the cluster

## Roster Apply

Apply the roster configuration by adding all current cluster nodes to the roster.

### Basic Usage

```bash
# Apply roster to default namespace
aerolab roster apply -n mycluster

# Apply roster to specific namespace
aerolab roster apply -n mycluster --namespace test
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Cluster name | `mydc` |
| `--namespace` | Namespace to apply roster to | `test` |
| `-l, --nodes` | Specific nodes to include | All running nodes |

### What It Does

1. Discovers all running nodes in the cluster
2. Gets node IDs from each node
3. Issues roster commands to add all nodes
4. Verifies roster is applied across all nodes

### When to Use

- **After cluster creation** with Strong Consistency enabled
- **After growing the cluster** (adding nodes)
- **After node replacement** or recovery
- **Before shrinking the cluster** (remove nodes from roster first)

## Roster Cheat

Display a quick reference guide for Strong Consistency and roster management.

### Basic Usage

```bash
aerolab roster cheat
```

### What's Shown

The cheat sheet includes:
- Strong Consistency concepts
- Roster management commands
- Common workflows
- Troubleshooting tips
- Best practices
- Quick command reference

## Complete Workflow Examples

### Initial Cluster Setup with Strong Consistency

```bash
# 1. Create cluster
aerolab cluster create -n sccluster -c 3 -d ubuntu -i 24.04 -v '8.*'

# 2. Enable Strong Consistency
aerolab conf sc -n sccluster -r 2

# 3. Restart to apply configuration
aerolab aerospike restart -n sccluster

# 4. Wait for cluster stability
aerolab aerospike is-stable -n sccluster -w -o 60

# 5. Apply roster
aerolab roster apply -n sccluster

# 6. Verify roster
aerolab roster show -n sccluster
```

### Growing a Strong Consistency Cluster

```bash
# 1. Check current roster
aerolab roster show -n sccluster

# 2. Grow the cluster (adds 2 nodes)
aerolab cluster grow -n sccluster -c 2 -d ubuntu -i 24.04 -v '8.*'

# 3. Update mesh configuration
aerolab conf fix-mesh -n sccluster

# 4. Restart Aerospike on new nodes
aerolab aerospike restart -n sccluster

# 5. Wait for stability
aerolab aerospike is-stable -n sccluster -w

# 6. Apply new roster (includes new nodes)
aerolab roster apply -n sccluster

# 7. Verify all nodes in roster
aerolab roster show -n sccluster
```

### Shrinking a Strong Consistency Cluster

```bash
# 1. Check current roster
aerolab roster show -n sccluster

# 2. Manually remove nodes from roster
aerolab attach asadm -n sccluster -l 1 -- \
  -e "manage roster remove <node-id> namespace test"

# 3. Verify roster updated
aerolab roster show -n sccluster

# 4. Wait for migrations to complete
aerolab aerospike is-stable -n sccluster -w

# 5. Destroy the unwanted nodes
aerolab cluster destroy -n sccluster -l 4-5 --force
```

### Recovering from Node Failure

```bash
# 1. Check roster status
aerolab roster show -n sccluster

# 2. If dead node is in roster, remove it
aerolab attach asadm -n sccluster -l 1 -- \
  -e "manage roster remove <dead-node-id> namespace test"

# 3. Add replacement node if needed
aerolab cluster grow -n sccluster -c 1 -d ubuntu -i 24.04 -v '8.*'

# 4. Update configuration
aerolab conf fix-mesh -n sccluster
aerolab aerospike restart -n sccluster

# 5. Apply new roster
aerolab roster apply -n sccluster

# 6. Verify
aerolab roster show -n sccluster
aerolab aerospike is-stable -n sccluster -w
```

## Roster Management Commands (Manual)

For advanced scenarios, you can use `asadm` directly:

```bash
# Show roster
aerolab attach asadm -n mycluster -l 1 -- \
  -e "show roster"

# Add node to roster
aerolab attach asadm -n mycluster -l 1 -- \
  -e "manage roster add <node-id> namespace test"

# Remove node from roster
aerolab attach asadm -n mycluster -l 1 -- \
  -e "manage roster remove <node-id> namespace test"

# Set roster to specific nodes
aerolab attach asadm -n mycluster -l 1 -- \
  -e "manage roster set <node-id-1> <node-id-2> namespace test"
```

## Understanding Roster Behavior

### Roster Size vs Replication Factor

- **Replication Factor (RF)**: Number of copies of data
- **Roster Size**: Number of nodes that can serve as replicas
- **Best Practice**: Roster size ≥ Replication Factor

```bash
# RF=2 requires at least 2 nodes in roster
aerolab conf sc -n mycluster -r 2
aerolab roster apply -n mycluster
```

### Quorum Requirements

For Strong Consistency:
- **Write quorum**: (Replication Factor / 2) + 1
- **Read quorum**: (Replication Factor / 2) + 1

Example with RF=3:
- Need 2 nodes available for reads/writes
- Can tolerate 1 node failure

## Best Practices

1. **Always apply roster after topology changes**
   ```bash
   aerolab roster apply -n mycluster
   ```

2. **Verify roster consistency across nodes**
   ```bash
   aerolab roster show -n mycluster
   ```

3. **Wait for cluster stability before roster changes**
   ```bash
   aerolab aerospike is-stable -n mycluster -w
   ```

4. **Remove nodes from roster before destroying**
   ```bash
   # Manual removal first
   aerolab attach asadm -- -e "manage roster remove <node-id> ..."
   
   # Then destroy
   aerolab cluster destroy -l <nodes> --force
   ```

5. **Keep roster size appropriate**
   - Don't include dead nodes
   - Include all active nodes
   - Ensure roster size ≥ RF

6. **Monitor roster generation numbers**
   - Should be consistent across nodes
   - Increases with each change
   - Mismatch indicates sync issues

## Common Issues and Solutions

**Roster not consistent across nodes:**
```bash
# Re-apply roster
aerolab roster apply -n mycluster

# Verify
aerolab roster show -n mycluster
```

**Node not in roster after adding:**
```bash
# Ensure node is running and stable
aerolab aerospike status -n mycluster

# Apply roster again
aerolab roster apply -n mycluster
```

**Quorum errors:**
- Check replication factor vs available nodes
- Ensure enough nodes are in roster and online
- RF=3 needs at least 2 nodes available

**Cannot write data after node failure:**
- Remove failed node from roster
- Ensure remaining nodes meet quorum
- Add replacement node if needed

## See Also

- [Configuration Management](conf.md) - Enable Strong Consistency with `conf sc`
- [Cluster Management](cluster.md) - Create and manage clusters
- [Aerospike Controls](aerospike.md) - Check stability and status
- [Attach Commands](attach.md) - Use asadm for advanced roster management

## Additional Resources

- [Aerospike Strong Consistency Documentation](https://docs.aerospike.com/server/architecture/consistency)
- [Roster Management Guide](https://docs.aerospike.com/server/operations/manage/consistency)
- Use `aerolab roster cheat` for quick reference

