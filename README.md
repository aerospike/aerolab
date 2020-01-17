# Aero-Lab v2.48

#### Spin up Aerospike clusters quickly in aws, docker on mac or docker/lxc on ubuntu 18.04)

### Note: if running on osx, grab aerolab-osx-aio (all in one) version.

### Grab it from [releases](https://github.com/citrusleaf/aerolab/releases)

### Supported backends

* docker
  * on mac
  * local on linux
  * via ssh
* lxc
  * local on linux
  * lxc via ssh
* AWS

### NEW: SUPPORT FOR ACCESSING ALL CONTAINERS DIRECTLY FROM MAC/WINDOWS HOST

You can now access the containers on aerolab directly (e.g. if your AMC is on 172.17.0.3 port 8081, you can directly go to http://172.17.0.3:8081)

You can see what IPs the containers have by executing `aerolab cluster-list`

To achive that, follow [this](tunnel-container/README.md) simple instruction.

#### See [here](https://drive.google.com/open?id=1voLJV12x0XMLe-lcN_SsP6NLUytMNI_e) for installation video - Install aerolab, docker and spinup 3 clusters with xdr and tls in 20 minutes.

#### See [here](docs/README.md) for usage instructions and howto. Especially the "Using help" and "Quick start"

### Grab it from [releases](https://github.com/citrusleaf/aerolab/releases)

###### See [FUTURE.md](FUTURE.md) for future features list

###### See [CHANGELOG.md](CHANGELOG.md) for version changes

###### See [VERSION.md](VERSION.md) for version number

###### See [AWS HowTo](docs/AWS.md)
