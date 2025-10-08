# Conf Package

The Conf package provides configuration management utilities for Aerospike database configurations. It includes specialized editors for different versions of Aerospike configuration files, enabling programmatic modification and validation of Aerospike server configurations.

## Key Features

- **Multi-Version Support** - Separate editors for different Aerospike versions
- **Configuration Parsing** - Parse and validate Aerospike configuration files
- **Programmatic Editing** - Modify configuration parameters through code
- **Version-Specific Logic** - Handle version-specific configuration differences
- **Validation** - Ensure configuration correctness and compatibility

## Subpackages

### aerospike
Contains Aerospike-specific configuration editors and utilities.

#### confeditor
Configuration editor for standard Aerospike versions:
- Configuration file parsing and modification
- Parameter validation and type checking
- Configuration generation and formatting

#### confeditor7
Specialized configuration editor for Aerospike 7.x versions:
- Version 7-specific configuration options
- New parameter support and validation
- Backward compatibility handling

## Architecture

The package uses a modular architecture where:

1. **Version Detection** - Automatically detect Aerospike version
2. **Editor Selection** - Choose appropriate editor based on version
3. **Configuration Parsing** - Parse existing configuration files
4. **Modification** - Apply programmatic changes
5. **Validation** - Ensure configuration correctness
6. **Output Generation** - Generate valid configuration files

## Usage Examples

### Basic Configuration Editing

```go
import "github.com/aerospike/aerolab/pkg/conf/aerospike/confeditor"

// Load existing configuration
config, err := confeditor.LoadConfig("/etc/aerospike/aerospike.conf")
if err != nil {
    log.Fatal(err)
}

// Modify configuration parameters
config.SetServicePort(3000)
config.SetHeartbeatPort(3002)
config.SetFabricPort(3001)

// Save modified configuration
err = config.Save("/etc/aerospike/aerospike.conf")
if err != nil {
    log.Fatal(err)
}
```

### Version 7 Specific Configuration

```go
import "github.com/aerospike/aerolab/pkg/conf/aerospike/confeditor7"

// Create new configuration for Aerospike 7.x
config := confeditor7.NewConfig()

// Set version 7 specific parameters
config.SetSecurityEnabled(true)
config.SetTLSConfiguration(tlsConfig)
config.SetNamespaceConfiguration("test", namespaceConfig)

// Generate configuration file
configData, err := config.Generate()
if err != nil {
    log.Fatal(err)
}

// Write to file
err = ioutil.WriteFile("/etc/aerospike/aerospike.conf", configData, 0644)
if err != nil {
    log.Fatal(err)
}
```

### Configuration Validation

```go
// Validate configuration before applying
errors := config.Validate()
if len(errors) > 0 {
    for _, err := range errors {
        log.Printf("Configuration error: %v", err)
    }
    return
}

// Apply validated configuration
err = config.Apply()
if err != nil {
    log.Fatal(err)
}
```

### Namespace Configuration

```go
// Configure namespace settings
namespace := &confeditor.NamespaceConfig{
    Name:               "test",
    ReplicationFactor:  2,
    MemorySize:         "1G",
    DefaultTTL:         "30d",
    StorageEngine:      "memory",
}

config.AddNamespace(namespace)

// Configure storage for namespace
storage := &confeditor.StorageConfig{
    Type:       "device",
    Devices:    []string{"/dev/sdb", "/dev/sdc"},
    FileSize:   "4G",
    DataInMemory: true,
}

namespace.SetStorage(storage)
```

### Network Configuration

```go
// Configure network settings
network := &confeditor.NetworkConfig{
    ServicePort:    3000,
    HeartbeatPort:  3002,
    FabricPort:     3001,
    InfoPort:       3003,
    AccessAddress:  "192.168.1.100",
    AlternateAccessAddress: "10.0.1.100",
}

config.SetNetwork(network)

// Configure TLS
tls := &confeditor.TLSConfig{
    Enabled:     true,
    CertFile:    "/etc/aerospike/certs/server.crt",
    KeyFile:     "/etc/aerospike/certs/server.key",
    CAFile:      "/etc/aerospike/certs/ca.crt",
    Protocols:   []string{"TLSv1.2", "TLSv1.3"},
}

config.SetTLS(tls)
```

## Configuration Sections

The package supports all major Aerospike configuration sections:

### Service Section
- Node ID and cluster configuration
- Process and thread settings
- Memory and CPU allocation
- Logging configuration

### Network Section
- Service, heartbeat, fabric, and info ports
- Access addresses and interfaces
- TLS/SSL configuration
- Network security settings

### Namespace Section
- Data storage configuration
- Replication settings
- Memory and disk allocation
- Index and data policies

### Security Section
- Authentication and authorization
- User and role management
- Access control policies
- Audit logging

### Logging Section
- Log file configuration
- Log levels and categories
- Rotation and retention policies
- Remote logging setup

## Version Compatibility

### Standard Editor (confeditor)
- Supports Aerospike versions 4.x, 5.x, 6.x
- Handles common configuration parameters
- Provides backward compatibility

### Version 7 Editor (confeditor7)
- Specialized for Aerospike 7.x features
- New security and performance options
- Enhanced namespace configuration
- Modern TLS and encryption support

## Error Handling

The package provides comprehensive error handling:

- **Parse Errors** - Detailed syntax error reporting
- **Validation Errors** - Parameter value validation
- **Version Conflicts** - Version-specific parameter warnings
- **File I/O Errors** - Configuration file access issues

## Integration

The Conf package integrates with:

- **Aerolab Cluster Management** - Automatic configuration generation
- **Installation Scripts** - Configuration template processing
- **Validation Tools** - Pre-deployment configuration checking
- **Backup Systems** - Configuration versioning and rollback

## Best Practices

### Configuration Management
1. **Version Detection** - Always detect Aerospike version before editing
2. **Validation** - Validate configurations before deployment
3. **Backup** - Keep backups of original configurations
4. **Testing** - Test configurations in development environments

### Security Considerations
1. **File Permissions** - Ensure proper file permissions for configuration files
2. **Credential Management** - Secure handling of authentication credentials
3. **TLS Configuration** - Proper certificate and key management
4. **Access Control** - Restrict configuration file access

### Performance Optimization
1. **Memory Settings** - Optimize memory allocation based on workload
2. **Storage Configuration** - Configure storage engines appropriately
3. **Network Tuning** - Optimize network settings for cluster communication
4. **Index Configuration** - Proper index sizing and configuration

## Extensibility

The package is designed for extensibility:

- **Custom Validators** - Add custom validation rules
- **Plugin Architecture** - Support for custom configuration processors
- **Template System** - Configuration template support
- **Hook System** - Pre/post processing hooks for configuration changes
