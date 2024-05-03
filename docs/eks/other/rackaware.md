# Deploying clusters and clients with rack awareness

## Deployment example description

* 2 zones/racks
* 2 aerospike nodes per rack - one per node
* 2 asbench client machines on a separate set of node (no load interference with servers)

## Flow

To achieve this the following has to be met:

* the nodes need to be labelled, with an `app=` label to pick which nodes are used for aerospike and which can be used by asbench
* the `kubectl` yaml files must correctly select based on the labels for both asbench and the servers

## Process

### Deploy the EKS cluster with 2 nodegroups

```bash
cd /root/eks
eksctl create cluster -f two-node-groups.yaml
```

### Deploy OLM and AKO

```bash
cd /root/deploy-olm-ako
./setup_olm.sh -f /root/eks/two-node-groups.yaml
```

### Set labels

List nodes: `kubectl get nodes -o wide`

Simple script to mark first 6 nodes as `aerospike` app:

```bash
AEROSPIKE=6
kubectl get nodes -o wide --no-headers |awk '{print $1}' |sort |head -${AEROSPIKE} |while read node; do kubectl label nodes ${node} app=aerospike; done
```

View labels: `kubectl get nodes --show-labels -o wide`

Simple script to make the last 2 nodes astools:

```bash
ASTOOLS=2
kubectl get nodes -o wide --no-headers |awk '{print $1}' |sort |tail -${ASTOOLS} |while read node; do kubectl label nodes ${node} app=astools; done
```

### Deploy aerospike cluster with label selector and 2 racks

```bash
cd /root/deploy-olm-ako/examples/clusters
kubectl apply -f aerospike_memory_rackaware_and_nodeselector.yaml -n aerospike
```

### Deploy clients

```bash
cd /root/deploy-olm-ako/examples/clients
kubectl apply -f asbench-nodeselector-and-tolerations.yaml -n aerospike
```
