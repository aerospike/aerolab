[Docs home](../README.md)

# AWS Firewall

## Basic use

By default AeroLab creates 2 internal managed firewall rules and one external:
* the `AeroLabServer-` group is used for inter-node communication
* the `AeroLabClient-` group is used for inter-node communication
* the `Aerolab-` group is used for communication with the node from the internet

Basic usage does not require any special setup, as AeroLab will create, manage and support the firewall rules and related instance tags.

The rules created will be automatically locked to use the caller's (the person calling the `aerolab` command) IP address for security reasons. Should the caller's IP change, the firewall rules will need updating to allow continued access. To do so, simply run:
```
aerolab config aws lock-security-groups
```

In order to remove the AeroLab managed internal firewall rules, `aerolab config aws delete-security-groups -i` command exists.

## Listing the rules

Execute `aerolab config aws list-security-groups` to list all firewall rules.

## Advanced use

Unless a security group ID is manually specified as an ID, AeroLab will always create the `AeroLabServer-` and `AeroLabClient-` firewall rules to allow for inter-node communication.

Advanced use allows for creating and/or assigning multiple firewall rules to a cluster to allow multiple users/locations to access the instances. These rules automatically lock down access by default to the IP of the caller. Use the `lock-security-groups -n RULENAME` to refresh the IP. A specific IP can also be assigned using this command, for example: `aerolab config aws lock-security-groups -n RULENAME -i 1.2.3.4/32`

To do so, rules can be created and assigned in one of the 2 ways:

### Manually creating the rules

Execute the following to create two rules called `robert` and `bob` as an example:

```
aerolab config aws create-security-groups -n robert
aerolab config aws create-security-groups -n bob
```

Then create the cluster, specifying the 2 tags:

```
aerolab cluster create -n testcluster2 -c 2 -I t3a.medium --secgroup-name=robert --secgroup-name=bob
```

### Automatically by running the cluster create command

If AeroLab is executed to create a cluster with firewall rule names that do not exist, these will be automatically created. Simply run:

```
aerolab cluster create -n testcluster2 -c 2 -I t3a.medium --secgroup-name=robert --secgroup-name=bob
```

If the 2 rules do not exist, they will be created.

### Adding a firewall rule name to an existing cluster

If a cluster already exists, and an additional rule has to be assigned, use the following command:

```
aerolab cluster add firewall -n CLUSTERNAME --secgroup-name=SecGroupPrefix
```

For example:

```
aerolab cluster add firewall -n testcluster2 --secgroup-name=bob2
```

If the specified rule does not exist, it will be created.

### Removing a firewall rule name from an existing cluster

In a similar way to addition, rule name can be removed with the same commands, by adding the `-r` or `--remove` to the commands:

```
aerolab cluster add firewall -n CLUSTERNAME --secgroup-name=SecGroupPrefix --remove
```

For example:

```
aerolab cluster add firewall -n testcluster2 --secgroup-name=bob2 --remove
```
