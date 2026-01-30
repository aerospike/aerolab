# Cloud Cluster Credentials Management

Commands for managing cluster credentials for Aerospike Cloud clusters.

## Commands Overview

- `cloud clusters credentials list` - List cluster credentials
- `cloud clusters credentials create` - Create cluster credentials
- `cloud clusters credentials delete` - Delete cluster credentials

## Prerequisites

- AWS backend configured
- Aerospike Cloud cluster created
- Cluster ID

## Cloud Clusters Credentials List

List all credentials for a cluster.

### Basic Usage

```bash
aerolab cloud clusters credentials list --cluster-id <cluster-id>
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--cluster-id` | Cluster ID | Yes |

### Examples

**List all credentials:**
```bash
aerolab cloud clusters credentials list --cluster-id <cluster-id>
```

**Get credential ID:**
```bash
CRED_ID=$(aerolab cloud clusters credentials list --cluster-id <cluster-id> | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')
```

### Output

The command outputs JSON (no need to specify `-o json` as it's the only output format). The output includes credential information:
- Credential ID
- Username
- Privileges (read-write, read-only, etc.)
- Status

## Cloud Clusters Credentials Create

Create new cluster credentials.

### Basic Usage

```bash
aerolab cloud clusters credentials create \
  --cluster-id <cluster-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--cluster-id` | Cluster ID | Yes |
| `--username` | Username | Yes |
| `--password` | Password | Yes |
| `--privileges` | Privileges: `read-write` or `read-only` | Yes |
| `--wait` | Wait for credential creation to complete | No |

### Examples

**Create read-write credentials:**
```bash
aerolab cloud clusters credentials create \
  --cluster-id <cluster-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

**Create read-only credentials:**
```bash
aerolab cloud clusters credentials create \
  --cluster-id <cluster-id> \
  --username readonly \
  --password readonlypass \
  --privileges read-only \
  --wait
```

**Create credentials without waiting:**
```bash
aerolab cloud clusters credentials create \
  --cluster-id <cluster-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write
```

### Privileges

- **read-write**: Full read and write access to the cluster
- **read-only**: Read-only access to the cluster

## Cloud Clusters Credentials Delete

Delete cluster credentials.

### Basic Usage

```bash
aerolab cloud clusters credentials delete \
  --cluster-id <cluster-id> \
  --credentials-id <credentials-id>
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--cluster-id` | Cluster ID | Yes |
| `--credentials-id` | Credential ID | Yes |

### Examples

**Delete credentials:**
```bash
aerolab cloud clusters credentials delete \
  --cluster-id <cluster-id> \
  --credentials-id <credentials-id>
```

**Delete credentials by username:**
```bash
# Get credential ID
CRED_ID=$(aerolab cloud clusters credentials list --cluster-id <cluster-id> | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')

# Delete credentials
aerolab cloud clusters credentials delete \
  --cluster-id <cluster-id> \
  --credentials-id $CRED_ID
```

**Warning**: This permanently deletes the credentials. The user will no longer be able to connect to the cluster.

## Common Workflows

### Create and Use Credentials

```bash
# 1. Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# 2. Create credentials
aerolab cloud clusters credentials create \
  --cluster-id $CID \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait

# 3. Get connection details
HOST=$(aerolab cloud clusters get host -n mydb)
CERT=$(aerolab cloud clusters get tls-cert -n mydb)

# 4. Save and upload certificate
echo "$CERT" > ca.pem
aerolab files upload ca.pem /opt/ca.pem

# 5. Connect using aql
aerolab attach aql -- \
  --tls-enable \
  --tls-name $HOST \
  --tls-cafile /opt/ca.pem \
  -h $HOST:4000 \
  -U myuser \
  -P mypassword \
  -c "show namespaces"
```

### Create Multiple Credentials

```bash
# Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# Create read-write user
aerolab cloud clusters credentials create \
  --cluster-id $CID \
  --username admin \
  --password adminpass \
  --privileges read-write \
  --wait

# Create read-only user
aerolab cloud clusters credentials create \
  --cluster-id $CID \
  --username readonly \
  --password readonlypass \
  --privileges read-only \
  --wait

# List all credentials
aerolab cloud clusters credentials list --cluster-id $CID
```

### Delete Credentials

```bash
# Get cluster ID
CID=$(aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "mydb") | .id')

# Get credential ID
CRED_ID=$(aerolab cloud clusters credentials list --cluster-id $CID | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')

# Delete credentials
aerolab cloud clusters credentials delete \
  --cluster-id $CID \
  --credentials-id $CRED_ID
```

## Tips

1. **Wait Flag**: Use `--wait` when creating credentials to ensure they're ready before use
2. **Privileges**: Use read-only credentials for monitoring and analytics applications
3. **Security**: Use strong passwords for credentials
4. **Credential Management**: Keep track of credential IDs for easier management
5. **Multiple Users**: Create multiple credentials for different applications/users
6. **Deletion**: Deleting credentials is permanent. Ensure you have other credentials before deleting

