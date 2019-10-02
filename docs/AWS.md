# How to setup and use AWS with aerolab

## Prerequisites

#### Create Credentials file

Make KEYID and SECRETKEY in AWS if needed to access the account using the APIs. The setup the credentials file:
```bash
$ cat ~/.aws/credentials
[default]
aws_access_key_id = KEYID
aws_secret_access_key = SECRETKEY
```

#### Pubkey switch is unused

#### Environment variables: aws keys location

Create a directory you will use for pubkeys as the aws backend will generate a new pubkey and use it for each cluster. 
```bash
$ export aerolabAWSkeys=/path/to/aws/keys
```

#### Environment variables: aws security groups

In the aws ec2 console, create a security group, which will have at least port 22 open for ssh and aerospike ports (preferably all ports) for privateIp communications.

Otherwise, you can create a security group that will have access to all ports. Then setup the environment variable for the security group to use.
```bash
$ export aerolabSecurityGroupId=sg-940b23ef
```

#### Optional: Environment variables: aws subnet ID to use

Default: default

```bash
$ export aerolabSubnetId=subnet-944515d9
```

## Deploy cluster in aws

Name cluster 'robert', with 3 nodes, mesh (you must use mesh) and a t2.small with 10GB disk.
```bash
./aerolab make-cluster -e aws -n robert -c 3 -m mesh -r t2.small:10
```

## Destroy cluster
```bash
./aerolab cluster-destroy -f 1 -e aws -n robert
```
