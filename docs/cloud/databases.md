# Cloud Cluster Management Commands

Commands for creating, managing, and operating Aerospike Cloud clusters.

## Commands Overview

- `cloud clusters create` - Create a new cluster
- `cloud clusters list` - List all clusters
- `cloud clusters update` - Update cluster configuration
- `cloud clusters delete` - Delete a cluster
- `cloud clusters peer-vpc` - Peer VPC with cluster
- `cloud clusters credentials` - Manage cluster credentials

## Prerequisites

- AWS backend configured
- AWS credentials with permissions for Aerospike Cloud
- VPC ID for cluster peering (can use "default" for default VPC)

## Cloud Clusters Create

Create a new Aerospike Cloud cluster.

### Basic Usage

```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id vpc-xxxxxxxxx
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `-n, --name` | Cluster name | Yes |
| `-i, --instance-type` | Instance type | Yes |
| `-r, --region` | AWS region | Yes |
| `--availability-zone-count` | Number of availability zones (1-3) | No (default: 2) |
| `--cluster-size` | Number of nodes in cluster | Yes |
| `--data-storage` | Data storage type: `memory`, `local-disk`, or `network-storage` | Yes |
| `--data-resiliency` | Data resiliency: `local-disk` or `network-storage` | No |
| `--data-plane-version` | Data plane version (default: `latest`) | No |
| `--vpc-id` | VPC ID to peer with (default: `default`) | No |
| `--cloud-cidr` | CIDR block for cloud cluster infrastructure. If `default`, auto-assigns starting from 10.128.0.0/19. When VPC-ID is specified, aerolab checks for collisions and finds the next available CIDR. | No (default: `default`) |
| `--force-route-creation` | Force route creation even if a route with the same destination CIDR already exists | No |

### Examples

**Create memory cluster:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id default
```

**Create local-disk cluster:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage local-disk \
  --vpc-id vpc-xxxxxxxxx
```

**Create network-storage cluster:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage network-storage \
  --vpc-id vpc-xxxxxxxxx
```

**Create with specific data plane version:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --data-plane-version 8.1.0 \
  --vpc-id default
```

**Create with 3 availability zones:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=3 \
  --cluster-size=3 \
  --data-storage memory \
  --vpc-id default
```

**Create with custom CIDR block:**
```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id vpc-xxxxxxxxx \
  --cloud-cidr 10.200.0.0/19
```

### VPC ID Resolution

If `--vpc-id` is set to `default`, Aerolab will automatically resolve the default VPC in your AWS account.

### CIDR Block Resolution

When creating a cluster with a VPC-ID, Aerolab performs automatic CIDR collision detection:

1. **Default behavior** (`--cloud-cidr default`): 
   - Checks if the default CIDR (10.128.0.0/19) is available in your VPC route tables
   - If available, uses the default CIDR
   - If already in use, automatically finds the next available CIDR (10.129.0.0/19, 10.130.0.0/19, etc.)

2. **Custom CIDR** (`--cloud-cidr 10.x.x.x/19`):
   - Validates that the specified CIDR is not already in use
   - If the CIDR conflicts with existing routes, fails with an error before creating the cluster

This ensures VPC peering routes don't conflict with existing routes in your VPC.

## Cloud Clusters List

List all Aerospike Cloud clusters.

### Basic Usage

```bash
aerolab cloud clusters list
```

### Output Formats

The command supports multiple output formats:
- **table** (default) - Formatted table view
- **json** - JSON output (use with `jq` for parsing)
- **json-indent** - Indented JSON output
- **jq** - Pass output through `jq` for filtering
- **text** - Plain text format
- **csv, tsv, html, markdown** - Additional formats

### Output

When using JSON output (with `-o json` or `-o json-indent`), the command outputs JSON with cluster information including:
- Cluster ID
- Cluster name
- Instance type
- Region
- Cluster size
- Status
- Connection details (host, port, TLS certificate)
- VPC information

### Examples

**List all clusters:**
```bash
aerolab cloud clusters list
```

**List and filter by name:**
```bash
aerolab cloud clusters list -o json | jq '.clusters[] | select(.name == "mydb")'
```

**Get cluster ID:**
```bash
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')
```

**Get connection host:**
```bash
HOST=$(aerolab cloud clusters get host -n mydb)
```

**Get TLS certificate:**
```bash
CERT=$(aerolab cloud clusters get tls-cert -n mydb)
```

## Cloud Clusters Update

Update cluster configuration.

### Basic Usage

```bash
aerolab cloud clusters update --cluster-id <database-id> --cluster-size 4 -i m5d.xlarge
```

### Options

| Option | Description |
|--------|-------------|
| `--cluster-id` | Cluster ID (required) |
| `--cluster-size` | New cluster size |
| `-i, --instance-type` | New instance type |

### Examples

**Update cluster size:**
```bash
aerolab cloud clusters update \
  --cluster-id <database-id> \
  --cluster-size 4
```

