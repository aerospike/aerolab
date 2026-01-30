# Aerospike Cloud Commands

Commands for managing Aerospike Cloud clusters and related resources.

## Overview

Aerospike Cloud commands allow you to:
- Create and manage Aerospike Cloud clusters
- Manage cluster credentials
- Manage secrets
- Peer VPCs with clusters
- List instance types

## Prerequisites

- AWS backend configured
- AWS credentials with permissions for Aerospike Cloud
- VPC ID for cluster peering

## Commands Overview

- `cloud clusters` - Cluster management
- `cloud secrets` - Secret management
- `cloud list-instance-types` - List available instance types
- `cloud gen-conf-templates` - Generate configuration templates from OpenAPI spec

## Quick Start

### 1. List Available Instance Types

```bash
aerolab cloud list-instance-types
```

**Note**: The command supports multiple output formats (table, json, json-indent, jq, etc.). When using `jq` for parsing, use `-o json`:
```bash
aerolab cloud list-instance-types -o json | jq '.'
```

### 2. Create a Cluster

```bash
aerolab cloud clusters create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory
```

### 3. List Clusters

```bash
aerolab cloud clusters list
```

### 4. Create Credentials

```bash
aerolab cloud clusters credentials create \
  --cluster-id <cluster-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

### 5. Get Connection Details

```bash
# Get host and TLS certificate
HOST=$(aerolab cloud clusters get host -n mydb)
CERT=$(aerolab cloud clusters get tls-cert -n mydb)

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

- [Cluster Management](clusters.md) - Create, update, delete clusters
- [Credentials Management](credentials.md) - Manage cluster credentials
- [Secrets Management](secrets.md) - Manage secrets
- [VPC Peering](vpc-peering.md) - Peer VPCs with clusters
- [Troubleshooting](troubleshooting.md) - Troubleshooting and advanced usage

## Generate Configuration Templates

For advanced users who want to customize the full Aerospike Cloud configuration, you can generate template files from the official OpenAPI specification:

```bash
# Generate templates in current directory
aerolab cloud gen-conf-templates

# Generate templates in specific directory
aerolab cloud gen-conf-templates -d ./templates
```

This creates four template files:
- `create-full.json` - Full create request body with all configurable options
- `create-aerospike-server.json` - Just the aerospikeServer section (namespaces, service, etc.)
- `update-full.json` - Full update request body
- `update-aerospike-server.json` - Just the aerospikeServer section for updates

Use these templates with the `--custom-conf` option in create/update commands:

```bash
# Use full request template
aerolab cloud clusters create -n mydb --custom-conf ./templates/create-full.json

# Use aerospikeServer-only template
aerolab cloud clusters create -n mydb -i m5d.large -r us-east-1 -s 2 -d memory \
  --custom-conf ./my-aerospike-config.json
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
  --vpc-id vpc-xxxxxxxxx

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

### Update Cluster

```bash
# Update cluster size and instance type
aerolab cloud clusters update \
  --cluster-id <cluster-id> \
  --cluster-size 4 \
  -i m5d.xlarge
```

### Delete Cluster

```bash
# Delete cluster
aerolab cloud clusters delete \
  --cluster-id <cluster-id> \
  --force \
  --wait
```

