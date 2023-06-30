[Docs home](../README.md)

# GCP Firewall

## Basic use

By default AeroLab creates 2 managed firewall rules:
* the `aerolab-managed-internal` group is used for inter-node communication
* the `aerolab-managed-external` group is used for communication with the node from the internet

Basic usage does not require any special setup, as AeroLab will create, manage and support the firewall rules and related instance tags.

The rules created will be automatically locked to use the caller's (the person calling the `aerolab` command) IP address for security reasons. Should the caller's IP change, the firewall rules will need updating to allow continued access. To do so, simply run:
```
aerolab config gcp lock-firewall-rules
```

In order to remove the AeroLab managed firewall rules, `aerolab config gcp delete-firewall-rules -i` command exists.

## Listing the rules

Execute `aerolab config gcp list-firewall-rules` to list all firewall rules.

## Advanced use

AeroLab will always create the `aerolab-managed-internal` firewall rule, and assign the relevant firewall tags `aerolab-server` or `aerolab-client` to the instances to allow for inter-node communication.

Advanced use allows for creating and/or assigning multiple firewall rules to a cluster to allow multiple users/locations to access the instances. These rules automatically lock down access by default to the IP of the caller. Use the `lock-firewall-rules -n RULENAME` to refresh the IP. A specific IP can also be assigned using this command, for example: `aerolab config gcp lock-firewall-rules -n RULENAME -i 1.2.3.4/32`

To do so, rules can be created and assigned in one of the 2 ways:

### Manually creating the rules

Execute the following to create two rules called `robert` and `bob` as an example:

```
aerolab config gcp create-firewall-rules -n robert
aerolab config gcp create-firewall-rules -n bob
```

Then create the cluster, specifying the 2 tags:

```
aerolab cluster create -n testcluster2 -c 2 --instance e2-medium --zone us-central1-a --firewall=robert --firewall=bob
```

### Automatically by running the cluster create command

If AeroLab is executed to create a cluster with firewall rule names that do not exist, these will be automatically created. Simply run:

```
aerolab cluster create -n testcluster2 -c 2 --instance e2-medium --zone us-central1-a --firewall=robert --firewall=bob
```

If the 2 rules do not exist, they will be created.

### Adding a firewall rule name to an existing cluster

If a cluster already exists, and an additional rule has to be assigned, use the following command:

```
aerolab cluster add firewall -n CLUSTERNAME --firewall=FIREWALLNAME
```

For example:

```
aerolab cluster add firewall -n testcluster2 --firewall=bob2
```

If the specified rule does not exist, it will be created.

### Removing a firewall rule name from an existing cluster

In a similar way to addition, rule name can be removed with the same commands, by adding the `-r` or `--remove` to the commands:

```
aerolab cluster add firewall -n CLUSTERNAME --firewall=FIREWALLNAME --remove
```

For example:

```
aerolab cluster add firewall -n testcluster2 --firewall=bob2 --remove
```
