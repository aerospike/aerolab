# Configuration File Management Commands

Commands for managing Aerospike configuration files on cluster nodes.

## Commands Overview

- `conf rackid` - Set rack IDs for nodes
- `conf sc` - Configure strong consistency
- `conf fix-mesh` - Fix mesh heartbeat configuration
- `conf adjust` - Adjust configuration parameters
- `conf namespace-memory` - Configure namespace memory settings
- `conf generator` - Generate configuration files

## Conf Rackid

Set rack IDs for cluster nodes to enable rack-aware replication.

### Basic Usage

```bash
aerolab conf rackid -l 1-2 -i 1
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list (comma-separated or ranges) |
| `-i, --rack-id` | Rack ID to assign |

### Examples

**Assign rack ID 1 to nodes 1-2:**
```bash
aerolab conf rackid -n mydc -l 1-2 -i 1
```

**Assign rack ID 2 to nodes 3-4:**
```bash
aerolab conf rackid -n mydc -l 3-4 -i 2
```

**Assign rack ID 3 to node 5:**
```bash
aerolab conf rackid -n mydc -l 5 -i 3
```

**Assign different rack IDs:**
```bash
aerolab conf rackid -n mydc -l 1 -i 1
aerolab conf rackid -n mydc -l 2 -i 2
aerolab conf rackid -n mydc -l 3 -i 3
```

### Workflow

After setting rack IDs, restart Aerospike to apply changes:

```bash
# 1. Set rack IDs
aerolab conf rackid -n mydc -l 1-2 -i 1
aerolab conf rackid -n mydc -l 3-4 -i 2

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

## Conf SC

Configure strong consistency for namespaces.

### Basic Usage

```bash
aerolab conf sc -r 2
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-r, --replication-factor` | Replication factor |
| `--namespace` | Namespace name (default: all) |
| `-v, --verbose` | Verbose output |

### Examples

**Set replication factor to 2:**
```bash
aerolab conf sc -n mydc -r 2
```

**Set replication factor for specific namespace:**
```bash
aerolab conf sc -n mydc -r 2 --namespace test
```

**With verbose output:**
```bash
aerolab conf sc -n mydc -r 2 -v
```

### What It Does

1. Sets `strong-consistency` to `true` in namespace configuration
2. Sets `replication-factor` to specified value
3. Adjusts replication factor if needed (must be <= node count)

### Workflow

```bash
# 1. Configure strong consistency
aerolab conf sc -n mydc -r 2

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 4. Apply roster (if using strong consistency)
aerolab roster apply -n mydc
```

## Conf Fix-Mesh

Automatically fix mesh heartbeat configuration.

### Basic Usage

```bash
aerolab conf fix-mesh
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |

### Examples

**Fix mesh configuration:**
```bash
aerolab conf fix-mesh -n mydc
```

### What It Does

1. Reads current cluster configuration
2. Updates mesh heartbeat configuration with all cluster nodes
3. Ensures all nodes can communicate with each other

### Workflow

```bash
# 1. Fix mesh configuration
aerolab conf fix-mesh -n mydc

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

## Conf Adjust

Adjust configuration parameters in Aerospike configuration files.

### Basic Usage

```bash
aerolab conf adjust set network.heartbeat.interval 250
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `set` | Set a parameter value |
| `get` | Get a parameter value |

### Examples

**Set heartbeat interval:**
```bash
aerolab conf adjust set network.heartbeat.interval 250
```

**Set namespace replication factor:**
```bash
aerolab conf adjust set namespace test.replication-factor 2
```

**Set memory size:**
```bash
aerolab conf adjust set namespace test.memory-size 10G
```

**Get parameter value:**
```bash
aerolab conf adjust get network.heartbeat.interval
```

**Set on specific nodes:**
```bash
aerolab conf adjust -n mydc -l 1-2 set network.heartbeat.interval 250
```

### Workflow

```bash
# 1. Adjust parameter
aerolab conf adjust set network.heartbeat.interval 250

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

## Conf Namespace-Memory

Configure namespace memory settings (AWS/GCP only).

### Basic Usage

```bash
aerolab conf namespace-memory -n mydc
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `--namespace` | Namespace name |

### Examples

**Configure namespace memory:**
```bash
aerolab conf namespace-memory -n mydc
```

**Configure specific namespace:**
```bash
aerolab conf namespace-memory -n mydc --namespace test
```

### What It Does

1. Analyzes available memory on nodes
2. Suggests or sets namespace memory sizes
3. Ensures memory allocation is appropriate

## Conf Generator

Generate Aerospike configuration files.

### Basic Usage

```bash
aerolab conf generator
```

Generates configuration files based on cluster setup.

## Common Workflows

### Configure Rack-Aware Replication

```bash
# 1. Set rack IDs
aerolab conf rackid -n mydc -l 1-2 -i 1
aerolab conf rackid -n mydc -l 3-4 -i 2
aerolab conf rackid -n mydc -l 5-6 -i 3

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

### Configure Strong Consistency

```bash
# 1. Configure strong consistency
aerolab conf sc -n mydc -r 2

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 4. Apply roster
aerolab roster apply -n mydc

# 5. Verify roster
aerolab roster show -n mydc
```

### Fix Mesh Configuration

```bash
# 1. Fix mesh configuration
aerolab conf fix-mesh -n mydc

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 4. Verify configuration
aerolab attach asinfo -n mydc -- -v "network"
```

### Adjust Configuration Parameters

```bash
# 1. Adjust heartbeat interval
aerolab conf adjust set network.heartbeat.interval 250

# 2. Adjust heartbeat timeout
aerolab conf adjust set network.heartbeat.timeout 10

# 3. Restart Aerospike
aerolab aerospike restart -n mydc

# 4. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 5. Verify changes
aerolab attach asinfo -n mydc -- -v "network"
```

### Complete Configuration Setup

```bash
# 1. Set rack IDs
aerolab conf rackid -n mydc -l 1-2 -i 1
aerolab conf rackid -n mydc -l 3-4 -i 2

# 2. Fix mesh configuration
aerolab conf fix-mesh -n mydc

# 3. Configure strong consistency
aerolab conf sc -n mydc -r 2

# 4. Adjust heartbeat settings
aerolab conf adjust set network.heartbeat.interval 250

# 5. Restart Aerospike
aerolab aerospike restart -n mydc

# 6. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 7. Apply roster
aerolab roster apply -n mydc

# 8. Verify configuration
aerolab attach asinfo -n mydc -- -v "cluster"
```

## Tips

1. **Always restart**: Configuration changes require Aerospike restart to take effect
2. **Wait for stability**: After restart, always wait for cluster stability
3. **Rack IDs**: Set rack IDs before starting Aerospike for best results
4. **Strong consistency**: Requires roster application after configuration
5. **Mesh configuration**: Fix mesh after adding/removing nodes
6. **Backup**: Consider backing up configuration files before making changes

