# AWS Spot Instances

AeroLab supports requesting spot instances from AWS instead of the default on-demand type.

This is subject to capacity availability. This may fail if capacity is not available. Your instances may be terminated by AWS without notice if they require the spare capacity.

Prices for spot instances vs the on-demand can be checked using `aerolab inventory instance-types`. In some cases spot instances cost as much as on-demand, in others they cost as little as 15% of the original price. Prices change without notice. AeroLab updates the instance-type and price cache every 24 hours.

## Requesting spot instances

In order to request a spot instance, add `--aws-spot-instance` to the `cluster create` or `client create` command.

## Behavior modifier

The type of spot instance that is requested is automatically adjusted based on the `--aws-terminate-on-poweroff` parameter.

If this is set, AWS will terminate and remove the instance should a `poweroff` or `shutdown` command be executed on the instance itself. In this mode, cluster stopping and starting is not allowed by AWS (aerospike may still be stopped and started). The spot instance type is `one-off`. If capacity is not available, the spot instance request will be terminated.

If not set, AWS will create a `persistent` spot instance type. In this type, the spot instance can be started and stopped as normal. It will also respond with a stopping behavior should the `poweroff` type command be run on the instance itself. In this mode, if capacity becomes unavailable, AWS will suspend the instance and resume it once capacity becomes available again.
