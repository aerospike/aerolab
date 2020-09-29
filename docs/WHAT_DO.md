# AeroLab is stuck not creating instances, or half broken ones, what do?

## Quick and Easy fix

### Stop and Destroy all clusters.

```
aerolab cluster-destroy -f -n all
```

### Nuke all templates

```
aerolab nuke-template -v all -d all -i all
```

### Delete installer files

#### If you don't know where you have them downloaded by aerolab, find them first:

```
find ~ -name "aerospike-server-*" 2>&1 |egrep "aerospike-server-(enterprise|community)"
```

Then delete them.

#### OR FIND & DELETE at once (this might be dangerous, think twice before doing something like that):

```
find ~ -name "aerospike-server-*.tgz" 2>&1 |egrep "aerospike-server-(enterprise|community)" |while read fn; do rm "${fn}"; done
```

### Try again

Now make-cluster again, you should see a line printed that says "downloading" and one that says it's making a template. If you don't see making templates, you have failed to nuke templates. If you see making template, but did not see the downloading of installer progress, you did not delete the installer files. Read this again and do it properly.

### Done ?

The installation should now complete successfully. If it does not:

1. follow the step above for deleting installation files (the one with the `find` command)
2. nuke all of docker: click on `docker icon`, then `Troubleshoot`, then `Clean / Purge data`.
3. wait for docker to restart
4. check you have spare free disk space on your machine (mac/windows/linux, whatever you use on the host)
5. try making the cluster again.

Now it should definitely work.

If it doesn't, contact `rglonek` for support, as ubuntu/centos have changed something that makes building the VMs break and this needs to be fixed within AeroLab.

## Slightly more complicated way

You can destroy just the affected clusters with `cluster-destroy` and then nuke just the template for that affected broken version with `nuke-template` if you want, followed by deleting the installer files just for that version of aerospike and OS if you do not want to loose anything else.
