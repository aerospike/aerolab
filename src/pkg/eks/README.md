# EKS Package

The EKS package provides Amazon Elastic Kubernetes Service (EKS) integration and management utilities for Aerolab. It includes automated expiry handling for EKS resources and cluster lifecycle management.

## Key Features

- **EKS Cluster Management** - Lifecycle management for EKS clusters
- **Automated Expiry** - Scheduled cleanup of expired EKS resources
- **Resource Cleanup** - Comprehensive cleanup of associated AWS resources
- **Integration with Aerolab** - Seamless integration with Aerolab's backend system
- **Template Support** - EKS cluster templates for common configurations

## Subpackages

### eksexpiry
Handles automated expiration and cleanup of EKS clusters and associated resources.

**Key Features:**
- Scheduled expiry processing
- CloudFormation stack cleanup
- EBS volume cleanup
- IAM resource cleanup
- OIDC provider cleanup

**Main Functions:**
- `Expiry()` - Main expiry processing function

### ekctl-templates
Contains YAML templates for various EKS cluster configurations using eksctl.

**Available Templates:**
- `auto-scaler.yaml` - Cluster with auto-scaling configuration
- `basic.yaml` - Basic EKS cluster setup
- `full-example.yaml` - Comprehensive cluster configuration example
- `load-balancer.yaml` - Cluster with load balancer configuration
- `two-node-groups.yaml` - Cluster with multiple node groups

## EKS Expiry Process

The EKS expiry system provides comprehensive cleanup of EKS clusters and all associated AWS resources:

### 1. Cluster Discovery
- Lists all EKS clusters in configured regions
- Checks expiration tags and timestamps
- Evaluates cluster lifecycle state

### 2. Expiration Evaluation
The system checks for expiration using two methods:

#### Direct Expiration Tag
```yaml
Tags:
  ExpireAt: "1640995200"  # Unix timestamp
```

#### Initial Expiry Duration
```yaml
Tags:
  initialExpiry: "24h"    # Duration from creation time
```

### 3. Resource Cleanup Sequence

When a cluster is marked for expiry, the cleanup process follows this order:

1. **CloudFormation Stack Discovery**
   - Lists all stacks matching pattern: `eksctl-{cluster-name}-*`
   - Excludes already deleted stacks
   - Sorts by creation time (reverse order for proper deletion sequence)

2. **EBS Volume Cleanup** (for cluster stack)
   - Identifies volumes with tags:
     - `kubernetes.io/cluster/{cluster-name}=owned`
     - `KubernetesCluster={cluster-name}`
   - Deletes persistent volumes created by EBS CSI driver

3. **IAM OIDC Provider Cleanup**
   - Lists OpenID Connect providers
   - Identifies providers with tag: `alpha.eksctl.io/cluster-name={cluster-name}`
   - Deletes associated identity providers

4. **CloudFormation Stack Deletion**
   - Deletes stacks in proper dependency order
   - Waits for complete deletion before proceeding
   - Handles deletion timeouts and retries

## Usage Examples

### Manual EKS Expiry

```go
import "github.com/aerospike/aerolab/pkg/eks/eksexpiry"

// Trigger manual expiry process
eksexpiry.Expiry()
```

### EKS Cluster Template Usage

```bash
# Using eksctl with Aerolab templates
eksctl create cluster -f /path/to/aerolab/src/pkg/eks/ekctl-templates/basic.yaml

# Using auto-scaler template
eksctl create cluster -f /path/to/aerolab/src/pkg/eks/ekctl-templates/auto-scaler.yaml
```

### Integration with Expiry Handler

```go
import (
    "github.com/aerospike/aerolab/pkg/expiry/expire"
    "github.com/aerospike/aerolab/pkg/backend/backends"
)

// Create expiry handler with EKS support
handler := &expire.ExpiryHandler{
    ExpireEksctl: true,  // Enable EKS expiry
    CleanupDNS:   true,
    Backend:      backend,
}

// Run comprehensive expiry including EKS
err := handler.Expire()
if err != nil {
    log.Fatal(err)
}
```

## EKS Template Configurations

### Basic Template
Minimal EKS cluster configuration:
```yaml
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig

metadata:
  name: basic-cluster
  region: us-west-2

nodeGroups:
  - name: ng-1
    instanceType: t3.medium
    desiredCapacity: 2
    minSize: 1
    maxSize: 4
```