**Update instance type:**
```bash
aerolab cloud clusters update \
  --cluster-id <database-id> \
  -i m5d.xlarge
```

**Update both cluster size and instance type:**
```bash
aerolab cloud clusters update \
  --cluster-id <database-id> \
  --cluster-size 4 \
  -i m5d.xlarge
```

**Note**: Updates may take time to complete. The cluster will be unavailable during updates.

## Cloud Clusters Delete

Delete an Aerospike Cloud cluster.

### Basic Usage

```bash
aerolab cloud clusters delete --cluster-id <database-id> --force --wait
```

### Options

| Option | Description |
|--------|-------------|
| `--cluster-id` | Cluster ID (required) |
| `--force` | Force deletion without confirmation |
| `--wait` | Wait for deletion to complete |

### Examples

**Delete cluster:**
```bash
aerolab cloud clusters delete \
  --cluster-id <database-id> \
  --force \
  --wait
```

**Delete cluster by name:**
```bash
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')
aerolab cloud clusters delete --cluster-id $CID --force --wait
```

**Warning**: This permanently deletes the cluster and all its data. Use with caution.

## Cloud Clusters Peer-VPC

Peer VPC with a cluster.

### Basic Usage

```bash
aerolab cloud clusters peer-vpc -d <database-id> -r us-east-1 --vpc-id vpc-xxxxxxxxx
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `-d, --cluster-id` | Cluster ID | Yes |
| `-r, --region` | AWS region | Yes |
| `--vpc-id` | VPC ID to peer with (default: `default`) | No |
| `--stage-initiate` | Execute only the initiate stage (request VPC peering from cloud) | No |
| `--stage-accept` | Execute only the accept stage (accept the VPC peering request) | No |
| `--stage-route` | Execute only the route stage (create route in VPC route table) | No |
| `--stage-associate-dns` | Execute only the DNS association stage (associate VPC with hosted zone) | No |
| `--force-route-creation` | Force route creation even if a route with the same destination CIDR already exists | No |

### Stage Execution Behavior

- **No stages specified**: All stages are executed in order (initiate → accept → route → associate-dns). Already completed stages are automatically skipped.
- **Specific stages specified**: Only the specified stages are executed.
- **Stage failure**: If a stage fails, further stages are aborted.

### Examples

**Peer VPC with cluster (all stages):**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx
```

**Execute only the initiate stage:**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-initiate
```

**Execute only the accept stage:**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-accept
```

**Execute only the route stage:**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-route
```

**Execute only the DNS association stage:**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-associate-dns
```

**Force route creation (replace existing conflicting route):**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-route \
  --force-route-creation
```

**Execute multiple specific stages:**
```bash
aerolab cloud clusters peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-accept \
  --stage-route
```

**Note**: VPC peering is typically done automatically during cluster creation. Use this command if you need to peer additional VPCs or re-run specific steps of the peering process.

## Cloud Clusters Wait

Wait for a cluster to reach a specific health.status.

### Basic Usage

```bash
aerolab cloud clusters wait -i <database-id> --status running
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `-i, --cluster-id` | Cluster ID | Yes |
| `-s, --status` | Wait for health.status to match any of these values (can be specified multiple times) | No (if --status-ne provided) |
| `--status-ne` | Wait for health.status to NOT match any of these values (can be specified multiple times) | No (if --status provided) |
| `--wait-timeout` | Timeout in seconds (0 = no timeout) | No (default: 600) |

### Examples

**Wait for cluster to be running:**
```bash
aerolab cloud clusters wait -i <database-id> --status running
```

**Wait for cluster to be running or updating:**
```bash
aerolab cloud clusters wait -i <database-id> --status running --status updating
```

**Wait for cluster to NOT be provisioning:**
```bash
aerolab cloud clusters wait -i <database-id> --status-ne provisioning
```

**Wait for cluster to NOT be provisioning or creating:**
```bash
aerolab cloud clusters wait -i <database-id> --status-ne provisioning --status-ne creating
```

**Wait with custom timeout:**
```bash
aerolab cloud clusters wait -i <database-id> --status running --wait-timeout 300
```

**Wait indefinitely (no timeout):**
```bash
aerolab cloud clusters wait -i <database-id> --status running --wait-timeout 0
```

### How It Works

