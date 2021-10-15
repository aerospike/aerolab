# How to deploy aerospike on k8s operator

## yaml file

Look at `deploy/samples/dim_nostorage_cluster_cr.yaml` and change to your needs. The definitions should be fairly explanatory, especially when looking at 'useful links' in the README file.

## Deploy

```
minikube kubectl -- apply -f deploy/samples/dim_nostorage_cluster_cr.yaml
```

## If changes need to be made, adjust the yaml file and re-run the apply command above

## Delete and apply again from scratch (yes, delete run twice and wait 1 minute)

```
minikube kubectl -- delete -n aerospike aerocluster
minikube kubectl -- delete -n aerospike aerocluster
sleep 60
minikube kubectl -- apply -f deploy/samples/dim_nostorage_cluster_cr.yaml
```

## Monitor state and if deployed

```
minikube kubectl -- get pod -n aerospike
minikube kubectl -- get statefulset -n aerospike
minikube kubectl -- logs -n aerospike aerocluster-0-0
minikube kubectl -- describe pods -n aerospike aerocluster-0-0
minikube kubectl -- -n aerospike logs -f $(minikube kubectl -- get pod -n aerospike |egrep -o 'aerospike-kubernetes-operator-[a-z0-9-]+')
```

## Note: if upgrading version, operator will rolling-restart the cluster, this may take a while (about 1 minute per node). Monitor with:

```
minikube kubectl -- get pod -n aerospike
```

## Get shell in container or check file

```
minikube kubectl -- exec --stdin --tty -n aerospike aerocluster-0-0 -- /bin/bash
minikube kubectl -- exec --stdin --tty -n aerospike aerocluster-0-0 -- cat /etc/aerospike/aerospike.conf
```
