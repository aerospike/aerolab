# Inventory Management Commands

Commands for viewing and managing all Aerolab resources.

## Commands Overview

- `inventory list` - List all resources
- `inventory ansible` - Export inventory in Ansible format
- `inventory genders` - Export inventory in genders format
- `inventory hostfile` - Export inventory in /etc/hosts format
- `inventory instance-types` - List available instance types (AWS/GCP)
- `inventory delete-project-resources` - Delete all project resources
- `inventory refresh-disk-cache` - Refresh inventory disk cache

## Inventory List

List all clusters, instances, and images.

### Basic Usage

```bash
aerolab inventory list
```

### Options

| Option | Description |
|--------|-------------|
| `-o, --output` | Output format: `table`, `tsv`, or `json` |

### Examples

**List all resources:**
```bash
aerolab inventory list
```

**List in TSV format:**
```bash
aerolab inventory list -o tsv
```

**List in JSON format:**
```bash
aerolab inventory list -o json
```

### Output

The `inventory list` command shows:
- All clusters with node information
- All instances
- All images
- Resource states (running, stopped, etc.)
- Resource details (instance types, IPs, etc.)

## Inventory Ansible

Export inventory in Ansible format.

### Basic Usage

```bash
aerolab inventory ansible
```

### Output

Generates Ansible inventory file format that can be used with Ansible playbooks.

### Examples

**Export to file:**
```bash
aerolab inventory ansible > ansible-inventory.ini
```

**Use with Ansible:**
```bash
aerolab inventory ansible > ansible-inventory.ini
ansible -i ansible-inventory.ini all -m ping
```

## Inventory Genders

Export inventory in genders format.

### Basic Usage

```bash
aerolab inventory genders
```

### Output

Generates genders configuration file format.

### Examples

**Export to file:**
```bash
aerolab inventory genders > genders.conf
```

## Inventory Hostfile

Export inventory in /etc/hosts format.

### Basic Usage

```bash
aerolab inventory hostfile
```

### Output

Generates /etc/hosts format with cluster names and IP addresses.

### Examples

**Export to file:**
```bash
aerolab inventory hostfile > hosts-file
```

**Append to /etc/hosts:**
```bash
aerolab inventory hostfile | sudo tee -a /etc/hosts
```

## Inventory Instance-Types

List available instance types (AWS/GCP only).

### Basic Usage

```bash
aerolab inventory instance-types
```

### Options

| Option | Description |
|--------|-------------|
| `-o, --output` | Output format: `table`, `tsv`, or `json` |

### Examples

**List instance types:**
```bash
aerolab inventory instance-types
```

**List in JSON format:**
```bash
aerolab inventory instance-types -o json
```

**List in TSV format:**
```bash
aerolab inventory instance-types -o tsv
```

### Output

Shows available instance types with:
- Instance type names
- CPU information
- Memory information
- Pricing (if available)

## Inventory Delete-Project-Resources

Delete all resources in the current project.

### Basic Usage

```bash
aerolab inventory delete-project-resources -f
```

### Options

| Option | Description |
|--------|-------------|
| `-f, --force` | Force deletion without confirmation |
| `--expiry` | Also delete expiry automation resources |

### Examples

**Delete all resources:**
```bash
aerolab inventory delete-project-resources -f
```

**Delete all resources including expiry:**
```bash
aerolab inventory delete-project-resources -f --expiry
```

**Warning**: This deletes ALL clusters, instances, and images in the current project. Use with caution.

## Inventory Refresh-Disk-Cache

Refresh the inventory disk cache.

### Basic Usage

```bash
aerolab inventory refresh-disk-cache
```

### Examples

**Refresh cache:**
```bash
aerolab inventory refresh-disk-cache
```

**Note**: This forces a refresh of the cached inventory. Useful if inventory cache is enabled and resources have changed.

## Common Workflows

### View All Resources

```bash
# List all resources
aerolab inventory list

# List in JSON format
aerolab inventory list -o json

# List in TSV format
aerolab inventory list -o tsv
```

### Export Inventory for Automation

```bash
# Export for Ansible
aerolab inventory ansible > ansible-inventory.ini

# Export for hosts file
aerolab inventory hostfile > hosts-file

# Export for genders
aerolab inventory genders > genders.conf
```

### Check Available Instance Types

```bash
# List instance types
aerolab inventory instance-types

# List in JSON format
aerolab inventory instance-types -o json
```

### Clean Up All Resources

```bash
# Delete all resources
aerolab inventory delete-project-resources -f

# Delete all resources including expiry
aerolab inventory delete-project-resources -f --expiry
```

### Refresh Cache

```bash
# Refresh inventory cache
aerolab inventory refresh-disk-cache
```

## Tips

1. **Export formats**: Use different export formats for integration with other tools
2. **Instance types**: Check available instance types before creating clusters
3. **Cleanup**: Use `delete-project-resources` carefully as it deletes everything
4. **Cache**: Refresh cache if resources have changed externally
5. **JSON format**: Use JSON format for programmatic access to inventory data

