# Cloud Database Management Commands

Commands for creating, managing, and operating Aerospike Cloud databases.

## Commands Overview

- `cloud databases create` - Create a new database
- `cloud databases list` - List all databases
- `cloud databases update` - Update database configuration
- `cloud databases delete` - Delete a database
- `cloud databases peer-vpc` - Peer VPC with database
- `cloud databases credentials` - Manage database credentials

## Prerequisites

- AWS backend configured
- AWS credentials with permissions for Aerospike Cloud
- VPC ID for database peering (can use "default" for default VPC)

## Cloud Databases Create

Create a new Aerospike Cloud database.

### Basic Usage

```bash
aerolab cloud databases create -n mydb \
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
| `-n, --name` | Database name | Yes |
| `-i, --instance-type` | Instance type | Yes |
| `-r, --region` | AWS region | Yes |
| `--availability-zone-count` | Number of availability zones (1-3) | No (default: 2) |
| `--cluster-size` | Number of nodes in cluster | Yes |
| `--data-storage` | Data storage type: `memory`, `local-disk`, or `network-storage` | Yes |
| `--data-resiliency` | Data resiliency: `local-disk` or `network-storage` | No |
| `--data-plane-version` | Data plane version (default: `latest`) | No |
| `--vpc-id` | VPC ID to peer with (default: `default`) | No |

### Examples

**Create memory database:**
```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id default
```

**Create local-disk database:**
```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage local-disk \
  --vpc-id vpc-xxxxxxxxx
```

**Create network-storage database:**
```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage network-storage \
  --vpc-id vpc-xxxxxxxxx
```

**Create with specific data plane version:**
```bash
aerolab cloud databases create -n mydb \
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
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=3 \
  --cluster-size=3 \
  --data-storage memory \
  --vpc-id default
```

### VPC ID Resolution

If `--vpc-id` is set to `default`, Aerolab will automatically resolve the default VPC in your AWS account.

## Cloud Databases List

List all Aerospike Cloud databases.

### Basic Usage

```bash
aerolab cloud databases list
```

### Output

The command outputs JSON with database information including:
- Database ID
- Database name
- Instance type
- Region
- Cluster size
- Status
- Connection details (host, port, TLS certificate)
- VPC information

### Examples

**List all databases:**
```bash
aerolab cloud databases list
```

**List and filter by name:**
```bash
aerolab cloud databases list | jq '.databases[] | select(.name == "mydb")'
```

**Get database ID:**
```bash
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')
```

**Get connection host:**
```bash
HOST=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.host')
```

**Get TLS certificate:**
```bash
CERT=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.tlsCertificate')
```

## Cloud Databases Update

Update database configuration.

### Basic Usage

```bash
aerolab cloud databases update --database-id <database-id> --cluster-size 4 -i m5d.xlarge
```

### Options

| Option | Description |
|--------|-------------|
| `--database-id` | Database ID (required) |
| `--cluster-size` | New cluster size |
| `-i, --instance-type` | New instance type |

### Examples

**Update cluster size:**
```bash
aerolab cloud databases update \
  --database-id <database-id> \
  --cluster-size 4
```

**Update instance type:**
```bash
aerolab cloud databases update \
  --database-id <database-id> \
  -i m5d.xlarge
```

**Update both cluster size and instance type:**
```bash
aerolab cloud databases update \
  --database-id <database-id> \
  --cluster-size 4 \
  -i m5d.xlarge
```

**Note**: Updates may take time to complete. The database will be unavailable during updates.

## Cloud Databases Delete

Delete an Aerospike Cloud database.

### Basic Usage

```bash
aerolab cloud databases delete --database-id <database-id> --force --wait
```

### Options

| Option | Description |
|--------|-------------|
| `--database-id` | Database ID (required) |
| `--force` | Force deletion without confirmation |
| `--wait` | Wait for deletion to complete |

### Examples

**Delete database:**
```bash
aerolab cloud databases delete \
  --database-id <database-id> \
  --force \
  --wait
```

**Delete database by name:**
```bash
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')
aerolab cloud databases delete --database-id $DID --force --wait
```

**Warning**: This permanently deletes the database and all its data. Use with caution.

## Cloud Databases Peer-VPC

Peer VPC with a database.

### Basic Usage

```bash
aerolab cloud databases peer-vpc --database-id <database-id> --vpc-id vpc-xxxxxxxxx
```

### Options

| Option | Description |
|--------|-------------|
| `--database-id` | Database ID (required) |
| `--vpc-id` | VPC ID to peer with (required) |

### Examples

**Peer VPC with database:**
```bash
aerolab cloud databases peer-vpc \
  --database-id <database-id> \
  --vpc-id vpc-xxxxxxxxx
```

**Note**: VPC peering is typically done automatically during database creation. Use this command if you need to peer additional VPCs.

## Cloud Databases Credentials

Manage database credentials. See [Credentials Management](credentials.md) for detailed documentation.

### Quick Reference

**List credentials:**
```bash
aerolab cloud databases credentials list --database-id <database-id>
```

**Create credentials:**
```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

**Delete credentials:**
```bash
aerolab cloud databases credentials delete \
  --database-id <database-id> \
  --credentials-id <credentials-id>
```

## Common Workflows

### Create Database and Connect

```bash
# 1. Create database
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id default

# 2. Get database ID
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')

# 3. Create credentials
aerolab cloud databases credentials create \
  --database-id $DID \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait

# 4. Get connection details
HOST=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.host')
CERT=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.tlsCertificate')

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

### Update Database

```bash
# 1. Get database ID
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')

# 2. Update cluster size
aerolab cloud databases update \
  --database-id $DID \
  --cluster-size 4 \
  -i m5d.xlarge

# 3. Wait for update to complete (check status)
aerolab cloud databases list | jq '.databases[] | select(.name == "mydb")'
```

### Delete Database

```bash
# 1. Get database ID
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')

# 2. Delete database
aerolab cloud databases delete \
  --database-id $DID \
  --force \
  --wait
```

## Tips

1. **VPC ID**: Use `default` to automatically use the default VPC
2. **Instance Types**: Use `cloud list-instance-types` to see available instance types
3. **Connection Details**: Always use TLS when connecting to Aerospike Cloud databases
4. **Credentials**: Create credentials before connecting to the database
5. **Updates**: Database updates may cause downtime. Plan accordingly
6. **Deletion**: Database deletion is permanent. Ensure you have backups if needed

