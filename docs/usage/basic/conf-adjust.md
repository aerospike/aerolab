# Custom Adjusting Aerospike configuration

This manual covers adjusting aerospike configuration parameters on the cluster nodes on the fly for the running cluster.

## Usage:

```
aerolab conf adjust

Usage: aerolab conf adjust [OPTIONS] {command} {path} [set-value1] [set-value2] [...set-valueX]

[OPTIONS]
        -n, --name=  Cluster name (default: mydc)
        -l, --nodes= Nodes list, comma separated. Empty=ALL

COMMANDS:
        delete - delete configuration/stanza
        set    - set configuration parameter
        create - create a new stanza

PATH: path.to.item or path.to.stanza, e.g. network.heartbeat

SET-VALUE: for the 'set' command - used to specify value of parameter; leave empty to crete no-value param

EXAMPLES:
        aerolab conf adjust create network.heartbeat
        aerolab conf adjust set network.heartbeat.mode mesh
        aerolab conf adjust set network.heartbeat.mesh-seed-address-port "172.17.0.2 3000" "172.17.0.3 3000"
        aerolab conf adjust create service
        aerolab conf adjust set service.proto-fd-max 3000
```

## Example:

Adjust XDR configuration to include authentication methods with enabled security.

### Create source and destination clusters

```
aerolab cluster create -c 5 -n src
aerolab cluster create -c 5 -n dst
```

### Connect the clusters via XDR

```
aerolab xdr connect -S src -D dst -M test,bar
```

### Adjust the running configuration on the source cluster

This will adjust all nodes to include authentication mode, username and password file parameters.

```
aerolab conf adjust -n src set "xdr.dc dst.auth-mode" internal
aerolab conf adjust -n src set "xdr.dc dst.auth-user" someUserOnDestination
aerolab conf adjust -n src set "xdr.dc dst.auth-password-file" /etc/aerospike/xdr-dst-password.txt
```

### Store password file for XDR to use on the source cluster

```
printf "userDestinationPassword" > xdr-dst-password.txt
aerolab files upload -n src xdr-dst-password.txt /etc/aerospike/xdr-dst-password.txt
rm xdr-dst-password.txt
```

### Restart source cluster

```
aerolab aerospike restart -n src
```
