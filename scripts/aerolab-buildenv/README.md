# Deploying Full Environment

Notes:
  * This script is designed to allow you to very quickly build a whole environment containing an ldap server with the option of<br>
    having tls enabled for both the Aerospike nodes ***and*** the ldap server itself along with a python and/or go client host.
  * It depends on the aerolab-ldap, aerolab-goclient and aerolab-pythonclient build components to function.

### First clone this repo

#### Using https

```bash
git clone https://github.com/citrusleaf/aerolab.git
```

#### Using git keys

```bash
git clone git@github.com:citrusleaf/aerolab.git
```
```

## Enter this directory

```bash
% cd aerolab/scripts/aerolab-buildenv
```

There is a config file in this directory that allows you to customise your environment. Specifically the following may be of interest.
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
The TEMPLATE is any standard aerolab template that is available, however the most useful in this instance are ***ldap.conf*** and ***ldap_tls.conf*** 
With the ***VERSION***, ***FEATURES*** and ***NETWORKMODE*** items, they also need the commandline switch including. For example VERSION="-v 5.7.0.13"


```bash
# YES/NO
BUILD_PYTHON="YES"
BUILD_GO="YES"
```
The BUILD options allow you to select which client containers will be built and started

Once you have set your config in the way you wish, you can move on to building the environment - from here on in, it's automated.
Any changes that you make to either your ldap ldif files or environment configuration will persist between uses. You can get back to the standard build by removing your directory and re-cloning the repo.


---
## Create new environment
One of the first thiongs that happens when you try and deploy your environment is that it will destroy whatever was built last time - if you change your config between building and destroying, then it may not destroy your environment correctly.

```bash
% ./deploy-env.sh
```

---
## Destroy
This will destroy the previously built environment

```bash
% ./destroy-env.sh
```

---
## Get Help
When you deploy your environemnt, it will altomatically give you a list of useful commands.<br>
This can be requested any time your environment is running in the following way :-

```bash
% ./deploy-env.sh help
```
