[Docs home](../../../README.md)

# Custom startup scripts


Both `cluster create` and `client create` support installing a startup script via their command line parameters.

### Cluster create

The available options are to install an early startup script and a late stop script.

The startup script will run before Aerospike starts and the stop script will run after Aerospike stops.

### Client create

The startup script will run every time a client machine is started.

### Custom options

Both clients and clusters will also attempt to start any and all scripts from `/opt/autoload`.

This directory may not exist by default, but once it is created, any files within it will be executed by bash on cluster/client startup.

These scripts start last, after everything else has started. Some `aerolab` features make use of this, such as `exporter` start script and `ams` client start scripts.
