# Custom start scripts

Both `cluster create` and `client create` support installing a start script via their command line parameters.

## Cluster create

The available options are to install an early start script and a late stop script.

The start script will run before aerospike starts and the stop script will run after aerospike stops.

## Client create

The startup script will run every time a client machine is started.

## Custom options

Both client and clusters will also attempt to start any and all scripts from `/opt/autoload`.

This directory by default may not exist, but once it is created, any files in there will be executed by bash on cluster/client start.

These scripts start last, after everything else has started. Some `aerolab` features make use of this, such as `exporter` start script and `ams` client start scripts.
