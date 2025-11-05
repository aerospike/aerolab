# Cloud Database Credentials Management

Commands for managing database credentials for Aerospike Cloud databases.

## Commands Overview

- `cloud databases credentials list` - List database credentials
- `cloud databases credentials create` - Create database credentials
- `cloud databases credentials delete` - Delete database credentials

## Prerequisites

- AWS backend configured
- Aerospike Cloud database created
- Database ID

## Cloud Databases Credentials List

List all credentials for a database.

### Basic Usage

```bash
aerolab cloud databases credentials list --database-id <database-id>
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--database-id` | Database ID | Yes |

### Examples

**List all credentials:**
```bash
aerolab cloud databases credentials list --database-id <database-id>
```

**Get credential ID:**
```bash
CRED_ID=$(aerolab cloud databases credentials list --database-id <database-id> | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')
```

### Output

The command outputs JSON (no need to specify `-o json` as it's the only output format). The output includes credential information:
- Credential ID
- Username
- Privileges (read-write, read-only, etc.)
- Status

## Cloud Databases Credentials Create

Create new database credentials.

### Basic Usage

```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--database-id` | Database ID | Yes |
| `--username` | Username | Yes |
| `--password` | Password | Yes |
| `--privileges` | Privileges: `read-write` or `read-only` | Yes |
| `--wait` | Wait for credential creation to complete | No |

### Examples

**Create read-write credentials:**
```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait
```

**Create read-only credentials:**
```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username readonly \
  --password readonlypass \
  --privileges read-only \
  --wait
```

**Create credentials without waiting:**
```bash
aerolab cloud databases credentials create \
  --database-id <database-id> \
  --username myuser \
  --password mypassword \
  --privileges read-write
```

### Privileges

- **read-write**: Full read and write access to the database
- **read-only**: Read-only access to the database

## Cloud Databases Credentials Delete

Delete database credentials.

### Basic Usage

```bash
aerolab cloud databases credentials delete \
  --database-id <database-id> \
  --credentials-id <credentials-id>
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--database-id` | Database ID | Yes |
| `--credentials-id` | Credential ID | Yes |

### Examples

**Delete credentials:**
```bash
aerolab cloud databases credentials delete \
  --database-id <database-id> \
  --credentials-id <credentials-id>
```

**Delete credentials by username:**
```bash
# Get credential ID
CRED_ID=$(aerolab cloud databases credentials list --database-id <database-id> | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')

# Delete credentials
aerolab cloud databases credentials delete \
  --database-id <database-id> \
  --credentials-id $CRED_ID
```

**Warning**: This permanently deletes the credentials. The user will no longer be able to connect to the database.

## Common Workflows

### Create and Use Credentials

```bash
# 1. Get database ID
DID=$(aerolab cloud databases list -o json | jq -r '.databases[] | select(.name == "mydb") | .id')

# 2. Create credentials
aerolab cloud databases credentials create \
  --database-id $DID \
  --username myuser \
  --password mypassword \
  --privileges read-write \
  --wait

# 3. Get connection details
HOST=$(aerolab cloud databases get host -n mydb)
CERT=$(aerolab cloud databases get tls-cert -n mydb)

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
# Get database ID
DID=$(aerolab cloud databases list -o json | jq -r '.databases[] | select(.name == "mydb") | .id')

# Create read-write user
aerolab cloud databases credentials create \
  --database-id $DID \
  --username admin \
  --password adminpass \
  --privileges read-write \
  --wait

# Create read-only user
aerolab cloud databases credentials create \
  --database-id $DID \
  --username readonly \
  --password readonlypass \
  --privileges read-only \
  --wait

# List all credentials
aerolab cloud databases credentials list --database-id $DID
```

### Delete Credentials

```bash
# Get database ID
DID=$(aerolab cloud databases list -o json | jq -r '.databases[] | select(.name == "mydb") | .id')

# Get credential ID
CRED_ID=$(aerolab cloud databases credentials list --database-id $DID | \
  jq -r '.credentials[] | select(.name == "myuser") | .id')

# Delete credentials
aerolab cloud databases credentials delete \
  --database-id $DID \
  --credentials-id $CRED_ID
```

## Tips

1. **Wait Flag**: Use `--wait` when creating credentials to ensure they're ready before use
2. **Privileges**: Use read-only credentials for monitoring and analytics applications
3. **Security**: Use strong passwords for credentials
4. **Credential Management**: Keep track of credential IDs for easier management
5. **Multiple Users**: Create multiple credentials for different applications/users
6. **Deletion**: Deleting credentials is permanent. Ensure you have other credentials before deleting

