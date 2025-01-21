# Using AWS backend subnets without public IPs

The below example deploys an aerolab `client` machine in a subnet with a Public IP, so that we can reach it.

Once that machine is deployed, with `aerolab`, on it, we connect to the machine and deploy the infrastructure from there into subnets without public IP assignments.

Requirements:
* the private subnet must have a NAT gateway route allowing the nodes to route out to the internet so that installation may proceed
  * this is only required for the creation of clients other than `none` and cluster node templates; `none` clients and cluster deployments from existing templates does not require an internet connection
* the subnet on which aerolab lives must be able to route to the private-ip subnet

```bash
# deploy a client machine in a public subnet; use a non-default sec group name
aerolab client create none -n aerolab -I t3a.xlarge --secgroup-name=external -U subnet-099516584ce4e870f

# install aerolab on the client machine
aerolab client configure aerolab --name aerolab

# copy permissions to the client machine
aerolab files upload -c -n aerolab ~/.aws /root/

# attach to the client machine
aerolab client attach -n aerolab

# on aerolab client machine - configure backend
aerolab config backend -t aws -r us-west-2 --aws-nopublic-ip

# setup firewall with a given private IP range
aerolab config aws create-security-groups -n aerolab2 -v vpc-0863aa2e8d3d379e1
aerolab config aws lock-security-groups -n aerolab2 -i 10.0.0.0/8 -v vpc-0863aa2e8d3d379e1

# deploy clusters and clients in private subnet with no public IPs
aerolab cluster create -v 7.0.0.2 -n testsrv -I t3a.xlarge -U subnet-05553cf8361f4dde1 --secgroup-name=aerolab2
aerolab client create none -n testcl -I t3a.xlarge -U subnet-05553cf8361f4dde1 --secgroup-name=aerolab2
```

## Cleanup:

```bash
aerolab cluster destroy -f -n testsrv
aerolab client destroy -f -n testcl,aerolab
```
