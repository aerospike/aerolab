# Aerospike Grafana Integrated stack

AWS EFS volumes can be auto-created and attached to the instances to provide permanent storage for the ingested log data when instances are terminated.

Additionally, instances can be terminated automatically when their max age or max inactivity period is reached.

## EFS volumes

Add the following parameters to the `aerolab agi create` command to also provision and attach an EFS volume in AWS.

Parameter | Description
--- | ---
`--aws-with-efs` | Enable AWS EFS for storage
`--aws-terminate-on-poweroff` | Terminate the instance instead of stopping it when maximum inactivity period or maximum age are reached
`--aws-efs-multizone` | Enable multi-zone support (if needed) for the volume at creation; this is 2x the price of single zone
`--aws-efs-name=` | Optionally name the EFS volume a different name than the AGI name
`--no-config-override` | Optionally specify this parameter to ensure existing configuration doesn't get overriden if it already exists on the EFS volume; this is useful when restarting AGI with an existing EFS volume with data and prior configuration; if a particular (or any) configuration doesn't exist, it will be created

If a volume already exists, it simply gets reattached.

## Listing

To list all EFS volumes, see `aerolab volume list` or `aerolab inventory list`.

## Deleting

To delete an EFS volume, see `aerolab volume delete`.