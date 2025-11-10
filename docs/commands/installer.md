# Installer Commands

Installer commands allow you to list and download Aerospike server and tools packages without creating clusters.

## Commands Overview

- `installer list-versions` - List available Aerospike versions
- `installer download` - Download Aerospike installer packages

## Why Use Installer Commands?

- **Offline installation** - Download packages for air-gapped environments
- **Version research** - Explore available Aerospike versions
- **Manual installation** - Get installers for custom deployment workflows
- **Package verification** - Inspect packages before deployment
- **Archive maintenance** - Keep local copies of specific versions

## Installer List Versions

List all available Aerospike server and tools versions.

### Basic Usage

```bash
# List all available versions
aerolab installer list-versions

# List with JSON output
aerolab installer list-versions -o json
```

### What's Shown

The command displays:
- **Aerospike Server versions** (Enterprise and Community)
- **Aerospike Tools versions**
- Version numbers
- Release dates
- Edition types (Enterprise/Community)
- Available architectures

### Output Format

```
Aerospike Server Versions:
  8.1.0.1 (Enterprise)
  8.1.0.0 (Enterprise)
  8.0.0.5 (Enterprise)
  7.2.0.3 (Enterprise)
  ...

Aerospike Tools Versions:
  10.2.1
  10.2.0
  10.1.0
  ...
```

### Filtering by Version

While `list-versions` shows all versions, you can use wildcards when downloading:

```bash
# See all 8.x versions
aerolab installer list-versions | grep "^  8\."

# See all 7.x versions
aerolab installer list-versions | grep "^  7\."
```

## Installer Download

Download Aerospike installer packages to your local machine.

### Basic Usage

```bash
aerolab installer download -d ubuntu -i 24.04 -v '8.1.0.1'
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-d, --distro` | Distribution (ubuntu, centos, rocky, debian, amazon) | Required |
| `-i, --distro-version` | Distribution version | Required |
| `-v, --aerospike-version` | Aerospike version (supports wildcards) | Required |
| `--tools-version` | Specific tools version (optional) | Latest compatible |
| `--download-dir` | Directory to save files | Current directory |
| `-a, --arch` | Architecture (amd64, arm64) | Current system architecture |

### Examples

**Download latest 8.x for Ubuntu 24.04:**
```bash
aerolab installer download -d ubuntu -i 24.04 -v '8.*'
```

**Download specific version:**
```bash
aerolab installer download -d ubuntu -i 24.04 -v '8.1.0.1'
```

**Download for ARM64:**
```bash
aerolab installer download -d ubuntu -i 24.04 -v '8.*' -a arm64
```

**Download for Rocky Linux:**
```bash
aerolab installer download -d rocky -i 9 -v '8.*'
```

**Download to specific directory:**
```bash
mkdir aerospike-packages
aerolab installer download -d ubuntu -i 24.04 -v '8.*' --download-dir aerospike-packages
```

### What Gets Downloaded

The command downloads:
1. **Aerospike Server package** (`.deb`, `.rpm`, or `.tgz`)
2. **Aerospike Tools package** (if applicable)
3. Both Community and Enterprise editions (where applicable)

### File Names

Downloaded files follow the pattern:
```
aerospike-server-<edition>-<version>-<distro>-<arch>.<ext>
aerospike-tools-<version>-<distro>-<arch>.<ext>
```

Examples:
- `aerospike-server-enterprise-8.1.0.1-ubuntu24.04-amd64.deb`
- `aerospike-tools-10.2.1-ubuntu24.04-amd64.deb`

## Supported Distributions

| Distribution | Versions | Package Format |
|--------------|----------|----------------|
| Ubuntu | 24.04, 22.04, 20.04, 18.04 | `.deb` |
| Debian | 13, 12, 11, 10, 9 | `.deb` |
| Rocky Linux | 9, 8 | `.rpm` |
| CentOS | 9, 7 | `.rpm` |
| Amazon Linux | 2023, 2 | `.rpm` |

## Architectures

- **amd64** (x86_64) - Intel/AMD 64-bit processors
- **arm64** (aarch64) - ARM 64-bit processors (AWS Graviton, Apple Silicon, etc.)

## Version Wildcards

The download command supports version wildcards:

- `'8.*'` - Latest 8.x version
- `'7.*'` - Latest 7.x version
- `'8.1.*'` - Latest 8.1.x version
- `'8.1.0.1'` - Exact version

