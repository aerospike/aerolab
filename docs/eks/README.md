# Deploying EKS clusters with aerolab for Aerospike

Aerolab can create client machines (containers/VMs) with preinstalled tools for the deployment of EKS clusters in AWS as well as the OLM (operator lifecycle manager) and AKO (aerospike kubernetes operator). This allows for the deployment of Aerospike in EKS clusters.

## Tools

Name | Description
--- | ---
`bootstrap` | bash script responsible for installing and updating EKS components in the VM/container; auto-run the first time during creation; can be run multiple times to update the components
`eksctl` | the EKSCTL application for creating and destroying EKS clusters; installed by `bootstrap`
`eksexpiry` | an application for changing/extending expiry of auto-expiring EKS clusters
`kubectl` | the latest version of the `kubectl` utility; installed by `bootstrap`
`aws` | the aws-cli v2 installed by `bootstrap`
`helm` | k8s `helm` tool installed by `bootstrap`
`/root/eks` | directory containing example `yaml` files for use with `eksctl`

## Usage

### Create the client machine

Provide a KEYID and SECRETKEY that the machine should use to interact with AWS.

```bash
aerolab client create eksctl -n eksctl -f /path/to/features.conf -r AWS-REGION -k AKIAxxxxxxxxxxxxxx -s xxxxxxxxxxxxxxxxxx
```

Alternatively, provide an instance policy to apply to the instance for authentication purposes (policy must exist):

```bash
aerolab client create eksctl -n eksctl -f /path/to/features.conf -r AWS-REGION -x AWS-POLICY-NAME
```

### Optional - configure timezone in eksctl client machine

This is particularly useful for the `eksexpiry` program to display local timezone instead of UTC:

```bash
aerolab client attach -n eksctl -- dpkg-reconfigure tzdata
```

### NOTE ON REGIONS AND TOOLS VERSIONS

If intending to switch to another region in the `eksctl` template `yaml` files, ensure that the above `bootstrap -n NEWREGION` is executed, replacing `NEWREGION` with actual AWS region. Without this, aerolab's expiry system will not work in the new region.

If required, all preinstalled tools can also be updated and example `yaml` files can be recreated automatically with the new region.

See [this page](update-switch-region.md) for information on how to update the tools in the `eksctl` client and switch regions.

### Connect to the eks tools machine

Connect to the EKS tools machine, from which all the below steps are then executed.

```bash
aerolab client attach -n eksctl
```

### Deploy an EKS cluster

In this example, we are deploying the `basic.yaml` example. Feel free to modify, tune and deploy another cluster. Cluster name is defined in the `yaml` file itself. See contents of `/root/eks` directory for more examples.

The EKS cluster expiry is defined by using a cluster tag in the yaml file itself. This can be changed later using the `eksexpiry` utility.

```bash
cd /root/eks
eksctl create cluster -f basic.yaml
```

### Using eksctl to show clusters and select a cluster to use

Get cluster details:

```bash
# list clusters
eksctl get clusters
# get cluster details
eksctl get cluster -f /root/eks/basic.yaml
eksctl get nodegroup -f /root/eks/basic.yaml
eksctl get addon -f /root/eks/basic.yaml
```

Select cluster for kubectl authentication:

```bash
eksctl utils write-kubeconfig -f /root/eks/basic.yaml
```

### Deploy OLM and AKO

While this can be done manually by following the official documentation, a deployment script has been provided.

```bash
cd /root/deploy-olm-ako
./setup_olm.sh # display usage help
./setup_olm.sh -f /root/eks/basic.yaml # apply olm and ako
```

### Deploy Aerospike - basic example

Deploy:

```bash
cd /root/deploy-olm-ako/examples/clusters
kubectl apply -f aerospike_memory_namespaces.yaml -n aerospike
```

Check status:

```bash
kubectl get all -n aerospike
kubectl get pods -n aerospike -o wide
```

Get logs:

```bash
# normal
kubectl -n aerospike logs asdb-dev-1-0 -f
# init debugging only, run this multiple times as k8s rotates the containers until a useful error appears
kubectl -n aerospike logs -f asdb-dev-1-0 -c aerospike-init
```

Troubleshooting events:

```bash
kubectl get events -n olm
kubectl get events -n operators
kubectl get events -n aerospike
```

Attach to asadm:

```bash
kubectl run -it --rm --restart=Never aerospike-tool -n aerospike --image=aerospike/aerospike-tools:latest -- asadm -h asdb-dev-1-0.asdb-dev.aerospike:3000 -U admin -Padmin123
```

### Change the EKS cluster expiry

Below 3 examples show how this can be achieved.

```bash
# using cluster name
eksexpiry --name CLUSTERNAME --region us-central-1 --in 30h
# using eksctl yaml file
eksexpiry --file /root/eks/basic.yaml --in 30h
# specify exact date instead of duration, format YYYY-MM-DD_hh:mm:ss[_TZ] ; if timezone is not specified, UTC is assumed
eksexpiry --name CLUSTERNAME --region us-central-1 --at 2024-02-11_05:40:15_0700
```

### Destroy the EKS cluster

The below 2 examples show how to destroy using cluster name or the definition yaml file.

```bash
eksctl delete cluster -n basic --disable-nodegroup-eviction
eksctl delete cluster -f /root/eks/basic.yaml --disable-nodegroup-eviction
```

## Cheatsheets

* [AMS - monitoring stack](other/ams.md)
* [Asbench - tools with asbench load](other/asbench.md)
* [Rack Aware clusters and clients](other/rackaware.md)
* [Apply CPU Policies](other/cpu_policy.md)
* [Debug PODs](other/debug_pods.md)
* [GP3 Storage Class](other/gp3_storage_class.md)
* [OLM Stuck in Terminating state when deleting a namespace](other/olm_stuck_deleting_namespace.md)

## Further reading, usage and examples

* Official Aerospike Kubernetes Operator manuals for installation, management and monitoring can be found [here](https://aerospike.com/docs/cloud/kubernetes/operator)
* The `deploy-olm-ako` script readme and associated Aerospike deployment example `yaml` files are hosted [here](https://github.com/colton-aerospike/deploy-olm-ako/tree/eksctl)
* The [eksctl yaml example files](https://github.com/eksctl-io/eksctl/tree/main/examples)
* [eksctl usage manuals](https://eksctl.io/usage/schema/#metadata-tags)
