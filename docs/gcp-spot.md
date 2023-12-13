# GCP Spot Instances

AeroLab supports requesting spot instances from GCP instead of the default on-demand type.

This is subject to capacity availability. This may fail if capacity is not available. Your instances may be terminated by AWS without notice if they require the spare capacity.

Prices for spot instances vs the on-demand can be checked using `aerolab inventory instance-types`. In some cases spot instances cost as much as on-demand, in others they cost as little as 15% of the original price. Prices change without notice. AeroLab updates the instance-type and price cache every 24 hours.

## Requesting spot instances

In order to request a spot instance, add `--gcp-spot-instance` to the `cluster create` or `client create` command.
