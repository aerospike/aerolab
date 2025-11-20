# Vagrant Backend for Aerolab

Local virtualization backend using HashiCorp Vagrant for development and testing.

## Quick Start

### Prerequisites

```bash
# Install Vagrant
brew install vagrant  # macOS
# or download from https://www.vagrantup.com/downloads

# Install a provider (choose one)
brew install --cask virtualbox      # VirtualBox (most common)
brew install --cask vmware-fusion   # VMware (commercial)
# or libvirt, hyper-v, parallels
```

### Configuration

Add to your Aerolab config:

```yaml
vagrant:
  provider: "virtualbox"  # or vmware_desktop, libvirt, hyperv, parallels
  regions:
    default:
      workDir: "~/.aerolab/vagrant"
```

## Features

### ✅ Implemented

- **Instance Management**: Create, list, start, stop, destroy VMs
- **SSH Access**: Full SSH command execution and SFTP support
- **Cluster Support**: Multi-node clusters with automatic networking
- **Tag Management**: Add, remove, and update instance tags
- **Metadata**: Store and retrieve arbitrary metadata
- **Expiry Tags**: Set expiration dates on instances
- **Multi-Provider**: Support for VirtualBox, VMware, libvirt, Hyper-V, Parallels
- **Parallel Operations**: Concurrent VM operations for performance
- **SSH Key Management**: Automatic SSH key generation and injection

### ⚠️ Limited

- **Volumes**: No separate volume objects (use synced folders in Vagrantfile)
- **Networks**: Provider-specific, defined in Vagrantfiles
- **Firewalls**: OS-level, not managed by virtualization layer
- **Images**: Curated list of common boxes (not dynamic)

### ❌ Not Implemented

- **Expiry System**: No automated cleanup (manual management)
- **VPC Peering**: Cloud-only feature
- **DNS Management**: Cloud-only feature
- **Pricing**: Local VMs have no cost

## Usage Examples

### Create Instance Params

```go
import "github.com/aerospike/aerolab/pkg/backend/clouds/bvagrant"

params := &bvagrant.CreateInstanceParams{
    Box:        "ubuntu/jammy64",
    BoxVersion: "",              // empty = latest
    CPUs:       2,
    Memory:     2048,            // MB
    DiskSize:   20,              // GB (provider-dependent)
    NetworkType: "private_network",
    NetworkIP:   "192.168.56.10",
    SyncedFolders: map[string]string{
        "/host/path": "/guest/path",
    },
    PortForwards: map[int]int{
        8080: 8080,  // guest:host
    },
    SkipSshReadyCheck: false,
}
```

### Create Instances

```go
input := &backends.CreateInstanceInput{
    ClusterName:     "mycluster",
    Nodes:           3,
    BackendType:     backends.BackendTypeVagrant,
    BackendSpecificParams: map[backends.BackendType]interface{}{
        backends.BackendTypeVagrant: params,
    },
    SSHKeyName: "default",
    Owner:      "developer",
    Tags: map[string]string{
        "environment": "dev",
    },
    Expires: time.Now().Add(7 * 24 * time.Hour),
}

output, err := vagrantBackend.CreateInstances(input, 10*time.Minute)
```

### List Instances

```go
instances, err := vagrantBackend.GetInstances(volumes, networks, firewalls)
for _, inst := range instances.Describe() {
    fmt.Printf("VM: %s, State: %s, IP: %s\n", 
        inst.Name, inst.InstanceState, inst.IP.Private)
}
```

### Execute Commands

```go
outputs := instances.Exec(&backends.ExecInput{
    Username:        "vagrant",
    ParallelThreads: 5,
    ConnectTimeout:  30 * time.Second,
    ExecDetail: sshexec.ExecDetail{
        Command: []string{"uname", "-a"},
        Stdout:  os.Stdout,
        Stderr:  os.Stderr,
    },
})
```

## Architecture

```
bvagrant/
├── init.go              # Backend initialization
├── common.go            # Shared constants
├── connect.go           # State cache and helpers
├── instances.go         # Core VM operations
├── volumes.go           # Volume stubs
├── networks.go          # Network stubs
├── firewalls.go         # Firewall stubs
├── images.go            # Box/image management
├── pricing.go           # Pricing info (always $0)
├── expiry.go            # Expiry stubs
└── not-implemented.go   # Cloud-specific features
```

## State Management

### Directory Structure

