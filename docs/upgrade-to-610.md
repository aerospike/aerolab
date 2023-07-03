# Upgrading AeroLab to 6.1.0

If using `AWS` or `GCP` backend, incompatible firewall features have been added.

In order to convert clusters and clients created with AeroLab versions `6.0` or below to `6.1` compatible setup, run the following commands after AeroLab has been upgraded to `6.1`.

## AWS

```
aerolab config aws create-security-groups

aerolab cluster add firewall -n CLUSTERNAME
aerolab client configure firewall -n CLIENTNAME

aerolab config aws lock-security-groups -n AeroLabServer
aerolab config aws lock-security-groups -n AeroLabClient
```

The commands `aerolab cluster add firewall -n CLUSTERNAME` and `aerolab client configure firewall -n CLIENTNAME` can be repeated multiple times if multiple clusters/clients are present.

## GCP

TODO
