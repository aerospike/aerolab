[Docs home](../README.md)

## Sub-topics:

[Custom VPC setup notes](vpc.md)

[Partitioning disks in AWS](partitioner/partition-disks.md)

[Use all disks for a namespace](partitioner/all-disks.md)

[Use all NVME disks for a namespace](partitioner/all-nvme-disks.md)

[Create Partitioning for Two Namespaces](partitioner/two-namespaces-nvme.md)

[Configure Shadow Devices](partitioner/with-shadow.md)

[Partition NVME and EBS for All-Flash Storage](partitioner/with-allflash.md)


# AeroLab on AWS

## Prerequisites

### Create a credentials file

There are two ways to create a credentials file:

#### Using aws-cli

1. Download and install [aws-cli](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
2. Run the command `aws configure`

#### Manually

Create the `~/.aws` directory, inside the directory create two files:

`~/.aws/credentials`

```toml
[default]
aws_access_key_id = KEYID
aws_secret_access_key = SECRETKEY
```

`~/.aws/config`

```toml
[default]
region = DEFAULT_REGION_TO_USE
```

### OPTIONAL: Configure an AWS account

You can use the default AWS subnets and security groups and
AeroLab will create the required security groups with minimal permissions required.

If you do not specify a VPC, AeroLab will create it, together with default subnets.

#### Security Groups

AeroLab clusters require a security group. The following rules allow full connectivity:

1. Create a security group (sg-xxxx) with the rule to allow all outbound (default) and inbound: port 22(tcp) from any IP address.

2. Edit the security group (sg-xxxx) and add a rule to allow all inbound on all ports coming from itself (source: sg-xxxx).

If you plan to deploy AMS or other clients:

1. Create a security group (sg-yyyy) with a rule to allow all outbound (default) and allow inbound from any IP adress to the following TCP ports: 22, 3000, 8888, 8080, 8081, 9200.

2. Edit the security group (sg-yyyy), adding 2 rules:
   a) Allow all ports from self source (sg-yyyy).
   b) Allow all ports from server source (sg-xxxx).

3. Edit the security group (sg-xxxx), and add a rule allowing all ports from the client source security group (sg-yyyy).

Use (sg-xxxx) for clusters and (sg-yyyy) for client machines.

#### Subnets

If creating a new subnet and/or VPC, configure the VPC and Subnet such that:
* the instances will have automatically assigned public DNS
* the instances will have automatically assigned public IP addresses

### Configure the AWS backend with AeroLab

The most basic configuration is:

```bash
aerolab config backend -t aws [-d /path/to/tmpdir/for-aerolab/to/use]
```

To specify a custom location where SSH keys are stored and override the
default AWS region configuration, extra parameters may be supplied:

```bash
aerolab config backend -t aws -p /PATH/TO/KEYS -r AWS_REGION [-d /path/to/tmpdir/for-aerolab/to/use]
```

It is possible to specify which AWS profile to use as follows:

```bash
aerolab config backend -t aws -P aws-profile-name
```

## Deploy an Aerospike cluster in AWS

Extra parameters are required when working with the `aws` backend as opposed to the `docker` backend.

Executing `aerolab cluster create help` once the backend has been selected displays the relevant options.

## Notes on AWS and custom VPCs

If you don't use the default VPC, certain conditions must be met when configuring a custom one.
Refer to the [VPC reference](vpc.md) for more information.

### Examples:

#### First subnet in default VPC/AZ:

```bash
aerolab cluster create -n testcluster -c 3 -m mesh -I t3a.medium -E 20
```

#### First subnet in default VPC for a given AZ:

```bash
aerolab cluster create -n testcluster -c 3 -m mesh -I t3a.medium -E 20 -U us-east-1a
```

#### Specify custom security group and subnet:

```bash
aerolab cluster create -n testcluster -c 3 -m mesh -I t3a.medium -E 20 -S sg-03430d698bffb44a3 -U subnet-06cc8a834647c4cc3
```

#### Lock security-groups so that machines are only accessible from the AeroLab IP address:

```bash
# default VPC
aerolab config aws lock-security-groups
# custom VPC
aerolab config aws lock-security-groups -v vpc-...
```

#### Lock security-groups so that machines are only accessible from a given IP range:

```bash
# default VPC
aerolab config aws lock-security-groups --ip 1.2.3.4/32
# custom VPC
aerolab config aws lock-security-groups --ip 1.2.3.4/32 -v vpc-...
```

### Destroy cluster:
```bash
aerolab cluster destroy -f -n testcluster
```

#### Destroy aerolab-managed security groups:

```bash
# default VPC
aerolab config aws delete-security-groups
# custom VPC
aerolab config aws delete-security-groups -v vpc-...
```

## Other commands

All commands are supported on both `aws` and `docker` backends and should behave exactly the same.

## Working with multiple regions

You can work with multiple regions by switching the backend:

```bash
aerolab config backend -t aws -r eu-west-1
# ...commands...
aerolab config backend -t aws -r us-east-1
# ...commands...
```

Alternatively, if you frequently use multiple regions, you can have
multiple configuration files:

```bash
# create a config called us.conf
AEROLAB_CONFIG_FILE=us.conf
aerolab config backend -t aws -r us-east-1

# create a config called eu.conf
AEROLAB_CONFIG_FILE=eu.conf
aerolab config backend -t aws -r eu-west-1

# since eu is the exported region variable, default commands execute against it
aerolab cluster create
aerolab attach shell -- asadm -e info

# execute an ad-hoc command on another region
AEROLAB_CONFIG_FILE=us.conf aerolab cluster create

# keep running in eu region
aerolab cluster destroy
```

## Note on shared AWS accounts and KeyPairs

AeroLab creates and destroys SSH key pairs as needed. However, if
a particular cluster is created by user X, user Y can only access
the cluster if user X shares their key pair for that cluster.

By default, keys are stored in `${HOME}/aerolab-keys`.
