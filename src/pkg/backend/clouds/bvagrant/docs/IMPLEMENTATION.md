# Vagrant Backend Implementation for Aerolab

## Overview

The Vagrant backend (`bvagrant`) provides local virtualization support for Aerolab using HashiCorp Vagrant. This backend enables users to create, manage, and interact with virtual machines on their local development machines using providers like VirtualBox, VMware, libvirt, Hyper-V, or Parallels.

## Architecture

### Core Components

The Vagrant backend follows the same architectural patterns as other Aerolab backends (AWS, GCP, Docker):

```
bvagrant/
├── init.go              # Initialization and configuration
├── common.go            # Shared constants and embedded resources
├── connect.go           # Vagrant state cache and helper functions
├── instances.go         # Core instance management (create/list/destroy/attach)
├── volumes.go           # Volume operations (minimal for Vagrant)
├── networks.go          # Network operations (minimal for Vagrant)
├── firewalls.go         # Firewall operations (minimal for Vagrant)
├── images.go            # Vagrant box/image management
├── pricing.go           # Pricing information (always $0 for local VMs)
├── expiry.go            # Expiry system integration
├── not-implemented.go   # Cloud-specific features not applicable
└── IMPLEMENTATION.md    # This file
```

### State Management

Unlike cloud providers with APIs, Vagrant state is managed through:

1. **Metadata Files**: Each VM has a `metadata.json` file storing Aerolab-specific tags and configuration
2. **Vagrantfiles**: Standard Vagrant configuration files generated per VM
3. **Vagrant CLI**: All operations execute through `vagrant` command-line tool
4. **State Cache**: In-memory cache to reduce expensive `vagrant status` calls

### Directory Structure

```
{workDir}/
├── {vm-uuid-1}/
│   ├── Vagrantfile         # VM configuration
│   ├── metadata.json       # Aerolab metadata
│   └── .vagrant/           # Vagrant internal state
├── {vm-uuid-2}/
│   ├── Vagrantfile
│   ├── metadata.json
│   └── .vagrant/
...
```

## Implementation Details

### Instance Creation Flow

1. **Validate Parameters**: Check required fields in `CreateInstanceParams`
2. **Generate VM ID**: Create unique UUID for VM directory
3. **Create Metadata**: Build metadata with tags, cluster info, and configuration
4. **Generate Vagrantfile**: Create Vagrantfile with:
   - Box name and version
   - CPU and memory allocation
   - Network configuration
   - Port forwarding
   - SSH key provisioning
   - Synced folders
5. **Execute `vagrant up`**: Start the VM with specified provider
6. **Wait for SSH**: Optionally verify SSH connectivity
7. **Update Inventory**: Refresh instance cache

### Instance Listing Flow

1. **Scan Work Directory**: Find all subdirectories in configured workDir
2. **Read Metadata**: Parse `metadata.json` from each VM directory
3. **Filter by Project**: Skip VMs not matching current project (unless `listAllProjects`)
4. **Get Vagrant Status**: Execute `vagrant status --machine-readable`
5. **Parse SSH Config**: Execute `vagrant ssh-config` for connection details
6. **Build Instance Objects**: Create `backends.Instance` with all details
7. **Cache Results**: Store in `s.instances` for quick access

### Instance Operations

#### Start
- Execute `vagrant up` in VM directory
- Wait for VM to reach running state
- Invalidate cache

#### Stop
- Execute `vagrant halt [-f]` in VM directory
- Optionally force shutdown
- Invalidate cache

#### Terminate/Destroy
- Execute `vagrant destroy -f` in VM directory
- Remove VM directory entirely
- Clear from state cache
- Clean up SSH keys if last VM in project

#### Execute Commands (SSH)
- Read SSH configuration from Vagrant
- Use Aerolab's `sshexec` package for command execution
- Support for parallel execution across multiple VMs
- Environment variable injection (cluster name, node number, etc.)

### Tag Management

Tags are stored in `metadata.json` and persisted to disk:

- **Add Tags**: Merge new tags into existing metadata, write back to file
- **Remove Tags**: Delete specified keys from metadata, write back to file
- **Expiry Tags**: Special handling for `TAG_EXPIRES` with RFC3339 format

### SSH Key Management

1. **Key Generation**: RSA 2048-bit keys generated on first instance creation
2. **Key Storage**: Stored in `{sshKeysDir}/{project}` and `{project}.pub`
3. **Key Injection**: Public key added to Vagrantfile provisioning script
4. **Key Reuse**: Same key used for all VMs in a project
5. **Key Cleanup**: Deleted when last VM in project is destroyed

### Provider Support

The backend supports multiple Vagrant providers through configuration:

- **VirtualBox** (default): Most common, cross-platform
- **VMware Desktop**: Commercial provider, better performance
- **libvirt**: Linux KVM/QEMU integration
- **Hyper-V**: Windows native virtualization
- **Parallels**: macOS commercial provider

