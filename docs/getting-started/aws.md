# Getting Started with AWS Backend

The AWS backend allows you to create and manage Aerospike clusters on Amazon Web Services (AWS) EC2 instances.

## Prerequisites

### AWS Account Setup

1. **AWS Account** - You need an active AWS account
2. **AWS CLI** - Install the [AWS CLI](https://aws.amazon.com/cli/)
3. **AWS Credentials** - Configure your AWS credentials

### Configure AWS Credentials

Aerolab uses standard AWS credential mechanisms. Set up your credentials using one of these methods:

#### Method 1: AWS CLI Configuration (Recommended)

```bash
aws configure
```

This will prompt for:
- AWS Access Key ID
- AWS Secret Access Key
- Default region (e.g., `us-east-1`)
- Default output format (can be `json`)

Credentials are stored in `~/.aws/credentials` and configuration in `~/.aws/config`.

#### Method 2: Environment Variables

```bash
export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
export AWS_DEFAULT_REGION=us-east-1
```

#### Method 3: AWS Profile

If you have multiple AWS accounts, use profiles:

```bash
aws configure --profile myprofile
```

Then specify the profile when configuring Aerolab:

```bash
aerolab config backend -t aws -r us-east-1 -P myprofile
```

### Required IAM Permissions

Your AWS credentials need permissions for:

- EC2 (instances, images, volumes, security groups, VPCs)
- IAM (for instance profiles if used)
- Route53 (for DNS management if used)
- EKS (if using EKS cluster name)

A minimal IAM policy would include:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:*",
        "iam:GetRole",
        "iam:PassRole"
      ],
      "Resource": "*"
    }
  ]
}
```

## Initial Setup

### 1. Configure AWS Backend

Configure Aerolab to use the AWS backend:

```bash
aerolab config backend -t aws -r us-east-1
```

This will:
- Set AWS as the default backend
- Set the default region to `us-east-1`
- Use your AWS credentials from `~/.aws/credentials`

### Using a Specific AWS Profile

If you have multiple AWS accounts:

```bash
aerolab config backend -t aws -r us-east-1 -P myprofile
```

### Optional: Enable Inventory Cache

If you're not sharing the AWS account with other users, enable inventory caching for faster operations:

```bash
aerolab config backend -t aws -r us-east-1 --inventory-cache
```

**Note**: Only use inventory cache if you're the sole user of the AWS account/project, as it caches resource state locally.

### Optional: Disable Public IPs

If you want to operate only on private IPs:

```bash
aerolab config backend -t aws -r us-east-1 --aws-nopublic-ip
```

### Optional: Set EKS Cluster Name

If you want to use a specific EKS cluster name:

```bash
aerolab config backend -t aws -r us-east-1 -P eks
```

### 2. Verify Configuration

Check your backend configuration:

```bash
aerolab config backend
```

You should see:
```
Config.Backend.Type = aws
Config.Backend.AWSProfile = 
Config.Backend.Region = us-east-1
Config.Backend.AWSNoPublicIps = false
Config.Backend.SshKeyPath = ${HOME}/.config/aerolab
```

### 3. Check Access

Verify you have access to AWS:

```bash
aerolab config backend -t aws --check-access
```

### 4. Clean Up Existing Resources (Optional)

If you have existing Aerolab resources, clean them up:

```bash
aerolab inventory delete-project-resources -f
```

Or with expiry:

```bash
aerolab inventory delete-project-resources -f --expiry
```

## AWS-Specific Configuration

### List Subnets

View available subnets:

```bash
aerolab config aws list-subnets
```

### List Security Groups

View existing security groups:

```bash
aerolab config aws list-security-groups
```

### Create Security Groups

Create a security group for Aerospike:

```bash
aerolab config aws create-security-groups -n aerolab-sg -p 3000-3005
```

This creates a security group allowing ports 3000-3005.

### Lock Security Groups

Lock a security group to prevent deletion:

```bash
aerolab config aws lock-security-groups -n aerolab-sg
```

### Delete Security Groups

Delete a security group:

```bash
aerolab config aws delete-security-groups -n aerolab-sg
```

## Creating Your First Cluster

### Basic Cluster Creation

Create a simple 2-node cluster:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

This command:
- Creates 2 nodes (`-c 2`)
- Uses Ubuntu 24.04 (`-d ubuntu -i 24.04`)
- Installs Aerospike version 8.x (`-v '8.*'`)
- Uses `t3a.xlarge` instance type (`-I t3a.xlarge`)
- Creates 20GB GP2 root disk (`--aws-disk type=gp2,size=20`)
- Sets expiry to 8 hours (`--aws-expire=8h`)

### Multiple Disks

Add multiple disks:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-disk type=gp2,size=20 \
  --aws-disk type=gp3,size=100,count=3 \
  --aws-expire=8h
```

This creates:
- One 20GB GP2 root disk
- Three 100GB GP3 data disks

