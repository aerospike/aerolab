# Upload/download files

### Make a 3-node cluster, forgetting to use a template conf file
```bash
$ ./aerolab make-cluster -c 3
Nov 06 10:08:46+0000 AERO-LAB[15412]: INFO     Performing sanity checks
Nov 06 10:08:46+0000 AERO-LAB[15412]: INFO     Checking if version template already exists
Nov 06 10:08:46+0000 AERO-LAB[15412]: INFO     Checking aerospike version
Nov 06 10:08:54+0000 AERO-LAB[15412]: INFO     Starting deployment
Nov 06 10:09:00+0000 AERO-LAB[15412]: INFO     Done
```

### Download aerospike.conf from node 1 in mydc, save to a file called myfile.conf
```bash
$ ./aerolab download -i /etc/aerospike/aerospike.conf -o myfile.conf
$ ls
CA          aerolab     myfile.conf templates
```

### After making changes in myfile.conf, reupload it to all nodes
```bash
$ ./aerolab upload -i myfile.conf -o /etc/aerospike/aerospike.conf
```

### After making more changes to myfile.conf, let's upload it to just node 2 (non-uniform config)
```bash
$ ./aerolab upload -l 2 -i myfile.conf -o /etc/aerospike/aerospike.conf
```

### Restart aerospike to see the damage
```bash
$ ./aerolab restart-aerospike
```
