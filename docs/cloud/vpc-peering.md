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
  --database-id <database-id> \
  --vpc-id vpc-xxxxxxxxx
```

### Find Database ID

```bash
DID=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .id')
```

### Peer VPC

```bash
aerolab cloud databases peer-vpc \
  --database-id $DID \
  --vpc-id vpc-xxxxxxxxx
```

## VPC Requirements

### CIDR Block

The VPC must have a valid CIDR block that doesn't conflict with the database's network.

### Route Tables

Ensure route tables are configured to route traffic to the peered VPC.

### Security Groups

Configure security groups to allow traffic on the required ports (typically port 4000 for Aerospike).

## Connection from Peered VPC

Once VPC peering is established, you can connect to the database from resources in the peered VPC:

```bash
# Get connection details
HOST=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.host')
CERT=$(aerolab cloud databases list | jq -r '.databases[] | select(.name == "mydb") | .connectionDetails.tlsCertificate')

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
   aerolab cloud databases list | jq '.databases[] | select(.name == "mydb") | .vpc'
   ```

## Tips

1. **Default VPC**: Use `default` for VPC ID to automatically use the default VPC
2. **CIDR Blocks**: Ensure VPC CIDR blocks don't conflict
3. **Security Groups**: Configure security groups to allow traffic on port 4000
4. **Route Tables**: Ensure route tables are configured correctly
5. **Multiple VPCs**: You can peer multiple VPCs with the same database

