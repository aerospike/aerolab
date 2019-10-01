# Make java client
```bash
$ ./aerolab make-client -l java -n myjava
...
Example:
cd /root/java/aerospike-client-java-*/benchmarks
mvn package
./run_benchmarks -h <ip_of_cluster_node>
```

### Connect and run java client
```bash
$ ./aerolab node-attach -n myjava
root@4b47d3ff291c:/# cd /root/java/aerospike-client-java-4.2.2/benchmarks/
root@4b47d3ff291c:~/java/aerospike-client-java-4.2.2/benchmarks# mvn package
...

./run_benchmarks -h 172.16.0.3
...
```
