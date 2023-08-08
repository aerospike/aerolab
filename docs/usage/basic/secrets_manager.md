# Installing and configuring the AWS Secrets Manager Agent

## Install Secret Agent

### Download

Head to the [artifacts download page](https://download.aerospike.com/artifacts/aerospike-secret-agent/) and download the relevant file. In this example we will be using version `1.0.0` on `ubuntu 22.04` on `x86_64` architecture.

### Installation

```
wget https://download.aerospike.com/artifacts/aerospike-secret-agent/1.0.0/aerospike-secret-agent_1.0.0-1ubuntu22.04_amd64.deb
aerolab files upload -n CLUSTERNAME aerospike-secret-agent_1.0.0-1ubuntu22.04_amd64.deb /root/aerospike-secret-agent_1.0.0-1ubuntu22.04_amd64.deb
aerolab attach shell -n CLUSTERNAME -l all --parallel -- dpkg -i /root/aerospike-secret-agent_1.0.0-1ubuntu22.04_amd64.deb
```

Note on CentOS/Amazon Linux: use `rpm -i file.rpm` or `yum localinstall file.rpm` instead of `dpkg -i file.deb` to install the secret agent on those platforms.

### Adjusting the config secret agent config file

There are multiple ways this can be achieved. Either the file can be downloaded using `aerolab files download`, then edited and reuploaded to all nodes using `aerolab files upload`, or alternatively it can be edited on first node and then synced to all other nodes.

#### Using download/edit/upload

This method has the advantage of allowing to creating a template configuration file and reuse it every time in the future.

```
aerolab files download -n CLUSTERNAME -l 1 /etc/aerospike-secret-agent/config.yaml secret-agent-config.yaml
vim secret-agent-config.yaml
aerolab files upload -n CLUSTERNAME secret-agent-config.yaml /etc/aerospike-secret-agent/config.yaml
```

#### Using in-place edit and sync

The quick setup-once method to see the file, edit it and sync it.

```
aerolab files edit -n CLUSTERNAME /etc/aerospike-secret-agent/config.yaml
aerolab files sync -n CLUSTERNAME -d CLUSTERNAME -p /etc/aerospike-secret-agent/config.yaml
```

## Configure Aerospike by live-patching the config file

It is possible to provide a template `aerospike.conf` during cluster creation with the `-o aerospike.conf` parameter. This will ship a configuration file that may have been prepared before deployment.

Should this not have been provided, an alternative method exists, by live-patching the configuration file, as seen below. Other parameters can be provided in the same manner.

The below examples configure fetching feature key file from the AWS Secrets Manager using named Secret and Value.

### Using address-port

```
aerolab conf adjust -n CLUSTERNAME set service.secrets-address-port "127.0.0.1 3005"
aerolab conf adjust -n CLUSTERNAME set service.feature-key-file secrets:TestingSecret:FeatureKey
aerolab aerospike restart -n CLUSTERNAME
```

### Using a domain socket

```
aerolab conf adjust -n CLUSTERNAME set service.secrets-uds-path /path/to/uds.sock
aerolab conf adjust -n CLUSTERNAME set service.feature-key-file secrets:TestingSecret:FeatureKey
aerolab aerospike restart -n CLUSTERNAME
```

## Links

More information can be found at the following URLs:
* [Configure aerospike to use secret agent](https://docs.aerospike.com/server/operations/configure/security/secrets)
* [Configure the secret agent](https://docs.aerospike.com/tools/secret-agent/aws-guide)
