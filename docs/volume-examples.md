# Volume usage examples

## GCP

### Manually create a volume, cluster and client machine, then attach, grow and detach

```
aerolab volume create -n test --zone us-central1-a
aerolab cluster create -n mydc --instance e2-standard-2 --zone us-central1-a
aerolab client create none -n client --instance e2-standard-2 --zone us-central1-a

aerolab volume mount -n test -N mydc
aerolab volume grow -n test -z us-central1-a -s 150
aerolab volume detach -n test -N mydc  -z us-central1-a

aerolab volume mount -n test -N client -c
aerolab volume grow -n test -z us-central1-a -s 160
aerolab volume detach -n test -N client -c -z us-central1-a

aerolab cluster destroy -f -n mydc; aerolab client destroy -f -n client; aerolab volume delete -n test -z us-central1-a; aerolab inventory list
```

### Create volumes during cluster/client creation, then recreate the cluster/client and reattach to the volume

```
aerolab cluster create -n mydc --instance e2-standard-2 --zone us-central1-a --gcp-vol-mount=test:/mnt/test --gcp-vol-create --gcp-vol-desc=testing
aerolab attach shell -- touch /mnt/test/bob
aerolab cluster destroy -f -n mydc
aerolab cluster create -n mydc --instance e2-standard-2 --zone us-central1-a --gcp-vol-mount=test:/mnt/test --gcp-vol-create --gcp-vol-desc=testing
aerolab attach shell -- ls /mnt/test/
aerolab cluster destroy -f -n mydc
aerolab volume delete -n test -z us-central1-a

aerolab client create none -n client --instance e2-standard-2 --zone us-central1-a --gcp-vol-mount=test:/mnt/test --gcp-vol-create --gcp-vol-desc=testing
aerolab attach client -n client -- touch /mnt/test/bob
aerolab client destroy -f -n client
aerolab client create none -n client --instance e2-standard-2 --zone us-central1-a --gcp-vol-mount=test:/mnt/test --gcp-vol-create --gcp-vol-desc=testing
aerolab attach client -n client -- ls /mnt/test/
aerolab client destroy -f -n client
aerolab volume delete -n test -z us-central1-a
```

### Create AGI instance with volumes, then destroy and reattach
aerolab agi create --source-local . --agi-label "testing this" -n withvol --zone us-central1-a --gcp-with-vol --gcp-vol-expire=30h
aerolab agi list
aerolab agi change-label -n withvol -l bob -z us-central1-a
aerolab agi list
aerolab agi destroy -f -n withvol
aerolab agi list
aerolab agi create --source-local . -n withvol --zone us-central1-a --gcp-with-vol --gcp-vol-expire=30h
aerolab agi list
aerolab agi delete -f -n withvol -z us-central1-a
aerolab agi list


## AWS

### Manually create a volume, cluster and client machine, then attach, grow and detach

```
aerolab volume create -n test
aerolab cluster create -n mydc --instance-type t3a.large
aerolab client create none -n client --instance-type t3a.large

aerolab volume mount -n test -N mydc
aerolab volume list

aerolab volume mount -n test -N client -c
aerolab volume list

aerolab cluster destroy -f -n mydc; aerolab client destroy -f -n client; aerolab volume delete -n test; aerolab inventory list
```

### Create volumes during cluster/client creation, then recreate the cluster/client and reattach to the volume

```
aerolab cluster create -n mydc --instance-type t3a.large --aws-efs-mount=test:/mnt/test --aws-efs-create
aerolab attach shell -- touch /mnt/test/bob
aerolab cluster destroy -f -n mydc
aerolab cluster create -n mydc --instance-type t3a.large --aws-efs-mount=test:/mnt/test --aws-efs-create
aerolab attach shell -- ls /mnt/test/
aerolab cluster destroy -f -n mydc
aerolab volume delete -n test

aerolab client create none -n client --instance-type t3a.large --aws-efs-mount=test:/mnt/test --aws-efs-create --aws-efs-onezone
aerolab attach client -n client -- touch /mnt/test/bob
aerolab client destroy -f -n client
aerolab client create none -n client --instance-type t3a.large --aws-efs-mount=test:/mnt/test --aws-efs-create --aws-efs-onezone
aerolab attach client -n client -- ls /mnt/test/
aerolab client destroy -f -n client
aerolab volume delete -n test
```

### Create AGI instance with volumes, then destroy and reattach

```
aerolab agi create --source-local . --agi-label "testing this" -n withvol --aws-with-efs --aws-efs-expire=30h
aerolab agi list
aerolab agi change-label -n withvol -l bob
aerolab agi list
aerolab agi destroy -f -n withvol
aerolab agi list
aerolab agi create --source-local . -n withvol --aws-with-efs --aws-efs-expire=30h
aerolab agi list
aerolab agi delete -f -n withvol
aerolab agi list
```
