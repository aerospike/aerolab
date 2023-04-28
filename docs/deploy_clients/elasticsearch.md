[Docs home](../../README.md)

# Deploy the Elasticsearch connector for Aerospike


1. Create an Aerospike cluster:

```bash
# aws
aerolab cluster create -c 2 -I t3a.medium -n mycluster
# docker
aerolab cluster create -c 2 -n mycluster
```

2. Create an Elasticsearch cluster, with Aerospike connector preinstalled on each node:

```bash
# aws
aerolab client create elasticsearch -c 2 -I t3a.large -n myes
# docker - limit each ES node to 4g ram
aerolab client create elasticsearch -c 2 -r 4 -n myes
```

3. Configure the Aerospike cluster to ship records to the connector:

```bash
aerolab xdr connect -S mycluster -D myes -c
```

### Testing

1. Insert 2000 test records:

```bash
aerolab data insert -n mycluster -a 1 -z 2000
```

2. Query Elasticsearch:

   - Run `aerolab client list` to get your Elasticsearch server's IP address.
   - Navigate to the Elasticsearch IP address, port `9200` in your browser to explore the data from Elasticsearch.

For best results, use the FireFox web browser. It has a built-in JSON format explorer,
as well as support for self-signed certificates once the warning is acknowledged.

Example URLs:

- Show summary: `https://ELASTICIP:9200/test/_search`
- Show up to 1000 records: `https://ELASTICIP:9200/test/_search?size=1000`
- Show records where bin name `mybin` has value `binvalue`: `https://ELASTICIP:9200/test/_search?q=mybin:binvalue`
- Show up to 1000 records from set name `myset`: `https://ELASTICIP:9200/test/_search?size=1000&q=metadata.set:myset`

Replace *`ELASTICIP`* with the IP address of your Elasticsearch server.
