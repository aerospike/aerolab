# Getting Started with Vagrant Backend

The Vagrant backend runs Aerospike clusters as local VMs managed by [Vagrant](https://developer.hashicorp.com/vagrant), using whichever hypervisor provider you have installed (VirtualBox, libvirt, or VMware). It behaves like a "real VM" alternative to the Docker backend, at the cost of slower create/boot times.

## Prerequisites

Install Vagrant **2.4.0 or newer**:

- [Install Vagrant](https://developer.hashicorp.com/vagrant/install)

You also need at least one supported provider:

- **VirtualBox** (recommended) - [Install VirtualBox](https://www.virtualbox.org/wiki/Downloads). Works out of the box; Vagrant detects `VBoxManage` on `PATH`.
- **libvirt** - requires `virsh` on `PATH` plus the `vagrant-libvirt` plugin (`vagrant plugin install vagrant-libvirt`). See the [libvirt box caveat](#libvirt-box-catalog-caveat) below — you must pass `--vagrant-box`/`--vagrant.box` explicitly.
- **VMware** - requires the `vagrant-vmware-desktop` plugin (`vagrant plugin install vagrant-vmware-desktop`), plus a VMware Workstation/Fusion license.
- **Hyper-V** - Windows only; no extra plugin needed, but requires Hyper-V to be enabled and Vagrant run from an elevated shell.

Verify installation:

```bash
vagrant --version
VBoxManage --version   # or: virsh --version / vagrant plugin list
```

## Configuration

### 1. Configure the Vagrant Backend

```bash
aerolab config backend -t vagrant
```

Optional flags:

```bash
aerolab config backend -t vagrant \
  --vagrant-provider virtualbox \
  --vagrant-subnet 192.168.56.0/24 \
  --vagrant-binary /usr/bin/vagrant
```

- `--vagrant-provider` - default provider passed to `vagrant up --provider=...`. Leave empty to let Vagrant pick.
- `--vagrant-subnet` - CIDR used for the private-network static IPs assigned to instances. Defaults to `192.168.56.0/24`.
- `--vagrant-binary` - explicit path to the `vagrant` executable, if it isn't on `PATH`.

Configuring the backend runs a preflight check (vagrant version + provider detection) before it is persisted, so a broken local environment never becomes the active backend.

### 2. Verify the Environment

Re-run the preflight check at any time:

```bash
aerolab config vagrant check
```

This prints the detected Vagrant version, installed plugins, detected providers, and any issues (e.g. version too old, or the configured default provider not found).

### 3. Manage Boxes

```bash
aerolab config vagrant list-boxes     # boxes registered with the local vagrant install
aerolab config vagrant delete-box -n bento/ubuntu-24.04
```

## Usage Examples

Create a 3-node cluster using the default box for the requested OS/version:

```bash
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*'
```

Override the box explicitly (bypasses OS/version-based box resolution entirely — required for libvirt, see below):

```bash
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*' --vagrant-box generic/ubuntu2204
```

Override the provider for a single cluster, and size the VMs:

```bash
aerolab cluster create -c 3 -d ubuntu -i 24.04 -v '8.*' \
  --vagrant-provider libvirt \
  --vagrant-box generic/ubuntu2204 \
  --cpus 4 --ram 4096
```

`aerolab instances create` exposes the same options under a `--vagrant.*` namespace (`--vagrant.box`, `--vagrant.provider`, `--vagrant.image`, `--vagrant.cpus`, `--vagrant.ram-mb`, `--vagrant.disk`).

## Limitations

- **No firewalls.** The vagrant backend has no firewall/security-group concept; firewall-related commands return "not implemented" for this backend.
- **No expiry daemon.** `--vagrant-expire` style auto-expiry is not installed; expiry commands are accepted as no-ops. Clean up manually with `aerolab cluster destroy`.
- **Volumes are synced folders, mounted at create time only.** A vagrant "volume" is a host directory bind-mounted into the VM via Vagrant's synced folders. There is no attach/detach/resize after creation — volumes must be specified at instance/cluster create time.
- **No `amazon-linux` distro.** The box catalog only covers Ubuntu, Debian, and Rocky Linux (see below); Amazon Linux images are not published for Vagrant.
- **Single zone.** Vagrant has one fixed "local" zone — there are no regions/zones to enable or disable.

### Box catalog and architecture support

| Distro | Version | Architectures |
|--------|---------|----------------|
| Ubuntu | 20.04   | amd64 |
| Ubuntu | 22.04   | amd64, arm64 |
| Ubuntu | 24.04   | amd64, arm64 |
| Debian | 11      | amd64 |
| Debian | 12      | amd64, arm64 |
| Rocky  | 8       | amd64 |
| Rocky  | 9       | amd64, arm64 |

Vagrant >= 2.4 auto-selects the box variant matching the host architecture.

### libvirt box catalog caveat

The default box catalog above resolves to `bento/*` boxes. **Bento does not publish libvirt boxes for any of these entries** — only VirtualBox (and in some cases VMware/Hyper-V) variants exist upstream. If you're using the libvirt provider, image-based box resolution will fail; you must pass `--vagrant-box`/`--vagrant.box` with a libvirt-compatible box, e.g.:

```bash
aerolab cluster create -c 3 -d ubuntu -i 22.04 -v '8.*' \
  --vagrant-provider libvirt --vagrant-box generic/ubuntu2204
```

`generic/*` boxes (published by [`generic`](https://app.vagrantup.com/generic) on Vagrant Cloud) support libvirt, VirtualBox, and VMware, and are a safe default whenever you're not on VirtualBox.

## Disk Usage

The vagrant backend touches disk in three places, each cleaned up differently:

1. **Downloaded Vagrant boxes** - `~/.vagrant.d/boxes`. This is a global Vagrant cache shared across projects and is **not** managed per-project by aerolab. Boxes accumulate here every time a new OS/version/provider combination is used. Clean up with:
   ```bash
   aerolab config vagrant delete-box -n bento/ubuntu-24.04
   # or directly:
   vagrant box remove bento/ubuntu-24.04
   ```

2. **VM disks** - live in the provider's own storage, not under aerolab's config directory:
   - VirtualBox: the configured "machine folder" (`VBoxManage list systemproperties | grep "Default machine folder"`).
   - libvirt: the provider's storage pool (commonly `/var/lib/libvirt/images`).

   These are removed automatically when `aerolab cluster destroy` / `instances destroy` runs `vagrant destroy`, but can be left behind if a VM is deleted outside of aerolab (e.g. directly via `VBoxManage` or `virsh`).

3. **Aerolab's own config directory** (`<configDir>/projects/<project>/config/vagrant/`) holds only lightweight state:
   - `clusters/<name>/` - the generated `Vagrantfile`, `aerolab.json` metadata, and Vagrant's own `.vagrant/` state directory for that cluster. Removed automatically on `cluster destroy`.
   - `volumes/<name>/` - the actual synced-folder contents for aerolab volumes (this is real data, not just metadata — back it up if it matters).
   - a transient `.box` file may appear briefly during `aerolab images create`; it is deleted automatically after `vagrant box add` completes.

If you ever need a full reset, `aerolab inventory delete-project-resources -f` clears clusters/volumes tracked by aerolab, but you'll still want to check `~/.vagrant.d/boxes` and the provider's VM storage for anything left behind by manual/out-of-band changes.

## Next Steps

- Explore [cluster management commands](../commands/cluster.md)
- Learn about [Aerospike daemon controls](../commands/aerospike.md)
- Check out [configuration management](../commands/config.md)
- See [advanced features](../commands/) for more options
