# Partitioning disks in AWS


AeroLab supports automated disk discovery and partitioning. The partitioner has 4 options:

Commands | Description
--- | ---
`cluster partition list` | List disks and partitions on all/chosen nodes.
`cluster partition create` | Create partitions on disks. This will also remove any previous partitions and filesystems on the chosen disks.
`cluster partition mkfs` | Make filesystems on chosen partitions, automatically mount in `/mnt`, and add the mountpoint to `fstab`.
`cluster partition conf` | Adjust Aerospike configuration to reflect newly created partitions. Supports devices, shadow devices and all-flash devices.

## Filters

Each command accepts filters where disks/partitions to affect can be chosen. The filters are exclusive, meaning that for a disk/partition to be selected, it must satisfy all filters.

:::note
Any disks with partitions mounted to `/` or `/boot*` are excluded from any listing or partition/disk number assignment and are not displayed or considered by AeroLab.
:::

Filter | Description
--- | ---
`--filter-type` | Filter by disk type. Available options are `nvme` or `ebs`. If set, selects all NVME disks, or all EBS disks. Commands which don't specify a disk type apply to all disks.
`--filter-disks` | Filter by disk number. AeroLab assigns disk numbers by disk name, in alphabetical order.
`--filter-partitions` | Filter by partition number. AeroLab assigns each partition a number, starting from 1 for each disk.

### Filter examples

* `--filter-type=nvme`: Take action on all NVME disks.
* `--filter-disks=1-3`: Take action on disks 1-3.
* `--filter-partitions=1,2`: Take action on partitions 1 and 2 on all disks.
* `--filter-type=nvme --filter-partitions=1,2`: Take action on partitions 1 and 2 only on NVME disks.
* `--filter-disks=1-3 --filter-partitions=1`: Take action on partition 1 on disks 1-3.

## Usage Examples

* [Use all disks for a namespace](all-disks.md)
* [Use all NVME disks for a namespace](all-nvme-disks.md)
* [Use all NVME disks for a namespace with also data in memory(aerospike 7.0.0+)](all-nvme-disks-memory.md)
* [Partition each NVME into 4 partitions and use 2 for each of the two namespaces](two-namespaces-nvme.md)
* [Partition NVME and EBS and configure shadow devices for a namespace](with-shadow.md)
* [Partition NVME and EBS, use all partitions for namespace, with first nvme partition of each nvme disk for all-flash](with-allflash.md)
