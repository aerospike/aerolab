# Aerospike Grafana Integrated stack

## Usage help

```
% aerolab agi help
Usage:
  aerolab [OPTIONS] agi [command]

Available commands:
  list            List AGI instances
  create          Create AGI instance
  start           Start AGI instance
  stop            Stop AGI instance
  status          Show status of an AGI instance
  details         Show details of an AGI instance
  destroy         Destroy AGI instance
  change-label    Change instance name label
  run-ingest      Retrigger log ingest again (will only do bits that have not been done before)
  attach          Attach to an AGI Instance
  add-auth-token  Add an auth token to AGI Proxy - only valid if token auth type was selected
  help            Print help
```

## Create usage help

```
% aerolab agi create help
Usage:
  aerolab [OPTIONS] agi create [create-OPTIONS]

[create command options]
      -n, --name=                      AGI name (default: agi)
          --agi-label=                 friendly label
          --source-local=              get logs from a local directory
          --source-sftp-enable         enable sftp source
          --source-sftp-threads=       number of concurrent downloader threads (default: 6)
          --source-sftp-host=          sftp host
          --source-sftp-port=          sftp port (default: 22)
          --source-sftp-user=          sftp user
          --source-sftp-pass=          sftp password
          --source-sftp-key=           key to use for sftp login for log download, alternative to password
          --source-sftp-path=          path on sftp to download logs from
          --source-sftp-regex=         regex to apply for choosing what to download, the regex is applied on paths AFTER the sftp-path specification, not the whole path; start wih ^
          --source-s3-enable           enable s3 source
          --source-s3-threads=         number of concurrent downloader threads (default: 6)
          --source-s3-region=          aws region where the s3 bucket is located
          --source-s3-bucket=          s3 bucket name
          --source-s3-key-id=          (optional) access key ID
          --source-s3-secret-key=      (optional) secret key
          --source-s3-path=            path on s3 to download logs from
          --source-s3-regex=           regex to apply for choosing what to download, the regex is applied on paths AFTER the s3-path specification, not the whole path; start wih ^
          --proxy-ssl-enable           switch to enable TLS on the proxy
          --proxy-ssl-cert=            if not provided snakeoil will be used
          --proxy-ssl-key=             if not provided snakeoil will be used
          --proxy-max-inactive=        maximum duration of inactivity by the user over which the server will poweroff (default: 1h)
          --proxy-max-uptime=          maximum uptime of the instance, after which the server will poweroff (default: 24h)
          --ingest-timeranges-enable   enable importing statistics only on a specified time range found in the logs
          --ingest-timeranges-from=    time range from, format: 2006-01-02T15:04:05Z07:00
          --ingest-timeranges-to=      time range to, format: 2006-01-02T15:04:05Z07:00
          --ingest-custom-source-name= custom source name to disaplay in grafana
          --ingest-patterns-file=      provide a custom patterns YAML file to the log ingest system
          --ingest-log-level=          1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL (default: 4)
          --ingest-cpu-profiling       enable log ingest cpu profiling
          --plugin-cpu-profiling       enable CPU profiling for the grafana plugin
          --plugin-log-level=          1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL (default: 4)
          ...
```

## Foreword on firewalls in AWS and GCP

The AeroLab-managed firewalls pre-7.1.0 do not open ports 80 and 443 to the user. As such, access will not be allowed. In order to remedy this, these firewalls must be updated. To do this:

#### AWS
```
aerolab config aws lock-security-groups
```

This will add the required ports to the firewall rules, also locking the IP to the caller's IP. See `aerolab config aws lock-security-groups help` for other usage options.

#### GCP
```
aerolab config gcp lock-firewall-rules
```

This will add the required ports to the firewall rules, also locking the IP to the caller's IP. See `aerolab config gcp lock-firewall-rules help` for other usage options.

## Usage example

### Create an instance and process logs from a local directory

```
aerolab config backend -t docker
aerolab agi create --agi-label "useful label" --source-local /path/to/log/directory
```

### See ingest status

```
aerolab agi list
```

### See AGI status in more detail, including estimated completion time

```
aerolab agi status
```

### Connect to the instance from the shell

```
aerolab agi attach
cd /opt/agi/files
ls
```

### Connect to grafana and other web services

```
aerolab agi add-auth-token
```

Copy the token, and visit the URL provided in `aerolab agi list`.

### Add logs from an S3 source to the instance

```
aerolab agi run-ingest --source-s3-enable --source-s3-region eu-west-1 --source-s3-bucket test-log-bucket --source-s3-path path/to/logs --source-s3-key-id AKIAKEYID --source-s3-secret-key "000+my+secret+key"
```

