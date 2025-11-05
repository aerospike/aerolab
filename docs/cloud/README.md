# Aerospike Cloud Commands

Commands for managing Aerospike Cloud databases and related resources.

## Overview

Aerospike Cloud commands allow you to:
- Create and manage Aerospike Cloud databases
- Manage database credentials
- Manage secrets
- Peer VPCs with databases
- List instance types

## Prerequisites

- AWS backend configured
- AWS credentials with permissions for Aerospike Cloud
- VPC ID for database peering

## Commands Overview

- `cloud databases` - Database management
- `cloud secrets` - Secret management
- `cloud list-instance-types` - List available instance types

## Quick Start

### 1. List Available Instance Types

```bash
aerolab cloud list-instance-types
```

**Note**: The command supports multiple output formats (table, json, json-indent, jq, etc.). When using `jq` for parsing, use `-o json`:
```bash
aerolab cloud list-instance-types -o json | jq '.'
```

### 2. Create a Database

```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id vpc-xxxxxxxxx
```

### 3. List Databases

```bash
aerolab cloud databases list
```

### 4. Create Credentials

```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

### 5. Get Connection Details

```bash
# Get host and TLS certificate
HOST=$(aerolab cloud databases get host -n mydb)
CERT=$(aerolab cloud databases get tls-cert -n mydb)

# Save certificate
echo "$CERT" > ca.pem

# Upload certificate
aerolab files upload ca.pem /opt/ca.pem

# Connect using aql
aerolab attach aql -- \
  --tls-enable \
  --tls-name $HOST \
  --tls-cafile /opt/ca.pem \
  -h $HOST:4000 \
  -U myuser \
  -P mypassword \
  -c "show namespaces"
```

## Documentation

- [Database Management](databases.md) - Create, update, delete databases
- [Credentials Management](credentials.md) - Manage database credentials
- [Secrets Management](secrets.md) - Manage secrets
- [VPC Peering](vpc-peering.md) - Peer VPCs with databases

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
  --vpc-id vpc-xxxxxxxxx

# 2. Get database ID
DID=$(aerolab cloud databases list -o json | jq -r '.databases[] | select(.name == "mydb") | .id')

# 3. Create credentials
aerolab cloud databases credentials create \
  --database-id $DID \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait

# 4. Get connection details
HOST=$(aerolab cloud databases get host -n mydb)
CERT=$(aerolab cloud databases get tls-cert -n mydb)

# 5. Save certificate
echo "$CERT" > ca.pem

# 6. Upload certificate
aerolab files upload ca.pem /opt/ca.pem

# 7. Connect
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
# Update cluster size and instance type
aerolab cloud databases update \
  --database-id <database-id> \
  --cluster-size 4 \
  -i m5d.xlarge
```

### Delete Database

```bash
# Delete database
aerolab cloud databases delete \
  --database-id <database-id> \
  --force \
  --wait
```

