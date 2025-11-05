# Aerospike Daemon Control Commands

Commands for controlling the Aerospike service on cluster nodes.

## Commands Overview

- `aerospike start` - Start Aerospike service
- `aerospike stop` - Stop Aerospike service
- `aerospike restart` - Restart Aerospike service
- `aerospike status` - Check Aerospike service status
- `aerospike upgrade` - Upgrade Aerospike version
- `aerospike cold-start` - Perform cold start (clean IPC resources)
- `aerospike is-stable` - Check if cluster is stable

## Aerospike Start

Start the Aerospike service on cluster nodes.

### Basic Usage

```bash
aerolab aerospike start
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name (comma-separated for multiple) |
| `-l, --nodes` | Node list (comma-separated or ranges like `1-3`) |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Start Aerospike on all nodes:**
```bash
aerolab aerospike start
```

**Start Aerospike on specific cluster:**
```bash
aerolab aerospike start -n mydc
```

**Start Aerospike on specific nodes:**
```bash
aerolab aerospike start -n mydc -l 1-2
```

**Start Aerospike on multiple clusters:**
```bash
aerolab aerospike start -n mydc,otherdc
```

## Aerospike Stop

Stop the Aerospike service on cluster nodes.

### Basic Usage

```bash
aerolab aerospike stop
```

### Options

Same as `aerospike start`.

### Examples

**Stop Aerospike on all nodes:**
```bash
aerolab aerospike stop
```

**Stop Aerospike on specific nodes:**
```bash
aerolab aerospike stop -n mydc -l 1-2
```

## Aerospike Restart

Restart the Aerospike service on cluster nodes.

### Basic Usage

```bash
aerolab aerospike restart
```

### Options

Same as `aerospike start`.

### Examples

**Restart Aerospike on all nodes:**
```bash
aerolab aerospike restart
```

**Restart Aerospike on specific nodes:**
```bash
aerolab aerospike restart -n mydc -l 1-2
```

**Note**: Restart stops and starts the service. Use after configuration changes.

## Aerospike Status

Check the status of the Aerospike service on cluster nodes.

### Basic Usage

```bash
aerolab aerospike status
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name (comma-separated for multiple) |
| `-l, --nodes` | Node list |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Check status on all nodes:**
```bash
aerolab aerospike status
```

**Check status on specific nodes:**
```bash
aerolab aerospike status -n mydc -l 1-2
```

### Output

The status command shows:
- Service status (running/stopped)
- Process information
- Port information
- Configuration file location

## Aerospike Upgrade

Upgrade Aerospike version on cluster nodes.

### Basic Usage

```bash
aerolab aerospike upgrade -v '8.*'
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `-v, --aerospike-version` | Target Aerospike version (supports wildcards) |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Upgrade all nodes to latest 8.x:**
```bash
aerolab aerospike upgrade -n mydc -v '8.*'
```

**Upgrade specific nodes:**
```bash
aerolab aerospike upgrade -n mydc -l 1-2 -v '8.1.0.1'
```

**Upgrade to specific version:**
```bash
aerolab aerospike upgrade -n mydc -v '8.1.0.1'
```

### Upgrade Process

1. Stops Aerospike service
2. Downloads and installs new version
3. Starts Aerospike service
4. Waits for cluster stability (if `-w` is used)

**Note**: Upgrade is performed one node at a time to maintain cluster availability.

## Aerospike Cold Start

Perform a cold start by cleaning IPC resources and restarting Aerospike.

### Basic Usage

```bash
aerolab aerospike cold-start
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `-t, --threads` | Number of parallel threads | `10` |

### Examples

**Cold start all nodes:**
```bash
aerolab aerospike cold-start -n mydc
```

**Cold start specific nodes:**
```bash
aerolab aerospike cold-start -n mydc -l 1-2
```

### What Cold Start Does

1. Cleans up IPC resources (`ipcrm --all`)
2. Starts Aerospike service
3. Useful for troubleshooting or after crashes

**Warning**: Cold start will clear shared memory segments. Use with caution in production.

## Aerospike Is-Stable

Check if the cluster is stable (all nodes joined, migrations complete).

### Basic Usage

```bash
aerolab aerospike is-stable
```

### Options

