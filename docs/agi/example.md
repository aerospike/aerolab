# AGI Usage examples

## Docker, GCP, AWS without EFS

### Local directory source with slack notification direct to user

```
aerolab agi create -n myagi --agi-label "some useful label" --source-local /path/to/logs --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

### Sftp source with slack notification direct to user

```
aerolab agi create -n myagi --agi-label "some useful label" --source-sftp-enable --source-sftp-host=sftp.example.com --source-sftp-user=USERNAME '--source-sftp-pass=PASSWORD' --source-sftp-path=/Path/To/Logs --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

### S3 source with slack notification direct to user

```
aerolab agi create -n myagi --agi-label "some useful label" --source-s3-enable --source-s3-region bucket-region --source-s3-bucket BUCKET-NAME --source-s3-path /Path/To/Logs --source-s3-key-id AKIAKEYID --source-s3-secret-key "s3+key" --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

## AWS with EFS

### Local directory source with slack notification direct to user

```
aerolab agi create -n myagi --aws-with-efs --aws-terminate-on-poweroff --agi-label "some useful label" --source-local /path/to/logs --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

### Sftp source with slack notification direct to user

```
aerolab agi create -n myagi --aws-with-efs --aws-terminate-on-poweroff --agi-label "some useful label" --source-sftp-enable --source-sftp-host=sftp.example.com --source-sftp-user=USERNAME '--source-sftp-pass=PASSWORD' --source-sftp-path=/Path/To/Logs --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

### S3 source with slack notification direct to user

```
aerolab agi create -n myagi --aws-with-efs --aws-terminate-on-poweroff --agi-label "some useful label" --source-s3-enable --source-s3-region bucket-region --source-s3-bucket BUCKET-NAME --source-s3-path /Path/To/Logs --source-s3-key-id AKIAKEYID --source-s3-secret-key "s3+key" --notify-slack-channel=...USERID... --notify-slack-token=xoxb-...
```

### Destroy the instance and later recreate it using the same efs volume and config

```
aerolab agi destroy -n myagi
aerolab agi create -n myagi --aws-with-efs --aws-terminate-on-poweroff --no-config-override
```

## Operating

Command | Description
--- | ---
`aerolab agi list` | get list of AGI instances and volumes, as well as percent ingest complete and the web access URL
`aerolab agi status -n myagi` | get detailed report of the AGI instance process
`aerolab agi sstop -n myagi` | stop the instance (stop paying for instance, pay just for storage)
`aerolab agi start -n myagi` | start the instance again
`aerolab agi change-label -n myagi` | change the friendly label
`aerolab agi run-ingest -n myagi` | retrigger log ingest (for example if more logs appeared in the sftp/s3 source location, or another location is specified)
`aerolab agi attach -n myagi` | attach to the instance terminal shell
`aerolab agi add-auth-token -n myagi` | generate an authentication token needed to access the instance web interfaces
`aerolab agi destroy -n myagi` | destroy the instance
`aerolab agi delete -n myagi` | destroy the instance and remove the EFS volume

## Notes

### Instance sizes in cloud instances

In AWS, default instance size is `r5a.xlarge` and in GCP it is `e2-highmem-4`. These can be adjusted to provide more RAM using `--instance-type ...` on AWS and `--instance ...` on GCP. To see a list of available instances and their prices, run `aerolab inventory instance-types`.

### AWS spot instances

When using AWS with EFS, it may be beneficial to run the instances as spot-instances. This reduces the usage prices. An instance can be recreated as on-demand at a later time if required, and it will just continue where it left off, using the EFS-backed storage. To do this, add `--aws-spot-instance`.
