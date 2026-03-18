# GP3 storage class

## Create configuration yaml

Create a file called `gp3.yaml` with the following contents:

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
allowVolumeExpansion: true
provisioner: ebs.csi.aws.com
volumeBindingMode: WaitForFirstConsumer
parameters:
  type: gp3
```

## Apply storage class

```bash
kubectl apply -f gp3.yaml
```
