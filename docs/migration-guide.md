# AeroLab 8 Migration Guide

This guide explains how to migrate from AeroLab 7.x to AeroLab 8.x. Please note the migration does not remove AeroLab 7.x configuration, ssh keys or support for the instances. Once the migration is complete, both v7 and v8 can be used side by side for the existing migrated resources.

## AGI (Aerospike Grafana Integration) Migration

### AGI Instances

AGI instances from AeroLab 7.x **are migrated** and can be managed by v8 AGI commands.

**What happens during migration:**
- `inventory migrate` detects AGI instances
- AGI-specific tags are preserved (without `v7-` prefix) so v8 commands can read them
- AGI volumes (EFS for AWS, Persistent Disks for GCP) are recognized and preserved
- The migration marker is added alongside existing tags

**After migration:**
- Use `aerolab agi list` to see migrated AGI instances
- Use `aerolab agi status -n NAME` to check AGI status
- Use `aerolab agi stop/start -n NAME` to manage instances
- AGI volumes are recognized by v8 and can be reused

**Note:** Some advanced AGI features may require recreation of the AGI instance for full v8 compatibility. Basic operations (start, stop, status, list) work immediately after migration.

### AGI Monitor Instances - Require Recreation

**AGI Monitor instances MUST be recreated after migration.** The migration will detect these instances and warn you, but they cannot be automatically upgraded because they run the aerolab binary internally.

**Why AGI Monitor requires recreation:**
- AGI Monitor instances run `aerolab agi monitor listen` as a systemd service
- The v7 aerolab binary on the instance is NOT compatible with v8 AGI instances
- The configuration format and systemd service may have changed

**Migration steps for AGI Monitor:**
1. Note your current monitor configuration (check cloud console tags or use v7 aerolab)
2. After running `inventory migrate`, destroy the old monitor:
   ```bash
   aerolab client destroy -n <monitor-name>
   ```
3. Recreate using v8:
   ```bash
   aerolab agi monitor create -n <monitor-name> [options]
   ```

The migration will list all AGI Monitor instances that need recreation.

## V7 Expiry System Removal

See [docs/migration-expiry-system.md](migration-expiry-system.md) for details on how to remove the v7 expiry system when it is no longer needed.

## Automatic Migration

When AeroLab 8 starts for the first time and no v8 configuration exists:

1. **If running as `aerolab8`** (renamed binary): Configuration is automatically migrated from `~/.aerolab` to `~/.config/aerolab`
2. **If old config has v7.9+ marker**: Configuration is automatically migrated
3. **Otherwise**: AeroLab 8 copies itself to `aerolab8` and downloads v7.9 to prevent accidental upgrades

This means most users upgrading from v7.9+ will have their configuration migrated automatically on first run.

## Manual Migration

### Config Migration

Use `aerolab config migrate` to migrate your configuration:

```bash
# Migrate with default paths (~/.aerolab → ~/.config/aerolab)
aerolab config migrate

# Migrate with custom paths
aerolab config migrate -o /path/to/old -n /path/to/new

# Migrate without prompts (skip inventory migration)
aerolab config migrate -f

# Migrate config and inventory together
aerolab config migrate -i
```

**What config migrate does:**
- Copies `conf` and `conf.ts` files to the new location
- Fixes Docker backend region if needed (clears invalid region setting)
- Optionally calls `inventory migrate` for the current backend

**Safe to run multiple times:** Running `config migrate` again will simply ensure all configuration files are in place. It will not duplicate or corrupt existing configs.

### Inventory Migration

Use `aerolab inventory migrate` to migrate cloud resource tags and SSH keys:

```bash
# Switch to AWS backend and migrate
aerolab config backend -t aws -r us-east-1
aerolab inventory migrate

# Switch to GCP backend and migrate
aerolab config backend -t gcp -r us-central1
aerolab inventory migrate
```

**What inventory migrate does:**
- Updates resource tags to the new AeroLab 8 format
- Copies SSH keys from the old location (`~/.aerolab`) to the new location (`~/.config/aerolab`) as needed

**Important notes:**
- **Docker backend is NOT supported** for inventory migration (Docker containers use local labels, not cloud tags)
- **Safe to run multiple times:** Each run ensures resource tags are updated and SSH keys are in place; repeated runs have no adverse effect
- **Run for each backend:** If you use multiple cloud providers, switch backends and run `inventory migrate` for each

## Migration Workflow

### Single Backend (AWS or GCP)

```bash
# 1. Run config migrate (will prompt for inventory migration)
aerolab config migrate
```

### Multiple Backends

```bash
# 1. Migrate config first
aerolab config migrate -f

# 2. Migrate AWS inventory
aerolab config backend -t aws -r us-east-1
aerolab inventory migrate

# 3. Migrate GCP inventory
aerolab config backend -t gcp -r us-central1
aerolab inventory migrate
```

### Docker Users

```bash
# Docker only needs config migration
aerolab config migrate -f
```

Docker containers are automatically recognized by AeroLab 8 through their labels—no inventory migration is needed or supported.

## Directory Structure

| Version | Default Path |
|---------|--------------|
| AeroLab 7.x | `~/.aerolab` |
| AeroLab 8.x | `~/.config/aerolab` |

You can override the v8 path using the `AEROLAB_HOME` environment variable.

## Troubleshooting

**"The $AEROLAB_HOME directory is pointing at an old version"**
- Your custom `AEROLAB_HOME` points to a v7 directory
- Pick a new directory and run: `aerolab config migrate -o $AEROLAB_HOME -n /new/path`
- Update `AEROLAB_HOME` to the new path

**Inventory resources not showing up**
- Ensure you've run `inventory migrate` for the correct backend
- Check that you're using the right region: `aerolab config backend -t aws -r <region>`

