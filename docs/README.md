# Usage instructions

* [Getting Started](GETTING_STARTED.md)
* [Aerolab help commands](USING_HELP.md)
* [Usage Examples](usage/README.md)
* [Setting up Aerospike clusters on AWS with AeroLab](aws/README.md)
  * [Using the partitioner](aws/partitioner/README.md)
* [Deploy Clients with AeroLab](usage/CLIENTS.md)
  * [Deploy a VS Code Client Machine](usage/vscode.md)
  * [Deploy a Trino server](usage/trino.md)
  * [Deploy a Jupyter Notebook Machine](usage/jupyter.md)
  * [Deploy an ElasticSearch connector](usage/elasticsearch.md)
  * [Deploy a Rest Gateway](usage/restgw.md)
* [Rest API](usage/REST.md)
* [Useful scripts](../scripts/README.md)
  * [Deploy an LDAP server](../scripts/aerolab-ldap/README.md)
  * [Build an Aerospike cluster with LDAP and TLS](../scripts/aerolab-buildenv/README.md)
* [Deploying a full stack - servers, asbench client and AMS monitoring - Docker](usage/fullstack.md)
* [Deploying a full stack - servers, asbench client and AMS monitoring - AWS](usage/fullstack_aws.md)

## Configuration Generator

AeroLab can generate basic `aerospike.conf` files by running: `aerolab conf generate`
