# Join aerolab cluster to another docker network

This is particularly useful if you deployed a stack using docker-compose with it's own docker network name and wish to bridge the networks so that the aerolab-deployed cluster can talk to the other docker network.

## Simple use

```
$ docker network connect <DOCKER NETWORK NAME> aero-<CLUSTER_NAME>_1
$ docker network connect <DOCKER NETWORK NAME> aero-<CLUSTER_NAME>_2
$ docker network connect <DOCKER NETWORK NAME> aero-<CLUSTER_NAME>_3
...
```

## Single-command use

Edit only the `CLUSTERNAME=""` to include the actual cluster name (without the aero_, just the name) and the `NETNAME=""` to contain the name of the docker network to bridge to the cluster.

```
$ CLUSTERNAME=""; NETNAME=""; docker container list -a -f "name=aero-${CLUSTERNAME}_" |egrep "aero-${CLUSTERNAME}_[0-9]+\$" |awk '{print $1}' |while read node; do docker network connect "${NETNAME}" "${node}"; done
```
