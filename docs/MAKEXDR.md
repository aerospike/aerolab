# Make XDR Clusters

## Simple 2-cluster active-passive 3 node to 2 node
```
$ ./aerolab make-xdr-clusters -c 3 -a 2
Nov 06 09:28:10+0000 AERO-LAB[13828]: INFO     --> Deploying dc1
Nov 06 09:28:10+0000 AERO-LAB[13828]: INFO     Performing sanity checks
Nov 06 09:28:10+0000 AERO-LAB[13828]: INFO     Checking if version template already exists
Nov 06 09:28:10+0000 AERO-LAB[13828]: INFO     Checking aerospike version
Nov 06 09:28:20+0000 AERO-LAB[13828]: INFO     Starting deployment
Nov 06 09:28:27+0000 AERO-LAB[13828]: INFO     Done
Nov 06 09:28:27+0000 AERO-LAB[13828]: INFO     --> Deploying dc2
Nov 06 09:28:27+0000 AERO-LAB[13828]: INFO     Performing sanity checks
Nov 06 09:28:27+0000 AERO-LAB[13828]: INFO     Checking if version template already exists
Nov 06 09:28:27+0000 AERO-LAB[13828]: INFO     Checking aerospike version
Nov 06 09:28:36+0000 AERO-LAB[13828]: INFO     Starting deployment
Nov 06 09:28:41+0000 AERO-LAB[13828]: INFO     Done
Nov 06 09:28:41+0000 AERO-LAB[13828]: INFO     --> xdrConnect running
Nov 06 09:28:41+0000 AERO-LAB[13828]: INFO     XdrConnect running
Nov 06 09:28:44+0000 AERO-LAB[13828]: INFO     Done, now restart the source cluster for changes to take effect.

$ ./aerolab restart-aerospike -n dc1
```

## Advanced XDR fun

### Define scenario

* cluster 1: 3 node, mesh
* cluster 2: 2 node, multicast
* cluster 3: 5 node, multicast with a different multicast group
* cluster 1 <-> cluster 2 (active-active) 2-way XDR replication
* cluster 3 failover - XDR destination for both cluster 1 and cluster 2
* namespace: bar

So, XDR-wise we ship this way:
* cluster 1 -> cluster 2, cluster 3
* cluster 2 -> cluster 1, cluster 3

### Deploy 3 clusters

##### Cluster 1 - 3-node mesh
```
$ ./aerolab make-cluster -n dc1 -c 3 -m mesh
Nov 06 09:32:04+0000 AERO-LAB[13974]: INFO     Performing sanity checks
Nov 06 09:32:04+0000 AERO-LAB[13974]: INFO     Checking if version template already exists
Nov 06 09:32:04+0000 AERO-LAB[13974]: INFO     Checking aerospike version
Nov 06 09:32:12+0000 AERO-LAB[13974]: INFO     Starting deployment
Nov 06 09:32:19+0000 AERO-LAB[13974]: INFO     Done
```

##### Cluster 2 - 2-node default multicast
```
$ ./aerolab make-cluster -n dc2 -c 2
Nov 06 09:32:49+0000 AERO-LAB[14015]: INFO     Performing sanity checks
Nov 06 09:32:50+0000 AERO-LAB[14015]: INFO     Checking if version template already exists
Nov 06 09:32:50+0000 AERO-LAB[14015]: INFO     Checking aerospike version
Nov 06 09:32:59+0000 AERO-LAB[14015]: INFO     Starting deployment
Nov 06 09:33:03+0000 AERO-LAB[14015]: INFO     Done
```

##### Cluster 3 - 5-node with changed multicast group
```
$ ./aerolab make-cluster -n dc3 -c 5 -m mcast -a 239.1.99.234 -p 9918
Nov 06 09:34:47+0000 AERO-LAB[14060]: INFO     Performing sanity checks
Nov 06 09:34:47+0000 AERO-LAB[14060]: INFO     Checking if version template already exists
Nov 06 09:34:47+0000 AERO-LAB[14060]: INFO     Checking aerospike version
Nov 06 09:34:55+0000 AERO-LAB[14060]: INFO     Starting deployment
Nov 06 09:35:07+0000 AERO-LAB[14060]: INFO     Done
```

### Connect XDR
```
$ ./aerolab xdr-connect -s dc1 -d dc2,dc3 -m bar
Nov 06 09:36:16+0000 AERO-LAB[14117]: INFO     XdrConnect running
Nov 06 09:36:19+0000 AERO-LAB[14117]: INFO     Done, now restart the source cluster for changes to take effect.

$ ./aerolab xdr-connect -s dc2 -d dc1,dc3 -m bar
Nov 06 09:36:39+0000 AERO-LAB[14146]: INFO     XdrConnect running
Nov 06 09:36:42+0000 AERO-LAB[14146]: INFO     Done, now restart the source cluster for changes to take effect.
```

### Restart aerospike
```
$ for i in dc1 dc2 dc3 ; do ./aerolab restart-aerospike -n $i ; done
```

### Check cluster size in all 3 DCs
```
$ for i in dc1 dc2 dc3 ; do ./aerolab node-attach -n $i -- grep CLUSTER-SIZE /var/log/aerospike.log |tail -1 ; done
Nov 06 2018 09:41:11 GMT: INFO (info): (ticker.c:172) NODE-ID bb9020011ac4202 CLUSTER-SIZE 3
Nov 06 2018 09:41:12 GMT: INFO (info): (ticker.c:172) NODE-ID bb9050011ac4202 CLUSTER-SIZE 2
Nov 06 2018 09:41:14 GMT: INFO (info): (ticker.c:172) NODE-ID bb9070011ac4202 CLUSTER-SIZE 5
```

### Check xdr status on node 1 of dc1 and dc2
```
./aerolab node-attach -n dc1 -- grep xdr /var/log/aerospike.log |grep CLUSTER_UP
Nov 06 2018 09:38:30 GMT: INFO (xdr): (xdr.c:547) [dc2]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497110044 mlst 1541497110044 (2018-11-06 09:38:30 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 2
Nov 06 2018 09:38:30 GMT: INFO (xdr): (xdr.c:547) [dc3]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497110044 mlst 1541497110044 (2018-11-06 09:38:30 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 5

$ ./aerolab node-attach -n dc2 -- grep xdr /var/log/aerospike.log |grep CLUSTER_UP
Nov 06 2018 09:38:34 GMT: INFO (xdr): (xdr.c:547) [dc1]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497114020 mlst 1541497114020 (2018-11-06 09:38:34 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 3
Nov 06 2018 09:38:34 GMT: INFO (xdr): (xdr.c:547) [dc3]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497114020 mlst 1541497114020 (2018-11-06 09:38:34 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 5
Nov 06 2018 09:39:34 GMT: INFO (xdr): (xdr.c:547) [dc1]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497174056 mlst 1541497174056 (2018-11-06 09:39:34 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 3
Nov 06 2018 09:39:34 GMT: INFO (xdr): (xdr.c:547) [dc3]: dc-state CLUSTER_UP timelag-sec 0 lst 1541497174056 mlst 1541497174056 (2018-11-06 09:39:34 GMT) fnlst 0 (-) wslst 0 (-) shlat-ms 0 rsas-ms 0.000 rsas-pct 0.0 con 0 errcl 0 errsrv 0 sz 5
```
