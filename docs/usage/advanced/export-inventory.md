# Exporting Aerolab Inventory

## Why?

The primary purpose for exporting the inventory is to allow for a natural way to extend or customize
your `aerolab` deployed clusters. `aerolab` knows what has been deployed, and the relevant details. 
By exporting inventories that other tools know about, those tools can pick up where `aerolab` finishes,
and this should provide a natural escape hatch.

For teams that share a cloud account, be advised that setting the `PROJECT_NAME` env variable limits the output to 
those instances that have been tagged with `project=<PROJECT_NAME>` either at creation time like this:
```shell
 aerolab cluster create ... \
          --tags=project=${PROJECT_NAME} \   # when using the AWS backend
          --label=project=${PROJECT_NAME} \  # When using the GCP backend
          ...
EOF
```
Note that you can pass both parameters, `aerolab` will silently ignore those that don't apply to the currently
configured backend. This allows the user to create scripts that work as the user switches backends. It is possible to 
manually via the cloud's console or CLI commands.

For example, like this in GCP:
```shell
gcloud compute instances add-labels "aerolab4-${CLUSTER_NAME}-${NODE_NO}" --labels=project=${PROJECT_NAME}
```

or like this in AWS:

```shell
aws ec2 create-tags \
    --resources ami-1234567890abcdef0 \
    --tags Key=project,Value=${PROJECT_NAME}
```

## Supported Formats

### Hostfile

These lines are to be used to update your `/etc/hosts` file. Doing so will allow you to move around the cluster more
easily, using the hostname or the node's name as it appears in your clouds console. Additionally, if you leverage this
pattern of configuring your systems, you can side step the need to hard-code IPs in in configuration files, because
after deploy, updating `/etc/hosts` will allow for the systems to leverage the OS's hostname resolution to find the right 
machine.

```shell
$ aerolab inventory hostfile
10.128.0.3    aerolab4-aerospike-1            aerospike-1
10.128.0.4    aerolab4-aerospike-2            aerospike-2
10.128.0.10   aerolab4-ghost-rider-cluster-1  ghost-rider-cluster-1
10.128.0.24   aerolab4-ghost-rider-cluster-2  ghost-rider-cluster-2
10.128.0.25   aerolab4-ghost-rider-cluster-3  ghost-rider-cluster-3
10.128.0.26   aerolab4-ghost-rider-cluster-4  ghost-rider-cluster-4
10.128.0.28   aerolab4-ghost-rider-cluster-5  ghost-rider-cluster-5
10.128.0.34   aerolab4-aerospike-client-1     aerospike-client-1
10.128.0.82   aerolab4-ann-client-1           ann-client-1
10.128.0.22   aerolab4-ghost-rider-1          ghost-rider-1
10.128.0.19   aerolab4-darkhold-1             darkhold-1
10.128.0.29   aerolab4-mephisto-1             mephisto-1
```

And you can filter,

```shell
$ PROJECT_NAME=mephisto aerolab inventory hostfile
10.128.0.3              aerolab4-aerospike-1         aerospike-1
10.128.0.4              aerolab4-aerospike-2         aerospike-2
10.128.0.34             aerolab4-aerospike-client-1  aerospike-client-1
10.128.0.19             aerolab4-darkhold-1          darkhold-1
10.128.0.29             aerolab4-mephisto-1          mephisto-1
```

### Ansible Inventory