- Checks the cluster health.status every 10 seconds
- If `--status` is specified: waits until health.status matches ANY of the specified values
- If `--status-ne` is specified: waits until health.status does NOT match ANY of the specified values (i.e., doesn't match any excluded status)
- If both are specified: both conditions must be met (status matches one of `--status` values AND does not match any of `--status-ne` values)
- Returns success when the condition(s) are met, or timeout if the timeout is reached
- At least one of `--status` or `--status-ne` must be provided

### Common Workflows

**Wait for cluster to be ready after creation:**
```bash
# Create cluster
aerolab cloud clusters create -n mydb ...

# Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# Wait for cluster to be running
aerolab cloud clusters wait -i $CID --status running
```

**Wait for cluster to finish updating:**
```bash
# Update cluster
aerolab cloud clusters update --cluster-id $CID --cluster-size 4

# Wait for cluster to NOT be updating
aerolab cloud clusters wait -i $CID --status-ne updating
```

**Wait for cluster with both conditions:**
```bash
# Wait for cluster to be running AND not be updating
aerolab cloud clusters wait -i $CID --status running --status-ne updating
```

**Wait for cluster to be ready (not provisioning or creating):**
```bash
# Wait for cluster to NOT be in provisioning or creating state
aerolab cloud clusters wait -i $CID --status-ne provisioning --status-ne creating
```

## Cloud Clusters Get

Get cluster connection details.

### Get Host

Get the cluster hostname.

#### Basic Usage

```bash
aerolab cloud clusters get host -n mydb
```

Or by cluster ID:

```bash
aerolab cloud clusters get host -i <database-id>
```

#### Options

| Option | Description | Required |
|--------|-------------|----------|
| `-n, --name` | Cluster name | No (if ID provided) |
| `-i, --cluster-id` | Cluster ID | No (if name provided) |

#### Examples

**Get host by name:**
```bash
aerolab cloud clusters get host -n mydb
```

**Get host by ID:**
```bash
aerolab cloud clusters get host -i <database-id>
```

**Use in scripts:**
```bash
HOST=$(aerolab cloud clusters get host -n mydb)
echo "Connecting to $HOST"
```

### Get TLS Certificate

Get the cluster TLS certificate.

#### Basic Usage

```bash
aerolab cloud clusters get tls-cert -n mydb
```

Or by cluster ID:

```bash
aerolab cloud clusters get tls-cert -i <database-id>
```

#### Options

Same as `get host`.

#### Examples

**Get TLS certificate by name:**
```bash
aerolab cloud clusters get tls-cert -n mydb
```

**Get TLS certificate by ID:**
```bash
aerolab cloud clusters get tls-cert -i <database-id>
```

**Save certificate to file:**
```bash
aerolab cloud clusters get tls-cert -n mydb > ca.pem
```

**Use in scripts:**
```bash
CERT=$(aerolab cloud clusters get tls-cert -n mydb)
echo "$CERT" > ca.pem
```

## Cloud Clusters Credentials

Manage cluster credentials. See [Credentials Management](credentials.md) for detailed documentation.

### Quick Reference

**List credentials:**
```bash
aerolab cloud clusters credentials list --cluster-id <database-id>
```

**Create credentials:**
```bash
aerolab cloud clusters credentials create \
  --cluster-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

**Delete credentials:**
```bash
aerolab cloud clusters credentials delete \
  --cluster-id <database-id> \
  --credentials-id <credentials-id>
```

## Common Workflows

### Create Cluster and Connect

```bash
# 1. Create cluster
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id default

# 2. Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# 3. Create credentials
aerolab cloud clusters credentials create \
  --cluster-id $CID \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait

# 4. Get connection details
HOST=$(aerolab cloud clusters get host -n mydb)
CERT=$(aerolab cloud clusters get tls-cert -n mydb)

# 5. Save and upload certificate
echo "$CERT" > ca.pem
aerolab files upload ca.pem /opt/ca.pem

# 6. Connect using aql
aerolab attach aql -- \
  --tls-enable \
  --tls-name $HOST \
  --tls-cafile /opt/ca.pem \
  -h $HOST:4000 \
  -U myuser \
  -P mypassword \
  -c "show namespaces"
```

### Update Cluster

```bash
# 1. Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# 2. Update cluster size
aerolab cloud clusters update \
  --cluster-id $CID \
  --cluster-size 4 \
  -i m5d.xlarge

# 3. Wait for update to complete (check status)
aerolab cloud clusters list -o json | jq '.clusters[] | select(.name == "mydb")'
```

### Delete Cluster

```bash
# 1. Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# 2. Delete cluster
aerolab cloud clusters delete \
  --cluster-id $CID \
  --force \
  --wait
```

## Tips

1. **VPC ID**: Use `default` to automatically use the default VPC
2. **Instance Types**: Use `cloud list-instance-types` to see available instance types
3. **Connection Details**: Always use TLS when connecting to Aerospike Cloud clusters
4. **Credentials**: Create credentials before connecting to the cluster
5. **Updates**: Cluster updates may cause downtime. Plan accordingly
6. **Deletion**: Cluster deletion is permanent. Ensure you have backups if needed
7. **Route Conflicts**: If a route already exists for the cluster CIDR, use `--force-route-creation` to replace it (use with caution)
8. **CIDR Collisions**: When using `--vpc-id`, aerolab automatically checks for CIDR collisions and finds the next available CIDR if the default (10.128.0.0/19) is already in use
9. **Custom CIDR**: Use `--cloud-cidr` to specify a custom CIDR block for the cloud cluster infrastructure
10. **Partial Peering**: Use `--stage-*` flags to run specific stages of the VPC peering process

