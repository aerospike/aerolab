# Cloud Secrets Management

Commands for managing secrets in Aerospike Cloud.

## Commands Overview

- `cloud secrets list` - List all secrets
- `cloud secrets create` - Create a new secret
- `cloud secrets delete` - Delete a secret

## Prerequisites

- AWS backend configured
- AWS credentials with permissions for Aerospike Cloud

## Cloud Secrets List

List all secrets.

### Basic Usage

```bash
aerolab cloud secrets list
```

### Examples

**List all secrets:**
```bash
aerolab cloud secrets list
```

**Filter secrets by description:**
```bash
aerolab cloud secrets list | jq '.secrets[] | select(.description == "aerolab")'
```

**Get secret ID:**
```bash
SECRET_ID=$(aerolab cloud secrets list | jq -r '.secrets[] | select(.description == "aerolab") | .id')
```

### Output

The command outputs JSON (no need to specify `-o json` as it's the only output format). The output includes secret information:
- Secret ID
- Name
- Description
- Status

## Cloud Secrets Create

Create a new secret.

### Basic Usage

```bash
aerolab cloud secrets create \
  --name "mysecret" \
  --description "My secret" \
  --value "secretvalue"
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--name` | Secret name | Yes |
| `--description` | Secret description | Yes |
| `--value` | Secret value | Yes |

### Examples

**Create a secret:**
```bash
aerolab cloud secrets create \
  --name "mysecret" \
  --description "My secret" \
  --value "secretvalue"
```

**Create a secret with specific description:**
```bash
aerolab cloud secrets create \
  --name "aerolab" \
  --description "aerolab" \
  --value "aerolab"
```

**Verify secret creation:**
```bash
aerolab cloud secrets list | jq '.secrets[] | select(.description == "aerolab")'
```

## Cloud Secrets Delete

Delete a secret.

### Basic Usage

```bash
aerolab cloud secrets delete --secret-id <secret-id>
```

### Options

| Option | Description | Required |
|--------|-------------|----------|
| `--secret-id` | Secret ID | Yes |

### Examples

**Delete secret:**
```bash
aerolab cloud secrets delete --secret-id <secret-id>
```

**Delete secret by description:**
```bash
# Get secret ID
SECRET_ID=$(aerolab cloud secrets list | jq -r '.secrets[] | select(.description == "aerolab") | .id')

# Delete secret
aerolab cloud secrets delete --secret-id $SECRET_ID
```

**Delete all secrets matching description:**
```bash
aerolab cloud secrets list | jq -r '.secrets[] | select(.description == "aerolab") | .id' | \
  while read line; do
    aerolab cloud secrets delete --secret-id $line
  done
```

**Warning**: This permanently deletes the secret. Use with caution.

## Common Workflows

### Create and Use Secrets

```bash
# 1. Create secret
aerolab cloud secrets create \
  --name "mysecret" \
  --description "aerolab" \
  --value "mysecretvalue"

# 2. Verify creation
aerolab cloud secrets list | jq '.secrets[] | select(.description == "aerolab")'

# 3. Get secret ID
SECRET_ID=$(aerolab cloud secrets list | jq -r '.secrets[] | select(.description == "aerolab") | .id')

# 4. Use secret (in your application)
# Note: Secrets are typically used by applications, not directly by Aerolab
```

### Clean Up Secrets

```bash
# Delete all secrets matching description
aerolab cloud secrets list | jq -r '.secrets[] | select(.description == "aerolab") | .id' | \
  while read line; do
    aerolab cloud secrets delete --secret-id $line
  done
```

## Tips

1. **Naming**: Use descriptive names and descriptions for secrets
2. **Security**: Keep secret values secure and don't expose them in logs
3. **Management**: Use consistent naming conventions for easier management
4. **Cleanup**: Regularly clean up unused secrets
5. **Verification**: Always verify secret creation before use
6. **Deletion**: Deleting secrets is permanent. Ensure you have backups if needed

