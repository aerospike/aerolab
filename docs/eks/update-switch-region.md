# AeroLab EKSCTL - updating the tools and switching regions

All of the below commands should be executed on the user's machine, not within the `eksctl` client machine.

### Update all the tools

```bash
# install new aerolab version inside the client
aerolab client configure aerolab -n eksctl
# install new bootstrap script
aerolab client attach -n eksctl -- aerolab client create eksctl --update-bootstrap
# use the bootstrap script to install new eksctl,kubectl,awscli-v2
aerolab client attach -n eksctl -- bash bootstrap
```

### Switch regions, installing expiry system in the new region

Replace `NEWREGION` with the name of AWS region you are switching to.

```bash
aerolab client attach -n eksctl -- bash bootstrap -n NEWREGION
```

### Recreate example eksctl yaml files

This command will recreate example eksctl yaml files, replacing the REGION specification with those with the provided AWS region.

```bash
aerolab client attach -n eksctl -- aerolab client create eksctl --install-yamls -r REGION
```
