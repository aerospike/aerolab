# AWS EFS and GCP extra volumes for cluster and clients

AeroLab supports basic creation and management of EFS volumes in AWS and extra disk volumes in GCP, as well as automated attachment, mounting and `MountPoint` creation.

## Managing volumes

```
% aerolab volume help
Usage:
  aerolab [OPTIONS] volume [command]

Global Options:
  --beep     cause the terminal to beep on exit; if specificied multiple times, will be once on success and >1 on failure
  --beepf    like beep, but does not trigger beep on success, only failures

Available commands:
  create  Create a volume
  list    List volumes
  mount   Mount a volume on a node
  delete  Delete a volume
  grow    GCP only: grow a volume; if the volume is not attached, the filesystem will not be resized automatically
  detach  GCP only: detach a volume for an instance
  help    Print help
```

In order to see the list of available parameters for a given command, append `help` to the end of the command. For example `aerolab volume create help`.

Volumes may be created, mounted and deleted from this interface.

## Volume at cluster/client creation

AeroLab supports automated creation of volumes and mounting at cluster/client creation time as well. Note that these will not be removed when cluster is destroyed, and `aerolab volume delete` must be separately executed.

The following parameters are exposed to user for the `cluster create` and `client create` commands in AWS backend mode:

Parameter | Description
--- | ---
`--aws-efs-mount=` | mount EFS volume; format: NAME:EfsPath:MountPath OR use NAME:MountPath to mount the EFS root
`--aws-efs-create` | set to create the EFS volume if it doesn't exist
`--aws-efs-onezone` | set to force the volume to be in one AZ only; half the price for reduced flexibility with multi-AZ deployments
`--aws-efs-expire` | if a volume has not had a `mount` executed against it using aerolab for this amount of time, expire the volume
`--gcp-vol-mount` | attach and mount GCP volume; format: NAME:MountPath
`--gcp-vol-create` | create volume if it does not exist
`--gcp-vol-expire` | if a volume has not had a `mount` executed against it using aerolab for this amount of time, expire the volume
`--gcp-vol-desc` | set volume disk description field
`--gcp-vol-label` | set a label; format: key=value; can be specified multiple times

If a volume is created at cluster creation, it gets mounted after the installation, but before Aerospike is started.
