# How to use k8s clusters on your machine

## Synopsis

This document covers the following:

1. [How to install kubernetes on your machine, with a version compatible with the Operator](install-minikube.md)
  * the above also covers start/stop/reset/delete/pause and other useful commands that you will be using
  * also covered: how to replace docker desktop with minikube which handles docker and k8s transparently
  * also covered: aerolab 100% compatible with minikube docker
2. How to deploy aerospike operator with an aerospike cluster
  * [download operator, create namespace, deploy operator, features file, add fs storage and user password secret](deploy-operator.md)
  * [create aerospike definition, deploy cluster, check state](deploy-aerospike.md)
  * [connect to aerospike](connect.md)

## Useful links

[https://docs.aerospike.com/docs/cloud/kubernetes/operator/Cluster-configuration-settings.html](https://docs.aerospike.com/docs/cloud/kubernetes/operator/Cluster-configuration-settings.html)
[https://docs.aerospike.com/docs/cloud/kubernetes/operator/Aerospike-configuration-mapping.html](https://docs.aerospike.com/docs/cloud/kubernetes/operator/Aerospike-configuration-mapping.html)
[https://docs.aerospike.com/docs/cloud/kubernetes/operator/Connect-to-the-Aerospike-cluster.html#obtain-the-aerospike-node-endpoints](https://docs.aerospike.com/docs/cloud/kubernetes/operator/Connect-to-the-Aerospike-cluster.html#obtain-the-aerospike-node-endpoints)
[https://github.com/aerospike/aerospike-kubernetes-operator/wiki/Cluster-configuration-settings](https://github.com/aerospike/aerospike-kubernetes-operator/wiki/Cluster-configuration-settings)

## Example ldap yaml aerospike file

You can deploy aerospike in k8s and still use the deploy-ldap.sh script to deploy ldap in docker.

1. First run deploy-ldap.sh and get the IP of your ldap container.
2. Get [ldap.yaml](ldap.yaml) definition of cluster, change LDAP IP to the IP you got from the script and apply the configuration.
3. Connect, using (change IP and port as required, see [connect.md](connect.md) for details):

```
minikube kubectl -- run -it --rm --restart=Never aerospike-tool -n aerospike --image=aerospike/aerospike-tools:latest -- asadm -h 192.168.64.10:30680 -U admin -P admin123
minikube kubectl -- run -it --rm --restart=Never aerospike-tool -n aerospike --image=aerospike/aerospike-tools:latest -- asadm -h 192.168.64.10:30680 --auth=EXTERNAL_INSECURE -U badwan -P blastoff
```
