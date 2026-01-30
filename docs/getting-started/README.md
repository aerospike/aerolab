# Getting Started with AeroLab

## Download & Install

Download the latest release from the [GitHub Releases page](https://github.com/aerospike/aerolab/releases).

### macOS

**Option 1: PKG Installer (recommended)**

Download `aerolab-macos-VERSION.pkg` and double-click to install. This universal package supports both Intel and Apple Silicon Macs.

**Option 2: ZIP Archive**

Download the appropriate zip file:
- Apple Silicon: `aerolab-macos-arm64-VERSION.zip`
- Intel: `aerolab-macos-amd64-VERSION.zip`

```bash
# Extract and install
unzip aerolab-macos-*.zip
chmod +x aerolab
sudo mv aerolab /usr/local/bin/
```

### Linux

**Option 1: DEB Package (Debian/Ubuntu)**

```bash
sudo dpkg -i aerolab-linux-amd64-VERSION.deb
# or for ARM64
sudo dpkg -i aerolab-linux-arm64-VERSION.deb
```

**Option 2: RPM Package (RHEL/CentOS/Fedora)**

```bash
sudo rpm -i aerolab-linux-amd64-VERSION.rpm
# or for ARM64
sudo rpm -i aerolab-linux-arm64-VERSION.rpm
```

**Option 3: ZIP Archive**

```bash
unzip aerolab-linux-amd64-VERSION.zip
# or for ARM64
unzip aerolab-linux-arm64-VERSION.zip

chmod +x aerolab
sudo mv aerolab /usr/local/bin/
```

### Windows

Download the appropriate zip file:
- x64: `aerolab-windows-amd64-VERSION.zip`
- ARM64: `aerolab-windows-arm64-VERSION.zip`

Extract and add the directory to your PATH, or double-click the executable from the explorer to have it self-install.

### Verify Installation

```bash
aerolab version
```

## Choose Your Backend

AeroLab supports three backends for running Aerospike clusters. Choose the one that fits your use case:

| Backend | Best For | Requirements |
|---------|----------|--------------|
| **[Docker](docker.md)** | Local development, quick testing | Docker, Docker Desktop, Podman, or Podman Desktop |
| **[AWS](aws.md)** | Production-like environments, performance testing | AWS account and credentials |
| **[GCP](gcp.md)** | Production-like environments, performance testing | GCP project and credentials |

### Docker Backend

The fastest way to get started. Runs Aerospike clusters locally using Docker containers.

```bash
aerolab config backend -t docker
```

→ **[Docker Getting Started Guide](docker.md)**

### AWS Backend

Deploy Aerospike clusters on AWS EC2 instances. Ideal for production-like testing and performance benchmarks.

```bash
aerolab config backend -t aws -r us-east-1
```

→ **[AWS Getting Started Guide](aws.md)**

### GCP Backend

Deploy Aerospike clusters on Google Cloud Compute Engine. Ideal for production-like testing and performance benchmarks.

```bash
aerolab config backend -t gcp -r us-central1 -o your-project-id
```

→ **[GCP Getting Started Guide](gcp.md)**

## Next Steps

After configuring your backend, create your first cluster:

```bash
# Create a 2-node cluster with Aerospike 8.x
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*'

# Check cluster status
aerolab cluster list

# Wait for cluster stability
aerolab aerospike is-stable -w

# View cluster status
aerolab aerospike status
```
