# Deploy AMC

### Simple deploy
```bash
$ ./aerolab deploy-amc
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Performing sanity checks
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Checking if version template already exists
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Checking aerospike version
Nov 06 13:53:01+0000 AERO-LAB[19538]: INFO     Starting deployment
Nov 06 13:53:02+0000 AERO-LAB[19538]: INFO     Done, to access amc console, visit http://localhost:8081
```

Now, from your computer, access the link given.

### Deploy, change port
```bash
$ ./aerolab deploy-amc -n amc2 -p 8082:8081
Nov 06 13:54:04+0000 AERO-LAB[19563]: INFO     Performing sanity checks
Nov 06 13:54:04+0000 AERO-LAB[19563]: INFO     Checking if version template already exists
Nov 06 13:54:04+0000 AERO-LAB[19563]: INFO     Checking aerospike version
Nov 06 13:54:07+0000 AERO-LAB[19563]: INFO     Starting deployment
Nov 06 13:54:08+0000 AERO-LAB[19563]: INFO     Done, to access amc console, visit http://localhost:8082
```

This will forward port 8082 from your local machine to the AMC 8081. Access the link to access this AMC console.

### Full procedure

###### Make cluster with 3 nodes
```
$ ./aerolab make-cluster -c 3
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Performing sanity checks
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking if version template already exists
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking aerospike version
Nov 04 12:45:15+0000 AERO-LAB[97520]: INFO     Starting deployment
Nov 04 12:45:18+0000 AERO-LAB[97520]: INFO     Done
```

###### Deploy amc
```bash
$ ./aerolab deploy-amc
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Performing sanity checks
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Checking if version template already exists
Nov 06 13:52:58+0000 AERO-LAB[19538]: INFO     Checking aerospike version
Nov 06 13:53:01+0000 AERO-LAB[19538]: INFO     Starting deployment
Nov 06 13:53:02+0000 AERO-LAB[19538]: INFO     Done, to access amc console, visit http://localhost:8081
```

###### Get list of IPs of the cluster we deployed (will need aerospike host IP for AMC after all)
```bash
$ ./aerolab cluster-list
[...]
NODE_NAME | NODE_IP
===================
aero-amc_1 | 172.17.0.5
aero-mydc_3 | 172.17.0.4
aero-mydc_2 | 172.17.0.3
aero-mydc_1 | 172.17.0.2
```

###### Access AMC

Access AMC via http://localhost:8081

When asked for node IP, use one of the above (172.17.0.2 for example), naturally default port 3000.

###### Destroy everything
```bash
$ ./aerolab cluster-destroy -f 1
$ ./aerolab cluster-destroy -f 1 -n amc
```