Ansible allows for executables to provide dynamic inventories, and `aerolab` is leveraging the "inventory scripts" method
 [[docs](https://docs.ansible.com/ansible/latest/dev_guide/developing_inventory.html#developing-inventory-scripts)].

`Ansible` expects to be able to call an executable directly, without subcommands. `aerolab` supports the same calling convention
that busybox does, where it changes its behavior based on what the shell invokes the binary as. Just like busy box, a single 
binary can be symlinked to masquerade as many separate utilities. To set this up, run `aerolab` like so:

```shell
[root@aerolab4-mephisto-1 mephisto]# aerolab showcommands
2024/08/01 17:39:20 +0000 Discovered absolute path: /usr/bin/aerolab
2024/08/01 17:39:20 +0000 > ln -s /usr/bin/aerolab /usr/local/bin/showconf
2024/08/01 17:39:20 +0000 > ln -s /usr/bin/aerolab /usr/local/bin/showsysinfo
2024/08/01 17:39:20 +0000 > ln -s /usr/bin/aerolab /usr/local/bin/showinterrupts
2024/08/01 17:39:20 +0000 > ln -s /usr/bin/aerolab /usr/local/bin/aerolab-ansible
2024/08/01 17:39:20 +0000 Done
```

This then enables the following behaviour

```shell
[root@aerolab4-mephisto-1 mephisto]# aerolab-ansible
{
  "_meta": {
    "hostvars": {
      ...
      "10.128.0.21": {
        "aerolab_cluster": "aerospike-client",
        "ansible_host": "10.128.0.21",
        "ansible_ssh_private_key_file": "/root/aerolab-keys/aerolab-gcp-aerospike-client",
        "ansible_user": "root",
        "instance_id": "aerolab4-aerospike-client-1",
        "node_name": "aerospike-client-1",
        "project": "mephisto"
      },
      "10.128.0.27": {
        "aerolab_cluster": "aerospike",
        "ansible_host": "10.128.0.27",
        "ansible_ssh_private_key_file": "/root/aerolab-keys/aerolab-gcp-aerospike",
        "ansible_user": "root",
        "instance_id": "aerolab4-aerospike-3",
        "node_name": "aerospike-3",
        "project": "mephisto"
      },
      ...
    }
  },
  "aerospike": {
    "hosts": [
      "10.128.0.3",
      ...
    ]
  },
  "avs": {
    "hosts": [
      "10.128.0.19",
      ...
    ]
  },
  "jumpbox": {
    "hosts": [
      "10.128.0.29"
    ]
  },
  "tools": {
    "hosts": [
      "10.128.0.21",
      ...
    ]
  }
}
```

And finally, you can tie it all together like this

```shell
ANSIBLE_STDOUT_CALLBACK=unixy ansible-playbook -f 64 -i /usr/local/bin/aerolab-ansible "${CURRENT_PROJECT_ROOT}"/project-ansible/project_environment.yaml
```

### Genders File

The High Performance Computing Systems Group, at the Lawrence Livermore National Laboratory, 
has a suite of tools that they use to address configuration management issues that arise on clusters.

+ [`genders`](https://github.com/chaos/genders/blob/master/TUTORIAL)
+ [`nodeattr`](https://linux.die.net/man/1/nodeattr)
+ [`pdsh`](https://github.com/chaos/pdsh)
+ [`pdcp`](https://linux.die.net/man/1/pdcp)
+ [`rpdcp`](https://linux.die.net/man/1/pdcp)

the `aerolab inventory genders` command generated a file that is conventionally placed at `/etc/genders`, and looks like

```shell
$ aerolab inventory genders
aerospike-1             aerospike,group=aerospike,project=mephisto,all,pdsh_rcmd_type=ssh
aerospike-2             aerospike,group=aerospike,project=mephisto,all,pdsh_rcmd_type=ssh
ghost-rider-cluster-1   ghost-rider-cluster,group=aerospike,project=ghost-rider,all,pdsh_rcmd_type=ssh
ghost-rider-cluster-2   ghost-rider-cluster,group=aerospike,project=ghost-rider,all,pdsh_rcmd_type=ssh
ghost-rider-cluster-3   ghost-rider-cluster,group=aerospike,project=ghost-rider,all,pdsh_rcmd_type=ssh
ghost-rider-cluster-4   ghost-rider-cluster,group=aerospike,project=ghost-rider,all,pdsh_rcmd_type=ssh
ghost-rider-cluster-5   ghost-rider-cluster,group=aerospike,project=ghost-rider,all,pdsh_rcmd_type=ssh
aerospike-client-1      aerospike-client,group=tools,project=mephisto,all,pdsh_rcmd_type=ssh
ann-client-1            ann-client,group=jumpbox,project=ann-client,all,pdsh_rcmd_type=ssh
ghost-rider-1           ghost-rider,group=jumpbox,project=ghost-rider,all,pdsh_rcmd_type=ssh
darkhold-1              darkhold,group=avs,project=mephisto,all,pdsh_rcmd_type=ssh
mephisto-1              mephisto,group=jumpbox,project=mephisto,all,pdsh_rcmd_type=ssh
```

and to filter down to a specific project

```shell
$ PROJECT_NAME=mephisto aerolab inventory genders
aerospike-1             aerospike,group=aerospike,project=mephisto,all,pdsh_rcmd_type=ssh
aerospike-2             aerospike,group=aerospike,project=mephisto,all,pdsh_rcmd_type=ssh
aerospike-client-1      aerospike-client,group=tools,project=mephisto,all,pdsh_rcmd_type=ssh
darkhold-1              darkhold,group=avs,project=mephisto,all,pdsh_rcmd_type=ssh
mephisto-1              mephisto,group=jumpbox,project=mephisto,all,pdsh_rcmd_type=ssh
```

Perhaps the biggest reason to leverage tools like `pdsh` or `rpdcp` over `aerolab cluster attach` or `aerolab files` is that 
the `/etc/genders` and `/etc/hosts` files serve as a cache of the deployed systems, which makes the LLNL tools feel magically 
fast.
