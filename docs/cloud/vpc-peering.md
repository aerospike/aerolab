# VPC Peering for Cloud Databases

Guide for peering VPCs with Aerospike Cloud databases.

## Overview

VPC peering allows your AWS resources to connect to Aerospike Cloud databases over a private network connection. This is typically done automatically during database creation, but you can also peer additional VPCs.

## Automatic VPC Peering

When creating a database, VPC peering is automatically set up with the specified VPC:

```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id vpc-xxxxxxxxx
```

### Using Default VPC

You can use `default` as the VPC ID to automatically use the default VPC:

```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --availability-zone-count=2 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id default
```

Aerolab will automatically:
1. Find the default VPC in your AWS account
2. Get the VPC CIDR block
3. Set up VPC peering with the database

## Manual VPC Peering

You can also peer additional VPCs with an existing database:

```bash
aerolab cloud databases peer-vpc \
  -d <database-id> \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx
```

### Find Database ID

```bash
DID=$(aerolab cloud databases list -o json | jq -r '.databases[] | select(.name == "mydb") | .id')
```

### Peer VPC

```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx
```

### Command Options

| Option | Description |
|--------|-------------|
| `-d, --database-id` | Database ID (required) |
| `-r, --region` | AWS region (required) |
| `--vpc-id` | VPC ID to peer with (default: `default`) |
| `--stage-initiate` | Execute only the initiate stage (request VPC peering from cloud) |
| `--stage-accept` | Execute only the accept stage (accept the VPC peering request) |
| `--stage-route` | Execute only the route stage (create route in VPC route table) |
| `--stage-associate-dns` | Execute only the DNS association stage (associate VPC with hosted zone) |
| `--force-route-creation` | Force route creation even if a conflicting route exists |

### Stage Execution

The VPC peering process consists of four stages:

1. **Initiate** (`--stage-initiate`): Request VPC peering from the cloud
2. **Accept** (`--stage-accept`): Accept the VPC peering request in your AWS account
3. **Route** (`--stage-route`): Create route in VPC route table to direct traffic to the peered database
4. **Associate DNS** (`--stage-associate-dns`): Associate VPC with the database's private hosted zone

**Behavior:**
- **No stages specified**: All stages are executed in order. Already completed stages are automatically skipped.
- **Specific stages specified**: Only the specified stages are executed.
- **Stage failure**: If a stage fails, further stages are aborted.

### Running Specific Stages

You can run individual stages of the peering process:

**Execute only the initiate stage:**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-initiate
```

**Execute only the accept stage:**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-accept
```

**Execute only the route stage:**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-route
```

**Execute only the DNS association stage:**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-associate-dns
```

**Force route creation (replace conflicting route):**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-route \
  --force-route-creation
```

**Execute multiple specific stages:**
```bash
aerolab cloud databases peer-vpc \
  -d $DID \
  -r us-east-1 \
  --vpc-id vpc-xxxxxxxxx \
  --stage-accept \
  --stage-route
```

## VPC Requirements

### CIDR Block

The VPC must have a valid CIDR block that doesn't conflict with the database's network.

### Cloud Database CIDR

By default, Aerospike Cloud databases are assigned a CIDR block starting from `10.128.0.0/19`. When you specify a VPC-ID during database creation, aerolab automatically:

1. **Checks for CIDR collisions**: Examines your VPC's route tables for existing routes
2. **Finds available CIDR**: If the default CIDR is already in use, automatically finds the next available one (10.129.0.0/19, 10.130.0.0/19, etc.)
3. **Applies the CIDR**: Uses the available CIDR for the database infrastructure

You can also specify a custom CIDR block using the `--cloud-cidr` option:

```bash
aerolab cloud databases create -n mydb \
  -i m5d.large \
  -r us-east-1 \
  --cluster-size=2 \
  --data-storage memory \
  --vpc-id vpc-xxxxxxxxx \
  --cloud-cidr 10.200.0.0/19
