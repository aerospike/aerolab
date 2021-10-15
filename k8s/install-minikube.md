# Installing minikube

## Prerequisites

First stop docker desktop. Minikube can be installed alongside it, but cannot normally be running at the same time. Don't worry, you can use minikube for docker/aerolab instead as well, so you don't have to switch between them.

Hypervisor requirements:

OS | Requirement
--- | ---
MacOS | None, since HyperKit is already available
Windows | Either VirtualBox or Windows Pro with HyperV enabled
Linux | None, since KVM is built into the kernel

## Installation

Head to [https://minikube.sigs.k8s.io/docs/start/](https://minikube.sigs.k8s.io/docs/start/) and follow the simple install manual.

If you are on Mac, you can just follow it using the below 2 lines:

```
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-darwin-amd64
sudo install minikube-darwin-amd64 /usr/local/bin/minikube
```

## Start new docker+kubernetes

```
minikube start --kubernetes-version v1.18.0 --memory=16g --disk-size=50g --cpus=4
minikube kubectl -- get pods -A
```

## Stop everything

```
minikube stop
```

## Restart existing docker+kubernetes

```
minikube start
```

## Reset/Nuke all

```
minikube stop
minikube delete --all
```

## Turn off k8s, keep only docker running

```
minikube pause
```

## Turn on k8s in running minikube

```
minikube unpause
```

## Point docker command to minikube

The below command will allow docker command cli and aerolab to work with minikube docker

If using zsh:

```
echo 'eval $(minikube -p minikube docker-env)' >> ~/.zshrc
```

If using bash:

```
echo 'eval $(minikube -p minikube docker-env)' >> ~/.bashrc
```

## Note

You can start the minikube, and leave it running. It will persist across reboots as well. Just pause and unpause the k8s as needed.