| Option | Description |
|--------|-------------|
| `-n, --name` | Cluster name |
| `-l, --nodes` | Node list |
| `-w, --wait` | Wait for stability |
| `-o, --timeout` | Timeout in seconds (0 = no timeout) | `0` |
| `-i, --ignore-migrations` | Ignore migrations when checking stability |
| `--namespace` | Namespace to check (default: all) |
| `-v, --verbose` | Verbose output |

### Examples

**Check if cluster is stable:**
```bash
aerolab aerospike is-stable -n mydc
```

**Wait for cluster to become stable:**
```bash
aerolab aerospike is-stable -n mydc -w
```

**Wait with timeout:**
```bash
aerolab aerospike is-stable -n mydc -w -o 30
```

**Wait and ignore migrations:**
```bash
aerolab aerospike is-stable -n mydc -w -o 30 -i
```

**Check specific namespace:**
```bash
aerolab aerospike is-stable -n mydc --namespace test
```

### Output

- Returns exit code 0 if cluster is stable
- Returns exit code 1 if cluster is not stable
- With `-w`, waits until cluster becomes stable or timeout expires

## Common Workflows

### Start Cluster and Wait for Stability

```bash
# 1. Start cluster nodes
aerolab cluster start -n mydc

# 2. Start Aerospike
aerolab aerospike start -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w -o 60

# 4. Check status
aerolab aerospike status -n mydc
```

### Restart After Configuration Changes

```bash
# 1. Modify configuration (using conf commands)
aerolab conf adjust set network.heartbeat.interval 250

# 2. Restart Aerospike
aerolab aerospike restart -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 4. Verify changes
aerolab attach asinfo -n mydc -- -v "network"
```

### Upgrade Cluster

```bash
# 1. Upgrade first node
aerolab aerospike upgrade -n mydc -l 1 -v '8.1.0.1'

# 2. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 3. Upgrade remaining nodes
aerolab aerospike upgrade -n mydc -l 2-5 -v '8.1.0.1'

# 4. Wait for final stability
aerolab aerospike is-stable -n mydc -w
```

### Troubleshooting with Cold Start

```bash
# 1. Stop Aerospike
aerolab aerospike stop -n mydc

# 2. Cold start to clean IPC resources
aerolab aerospike cold-start -n mydc

# 3. Wait for stability
aerolab aerospike is-stable -n mydc -w

# 4. Check status
aerolab aerospike status -n mydc
```

### Rolling Restart

```bash
# Restart nodes one at a time
for node in 1 2 3 4 5; do
  aerolab aerospike restart -n mydc -l $node
  aerolab aerospike is-stable -n mydc -w -o 30
done
```

## Tips

1. **Always Wait for Stability**: After start/restart/upgrade, use `is-stable -w` to ensure the cluster is ready
2. **Rolling Upgrades**: Upgrade nodes one at a time in production for zero downtime
3. **Timeout Settings**: Use appropriate timeouts with `is-stable` to avoid long waits
4. **Cold Start**: Use cold start only when necessary, as it clears IPC resources
5. **Status Checks**: Regularly check status to monitor cluster health
6. **Parallel Operations**: Use `-t` to control parallel operations for faster execution

## Troubleshooting

### Service Won't Start

1. Check service status:
   ```bash
   aerolab aerospike status -n mydc
   ```

2. Check logs:
   ```bash
   aerolab logs show -n mydc
   ```

3. Check configuration:
   ```bash
   aerolab attach shell -n mydc -- cat /etc/aerospike/aerospike.conf
   ```

### Cluster Not Stable

1. Check cluster state:
   ```bash
   aerolab attach asinfo -n mydc -- -v "cluster"
   ```

2. Check for migrations:
   ```bash
   aerolab attach asinfo -n mydc -- -v "migrate"
   ```

3. Wait with verbose output:
   ```bash
   aerolab aerospike is-stable -n mydc -w -v
   ```

### Upgrade Issues

1. Check current version:
   ```bash
   aerolab attach asinfo -n mydc -- -v "version"
   ```

2. Verify upgrade completed:
   ```bash
   aerolab aerospike status -n mydc
   ```

3. Check logs for errors:
   ```bash
   aerolab logs show -n mydc
   ```

