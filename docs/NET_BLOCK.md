# Block and unblock networks

### Make xdr cluster with 5 nodes on source and 4 on destination
```bash
$ ./aerolab make-xdr-clusters -c 5 -a 4
Nov 06 09:58:54+0000 AERO-LAB[14542]: INFO     --> Deploying dc1
Nov 06 09:58:54+0000 AERO-LAB[14542]: INFO     Performing sanity checks
Nov 06 09:58:54+0000 AERO-LAB[14542]: INFO     Checking if version template already exists
Nov 06 09:58:54+0000 AERO-LAB[14542]: INFO     Checking aerospike version
Nov 06 09:59:01+0000 AERO-LAB[14542]: INFO     Starting deployment
Nov 06 09:59:06+0000 AERO-LAB[14542]: INFO     Done
Nov 06 09:59:06+0000 AERO-LAB[14542]: INFO     --> Deploying dc2
Nov 06 09:59:06+0000 AERO-LAB[14542]: INFO     Performing sanity checks
Nov 06 09:59:06+0000 AERO-LAB[14542]: INFO     Checking if version template already exists
Nov 06 09:59:06+0000 AERO-LAB[14542]: INFO     Checking aerospike version
Nov 06 09:59:16+0000 AERO-LAB[14542]: INFO     Starting deployment
Nov 06 09:59:25+0000 AERO-LAB[14542]: INFO     Done
Nov 06 09:59:25+0000 AERO-LAB[14542]: INFO     --> xdrConnect running
Nov 06 09:59:25+0000 AERO-LAB[14542]: INFO     XdrConnect running
Nov 06 09:59:27+0000 AERO-LAB[14542]: INFO     Done, now restart the source cluster for changes to take effect.

$ ./aerolab restart-aerospike -n dc1
```

### Block network connections

###### Block dc1 nodes 1,2 to dc2 nodes 1,2,3 port 3000 (xdr half-broken), using reject
```bash
$ ./aerolab net-block -s dc1 -l 1,2 -d dc2 -i 1,2,3
Nov 06 10:02:40+0000 AERO-LAB[14813]: INFO     Starting net blocker
Nov 06 10:02:42+0000 AERO-LAB[14813]: INFO     Done
```

###### Check block list
```bash
$ ./aerolab net-list
RULES:
dc1_1 => dc2_1:3000 REJECT (rule:INPUT  on:dc2_1)
dc1_1 => dc2_2:3000 REJECT (rule:INPUT  on:dc2_2)
dc1_1 => dc2_3:3000 REJECT (rule:INPUT  on:dc2_3)
dc1_2 => dc2_1:3000 REJECT (rule:INPUT  on:dc2_1)
dc1_2 => dc2_2:3000 REJECT (rule:INPUT  on:dc2_2)
dc1_2 => dc2_3:3000 REJECT (rule:INPUT  on:dc2_3)
```

###### Block dc1 node 1 from communicating to dc1 node 2 ports 3001, 3002 (drop packets)
```bash
$ ./aerolab net-block -s dc1 -l 1 -d dc1 -i 2 -t drop -p 3001,3002
Nov 06 10:05:50+0000 AERO-LAB[15003]: INFO     Starting net blocker
Nov 06 10:05:51+0000 AERO-LAB[15003]: INFO     Done
```

###### Unblock xdr
```bash
$ ./aerolab net-unblock -s dc1 -l 1,2 -d dc2 -i 1,2,3
Nov 06 10:06:24+0000 AERO-LAB[15019]: INFO     Starting net blocker
Nov 06 10:06:25+0000 AERO-LAB[15019]: INFO     Done

```