### Auto-Scaler Template
Cluster with Kubernetes auto-scaler:
```yaml
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig

metadata:
  name: auto-scaler-cluster
  region: us-west-2

nodeGroups:
  - name: ng-1
    instanceType: t3.medium
    desiredCapacity: 2
    minSize: 1
    maxSize: 10
    tags:
      k8s.io/cluster-autoscaler/enabled: "true"
      k8s.io/cluster-autoscaler/auto-scaler-cluster: "owned"

addons:
  - name: cluster-autoscaler
```

### Load Balancer Template
Cluster with AWS Load Balancer Controller:
```yaml
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig

metadata:
  name: lb-cluster
  region: us-west-2

iam:
  withOIDC: true

addons:
  - name: aws-load-balancer-controller
    serviceAccountRoleARN: arn:aws:iam::ACCOUNT:role/AmazonEKSLoadBalancerControllerRole

nodeGroups:
  - name: ng-1
    instanceType: t3.medium
    desiredCapacity: 2
```

## Resource Tagging Strategy

The EKS package uses a comprehensive tagging strategy for resource management:

### Cluster Tags
- `ExpireAt` - Unix timestamp for absolute expiration
- `initialExpiry` - Duration string for relative expiration
- `CreatedBy` - User or system that created the cluster
- `Project` - Associated Aerolab project name

### Node Group Tags
- `k8s.io/cluster-autoscaler/enabled` - Enable auto-scaling
- `k8s.io/cluster-autoscaler/{cluster-name}` - Cluster association
- `NodeGroup` - Node group identifier

### Volume Tags
- `kubernetes.io/cluster/{cluster-name}` - Cluster ownership
- `KubernetesCluster` - Cluster name for CSI volumes
- `kubernetes.io/created-for/pv/name` - Persistent volume association

## Error Handling

The EKS package provides robust error handling:

### Cluster Operations
- **API Rate Limiting** - Automatic retry with exponential backoff
- **Permission Errors** - Clear error messages for IAM issues
- **Resource Conflicts** - Handling of resources in transition states

### Cleanup Operations
- **Dependency Errors** - Proper handling of resource dependencies
- **Timeout Handling** - Configurable timeouts for long operations
- **Partial Failures** - Continue cleanup even if some resources fail

### CloudFormation Integration
- **Stack State Validation** - Verify stack states before operations
- **Waiter Integration** - Use AWS waiters for operation completion
- **Error Propagation** - Detailed error context from CloudFormation

## Monitoring and Logging

### Expiry Logging
- **Resource Discovery** - Log all discovered clusters and their expiry status
- **Cleanup Progress** - Detailed logging of each cleanup step
- **Error Reporting** - Comprehensive error logging with context

### Metrics Integration
- **CloudWatch Metrics** - Custom metrics for expiry operations
- **Performance Tracking** - Timing metrics for optimization
- **Success/Failure Rates** - Operational health monitoring

## Security Considerations

### IAM Permissions
Required IAM permissions for EKS expiry operations:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "eks:ListClusters",
        "eks:DescribeCluster",
        "cloudformation:ListStacks",
        "cloudformation:DeleteStack",
        "cloudformation:DescribeStacks",
        "ec2:DescribeVolumes",
        "ec2:DeleteVolume",
        "iam:ListOpenIDConnectProviders",
        "iam:ListOpenIDConnectProviderTags",
        "iam:DeleteOpenIDConnectProvider"
      ],
      "Resource": "*"
    }
  ]
}
```

### Resource Validation
- **Ownership Verification** - Verify resource ownership before deletion
- **Tag Validation** - Validate expiry tags before processing
- **Region Restrictions** - Limit operations to configured regions

## Integration Points

### Aerolab Backend
- **Inventory Integration** - EKS clusters appear in Aerolab inventory
- **Credential Management** - Uses Aerolab's credential system
- **Region Management** - Respects Aerolab's region configuration

### AWS Services
- **EKS API** - Direct integration with EKS service
- **CloudFormation** - Stack management and cleanup
- **EC2** - Volume and instance management
- **IAM** - Identity provider cleanup

### Monitoring Systems
- **CloudWatch** - Metrics and logging integration
- **AWS Config** - Resource compliance monitoring
- **Cost Management** - Resource cost tracking and optimization
