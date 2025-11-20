# Vagrant Backend - Fixed Issues

## ✅ Issue 1: attach shell hangs - FIXED
**Problem:** `aerolab attach shell` would hang when connecting to Vagrant VMs.

**Root Cause:** The code was using `vagrant ssh -c "command"` for all operations, which doesn't support interactive terminal mode properly.

**Fix:** Modified `InstancesExec()` to detect when `Terminal=true` and no command is specified (interactive shell mode), and use `vagrant ssh` directly without the `-c` flag.

## ✅ Issue 2: Aerospike installs twice - FIXED  
**Problem:** Aerospike was being installed twice during `cluster create`:
- Once in `InstancesCreate` (called internally)
- Again in `ClusterCreate` (redundant)

**Fix:** Removed the duplicate Aerospike installation code from `ClusterCreate`. Now installation only happens once in `InstancesCreate`, which is called by `ClusterCreate`.

## ✅ Issue 3: Permission denied errors - FIXED
**Problem:** Getting "permission denied" errors when trying to configure Aerospike after installation.

**Root Cause:** Combination of two issues:
1. SFTP was connecting as vagrant user instead of root
2. SSH keys weren't properly shared for root access

**Fix:**
1. Updated `ssh-key-setup.sh` to copy Vagrant's SSH key to root's authorized_keys (in addition to aerolab's key)
2. Updated `InstancesGetSftpConfig()` to use aerolab's SSH key (which is installed for both vagrant and root) and connect as the requested user (root)
3. Root SSH access now works properly for SFTP file operations

## How It Works Now

### SSH/SFTP for Root
1. During VM provisioning, `ssh-key-setup.sh` runs and:
   - Copies Vagrant's default SSH key to `/root/.ssh/authorized_keys`
   - Adds aerolab's SSH key to both vagrant and root users
2. SFTP operations use aerolab's key and connect as root directly
3. Command execution uses `vagrant ssh -c "sudo -i bash -c 'command'"` for root access

### Interactive Shell (attach shell)
- Detects `Terminal=true` with no command
- Uses `vagrant ssh` directly (no `-c` flag)
- Provides full interactive terminal experience

### Non-Interactive Commands
- Uses `vagrant ssh -c "command"` for single commands
- Supports sudo for root access
- Handles environment variables properly

