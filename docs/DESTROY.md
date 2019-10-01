#### Destroy cluster with default name (mydc)

```bash
$ ./aerolab cluster-stop

$ ./aerolab cluster-destroy
```

#### Stop and destroy 'testme' cluster in one command
```bash
$ ./aerolab cluster-destroy -f 1 -n testme
```

#### Stop and destroy only node 2 in cluster 'mycluster'
```bash
$ ./aerolab cluster-destroy -f 1 -n mycluster -l 2
```