### Add logs from an sftp source to the instance

```
aerolab agi run-ingest --source-sftp-enable --source-sftp-host test.example.com --source-sftp-port 22 --source-sftp-user example --source-sftp-pass secret --source-sftp-path path/to/logs
```

### Change instance friendly label

```
aerolab agi change-label -l "my new descriptive label"
```

### Destroy the instance

```
aerolab agi destroy
```

## Other features

### Automated instance stop

AGI instances are tuned for cost saving by default. The `--proxy-max-uptime` is by default set to 24 hours, and `--proxy-max-inactive` is set to 1 hour. When one of these limits is breached, the AGI instance will poweroff and enter a stopped state.

The `max-uptime` refers to the absolute maximum uptime of the instance, after which it will be stopped, regardless of whether it was being used. Setting this to `0` disables the feature.

The `max-inactive` feature will only stop the node if the following conditions are met:
* log ingest is not running (finished)
* it has been an hour since log ingest finished
* the user has not interacted with the instance for an hour by either being attached to it, using the web console, or interacting with grafana

Following an instance stop, it can be started again when it is needed with `aerolab agi start`.

### Ingest time ranges

At time, the aerospike logs provided may be large and span large dates, for example the last 6 months. In this case, log ingest may take a while. In order to speed this up, and provide a better overall experience, the user can select a time range for the statistic import to the storage database. This willl greatly improve the ingest speed as well as grafana responsiveness.

The example below shows selecting to ingest only the month of September (UTC) from local source directory:

```
aerolab agi create --agi-label "useful label" --source-local /path/to/logs --ingest-timeranges-enable --ingest-timeranges-from 2023-09-01T00:00:00Z00:00 --ingest-timeranges-to 2023-10-01T00:00:00Z00:00
```

### TLS support

AeroLab AGI can be secured by using a TLS connection on the proxy. To do this, simply add `--proxy-ssl-enable`. This will result in a self-generated `snakeoil` certificate being served to the clients, and `https` will be enabled. To provide own certificates, `--proxy-ssl-cert` and `--proxy-ssl-key` can also be specified.

### Setting defaults for frequently used parameters

If for example one wishes to always use SSL (snakeoil), the default can be provided as follows:
* find the parameter name with `aerolab config defaults |grep -i AGI |grep -i Create |grep -i SSL`
* apply a default to the parameter: `aerolab config defaults -k AGI.Create.ProxyEnableSSL -v true`

The same can be set for any parameter within AGI, or whole AeroLab for that matter.

Wildcards are also supported. For example to configure the local source to always be enabled to a specific directory for both the `create` and `run-ingest` commands (for creating and re-ingesting new logs):
* find the parameter names, then check their collective values: `aerolab config defaults -k '*.LocalSource'`
* apply a value to both parameters: `aerolab config defaults -k '*.LocalSource' -v /path/to/log/dir/`

This will automatically set the local source parameter to the configured value for both commands for all future runs on AeroLab AGI `create` and `run-ingest` commands.

To reset to defaults, simply run: `aerolab config defaults -k '*.LocalSource' -r`

### Regex support

AeroLab AGI logingest allows for further picking the logs files to download by regex for S3 and SFTP sources. Example:

```
... --source-sftp-path /aerospike/logs/ --source-sftp-regex '^[1-5].*'
```

This would first enter the folder `/aerospike/logs/` and then apply the specified regex on files and folders following. For example `/aerospike/logs/5-somenode.log` will be matched, while `/aerospike/logs/somenode-5.log` would not.

The same functionality as the example above applies to the S3 source, using the `--source-s3-regex` parameter.

### S3 credentials are optional

If S3 credentials are not provided, AGI logingest will attempt to use the system-wide credentials (be it the one in `.aws/credentials` or provided as an instance policy if in AWS).

### Multi-source support

At any point during `agi create` or `agi run-ingest` multiple sources can be provided. AeroLab AGI can handle pulling logs from local source, sftp and s3 at the same time. This does not have to be done in steps.

## Graphing logs from an AeroLab cluster

### Get logs

#### AWS/GCP
```
aerolab logs get -n myCluster -d ./myCluster-logs/ --journal
```

#### Docker
```
aerolab logs get -n myCluster -d ./myCluster-logs/ -p /var/log/aerospike.log
```

### Start AGI
```
aerolab agi create --name myAgi --agi-label "myCluster" --source-local ./myCluster-logs
```

## Sizing

The below if average data size for logs. This may vary.

### Default (data in memory)

```
10GB logs requires 14GB on disk and 17.4GB RAM
```

### No data in memory (--no-dim)

```
10GB logs requires 14GB on disk and 3.4GB RAM
```
