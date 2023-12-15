# AGI Monitor

AeroLab has a service called `agi-monitor` for deployments in GCP and AWS. The monitor listens to events coming from `agi` instances and serves the following purposes:
* if there is an event notifying of `spot-instance-termination` by the cloud provider due to capacity issues, the monitor will recreate the instance as an `on-demand` automatically to ensure service continuity
* on all other progress events, the monitor checks log sizes against available RAM and estimated required instance size; if the instance is predicted to be too small, the monitor will terminate that instance and start it as a bigger one accordingly; agi is smart enough to continue where it left off
* on GCP, the monitor also keeps track of disk space usage and requirement, and dynamically grows the volume as needed to allow `agi` to continue running; on AWS this is not an issue as EFS volumes are unlimited in size

## AWS Prerequisite

In AWS, an IAM instance policy must be created first, in order to allow the instance to assume the required role to perform it's actions. To do that, head to the AWS Web Console, navigate to the `IAM` service, and set it up as seen [here](aws-create-policy-step-1.png) and [here](aws-create-policy-summary.png).

## Requirements

In order for the AGI monitor to be able to perform sizing or spot-to-ondemand rotation, the AGI instances must be started with extra volumes. On GCP that is `--gcp-with-vol` and on AWS that is `--aws-with-efs` parameter when creating AGI instances.

## Usage

The easiest way to deploy, use and have running AGI monitor is by telling the `agi create` command to automatically work out if a monitor exists, and, if not, create one.

Below example starts an AGI instance, processing local logs, using EFS and an AGI monitor. Add `--aws-spot-instance` or `--gcp-spot-instance` to request AGI spot instances.

```
aerolab agi create --source-local ./logs/ --with-monitor --aws-with-efs
```

## Advanced usage

The AGI monitor can be manually deployed, controlled and fine-tuned with `aerolab agi monitor` commands. Simply run `aerolab agi monitor help` for more information. Likewise, run `aerolab agi monitor create help`, and so forth.

## Troubleshooting

The monitor logs can be checked as `aerolab client attach -n agimonitor -- journalctl -u agimonitor -f`. The monitor can be normally attached to, and interacted with, as it is simply an AeroLab client machine.

## Removing the AGI monitor

The monitor is deployed as a standard AeroLab client with a client type of `agimonitor`. As such, it is visible from `aerolab client list` and can be removed with `aerolab client destroy -n agimonitor -f` command.