Provider selection via `credentials.VAGRANT.Provider` configuration.

## Configuration

### Credentials Structure

```yaml
vagrant:
  provider: "virtualbox"  # or vmware_desktop, libvirt, hyperv, parallels
  regions:
    default:
      workDir: "/path/to/vagrant/vms"
    dev:
      workDir: "/path/to/dev/vms"
    test:
      workDir: "/path/to/test/vms"
```

### CreateInstanceParams

```go
type CreateInstanceParams struct {
    Box               string            // e.g. "ubuntu/jammy64"
    BoxVersion        string            // optional, empty = latest
    CPUs              int               // CPU core count
    Memory            int               // Memory in MB
    DiskSize          int               // Disk size in GB (provider-dependent)
    NetworkType       string            // "private_network" or "public_network" (requires NetworkIP)
    NetworkIP         string            // Static IP for network (required if NetworkType is set)
    SyncedFolders     map[string]string // {hostPath}:{guestPath}
    PortForwards      map[int]int       // {guest}:{host}
    SkipSshReadyCheck bool              // Skip SSH connectivity check
}
```

## Caching Strategy

### Vagrant State Cache

```go
type vagrantStateCache struct {
    cache map[string]*vagrantVMState
    mu    sync.RWMutex
}
```

- **Purpose**: Reduce expensive `vagrant status` calls
- **Invalidation**: On any state-changing operation (start/stop/destroy)
- **Thread-Safe**: RWMutex for concurrent access

### Backend Cache Integration

The backend integrates with Aerolab's inventory cache system:

- **Cache Invalidation**: Calls `invalidateCacheFunc(backends.CacheInvalidateInstance)` after operations
- **Forced Refresh**: `GetInstances()` always scans disk, doesn't rely on memory cache
- **Cache Invalidation Types**:
  - `CacheInvalidateInstance`: After instance operations
  - `CacheInvalidateVolume`: After volume operations (minimal for Vagrant)

## Limitations and Not Implemented Features

### Not Applicable for Vagrant

These features don't make sense for local virtualization:

- **Expiry System**: Automated resource cleanup (less critical for local VMs)
- **VPC Peering**: Cloud networking concept
- **Route Management**: Cloud routing tables
- **DNS Management**: Cloud DNS services
- **Account ID**: Cloud account identifiers
- **Spot Instances**: Cloud cost optimization

### Limited Implementation

- **Volumes**: Vagrant uses synced folders instead of separate volume objects
- **Networks**: Provider-specific, defined in Vagrantfiles
- **Firewalls**: Managed by guest OS, not virtualization layer
- **Images**: Curated list, doesn't query Vagrant Cloud API dynamically
- **Pricing**: Always $0 (local resources)

### Why Not Implemented?

1. **Different Paradigm**: Vagrant is file-based, not API-based
2. **Provider Variations**: Features vary by provider (VirtualBox vs VMware vs libvirt)
3. **Local Focus**: Designed for development, not production infrastructure
4. **Complexity vs Benefit**: Some features would require significant effort with minimal value

## Design Decisions

### Why UUID-based Directories?

- **Uniqueness**: Prevents name collisions
- **Portability**: Works across all file systems
- **Predictability**: Easy to manage programmatically
- **Isolation**: Each VM is completely independent

### Why Metadata JSON Files?

- **Persistence**: Survives Vagrant destroy/up cycles
- **Extensibility**: Easy to add new fields without Vagrant limitations
- **Compatibility**: Standard JSON, human-readable and editable
- **Independence**: Not tied to Vagrant's internal metadata format

### Why CLI Instead of Vagrant Go Library?

- **Reliability**: Vagrant CLI is stable and well-tested
- **Compatibility**: Works with all Vagrant versions
- **Simplicity**: No complex library dependencies
- **Debugging**: Easy to reproduce issues with CLI commands

### Why One VM Per Directory?

- **Isolation**: Each VM has its own Vagrantfile and state
- **Flexibility**: Can customize per-VM configuration
- **Parallelism**: Operations on different VMs don't conflict
- **Cleanup**: Easy to remove VM by deleting directory

### Why Generate Vagrantfiles Programmatically?

- **Control**: Full control over configuration
- **Validation**: Ensure correctness at generation time
- **Templating**: Easy to add SSH keys and provisioning
- **Consistency**: All VMs follow same patterns

## Error Handling

### Command Execution Errors

```go
cmd := exec.Command("vagrant", "up")
output, err := cmd.CombinedOutput()
if err != nil {
    return fmt.Errorf("vagrant up failed: %w: %s", err, string(output))
}
```

- Include both error and command output for debugging
- Use `errors.Join()` for multiple errors
- Invalidate cache even on errors (state may have changed)

### File System Errors

- Check for `os.IsNotExist()` to differentiate missing files
- Use `os.RemoveAll()` for cleanup (handles missing paths gracefully)
- Create directories with `os.MkdirAll()` to avoid race conditions