```
{workDir}/
├── {vm-uuid-1}/
│   ├── Vagrantfile
│   ├── metadata.json
│   └── .vagrant/
├── {vm-uuid-2}/
│   ├── Vagrantfile
│   ├── metadata.json
│   └── .vagrant/
...
```

### Metadata Format

```json
{
  "aerolab.version": "7.0.0",
  "aerolab.project": "myproject",
  "aerolab.cluster.name": "mycluster",
  "aerolab.cluster.uuid": "abc-123",
  "aerolab.node.no": "1",
  "aerolab.name": "myproject-mycluster-1",
  "aerolab.owner": "developer",
  "aerolab.description": "Test cluster",
  "aerolab.expires": "2025-12-31T23:59:59Z",
  "aerolab.os.name": "ubuntu",
  "aerolab.os.version": "22.04",
  "boxName": "ubuntu/jammy64",
  "boxVersion": "20231201.0.0",
  "cpus": "2",
  "memory": "2048",
  "createTime": "2025-11-20T10:30:00Z"
}
```

## Supported Boxes

Pre-configured list includes:

- ubuntu/jammy64 (22.04)
- ubuntu/focal64 (20.04)
- generic/ubuntu2204
- generic/ubuntu2004
- centos/7
- generic/rhel8
- generic/rhel9
- debian/bullseye64

Any Vagrant Cloud box can be used via `Box` parameter.

## Performance

### Typical Operation Times

- **Create Instance**: 30s - 5min (depending on box download)
- **Start Instance**: 10s - 30s
- **Stop Instance**: 5s - 15s
- **Destroy Instance**: 5s - 30s
- **List Instances**: 100ms - 2s (depends on VM count)
- **SSH Command**: 100ms - 500ms

### Optimization Tips

1. **Pre-download boxes**: `vagrant box add ubuntu/jammy64`
2. **Use local box cache**: Boxes are cached after first download
3. **Adjust parallel threads**: Increase for faster bulk operations
4. **Skip SSH check**: Use `SkipSshReadyCheck: true` when not needed
5. **Use SSD storage**: Significantly faster VM creation and operation

## Troubleshooting

### VMs Not Listed

```bash
# Check work directory
ls -la ~/.aerolab/vagrant/

# Verify metadata exists
cat ~/.aerolab/vagrant/{vm-uuid}/metadata.json

# Check Vagrant status
cd ~/.aerolab/vagrant/{vm-uuid}
vagrant status
```

### SSH Connection Fails

```bash
# Test Vagrant SSH directly
cd ~/.aerolab/vagrant/{vm-uuid}
vagrant ssh

# Check SSH config
vagrant ssh-config

# Verify keys
ls -la ~/.aerolab/ssh-keys/
```

### Slow Performance

```bash
# Check host resources
top
df -h

# Check provider version
VBoxManage --version
# or
vmrun -v

# Reduce VMs or increase host resources
```

### Provider Errors

```bash
# Reinstall provider
brew reinstall virtualbox

# Check provider daemon
VBoxManage list vms

# Check for port conflicts
lsof -i -P | grep LISTEN
```

## Limitations

### By Design

- **No cloud features**: VPC, DNS, pricing, etc.
- **Local only**: Can't manage remote Vagrant hosts
- **Provider-dependent**: Some features vary by provider
- **No snapshots**: Not yet implemented (possible future feature)

### Technical

- **Disk space**: Each VM requires significant disk (5-50GB)
- **Memory**: Each VM consumes configured RAM
- **CPU**: Limited by host CPU count
- **Networking**: Provider-specific limitations

## Documentation

- [IMPLEMENTATION.md](./IMPLEMENTATION.md) - Detailed implementation guide
- [DESIGN-DECISIONS.md](./DESIGN-DECISIONS.md) - Design rationale and trade-offs

## Contributing

When contributing to the Vagrant backend:

1. **Test multiple providers**: If possible, test VirtualBox and VMware
2. **Include error output**: Vagrant CLI output is essential for debugging
3. **Update documentation**: Keep docs in sync with code
4. **Follow patterns**: Match existing backend structure
5. **Consider performance**: Vagrant operations can be slow

## Support

- **Vagrant Issues**: https://github.com/hashicorp/vagrant/issues
- **Provider Issues**: Contact provider vendor
- **Aerolab Issues**: Create issue in Aerolab repository

## License

Same as Aerolab project license.

