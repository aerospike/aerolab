# Vagrant Backend Design Decisions

This document explains the key design decisions made during the implementation of the Vagrant backend for Aerolab, including rationale, trade-offs, and alternatives considered.

## Table of Contents

1. [Architecture Decisions](#architecture-decisions)
2. [State Management](#state-management)
3. [SSH and Authentication](#ssh-and-authentication)
4. [Caching Strategy](#caching-strategy)
5. [Feature Scope](#feature-scope)
6. [Error Handling](#error-handling)
7. [Performance Trade-offs](#performance-trade-offs)

## Architecture Decisions

### Decision: Use Vagrant CLI Instead of Go Library

**Rationale:**
- Vagrant's official Go library is not actively maintained
- CLI interface is stable, well-documented, and universally available
- Easier to debug (can reproduce issues with direct CLI commands)
- No complex library dependencies or version conflicts

**Trade-offs:**
- **Pro**: Reliability, compatibility, simplicity
- **Pro**: Works with any Vagrant version
- **Pro**: Easy to test and debug
- **Con**: Slower than native Go API calls
- **Con**: Requires parsing CLI output
- **Con**: Requires Vagrant to be installed

**Alternatives Considered:**
1. **go-vagrant library**: Unmaintained, incomplete API coverage
2. **vagrant-go**: Third-party, lacks official support
3. **Direct provider APIs**: Too complex, defeats purpose of Vagrant abstraction

### Decision: One VM Per Directory

**Rationale:**
- Vagrant is designed around the concept of one Vagrantfile per environment
- Simplifies state management and isolation
- Makes cleanup trivial (delete directory)
- Enables parallel operations without conflicts
- Easier to customize per-VM configuration

**Trade-offs:**
- **Pro**: Complete isolation between VMs
- **Pro**: No Vagrantfile conflicts
- **Pro**: Parallel-friendly architecture
- **Con**: More directories to manage
- **Con**: Can't use Vagrant multi-machine features
- **Con**: More disk space for metadata

**Alternatives Considered:**
1. **Single Vagrantfile with multi-machine**: Complex state management, hard to scale
2. **Per-cluster directories**: Less isolation, harder to manage individual VMs
3. **Global Vagrantfile**: No isolation, poor scalability

### Decision: UUID-based Directory Names

**Rationale:**
- Guarantees uniqueness across all VMs
- Prevents naming conflicts and collisions
- Works on all file systems (no special character issues)
- Makes programmatic management easier
- Enables predictable path construction

**Trade-offs:**
- **Pro**: Zero collision risk
- **Pro**: File system agnostic
- **Pro**: Easy to generate and validate
- **Con**: Not human-friendly for manual navigation
- **Con**: Requires metadata.json for meaningful names

**Alternatives Considered:**
1. **Cluster-Node naming**: Risk of collisions, special character issues
2. **Sequential numbers**: Race conditions, reuse problems
3. **Hash of parameters**: Complex, still opaque

## State Management

### Decision: External metadata.json Files

**Rationale:**
- Vagrant's internal metadata is provider-specific and complex
- JSON is universal, human-readable, and easy to parse
- Survives Vagrant internal state changes
- Can store arbitrary Aerolab-specific metadata
- Independent of Vagrant's .vagrant directory

**Trade-offs:**
- **Pro**: Full control over metadata format
- **Pro**: Survives Vagrant operations
- **Pro**: Easy to extend and modify
- **Con**: Potential for drift from actual VM state
- **Con**: Additional file to maintain

**Alternatives Considered:**
1. **Vagrant tags**: Provider-specific, limited support
2. **Extended Vagrantfile**: Not designed for dynamic metadata
3. **SQLite database**: Overkill, adds dependency
4. **YAML files**: Less standard for programmatic access

### Decision: Stateless GetInstances()

**Rationale:**
- Always scans disk for current state
- No risk of stale in-memory cache
- Handles external Vagrant operations (manual `vagrant up/destroy`)
- Simpler to reason about and debug

**Trade-offs:**
- **Pro**: Always accurate
- **Pro**: Detects external changes
- **Pro**: No cache invalidation bugs
- **Con**: Slower than memory-only approach
- **Con**: I/O intensive for large VM counts

**Alternatives Considered:**
1. **Pure memory cache**: Fast but stale, misses external changes
2. **Hybrid with TTL**: Complex, still has staleness window
3. **File watching**: OS-specific, complex, resource-intensive

## SSH and Authentication

### Decision: Single SSH Key Per Project

**Rationale:**
- Simplifies key management
- Enables seamless movement between VMs
- Consistent with other backends (Docker, AWS)
- Easier to configure external tools

**Trade-offs:**
- **Pro**: Simple and consistent
- **Pro**: Easy to share across team
- **Pro**: Works with all VMs in project
- **Con**: Single key compromise affects all VMs
- **Con**: No per-VM key rotation

**Alternatives Considered:**
1. **Per-VM keys**: Complex management, harder to use
2. **Per-cluster keys**: Middle ground, but still complex
3. **Vagrant's default keys**: Insecure for production use

### Decision: Key Injection via Vagrantfile Provisioning

**Rationale:**
- Works with all providers and boxes
- Reliable and simple mechanism
- Adds to both root and default user
- No dependency on box-specific features

**Trade-offs:**
- **Pro**: Universal compatibility
- **Pro**: Runs on every `vagrant up`
- **Pro**: Multiple user support
- **Con**: Re-runs on reprovisioning
- **Con**: Key visible in Vagrantfile

**Alternatives Considered:**
1. **Cloud-init**: Not supported by all boxes/providers
2. **Packer-baked images**: Too complex for this use case
3. **Manual post-creation**: Error-prone, not automated

## Caching Strategy

### Decision: Lightweight State Cache

**Rationale:**
- Reduce expensive `vagrant status` calls
- Only cache VM state, not full details
- Simple invalidation on operations
- Thread-safe concurrent access

**Trade-offs:**
- **Pro**: Significantly faster repeated status checks
- **Pro**: Simple invalidation logic
- **Pro**: Small memory footprint
- **Con**: Can be stale between operations
- **Con**: Doesn't persist across restarts

**Alternatives Considered:**
1. **No caching**: Too slow for large VM counts
2. **Full instance cache**: Complex, high staleness risk
3. **Persistent disk cache**: Complexity, staleness issues

### Decision: Invalidate on All State Changes

**Rationale:**
- Conservative approach prevents stale data
- Simple to implement and understand
- Better to be slow and correct than fast and wrong

**Trade-offs:**
- **Pro**: No stale cache bugs
- **Pro**: Simple logic
- **Pro**: Easy to debug
- **Con**: More cache misses
- **Con**: More `vagrant status` calls

## Feature Scope

### Decision: Minimal Volume Implementation

**Rationale:**
- Vagrant volumes are fundamentally different from cloud volumes
- Synced folders serve the same purpose
- Would require complex provider-specific code
- Limited practical value

**Trade-offs:**
- **Pro**: Simpler implementation
- **Pro**: Focused on core functionality
- **Pro**: Less maintenance burden
- **Con**: Less feature parity with cloud backends

**Alternatives Considered:**
1. **Full volume implementation**: Complex, provider-dependent
2. **Synced folder mapping**: Confusing terminology mismatch
3. **No volume support**: Current approach

### Decision: No Automated Expiry System

**Rationale:**
- Local VMs don't incur ongoing costs
- Less critical for development environments
- Users can manually manage VM lifecycle
- Reduces implementation complexity

**Trade-offs:**
- **Pro**: Simpler backend code
- **Pro**: Less background processing
- **Pro**: User has full control
- **Con**: Orphaned VMs consume disk
- **Con**: Less feature parity

**Alternatives Considered:**
1. **Simple cron-based expiry**: Possible future enhancement
2. **Manual expiry checking**: Current approach via tags
3. **No expiry support**: Current implementation

### Decision: Curated Box List

**Rationale:**
- Vagrant Cloud API is rate-limited
- Most users need only common distributions
- Easy to extend list as needed
- Fast and reliable

**Trade-offs:**
- **Pro**: No API dependencies
- **Pro**: Fast response
- **Pro**: Curated, tested images
- **Con**: May miss specific box versions
- **Con**: Requires manual updates

**Alternatives Considered:**
1. **Vagrant Cloud API**: Rate limits, complexity
2. **Local box cache parsing**: Implementation complexity
3. **User-provided box list**: Configuration burden

## Error Handling

### Decision: Include CLI Output in Errors

**Rationale:**
- Vagrant CLI provides detailed error messages
- Essential for debugging failures
- Helps users understand what went wrong

**Trade-offs:**
- **Pro**: Better debugging experience
- **Pro**: More informative errors
- **Pro**: Easier support
- **Con**: Potentially verbose error messages

### Decision: Continue on Partial Failures

**Rationale:**
- When operating on multiple VMs, don't fail fast
- Collect all errors and report together
- Enables partial success scenarios

**Trade-offs:**
- **Pro**: Better user experience for bulk operations
- **Pro**: Can succeed for some VMs even if others fail
- **Pro**: More information about what failed
- **Con**: More complex error handling
- **Con**: May leave system in inconsistent state

## Performance Trade-offs

### Decision: Parallel VM Operations

**Rationale:**
- Vagrant operations are I/O and CPU intensive
- Multiple VMs can be created/destroyed in parallel
- Significantly reduces total operation time
- User-configurable thread count

**Trade-offs:**
- **Pro**: Faster bulk operations (linear scaling)
- **Pro**: Better resource utilization
- **Pro**: User can control parallelism
- **Con**: Higher resource usage spikes
- **Con**: More complex error handling

### Decision: Optional SSH Ready Check

**Rationale:**
- SSH readiness check adds significant time
- Not always necessary (e.g., testing, provisioning)
- Let user decide based on use case

**Trade-offs:**
- **Pro**: Faster when not needed
- **Pro**: Flexibility for different use cases
- **Pro**: User control
- **Con**: May return "ready" VMs that aren't fully accessible

### Decision: No Background Status Polling

**Rationale:**
- Adds complexity without clear benefit
- `vagrant status` is expensive
- Users can explicitly refresh when needed
- Doesn't fit stateless architecture

**Trade-offs:**
- **Pro**: Simpler implementation
- **Pro**: Lower background resource usage
- **Pro**: Explicit refresh model
- **Con**: Status may be stale
- **Con**: No real-time updates

## Security Decisions

### Decision: SSH Keys in Plain Text

**Rationale:**
- Consistent with SSH key standard practices
- File system permissions provide security
- Required for Vagrant integration
- Users control key storage location

**Trade-offs:**
- **Pro**: Standard SSH practice
- **Pro**: Works with existing tools
- **Pro**: No additional key management
- **Con**: Keys stored unencrypted
- **Con**: Relies on file system security

**Mitigation:**
- Keys stored with 0600 permissions
- Directory permissions set to 0700
- User can configure custom key paths

### Decision: Root User SSH Access

**Rationale:**
- Required for Aerolab operations (install software, modify config)
- Consistent with other backends
- Common in development environments

**Trade-offs:**
- **Pro**: Full system access for operations
- **Pro**: Consistent with cloud backends
- **Pro**: Simplifies provisioning
- **Con**: Security risk if key compromised
- **Con**: Less aligned with production practices

## Future Considerations

### Potential Changes

These decisions may be revisited based on user feedback:

1. **API Integration**: If a mature Vagrant Go library emerges
2. **Multi-machine Vagrantfiles**: If isolation requirements change
3. **Dynamic Box Discovery**: If Vagrant Cloud API becomes more accessible
4. **Persistent Cache**: If performance becomes critical
5. **Background Monitoring**: If real-time status becomes important

### Extension Points

The architecture supports these future enhancements without major refactoring:

1. **Plugin Support**: Vagrantfile generation can include plugin config
2. **Custom Provisioners**: Shell provisioning can be extended
3. **Snapshot Management**: Can leverage `vagrant snapshot` commands
4. **Multiple Providers**: Already supports provider configuration
5. **Resource Validation**: Can add pre-flight checks for CPU/memory

## Conclusion

These design decisions prioritize:
- **Reliability** over performance where necessary
- **Simplicity** over feature completeness
- **Compatibility** over optimization
- **Debuggability** over abstraction

The Vagrant backend aims to provide a solid local development experience while maintaining consistency with Aerolab's cloud backends. Trade-offs favor developer experience and maintainability over absolute feature parity with cloud providers.

