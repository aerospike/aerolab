# Backend Package

The Backend package provides a unified interface for managing cloud infrastructure across multiple cloud providers (AWS, GCP, Docker). It abstracts cloud-specific operations and provides a consistent API for cluster management, instance provisioning, and resource management.

## Overview

This package serves as the main entry point for cloud backend operations in Aerolab. It initializes and manages different cloud backends through a plugin-like architecture where each cloud provider is implemented as a separate backend.

## Supported Backends

- **AWS (baws)** - Amazon Web Services backend
- **GCP (bgcp)** - Google Cloud Platform backend  
- **Docker (bdocker)** - Local Docker backend for development/testing

## Key Components

### Main Functions

- `New(project string, c *Config, pollInventoryHourly bool, enabledBackends []backends.BackendType, setInventory *backends.Inventory) (backends.Backend, error)` - Creates a new backend instance with specified configuration and enabled cloud providers

### Configuration

The `Config` type (alias for `backends.Config`) contains all necessary configuration for backend initialization including:
- Root directory for backend data
- Caching settings
- Log level configuration
- Aerolab version information
- Credentials for cloud providers

## Subpackages

### backends
Core backend interface definitions and common functionality shared across all cloud providers.

### clouds
Cloud-specific implementations:
- **baws** - AWS implementation with EC2, EKS, IAM, Route53 integration
- **bgcp** - GCP implementation with Compute Engine integration
- **bdocker** - Docker implementation for local development

### cache
Caching layer for backend operations to improve performance and reduce API calls.

## Usage

### Basic Backend Initialization

```go
import (
    "github.com/aerospike/aerolab/pkg/backend"
    "github.com/aerospike/aerolab/pkg/backend/backends"
)

config := &backend.Config{
    RootDir:         "/path/to/aerolab",
    Cache:           true,
    LogLevel:        4,
    AerolabVersion:  "8.0.0",
    ListAllProjects: false,
}

// Initialize backend with AWS and GCP support
backend, err := backend.New("my-project", config, true, 
    []backends.BackendType{backends.BackendTypeAWS, backends.BackendTypeGCP}, nil)
if err != nil {
    log.Fatal(err)
}
```

### Multi-Cloud Operations

The backend package allows you to work with multiple cloud providers simultaneously:

```go
// Add regions for different providers
err = backend.AddRegion(backends.BackendTypeAWS, "us-west-2")
err = backend.AddRegion(backends.BackendTypeGCP, "us-central1-a")

// Operations work across all configured backends
inventory := backend.GetInventory()
instances := inventory.Instances.Describe()
```

## Architecture

The backend package uses a plugin architecture where:

1. Each cloud provider implements the `backends.Cloud` interface
2. Backends are registered during package initialization
3. The main backend multiplexes operations across enabled backends
4. Common operations (caching, inventory management) are handled centrally

This design allows for easy extension to new cloud providers while maintaining a consistent API for users.

## Error Handling

The backend package provides comprehensive error handling with context about which cloud provider and operation failed. Errors are wrapped to provide clear debugging information.

## Thread Safety

All backend operations are designed to be thread-safe, with appropriate locking mechanisms for shared resources like inventory caches and configuration data.
