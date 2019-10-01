#### Simple make cluster

```bash
$ ./aerolab make-cluster -U DOWNLOADUSER -P 'DOWNLOADPASS'
Nov 04 12:39:22+0000 AERO-LAB[97410]: INFO     Performing sanity checks
Nov 04 12:39:23+0000 AERO-LAB[97410]: INFO     Checking if version template already exists
Nov 04 12:39:23+0000 AERO-LAB[97410]: INFO     Checking aerospike version
Nov 04 12:39:30+0000 AERO-LAB[97410]: INFO     Starting deployment
Nov 04 12:39:32+0000 AERO-LAB[97410]: INFO     Done
```

#### Make cluster, giving it a name and selecting node count, require mesh
```bash
$ ./aerolab make-cluster -U DOWNLOADUSER -P 'DOWNLOADPASS' -n mycluster -c 3 -m mesh
Nov 04 12:42:36+0000 AERO-LAB[97470]: INFO     Performing sanity checks
Nov 04 12:42:36+0000 AERO-LAB[97470]: INFO     Checking if version template already exists
Nov 04 12:42:36+0000 AERO-LAB[97470]: INFO     Checking aerospike version
Nov 04 12:42:43+0000 AERO-LAB[97470]: INFO     Starting deployment
Nov 04 12:42:50+0000 AERO-LAB[97470]: INFO     Done
```

#### Configure a config file so that we don't need to specify download user and password
```bash
$ cat ~/aero-lab-common.conf 
[Common]
Username="DOWNLOADUSER"
Password="DOWNLOADPASS"

$ ./aerolab make-cluster -n testme
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Performing sanity checks
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking if version template already exists
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking aerospike version
Nov 04 12:45:15+0000 AERO-LAB[97520]: INFO     Starting deployment
Nov 04 12:45:18+0000 AERO-LAB[97520]: INFO     Done
```

#### Deploy a single-node cluster and forward local port 3000 to that node's port 3000 (useful for dev purposes against a single node)
```bash
$ ./aerolab make-cluster -h 3000:3000 -n testfwd
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Performing sanity checks
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking if version template already exists
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking aerospike version
Nov 04 12:45:15+0000 AERO-LAB[97520]: INFO     Starting deployment
Nov 04 12:45:18+0000 AERO-LAB[97520]: INFO     Done
```
