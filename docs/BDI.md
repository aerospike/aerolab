# How to replicate the strong consistency and rack awareness plus node-id like the BDI

#### Create directory bdi
```
$ mkdir bdi
```

#### Enter the directory
```
$ cd bdi
```

#### Create a conf file
```
$ cat <<'EOF' > bdi.conf
[Common]
Username="DOWNLOAD_USER"
Password="DOWNLOAD_PASSWORD"
ChDir="/path/to/folder/bdi"
EOF
```

#### copy features.conf and tls-strong.conf to bdi folder

#### Create a 6-node cluster
```
$ aerolab make-cluster --config=bdi.conf -n bdi -m mesh -c 6 -o tls-strong.conf -f features.conf
Nov 30 13:13:49+0000 AERO-LAB[37692]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
Nov 30 13:13:49+0000 AERO-LAB[37692]: INFO     Checking if version template already exists
Nov 30 13:13:49+0000 AERO-LAB[37692]: INFO     Checking aerospike version
Nov 30 13:13:59+0000 AERO-LAB[37692]: INFO     Downloading aerospike tarball
Nov 30 13:14:06+0000 AERO-LAB[37692]: INFO     Creating template
Nov 30 13:14:41+0000 AERO-LAB[37692]: INFO     Starting deployment
Nov 30 13:14:57+0000 AERO-LAB[37692]: INFO     Done
```

#### Install gnu-sed because osx sed is useless
```
$ brew install gnu-sed
```

#### Configure the 6 nodes with node-id and rack-id

The lines below work for first three nodes and then the next three. We download the config file, use sed to add node-id and rack-id and upload the config file back.

```
$ for i in 1 2 3
do aerolab download --config=bdi.conf -n bdi -l ${i} -i /etc/aerospike/aerospike.conf -o aerospike.conf
gsed "s/proto-fd-max/node-id A${i}\nproto-fd-max/g" aerospike.conf > newconf.conf
gsed "s/replication-factor 2/replication-factor 2\nrack-id 1/g" newconf.conf > newconf2.conf
aerolab upload --config=bdi.conf -n bdi -l ${i} -i newconf2.conf -o /etc/aerospike/aerospike.conf
done

$ for i in 4 5 6
do aerolab download --config=bdi.conf -n bdi -l ${i} -i /etc/aerospike/aerospike.conf -o aerospike.conf
gsed "s/proto-fd-max/node-id B${i}\nproto-fd-max/g" aerospike.conf > newconf.conf
gsed "s/replication-factor 2/replication-factor 2\nrack-id 2/g" newconf.conf > newconf2.conf
aerolab upload --config=bdi.conf -n bdi -l ${i} -i newconf2.conf -o /etc/aerospike/aerospike.conf
done
```

#### Setup TLS
```
$ aerolab gen-tls-certs --config=bdi.conf -n bdi
Nov 06 09:25:59+0000 AERO-LAB[13764]: INFO     Generating TLS certificates and reconfiguring hosts
Nov 06 09:26:04+0000 AERO-LAB[13764]: INFO     Done
```

#### Restart aerospike
```
$ aerolab restart-aerospike --config=bdi.conf -n bdi
```

#### Set the roster
```
$ aerolab node-attach --config=bdi.conf -n bdi -l 1 -- asadm --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333 -e "asinfo -v 'roster-set:namespace=bar;nodes=B5@2,B4@2,A3@1,A2@1,A1@1'"
Seed:        [('127.0.0.1', 3000, None)]
Config_file: /root/.aerospike/astools.conf, /etc/aerospike/astools.conf
a967d1eaa057:3000 (172.17.0.2) returned:
ok

172.17.0.3:3000 (172.17.0.3) returned:
ok

172.17.0.4:3000 (172.17.0.4) returned:
ok

172.17.0.7:3000 (172.17.0.7) returned:
ok

172.17.0.6:3000 (172.17.0.6) returned:
ok

172.17.0.5:3000 (172.17.0.5) returned:
ok
```

#### Recluster
```
$ aerolab node-attach --config=bdi.conf -n bdi -l 1 -- asadm --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333 -e "asinfo -v 'recluster:namespace=bar'"
Seed:        [('127.0.0.1', 3000, None)]
Config_file: /root/.aerospike/astools.conf, /etc/aerospike/astools.conf
a967d1eaa057:3000 (172.17.0.2) returned:
ignored-by-non-principal

172.17.0.3:3000 (172.17.0.3) returned:
ignored-by-non-principal

172.17.0.4:3000 (172.17.0.4) returned:
ignored-by-non-principal

172.17.0.7:3000 (172.17.0.7) returned:
ok

172.17.0.6:3000 (172.17.0.6) returned:
ignored-by-non-principal

172.17.0.5:3000 (172.17.0.5) returned:
ignored-by-non-principal
```

#### See the roster
```
$ aerolab node-attach --config=bdi.conf -n bdi -l 1 -- asadm --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333 -e "asinfo -v 'roster:namespace=bar'"
Seed:        [('127.0.0.1', 3000, None)]
Config_file: /root/.aerospike/astools.conf, /etc/aerospike/astools.conf
3612eeaacb24:3000 (172.17.0.2) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1

172.17.0.3:3000 (172.17.0.3) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1

172.17.0.4:3000 (172.17.0.4) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1

172.17.0.7:3000 (172.17.0.7) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1

172.17.0.6:3000 (172.17.0.6) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1

172.17.0.5:3000 (172.17.0.5) returned:
roster=B2,B1,A3,A2,A1:pending_roster=B2,B1,A3,A2,A1:observed_nodes=B6@2,B5@2,B4@2,A3@1,A2@1,A1@1
```

## Some practice changes within the cluster

### Java benchmark
```
$ aerolab cluster-list |grep aero-bdi_1
b5963a963e33        aero-ubuntu_18.04:4.5.0.4   "/bin/bash"         33 minutes ago      Up 33 minutes                           aero-bdi_1
aero-bdi_1 | 172.17.0.2

$ aerolab make-client -n bdi-java -l java
[...]

$ aerolab copy-tls-certs -s bdi -d bdi-java
Jan 18 09:22:49+0000 AERO-LAB[42058]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible

$ aerolab node-attach -n bdi-java

root@97d1c262377e:/# keytool -import -alias tls1 -file /etc/aerospike/ssl/tls1/cacert.pem -keystore /key.store -storePass aerospike
[...]
Trust this certificate? [no]:  yes
Certificate was added to keystore

root@97d1c262377e:/# cd /root/java/aerospike-client-java-*/benchmarks

root@97d1c262377e:/# mvn package

root@97d1c262377e:/# cd target

root@97d1c262377e:/# java -Djavax.net.ssl.trustStore=/key.store -Djavax.net.ssl.trustStorePassword=aerospike -jar aerospike-benchmarks-4.2.3-jar-with-dependencies.jar -h "172.17.0.2:tls1:4333" -tlsEnable
```
### Stop nodes - one per rack, and start them back up
```
$ aerolab stop-aerospike --config=bdi.conf -n bdi -l 2,4
$ aerolab start-aerospike --config=bdi.conf -n bdi -l 2,4
```

### Get all the logs from all the nodes
```
$ mkdir logs
$ aerolab get-logs -n bdi -o logs/
Jan 18 09:17:22+0000 AERO-LAB[41916]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
$ cd logs
$ ls
```
