# How to get asbenchmark working

## Summary

Asbenchmark needs java - which is 500MB extra per node container - which is why we don't preinstall java

Asbenchmark ships with aerospike-tools, but won't run until you install java

## How to get it working

#### Deploy 3-node cluster
```bash
$ ./aerolab make-cluster -c 3
Nov 06 19:24:59+0000 AERO-LAB[23973]: INFO     Performing sanity checks
Nov 06 19:24:59+0000 AERO-LAB[23973]: INFO     Checking if version template already exists
Nov 06 19:24:59+0000 AERO-LAB[23973]: INFO     Checking aerospike version
Nov 06 19:25:07+0000 AERO-LAB[23973]: INFO     Starting deployment
Nov 06 19:25:12+0000 AERO-LAB[23973]: INFO     Done
```

#### Attach to node one and install java
```bash
$ ./aerolab node-attach -- apt-get -y install default-jre
[...]
done.
```

#### Asbenchmark works on that node
```bash
$ ./aerolab node-attach
root@04d0f4b47fa1:/# asbenchmark
Benchmark: 127.0.0.1 3000, namespace: test, set: testset, threads: 16, workload: READ_UPDATE
read: 50% (all bins: 100%, single bin: 0%), write: 50% (all bins: 100%, single bin: 0%)
keys: 100000, start key: 0, transactions: 0, bins: 1, random values: false, throughput: unlimited
read policy:
    socketTimeout: 0, totalTimeout: 0, maxRetries: 2, sleepBetweenRetries: 0
    consistencyLevel: CONSISTENCY_ONE, replica: SEQUENCE, reportNotFound: false
write policy:
    socketTimeout: 0, totalTimeout: 0, maxRetries: 0, sleepBetweenRetries: 0
    commitLevel: COMMIT_ALL
Sync: connPoolsPerNode: 1
bin[0]: integer
debug: false
2018-11-06 19:27:20.996 INFO Thread main Add node BB9020011AC4202 127.0.0.1 3000
2018-11-06 19:27:21.014 INFO Thread main Add node BB9040011AC4202 172.17.0.4 3000
2018-11-06 19:27:21.015 INFO Thread main Add node BB9030011AC4202 172.17.0.3 3000
2018-11-06 19:27:21.945 write(tps=4845 timeouts=0 errors=0) read(tps=4907 timeouts=0 errors=0) total(tps=9752 timeouts=0 errors=0)
2018-11-06 19:27:22.947 write(tps=6898 timeouts=0 errors=0) read(tps=6764 timeouts=0 errors=0) total(tps=13662 timeouts=0 errors=0)
```
