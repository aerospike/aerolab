# CPU Policy

### Apply

```bash
kubectl get nodes
kubectl debug node/ -it --image=ubuntu:22.04
chroot /host
cp /etc/kubernetes/kubelet/kubelet-config.json /etc/kubernetes/kubelet/kubelet-config.json.back
jq '. += { "cpuManagerPolicy":"static"}' /etc/kubernetes/kubelet/kubelet-config.json.back > /etc/kubernetes/kubelet/kubelet-config.json
rm -f /var/lib/kubelet/cpu_manager_state
nohup systemctl restart kubelet > /var/log/myrestart 2>&1 &
```

### Verify

```bash
kubectl get nodes
kubectl debug node/ -it --image=ubuntu:22.04
chroot /host
diff /etc/kubernetes/kubelet/kubelet-config.json /etc/kubernetes/kubelet/kubelet-config.json.back
journalctl -u kubelet |grep "Starting CPU manager"
journalctl -u kubelet -f
```
