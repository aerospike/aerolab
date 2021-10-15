# How to deploy aerospike operator on minikube

## Clone Repository

```
git clone https://github.com/aerospike/aerospike-kubernetes-operator.git
cd aerospike-kubernetes-operator
git checkout 1.0.1
```

## Create namespace (like isolation thingie)

```
minikube kubectl -- create namespace aerospike
```

## Deploy operator

```
minikube kubectl -- apply -f deploy/crds/aerospike.com_aerospikeclusters_crd.yaml
minikube kubectl -- apply -f deploy/rbac.yaml
minikube kubectl -- apply -f deploy/operator.yaml
minikube kubectl -- get pod -n aerospike
minikube kubectl -- -n aerospike logs -f $(minikube kubectl -- get pod -n aerospike |egrep -o 'aerospike-kubernetes-operator-[a-z0-9-]+')
```

## Features file

```
cat <<'EOF' > ./deploy/secrets/features.conf
<FEATURES.CONF FILE CONTENTS HERE>
EOF
```

## Create storage and apply secrets for password

```
sed 's/microk8s.io\/hostpath/k8s.io\/minikube-hostpath/g' deploy/samples/storage/microk8s_filesystem_storage_class.yaml > deploy/samples/storage/minikube.yaml
minikube kubectl -- apply -f deploy/samples/storage/minikube.yaml
minikube kubectl -- get storageclass
minikube kubectl -- -n aerospike create secret generic aerospike-secret --from-file=deploy/secrets
minikube kubectl -- -n aerospike create secret generic auth-secret --from-literal=password='admin123'
```
