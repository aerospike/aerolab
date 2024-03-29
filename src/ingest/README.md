# LogIngest

## Flow

```
init->download->unpack->preprocess->dbconnect->processlogs->processcf
init->(progress-tracker)
```

## Components

name | description
--- | ---
init | initialize a new ingest system
download | download files from s3/sftp/aerolab cluster
unpack (+compression) | unpack the files recursively
preprocess | identify files as logs/collectinfo/other, rename and store collectinfo, rename and store logs according to discovered cluster name and node ID; also handle custom log formats (json, etc)
preprocess-special | special (tab/json) format handler
dbconnect | connect to the backend database
processlogs | actually process the logs, inserting the statistics into the aerospike database
processcf | find out which node the collectinfo was gathered on, and rename the files accordingly to match log prefixes
trackprogress | track progress in a json file and to stderr/out
enum | enumerate through the dirty directory; used by multiple components
logstream | handle logs as singular streams (one per log file) by actually extracting patterns and timestamp and returning the result