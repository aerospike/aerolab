# V7 Expiry System Removal

**Important:** The v7 expiry system is NOT automatically removed during migration. If you had the expiry system installed in v7, it will continue running alongside the v8 expiry system.

## Why is the v7 expiry system not auto-deleted?

The migration process focuses on tagging resources and copying configuration files. Removing cloud infrastructure components (Lambda functions, Cloud Scheduler jobs, IAM roles, etc.) requires careful handling and explicit user consent, as these components might be shared or have dependencies.

## Detecting v7 Expiry System

AeroLab v8 will warn you if it detects a v7 expiry system still installed when you:
- Create clusters, instances, or volumes with expiry enabled
- Run expiry-related commands (`aerolab config expiry-install`, `aerolab config expiry-list`, etc.)

## How to Remove the v7 Expiry System

To remove the v7 expiry system, you need to use AeroLab v7. Download the v7.9.0 release zip file for your platform and use its expiry removal command.

### Step 1: Download AeroLab v7.9.0

Download the appropriate zip file for your platform from the [v7.9.0 release](https://github.com/aerospike/aerolab/releases/tag/v7.9.0):

| Platform | Download Link |
|----------|---------------|
| Linux (amd64) | [aerolab-linux-amd64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-linux-amd64-7.9.0.zip) |
| Linux (arm64) | [aerolab-linux-arm64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-linux-arm64-7.9.0.zip) |
| macOS (Intel) | [aerolab-macos-amd64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-macos-amd64-7.9.0.zip) |
| macOS (Apple Silicon) | [aerolab-macos-arm64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-macos-arm64-7.9.0.zip) |
| Windows (amd64) | [aerolab-windows-amd64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-windows-amd64-7.9.0.zip) |
| Windows (arm64) | [aerolab-windows-arm64-7.9.0.zip](https://github.com/aerospike/aerolab/releases/download/v7.9.0/aerolab-windows-arm64-7.9.0.zip) |

### Step 2: Extract and Configure

**Linux/macOS:**
```bash
# Extract the zip file
unzip aerolab-*.zip

# Make it executable (Linux/macOS only)
chmod +x aerolab

# Rename for clarity
mv aerolab aerolab7
```

**Windows (PowerShell):**
```powershell
# Extract the zip file
Expand-Archive aerolab-windows-*.zip -DestinationPath .

# Rename for clarity
Rename-Item aerolab.exe aerolab7.exe
```

### Step 3: Remove the v7 Expiry System

**AWS:**
```bash
# First, configure the backend to the region where expiry was installed
./aerolab7 config backend -t aws -r REGION

# Remove the v7 expiry system
./aerolab7 config aws expiry-remove
```

**GCP:**
```bash
# First, configure the backend to the region where expiry was installed
./aerolab7 config backend -t gcp -r REGION

# Remove the v7 expiry system
./aerolab7 config gcp expiry-remove
```

Replace `REGION` with the appropriate region (e.g., `us-east-1` for AWS, `us-central1` for GCP).

> **Note:** If you had the expiry system installed in multiple regions, you'll need to repeat the process for each region.

## What v7 Expiry System Resources Look Like

**AWS v7 Expiry System:**
- Lambda function: `aerolab-expiries`
- EventBridge Scheduler: `aerolab-expiries`
- IAM roles: `aerolab-expiries`, `AerolabRole`

**GCP v7 Expiry System:**
- Cloud Function: `aerolab-expiries`
- Cloud Scheduler Job: `aerolab-expiries`
- Storage Bucket: `aerolab-{projectId}` (with `expiry-system.json` object)
