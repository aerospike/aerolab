# Partitioning disks in AWS

AeroLab supports automated disk discovery and partitioning. The partitioner has 4 options:

Commands | Description
--- | ---
`cluster partition list` | list disks and partitions on all/chosen nodes
`cluster partition create` | create partitions on disks; this will also remove any previous partitions and filesystems on the chosen disks
`cluster partition mkfs` | make filesystems on chosen partitions; automatically mounts in `/mnt` and adds the mountpoint to `fstab`
`cluster partition conf` | adjust aeroapike configuration to newly created partitions; support devices, shadow devices and all-flash devices

## Filters

Each commands accepts filters, where disks/partitions to affect can be chosen. The filters are exclusive, which means for a disk/partition to be selected, it must satisfy all filters.

Note that any disks with partitions mounted to `/` or `/boot*` will automatically be exluded from any listing or partition/disk number assignment and will not be displayed or considered by AeroLab.

Filter | Description
--- | ---
`--filter-type` | filter by disk type, available options are `nvme` or `ebs`; if set, this will select all NVME disks, or all EBS disks; unset means ALL disks
`--filter-disks` | filter by disk number; disk numbers are assigned by AeroLab by name (each disk is assigned a number, using disk name in alphabetical order)
`--filter-partitions` | filter by partition number; partition numbers are assigned by AeroLab by name, starting from 1 onwards for each disk

### Filter examples

* `--filter-type=nvme` will result in an action taken on all NVME disks
* `--filter-disks=1-3` will result in an action taken on disks 1-3
* `--filter-partitions=1,2` will result in an action taken on partitions 1 and 2 on all disks
* `--filter-type=nvme --filter-partitions=1,2` will result in an action taken on partitions 1 and 2 on all NVME disks only
* `--filter-disks=1-3 --filter-partitions=1` will result in an action taken on partition 1 on disks 1 up to 3

## Usage Examples

* [Use all disks for a namespace](all-disks.md)
* [Use all NVME disks for a namespace](all-nvme-disks.md)
* [Partition each NVME into 4 partitions and use 2 for each of the two namespaces](two-namespaces-nvme.md)
* [Partition NVME and EBS and configure shadow devices for a namespace](with-shadow.md)
* [Partition NVME and EBS, use all partitions for namespace, with first nvme partition of each nvme disk for all-flash](with-allflash.md)
