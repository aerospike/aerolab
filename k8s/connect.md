# Connect to k8s cluster

## Grab IP and port

```
minikube kubectl -- -n aerospike describe aerospikecluster aerocluster
```

Grab the IP and port.

## Connect - change IP and port as required

```
minikube kubectl -- run -it --rm --restart=Never aerospike-tool -n aerospike --image=aerospike/aerospike-tools:latest -- asadm -h 192.168.64.10:30680 -U admin -P admin123
```