**Note:** Always quote wildcards to prevent shell expansion:
```bash
# Good
aerolab installer download -v '8.*' ...

# Bad (shell might expand the *)
aerolab installer download -v 8.* ...
```

## Use Cases

### Offline Installation

Download packages for deployment in air-gapped environments:

```bash
# Download all required packages
mkdir offline-install
cd offline-install

# Download server
aerolab installer download -d ubuntu -i 24.04 -v '8.*' --download-dir .

# Transfer to air-gapped environment and install manually
```

### Version Testing

Download multiple versions for testing:

```bash
mkdir test-versions
cd test-versions

# Download different versions
aerolab installer download -d ubuntu -i 24.04 -v '8.1.0.1' --download-dir v8.1.0.1
aerolab installer download -d ubuntu -i 24.04 -v '8.0.0.5' --download-dir v8.0.0.5
aerolab installer download -d ubuntu -i 24.04 -v '7.2.0.3' --download-dir v7.2.0.3
```

### Manual Installation

Download and manually install on existing systems:

```bash
# Download
aerolab installer download -d ubuntu -i 24.04 -v '8.*'

# Transfer to target server
scp aerospike-*.deb user@server:/tmp/

# Install on server
ssh user@server
sudo dpkg -i /tmp/aerospike-server-*.deb
sudo dpkg -i /tmp/aerospike-tools-*.deb
```

### Package Verification

Download packages to inspect contents or verify integrity:

```bash
# Download
aerolab installer download -d ubuntu -i 24.04 -v '8.*'

# Inspect DEB package
dpkg -c aerospike-server-*.deb

# Inspect RPM package
rpm -qlp aerospike-server-*.rpm
```

### Building Custom Images

Download packages to include in custom Docker images or VM images:

```bash
# Dockerfile
FROM ubuntu:24.04

# Download packages using aerolab
RUN aerolab installer download -d ubuntu -i 24.04 -v '8.*'

# Install
RUN dpkg -i aerospike-*.deb
```

## Comparison with Cluster Commands

| Feature | installer download | cluster create |
|---------|-------------------|----------------|
| Downloads packages | ✅ Yes | ✅ Yes (internal) |
| Creates instances | ❌ No | ✅ Yes |
| Installs Aerospike | ❌ No | ✅ Yes |
| Configures Aerospike | ❌ No | ✅ Yes |
| Use case | Manual setup | Automated deployment |

## Best Practices

1. **Use version wildcards for latest versions**
   ```bash
   aerolab installer download -v '8.*' ...
   ```

2. **Organize downloads by version**
   ```bash
   mkdir -p packages/v8.1.0.1
   aerolab installer download --download-dir packages/v8.1.0.1 ...
   ```

3. **Match architecture to target system**
   ```bash
   # For AWS Graviton instances
   aerolab installer download -a arm64 ...
   ```

4. **Check available versions first**
   ```bash
   aerolab installer list-versions
   aerolab installer download -v 'X.Y.Z' ...
   ```

5. **Verify downloads**
   ```bash
   # Check file sizes and dates
   ls -lh aerospike-*.deb
   
   # Verify package contents
   dpkg -I aerospike-server-*.deb
   ```

## Package Management

### Debian/Ubuntu (.deb)

```bash
# View package info
dpkg -I aerospike-server-*.deb

# List contents
dpkg -c aerospike-server-*.deb

# Install
sudo dpkg -i aerospike-server-*.deb

# Remove
sudo dpkg -r aerospike-server-enterprise
```

### RPM-based (.rpm)

```bash
# View package info
rpm -qip aerospike-server-*.rpm

# List contents
rpm -qlp aerospike-server-*.rpm

# Install
sudo rpm -i aerospike-server-*.rpm

# Remove
sudo rpm -e aerospike-server-enterprise
```

## Troubleshooting

**Download fails:**
- Check internet connectivity
- Verify distribution and version are supported
- Try exact version instead of wildcard
- Check available disk space

**Version not found:**
```bash
# List all available versions first
aerolab installer list-versions

# Use exact version from the list
aerolab installer download -v '8.1.0.1' ...
```

**Wrong architecture downloaded:**
```bash
# Explicitly specify architecture
aerolab installer download -a arm64 ...
```

**Wildcard not working:**
```bash
# Always quote wildcards
aerolab installer download -v '8.*' ...  # Good
aerolab installer download -v 8.*    ...  # Bad
```

## See Also

- [Cluster Management](cluster.md) - Automated cluster deployment
- [Templates](templates.md) - Pre-configured deployment images
- [Configuration](config.md) - Configure Aerolab backends

