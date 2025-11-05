# File Operations Commands

Commands for transferring and managing files on cluster nodes.

## Commands Overview

- `files upload` - Upload files to nodes
- `files download` - Download files from nodes
- `files sync` - Sync files across nodes
- `files edit` - Edit files on nodes

## Files Upload

Upload files from local system to cluster nodes.

### Basic Usage

```bash
aerolab files upload -n mydc local-file.txt /tmp/remote-file.txt
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list (comma-separated or ranges) |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Upload to all nodes:**
```bash
aerolab files upload -n mydc local-file.txt /tmp/remote-file.txt
```

**Upload to specific node:**
```bash
aerolab files upload -n mydc -l 1 local-file.txt /tmp/remote-file.txt
```

**Upload to multiple nodes:**
```bash
aerolab files upload -n mydc -l 1-3 local-file.txt /tmp/remote-file.txt
```

**Upload with custom permissions:**
Files are uploaded with default permissions. Use shell access to change permissions after upload if needed.

## Files Download

Download files from cluster nodes to local system.

### Basic Usage

```bash
aerolab files download -n mydc /tmp/remote-file.txt ./local-dir/
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Download from all nodes:**
```bash
aerolab files download -n mydc /tmp/remote-file.txt ./local-dir/
```

**Download from specific node:**
```bash
aerolab files download -n mydc -l 1 /tmp/remote-file.txt ./local-dir/
```

**Download from multiple nodes:**
```bash
aerolab files download -n mydc -l 1-3 /tmp/remote-file.txt ./local-dir/
```

**Note**: Files from different nodes are saved with node numbers appended.

## Files Sync

Sync files from one node to all other nodes in the cluster.

### Basic Usage

```bash
aerolab files sync -n mydc -l 1 /tmp/file.txt
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Source node (file is copied from this node) |

### Examples

**Sync from node 1:**
```bash
aerolab files sync -n mydc -l 1 /tmp/file.txt
```

This copies `/tmp/file.txt` from node 1 to all other nodes in the cluster.

**Sync configuration file:**
```bash
aerolab files sync -n mydc -l 1 /etc/aerospike/aerospike.conf
```

### Workflow

1. File is read from the source node
2. File is uploaded to all other nodes in the cluster
3. Useful for synchronizing configuration files across nodes

## Files Edit

Edit files on cluster nodes.

### Basic Usage

```bash
aerolab files edit -n mydc /tmp/file.txt
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |

### Examples

**Edit file on all nodes:**
```bash
aerolab files edit -n mydc /tmp/file.txt
```

**Edit file on specific node:**
```bash
aerolab files edit -n mydc -l 1 /tmp/file.txt
```

**Note**: Opens the file in your default editor. Changes are saved back to the node.

## Common Workflows

### Upload Configuration File

```bash
# 1. Upload custom configuration
aerolab files upload -n mydc custom-aerospike.conf /etc/aerospike/aerospike.conf

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

### Sync Configuration Across Nodes

```bash
# 1. Edit configuration on node 1
aerolab files edit -n mydc -l 1 /etc/aerospike/aerospike.conf

# 2. Sync to all other nodes
aerolab files sync -n mydc -l 1 /etc/aerospike/aerospike.conf

# 3. Restart Aerospike
aerolab aerospike restart -n mydc

# 4. Wait for stability
aerolab aerospike is-stable -n mydc -w
```

### Download Logs

```bash
# Download logs from all nodes
aerolab files download -n mydc /var/log/aerospike/aerospike.log ./logs/
```

### Upload Scripts

```bash
# Upload script to all nodes
aerolab files upload -n mydc local-script.sh /opt/script.sh

# Make executable
aerolab attach shell -n mydc -- chmod +x /opt/script.sh

# Run script
aerolab attach shell -n mydc -- /opt/script.sh
```

### Upload Certificate Files

```bash
# Upload TLS certificate
aerolab files upload -n mydc ca.pem /opt/ca.pem

# Upload client certificate
aerolab files upload -n mydc client.pem /opt/client.pem

# Upload key file
aerolab files upload -n mydc client.key /opt/client.key
```

## Tips

1. **Sync for configuration**: Use `files sync` to keep configuration files consistent across nodes
2. **Node-specific files**: Use `-l` to target specific nodes when needed
3. **Download logs**: Use `files download` to download logs for analysis
4. **Edit carefully**: Use `files edit` for quick edits, but consider using `files upload` for complex changes
5. **Permissions**: Files are uploaded with default permissions. Use shell access to change permissions if needed