### Custom Subnet

Specify a subnet:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  -U subnet-12345678 \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

### Custom Security Groups

Add custom security groups:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --secgroup-name aerolab-sg \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

### Public IP

Enable public IP access:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  -L \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

### Spot Instances

Use spot instances for cost savings:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-spot-instance \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

### Custom Tags

Add custom tags:

```bash
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --tags Environment=Development \
  --tags Team=Platform \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

## Resource Expiry Management

### Install Expiry Automation

Install automated resource expiry:

```bash
aerolab config aws expiry-install
```

This installs a Lambda function that automatically cleans up expired resources.

### List Expiry Rules

View expiry rules:

```bash
aerolab config aws expiry-list
```

### Set Expiry Run Frequency

Configure how often expiry runs:

```bash
aerolab config aws expiry-run-frequency -f 20
```

This sets expiry to run every 20 minutes.

### Remove Expiry

Remove expiry automation:

```bash
aerolab config aws expiry-remove
```

## Starting and Stopping Clusters

### Start Cluster

Start all nodes in the cluster:

```bash
aerolab cluster start
```

### Stop Cluster

Stop all nodes:

```bash
aerolab cluster stop
```

**Note**: Stopping instances in AWS doesn't delete them, but you'll still be charged for EBS volumes. Use expiry or destroy to completely remove resources.

## Managing Aerospike Service

### Start Aerospike

```bash
aerolab aerospike start
```

### Stop Aerospike

```bash
aerolab aerospike stop
```

### Restart Aerospike

```bash
aerolab aerospike restart
```

### Check Status

```bash
aerolab aerospike status
```

### Wait for Cluster Stability

```bash
aerolab aerospike is-stable -w
```

## Connecting to Nodes

### Shell Access

```bash
aerolab attach shell -n mydc -l 1
```

### Aerospike Tools

```bash
# AQL
aerolab attach aql -n mydc -- -c "show namespaces"

# asinfo
aerolab attach asinfo -n mydc -- -v "cluster-stable"

# asadm
aerolab attach asadm -n mydc -- -e info
```

## File Operations

### Upload Files

```bash
aerolab files upload -n mydc local-file.txt /tmp/remote-file.txt
```

### Download Files

```bash
aerolab files download -n mydc /tmp/remote-file.txt ./local-dir/
```

### Sync Files

```bash
aerolab files sync -n mydc -l 1 /tmp/file.txt
```

## AWS-Specific Features

### EFS (Elastic File System)

Create and mount EFS volumes:

```bash
# Create cluster with EFS mount
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-efs-create \
  --aws-efs-mount myefs:/mnt/efs \
  --aws-disk type=gp2,size=20 \
  --aws-expire=8h
```

### Add Public IP Later

Add public IP to existing cluster:

```bash
aerolab cluster add public-ip -n mydc
```

### Add Firewall Rules

Add firewall rules to cluster:

```bash
aerolab cluster add firewall -n mydc -f aerolab-sg
```

## Cleanup

### Destroy a Cluster

```bash
aerolab cluster destroy -n mydc --force
```

### Clean Up All Resources

```bash
aerolab inventory delete-project-resources -f --expiry
```

## Common Workflows

### Complete Workflow Example

```bash
# 1. Configure backend
aerolab config backend -t aws -r us-east-1

# 2. Create security group
aerolab config aws create-security-groups -n aerolab-sg -p 3000-3005

# 3. Create cluster
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*' \
  -I t3a.xlarge \
  --aws-disk type=gp2,size=20 \
  --aws-disk type=gp3,size=100,count=3 \
  --secgroup-name aerolab-sg \
  --aws-expire=8h

# 4. Start cluster
aerolab cluster start

# 5. Start Aerospike
aerolab aerospike start

# 6. Wait for stability
aerolab aerospike is-stable -w

# 7. Check status
aerolab aerospike status

# 8. Use the cluster
aerolab attach aql -n mydc -- -c "show namespaces"

# 9. Clean up
aerolab cluster destroy -n mydc --force
```

## Troubleshooting

### Credential Issues

If you see authentication errors:

```bash
# Check AWS credentials
aws sts get-caller-identity

# Verify credentials file
cat ~/.aws/credentials
```

### Region Issues

If resources aren't found, check you're in the right region:

```bash
aerolab config backend
```

### Permission Issues

If you see permission denied errors, check your IAM permissions:

```bash
aws iam get-user
```

### Instance Type Availability

Check available instance types in your region:

```bash
aerolab inventory instance-types
```

### Network Issues

If nodes can't communicate:

1. Check security groups
2. Verify VPC configuration
3. Check subnet settings

## Next Steps

- Explore [cluster management commands](commands/cluster.md)
- Learn about [Aerospike daemon controls](commands/aerospike.md)
- Check out [AWS-specific volume management](commands/volumes.md)
- See [Aerospike Cloud integration](cloud/README.md)

