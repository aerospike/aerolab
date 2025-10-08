# Expiry Package

The Expiry package provides automated resource expiration and cleanup functionality for cloud resources managed by Aerolab. It supports multiple deployment environments including AWS Lambda, Google Cloud Functions, and standalone server execution.

## Key Features

- **Multi-Cloud Support** - Works with AWS, GCP, and other cloud providers
- **Multiple Deployment Options** - AWS Lambda, GCP Cloud Functions, Cloud Run, or standalone server
- **Resource Cleanup** - Automated cleanup of expired instances, volumes, and EKS clusters
- **DNS Cleanup** - Stale DNS record cleanup
- **EKS Integration** - Comprehensive EKS cluster and associated resource cleanup
- **Concurrent Processing** - Thread-safe operations with proper locking

## Main Components

### ExpiryHandler
The main handler that orchestrates the expiration process:

```go
type ExpiryHandler struct {
    Backend      backends.Backend
    ExpireEksctl bool
    CleanupDNS   bool
    lock         sync.Mutex
    Credentials  *clouds.Credentials
}
```

### Main Functions
- `Expire() error` - Main expiration process that handles all resource cleanup
- `expireEksctl(region string) error` - EKS-specific cleanup for a given region

## Deployment Modes

### AWS Lambda
Triggered by CloudWatch events or manual invocation:
- Environment variable: `AWS_LAMBDA_FUNCTION_NAME` (not empty)
- Supports region-specific operations
- Integrates with AWS services (EC2, EKS, IAM, CloudFormation)

### Google Cloud Functions
HTTP-triggered function for GCP resources:
- Environment variable: `FUNCTION_TARGET` (not empty)
- Supports GCP project-specific operations
- Integrates with GCP Compute Engine

### Google Cloud Run
Container-based execution for scalable operations:
- Environment variable: `K_SERVICE` (not empty)
- Long-running container support
- HTTP endpoint for triggering

### Standalone Server
Direct execution for development or custom deployments:
- No special environment variables required
- Command-line interface support
- Local configuration file support

## Environment Variables

### Common Variables
- `AEROLAB_LOG_LEVEL` - Log level (0-6)
- `AEROLAB_VERSION` - Aerolab version string
- `AEROLAB_CLEANUP_DNS` - Enable DNS cleanup ("true"/"false")

### AWS-Specific Variables
- `AWS_REGION` - AWS region for operations
- `AEROLAB_EXPIRE_EKSCTL` - Enable EKS cluster expiration ("true"/"false")

### GCP-Specific Variables
- `GCP_REGION` - GCP region for operations
- `GCP_PROJECT` - GCP project ID

## Usage Examples

### AWS Lambda Deployment

```go
// Lambda function handler
func main() {
    h := &expire.ExpiryHandler{
        ExpireEksctl: os.Getenv("AEROLAB_EXPIRE_EKSCTL") == "true",
        CleanupDNS:   os.Getenv("AEROLAB_CLEANUP_DNS") == "true",
    }
    
    // Initialize backend with AWS support
    backend, err := backend.New("expiry", &backend.Config{
        RootDir:         "/tmp/aerolab",
        LogLevel:        logger.LogLevel(4),
        AerolabVersion:  os.Getenv("AEROLAB_VERSION"),
        ListAllProjects: true,
    }, false, []backends.BackendType{backends.BackendTypeAWS}, nil)
    
    if err != nil {
        log.Fatal(err)
    }
    
    h.Backend = backend
    lambda.Start(func(ctx context.Context, event interface{}) (interface{}, error) {
        return event, h.Expire()
    })
}
```

### Standalone Server

```go
func main() {
    h := &expire.ExpiryHandler{
        ExpireEksctl: true,
        CleanupDNS:   true,
    }
    
    // Initialize backend
    backend, err := backend.New("expiry-server", config, true, 
        []backends.BackendType{backends.BackendTypeAWS, backends.BackendTypeGCP}, nil)
    if err != nil {
        log.Fatal(err)
    }
    
    h.Backend = backend
    
    // Run expiration process
    err = h.Expire()
    if err != nil {
        log.Fatal(err)
    }
}
```

### GCP Cloud Function

```go
func ExpireResources(w http.ResponseWriter, r *http.Request) {
    h := &expire.ExpiryHandler{
        CleanupDNS: os.Getenv("AEROLAB_CLEANUP_DNS") == "true",
    }
    
    // Initialize GCP backend
    backend, err := backend.New("gcp-expiry", config, false, 
        []backends.BackendType{backends.BackendTypeGCP}, nil)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    h.Backend = backend
    err = h.Expire()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Expiry completed successfully"))
}
```

## Expiration Process

The expiration process follows these steps:

1. **Lock Acquisition** - Ensures only one expiration process runs at a time
2. **Inventory Refresh** - Forces refresh of cloud resource inventory
3. **Instance Cleanup** - Terminates expired instances
4. **Volume Cleanup** - Deletes expired volumes not set to delete-on-termination
5. **DNS Cleanup** - Removes stale DNS records (if enabled)
6. **EKS Cleanup** - Comprehensive EKS cluster cleanup (if enabled)

### EKS Cleanup Process

For EKS clusters, the cleanup process includes:

1. **Cluster Discovery** - List and evaluate EKS clusters for expiration
2. **Expiration Check** - Check `ExpireAt` tag or calculate from `initialExpiry`
3. **CloudFormation Cleanup** - Delete eksctl-created CloudFormation stacks
4. **EBS Volume Cleanup** - Delete persistent volumes created by EBS CSI driver
5. **IAM Cleanup** - Remove OIDC identity providers
6. **Stack Deletion** - Wait for complete stack deletion

## Resource Identification

Resources are identified for expiration based on:

### Instances
- Expiration timestamp in metadata
- Current state (running, stopped, failed, etc.)
- Cluster association and tags

### Volumes
- Expiration timestamp
- Delete-on-termination setting (false = eligible for cleanup)
- Attachment status

### EKS Clusters
- `ExpireAt` tag with Unix timestamp
- `initialExpiry` tag with duration from creation time
- CloudFormation stack naming patterns (`eksctl-{cluster-name}-*`)

## Error Handling

The package provides comprehensive error handling:

- **Connection Errors** - Retry logic for transient failures
- **Permission Errors** - Clear error messages for access issues
- **Resource Conflicts** - Handling of resources in transition states
- **Timeout Handling** - Configurable timeouts for long operations

## Logging

Detailed logging throughout the expiration process:

- **Resource Discovery** - Log found resources and their expiration status
- **Cleanup Operations** - Log each cleanup action with resource details
- **Error Reporting** - Detailed error context and troubleshooting information
- **Performance Metrics** - Timing information for optimization

## Security Considerations

- **Credential Management** - Secure handling of cloud provider credentials
- **Resource Validation** - Verify resource ownership before deletion
- **Audit Logging** - Comprehensive logging for compliance and troubleshooting
- **Access Control** - Proper IAM permissions for cleanup operations

## Monitoring and Alerting

The package supports monitoring through:

- **CloudWatch Integration** - AWS Lambda metrics and logs
- **GCP Monitoring** - Cloud Functions and Cloud Run metrics
- **Custom Metrics** - Resource cleanup counts and timing
- **Error Alerting** - Failed cleanup notifications

## Thread Safety

All operations are designed to be thread-safe:

- **Mutex Protection** - Prevents concurrent expiration processes
- **Atomic Operations** - Safe resource state transitions
- **Connection Pooling** - Thread-safe cloud provider client management
