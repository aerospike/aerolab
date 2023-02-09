# Deploying the ElasticSearch connector for Aerospike

## Create an aerospike cluster

```bash
# aws
aerolab cluster create -c 2 -I t3a.medium -n mycluster
# docker
aerolab cluster create -c 2 -n mycluster
```

## Create an elasticsearch cluster, with aerospike connector preinstlaled on each node

```bash
# aws
aerolab client create elasticsearch -c 2 -I t3a.large -n myes
# docker - limit each ES node to 4g ram
aerolab client create elasticsearch -c 2 -r 4 -n myes
```

## Configure the aerospike cluster to ship records to the connector

```bash
aerolab xdr connect -S mycluster -D myes -c
```

## Testing

### Insert 2000 test records

```bash
aerolab data insert -n mycluster -a 1 -z 2000
```

### Query elasticsearch

Visit the elasticsearch IP, as seen from `aerolab client list` command, port `9200` in your browser to explore the data from ElasticSearch.

For best results, use FireFox, as it has a builtin `JSON` format explorer as well as support for self-signed certificates once the warning is acknowledged.

Example URLs:
 * show summary: https://ELASTICIP:9200/test/_search
 * show up to 1000 records: https://ELASTICIP:9200/test/_search?size=1000
 * show records where bin name `mybin` has value `binvalue`: https://ELASTICIP:9200/test/_search?q=mybin:binvalue
 * show up to 1000 records from set name `myset`: https://ELASTICIP:9200/test/_search?size=1000&q=metadata.set:myset
