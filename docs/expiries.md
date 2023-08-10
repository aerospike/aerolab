# Automatic system expiry for cloud-based instances.

## Creating clusters

AeroLab version 7.0.0 and above have an automated expiry system for cloud-based instances.

By default, all instances expire after 30 hours. This can be modified using the `--aws-expire` parameter for `AWS` and `--gcp-expire` parameter for `GCP`. For example to expire after 50 hours:

```
aerolab cluster create -c 5 -n mycluster --aws-expire 50h ...
```

Available settings are `smh` denoting `seconds, minutes, hours`. Note that the expiry exact time will be affected by how often the expiry system scheduler runs.

A special option of `0` disables expiries for a given cluster/client.

## Adjusting expiries for existing clusters and clients

Expiries can be adjusted as follows:

```
aerolab cluster add expiry -n mycluster --expire 50h
aerolab client configure expiry -n myclient --expire 50h
```

The 50 hours in the above example is counted from the point of when the command is run.

## Adjusting and handling the expiry system

Commands to administer the expiry system have been added to `aerolab config aws|gcp`. Run `aerolab config aws help` and `aerolab config gcp help` for more details.

The following three features have been added:
* `expiry-install` - this runs automatically as well during cluster creation if the cluster expiry is set to non-zero
* `expiry-remove` - removes the expiry system from AWS
* `expiry-run-frequency` - adjust the frequency at how often the expiry lambda runs to check for expires clusters. Default is 10 minutes.

Run `aerolab config aws COMMAND help`, where `COMMAND` is one of the above 3 commands, for more information on usage.
