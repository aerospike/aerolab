# Deploying python client container

## Deploying

### Run a container

```
aerolab deploy-container -n pythonclient
```

### Install python client

```
aerolab node-attach -n pythonclient -- bash -c "apt-get update && apt-get -y install python3 python3-pip && pip3 install --upgrade wheel && pip3 install aerospike"
```

## Connect

```
aerolab node-attach -n pythonclient
```

## Destroy

```
aerolab cluster-destroy -f -n pythonclient
```