```

**Note**: If you specify a custom CIDR that is already in use in your VPC routes, the command will fail with an error before creating the database.

### Route Tables

Ensure route tables are configured to route traffic to the peered VPC.

### Security Groups

Configure security groups to allow traffic on the required ports (typically port 4000 for Aerospike).

## Connection from Peered VPC

Once VPC peering is established, you can connect to the database from resources in the peered VPC:

```bash
# Get connection details
HOST=$(aerolab cloud databases get host -n mydb)
CERT=$(aerolab cloud databases get tls-cert -n mydb)

# Save certificate
echo "$CERT" > ca.pem

# Upload certificate to your instance
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

## Troubleshooting

### VPC Not Found

If you see "VPC not found" errors:

1. Verify the VPC ID:
   ```bash
   aws ec2 describe-vpcs --vpc-ids vpc-xxxxxxxxx
   ```

2. Check VPC CIDR block:
   ```bash
   aws ec2 describe-vpcs --vpc-ids vpc-xxxxxxxxx --query 'Vpcs[0].CidrBlock'
   ```

### Peering Not Working

If peering is not working:

1. Check route tables:
   ```bash
   aws ec2 describe-route-tables --filters "Name=vpc-id,Values=vpc-xxxxxxxxx"
   ```

2. Check security groups:
   ```bash
   aws ec2 describe-security-groups --filters "Name=vpc-id,Values=vpc-xxxxxxxxx"
   ```

3. Verify peering status:
   ```bash
   aerolab cloud databases list -o json | jq '.databases[] | select(.name == "mydb") | .vpc'
   ```

### Route Already Exists

If you see an error like "route for X.X.X.X/X already exists in route table but points to a different target":

This means there's already a route in your VPC's route table for the database's CIDR block, but it points to a different destination (e.g., another peering connection, NAT gateway, or internet gateway).

**Options:**

1. **Review the existing route** to understand why it exists:
   ```bash
   aws ec2 describe-route-tables \
     --filters "Name=vpc-id,Values=vpc-xxxxxxxxx" \
     --query 'RouteTables[*].Routes[?DestinationCidrBlock==`X.X.X.X/X`]'
   ```

2. **Force route replacement** if you're sure you want to replace the existing route:
   ```bash
   aerolab cloud databases peer-vpc \
     -d <database-id> \
     -r us-east-1 \
     --vpc-id vpc-xxxxxxxxx \
     --stage-route \
     --force-route-creation
   ```

3. **Manually delete the conflicting route** and retry:
   ```bash
   aws ec2 delete-route \
     --route-table-id rtb-xxxxxxxxx \
     --destination-cidr-block X.X.X.X/X
   
   aerolab cloud databases peer-vpc \
     -d <database-id> \
     -r us-east-1 \
     --vpc-id vpc-xxxxxxxxx \
     --stage-route
   ```

**Warning**: Replacing or deleting routes may affect other services using the same CIDR block. Make sure you understand the implications before using `--force-route-creation`.

## Tips

1. **Default VPC**: Use `default` for VPC ID to automatically use the default VPC
2. **CIDR Blocks**: Ensure VPC CIDR blocks don't conflict with the cloud database CIDR
3. **Auto CIDR Resolution**: When creating databases with `--vpc-id`, aerolab automatically detects CIDR collisions and finds available CIDRs
4. **Custom CIDR**: Use `--cloud-cidr` during database creation to specify a custom CIDR block
5. **Security Groups**: Configure security groups to allow traffic on port 4000
6. **Route Tables**: Ensure route tables are configured correctly
7. **Multiple VPCs**: You can peer multiple VPCs with the same database
8. **Route Conflicts**: Use `--force-route-creation` to replace existing conflicting routes (use with caution)
9. **Stage Execution**: Use `--stage-initiate`, `--stage-accept`, `--stage-route`, or `--stage-associate-dns` to run specific stages
10. **Resumable Peering**: If peering fails mid-way, you can re-run the command - completed stages will be skipped automatically

