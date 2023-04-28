[Docs home](../../README.md)

# Build a cluster with LDAP and TLS


This script is designed to allow you to build a whole environment containing
an LDAP server with the option of having TLS enabled for both the Aerospike nodes and
the LDAP server itself, along with a Python and/or Go client host.
This script depends on the `aerolab-ldap`, `aerolab-goclient` and `aerolab-pythonclient` build
components to function.

### Clone the repo

#### Using HTTPS

```bash
git clone https://github.com/aerospike/aerolab.git
```

#### Using git keys

```bash
git clone git@github.com:aerospike/aerolab.git
```

## Enter the directory

```bash
cd aerolab/scripts/aerolab-buildenv
```

The config file in this directory allows you to customize your environment.
Note the following options:

```bash
#Aerospike Configuration
TEMPLATE=ldap_tls.conf
NODES=2

#VERSION should be "-v x.x.x.x"  If it's not set, then Latest will be used
VERSION=""

#FEATURES should be "-f <pathtofile>"  If it's not set, then ~/aero-lab-common.conf setting will be used
FEATURES=""

#NETWORKMODE should be "-m <type>"  If it's not set, then ~/aero-lab-common.conf setting will be used
NETWORKMODE=""
```

The TEMPLATE is any available standard AeroLab configuration file. The most useful in
this instance are `ldap.conf` and `ldap_tls.conf`.

With the `VERSION`, `FEATURES` and `NETWORKMODE` items, they also require the appropriate
command-line switch. For example, `VERSION="-v 5.7.0.13"`.


```bash
# YES/NO
BUILD_PYTHON="YES"
BUILD_GO="YES"
```
The BUILD options allow you to select which client containers will be built and started.

Once you have set your configurations, you can build the environment. From this point on, the process is automated.
Any changes that you make to either your LDAP `ldif` files or environment configuration will persist between uses. To restore the standard build, remove your directory and re-clone the repo.


---
## Create new environment
One of the first things that happens when you deploy your environment is that it will destroy whatever was built last time. If you change your config between building and destroying, it may not destroy your environment correctly.

```bash
./deploy-env.sh
```

---
## Destroy
This will destroy the previously-built environment:

```bash
./destroy-env.sh
```

---
## Get Help
When you deploy your environment, it will automatically give you a list of useful commands.
You can see the list at any time with the following command:

```bash
./deploy-env.sh help
```
