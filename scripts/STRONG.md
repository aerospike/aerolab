# Automated deploy of SC cluster

Notes:
  * This script is designed to allow quick deployment of an SC cluster.<br>
    It performs all the required steps to set the roster.


## First clone this repo

```
Via Web
% git clone https://github.com/citrusleaf/aerolab.git

Via SSH
% git clone git@github.com:citrusleaf/aerolab.git
```

## Enter this directory
```
% cd aerolab/scripts
```

## Usage

```
% ./deploy-sc --help

deploy-sc.sh
Usage :-

 -c|--nodes <node_count> default=3 : Range 1-9
 -r|--replication <replication_factor> default=2 : Range 1-3
 -n|--namespace <namespace> default=bar
 -v|--ver <Aerospike Version> default=latest
 -l|--labname <name_of_cluster> default=test_sc
 -s|--sc <true/false> default=true
 -?|-h|--help This Messaage

```
With the defaults, a 3 node cluster with the latest version of Aerospike will be deployed<br>