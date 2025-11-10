# Instance Management Commands

Instance management commands provide low-level control over compute instances without Aerospike-specific configurations. These are the building blocks used by cluster commands.

## Commands Overview

- `instances create` - Create a new instance cluster
- `instances grow` - Grow an existing instance cluster
- `instances apply` - Automatically create, grow, or shrink to match desired cluster size
- `instances list` - List all instances
- `instances attach` - Attach to an instance or cluster
- `instances start` - Start instances
- `instances stop` - Stop instances
- `instances restart` - Restart instances
- `instances update-hosts-file` - Update the hosts file on instances
- `instances add-tags` - Add tags to instances
- `instances remove-tags` - Remove tags from instances
- `instances assign-firewalls` - Assign firewalls/security groups to instances
- `instances remove-firewalls` - Remove firewalls/security groups from instances
- `instances change-expiry` - Change the expiry time of instances
- `instances destroy` - Destroy instances

## When to Use Instances vs Clusters

- **Use `cluster` commands** for Aerospike clusters (includes Aerospike installation and configuration)
- **Use `instances` commands** for:
  - Creating custom environments
  - Non-Aerospike workloads
  - Fine-grained control over infrastructure
  - Building custom automation

## Instances Create

Create raw compute instances without Aerospike installation.

### Basic Usage

```bash
aerolab instances create -n myinstances -c 2 -d ubuntu -i 24.04
```

### Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `-n, --name` | Instance cluster name | `mydc` |
| `-c, --count` | Number of instances | `1` |
| `-d, --distro` | Distribution (ubuntu, centos, rocky, debian, amazon) | Required |
| `-i, --distro-version` | Distribution version | Required |
| `-P, --parallel-threads` | Number of parallel threads | `10` |

### Backend-Specific Options

**AWS:**
- `-I, --instance-type` - Instance type
- `--aws-disk` - Disk specifications
- `--aws-expire` - Expiry time
- `--tags` - Custom tags
- `--secgroup-name` - Security groups

**GCP:**
- `--instance` - Instance type
- `--zone` - Zone name
- `--gcp-disk` - Disk specifications
- `--gcp-expire` - Expiry time
- `--label` - Custom labels

## Instances Grow

Add more instances to an existing instance cluster.

```bash
aerolab instances grow -n myinstances -c 2 -d ubuntu -i 24.04
```

## Instances Apply

Automatically adjust the instance cluster to match a desired size.

```bash
# Grow to 5 instances
aerolab instances apply -n myinstances -c 5 -d ubuntu -i 24.04

# Shrink to 2 instances (requires --force)
aerolab instances apply -n myinstances -c 2 --force
```

This command will:
- Create the cluster if it doesn't exist
- Grow the cluster if current size < desired size
- Shrink the cluster if current size > desired size (with `--force`)
- Do nothing if already at desired size

## Instances List

List all instances across all clusters.

```bash
# List all instances
aerolab instances list

# List specific cluster
aerolab instances list -n myinstances

# JSON output
aerolab instances list -o json

# TSV output
aerolab instances list -o tsv
```

## Instances Start/Stop/Restart

Control instance power state.

```bash
# Start all instances
aerolab instances start

# Start specific cluster
aerolab instances start -n myinstances

# Start specific nodes
aerolab instances start -n myinstances -l 1-3

# Stop instances
aerolab instances stop -n myinstances

# Restart instances
aerolab instances restart -n myinstances
```

## Instances Attach

Attach to instances and run commands.

```bash
# Attach to all instances
aerolab instances attach -l all -- ls /tmp

# Attach in parallel
aerolab instances attach -l all --parallel -- hostname

# Attach to specific nodes
aerolab instances attach -n myinstances -l 1,3 -- uptime
```

## Instances Update Hosts File

Update the /etc/hosts file on instances with cluster information.

```bash
aerolab instances update-hosts-file -n myinstances
```

## Tag Management

Add or remove tags from instances (AWS/GCP only).

```bash
# Add tags
aerolab instances add-tags -n myinstances --tag env=production --tag team=devops

# Remove tags
aerolab instances remove-tags -n myinstances --tag team
```

## Firewall Management

Assign or remove firewalls/security groups from instances (AWS/GCP only).

```bash
# Assign firewall
aerolab instances assign-firewalls -n myinstances --firewall my-custom-fw

# Remove firewall
aerolab instances remove-firewalls -n myinstances --firewall my-custom-fw
```

## Change Expiry

Change the expiry time of instances (AWS/GCP only).

```bash
# Set new expiry time
aerolab instances change-expiry -n myinstances --expire 24h

# Remove expiry (set to 0)
aerolab instances change-expiry -n myinstances --expire 0
```

## Instances Destroy

Destroy instances.

```bash
# Destroy entire cluster (requires --force)
aerolab instances destroy -n myinstances --force

# Destroy specific nodes
aerolab instances destroy -n myinstances -l 4-5 --force
```

## Differences from Cluster Commands

| Feature | instances | cluster |
|---------|-----------|---------|
| Aerospike Installation | ❌ No | ✅ Yes |
| Aerospike Configuration | ❌ No | ✅ Yes |
| Auto-start Aerospike | ❌ No | ✅ Yes (optional) |
| Templates | ❌ No | ✅ Yes |
| Feature Files | ❌ No | ✅ Yes |
| Use Case | Raw infrastructure | Aerospike clusters |

## See Also

- [Cluster Management](cluster.md) - High-level cluster management with Aerospike
- [Templates](templates.md) - Manage instance templates
- [Images](images.md) - Manage system images

