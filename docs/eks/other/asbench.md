# Deploying astools containers with asbench load

## Prep

```bash
cd /root/deploy-olm-ako/examples/clients
```

Modify `asbench-deployment.yaml` to your needs (asbench command is there).

## Deploy

```bash
kubectl apply -f asbench-deployment.yaml -n aerospike
```

## Check status

```bash
kubectl get pods -n aerospike -o wide
```

## Check client logs

```bash
kubectl -n aerospike logs astools-deployment-xxxx
```

## Kill the clients

```bash
kubectl delete -f asbench-deployment.yaml -n aerospike
```
