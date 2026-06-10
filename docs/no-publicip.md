# Using cloud backends without public IPs

Both the AWS and GCP backends can deploy onto subnets/networks where instances are not assigned public IPs, operating on private IPs only:

* AWS: `aerolab config backend -t aws ... --aws-nopublic-ip`
* GCP: `aerolab config backend -t gcp ... --gcp-nopublic-ip`

When the relevant option is set, aerolab will not request a public IP for the instances it creates and will connect to them over their private IPs.

Because aerolab connects over private IPs, it must run from a machine that can route to the private subnet/network. The example below shows the AWS workflow: it first deploys an aerolab `client` machine in a subnet *with* a public IP (so we can reach it), then connects to that machine and deploys the rest of the infrastructure from there into subnets without public IP assignments. The same pattern applies to GCP using `--gcp-nopublic-ip`.

Requirements:
* the private subnet/network must have a route out to the internet (AWS: NAT gateway; GCP: Cloud NAT) so that installation may proceed
  * this is only required for the creation of clients other than `none` and cluster node templates; `none` clients and cluster deployments from existing templates do not require an internet connection
* the subnet/network on which aerolab lives must be able to route to the private-ip subnet/network

## AWS example

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