### SSH Errors

- Retry SSH connections with exponential backoff
- Provide detailed error messages with hostname and port
- Fall back to Aerolab-managed keys if Vagrant keys fail

## Performance Considerations

### Parallel Operations

```go
parallelize.ForEachLimit(instances, threads, func(i *backends.Instance) {
    // Operation on instance
})
```

- Use Aerolab's `parallelize` package for concurrent operations
- Configurable thread count for user control
- Separate goroutines per VM for start/stop/destroy

### Expensive Operations

1. **`vagrant status`**: ~500ms-1s per VM
2. **`vagrant ssh-config`**: ~500ms per VM
3. **`vagrant up`**: 30s-5min depending on box and provider
4. **`vagrant destroy`**: 5s-30s per VM

Mitigation:
- Cache status results
- Batch operations where possible
- Run operations in parallel
- Skip SSH checks when not needed

## Testing Considerations

### Unit Testing

- Mock Vagrant CLI with test fixtures
- Test Vagrantfile generation without actual VMs
- Validate metadata JSON parsing/generation
- Test cache invalidation logic

### Integration Testing

- Requires Vagrant and provider installed
- Requires significant disk space and time
- Should test multiple providers if available
- Should test failure scenarios (out of memory, disk full)

### Manual Testing Checklist

- [ ] Create single VM
- [ ] Create cluster (3+ VMs)
- [ ] List instances
- [ ] Start/stop instances
- [ ] Destroy instances
- [ ] SSH into instances
- [ ] Execute commands on instances
- [ ] Add/remove tags
- [ ] Change expiry
- [ ] Multiple regions
- [ ] Different providers
- [ ] Different box images

## Future Enhancements

### Possible Improvements

1. **Dynamic Box Discovery**: Query Vagrant Cloud API for available boxes
2. **Local Box Cache**: List locally cached boxes with `vagrant box list`
3. **Snapshots**: Support `vagrant snapshot` for quick rollback
4. **Plugin Support**: Enable Vagrant plugins (vagrant-vbguest, etc.)
5. **Multi-Machine Vagrantfiles**: Single file managing multiple related VMs
6. **Resource Limits**: Validate CPU/memory against host capabilities
7. **Status Monitoring**: Background polling for VM state changes
8. **Better Network Management**: Parse provider-specific network info
9. **Volume Snapshots**: Backup/restore synced folders
10. **ARM64 Support**: Support for ARM-based boxes

### Potential Optimizations

1. **Batch Status Queries**: Single `vagrant global-status` for all VMs
2. **Incremental Updates**: Only refresh changed VMs
3. **Persistent Cache**: Disk-based cache for faster startup
4. **Connection Pooling**: Reuse SSH connections for multiple commands
5. **Async Operations**: Non-blocking VM creation with status updates

## Troubleshooting

### Common Issues

#### VMs Not Listed
- Check workDir exists and is readable
- Verify metadata.json exists and is valid JSON
- Check project name matches current context

#### SSH Connection Fails
- Verify VM is running (`vagrant status`)
- Check SSH port forwarding
- Verify SSH keys exist in sshKeysDir
- Try `vagrant ssh` directly for comparison

#### Vagrant Commands Hang
- Check provider daemon is running (VBoxManage, VMware, etc.)
- Verify no conflicting VMs with same name
- Check host resources (CPU, memory, disk)

#### Slow Performance
- Reduce parallel thread count
- Upgrade provider (e.g., VirtualBox 7.0+)
- Allocate more host resources
- Use SSD for VM storage

## Comparison with Other Backends

### vs Docker (bdocker)

**Similarities:**
- Local execution
- No cloud costs
- File-based state
- SSH access via port forwarding

**Differences:**
- Full VMs vs containers
- Slower startup (minutes vs seconds)
- More resource intensive
- Better OS isolation
- Support for systemd and full init systems

### vs AWS (baws) / GCP (bgcp)

**Similarities:**
- Instance lifecycle management
- SSH-based command execution
- Tagging and metadata
- Cluster management

**Differences:**
- No API, uses CLI
- No networking abstractions
- No managed services (EFS, EBS, RDS)
- No billing/cost tracking
- Limited to local resources

## Contributing

When contributing to the Vagrant backend:

1. **Follow Patterns**: Match existing backend patterns (bdocker, baws)
2. **Document Decisions**: Explain why something is not implemented
3. **Test Thoroughly**: Test with multiple providers if possible
4. **Handle Errors**: Provide detailed error messages
5. **Consider Performance**: Vagrant operations can be slow
6. **Update Documentation**: Keep this file current

## References

- [Vagrant Documentation](https://www.vagrantup.com/docs)
- [Vagrant Providers](https://www.vagrantup.com/docs/providers)
- [Vagrant Boxes](https://app.vagrantup.com/boxes/search)
- [Aerolab Backend Interface](../backends/backendInventories.go)

