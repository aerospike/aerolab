# Deploying AMS (Aerospike Monitoring Stack)

## Deploy AMS stack

```bash
kubectl apply -f https://docs.aerospike.com/assets/files/aerospike-monitoring-stack-94e2fe3fdeb885745e1b097958d592da.yaml
```

## Deploy Grafana Load Balancer

Save this yaml as `grafana-loadbalancer.yaml`:

```bash
apiVersion: v1
kind: Service
metadata:
  name: aerospike-monitoring-grafana-service-loadbalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"
spec:
  type: LoadBalancer
  selector:
    app: aerospike-monitoring-stack-grafana 
  ports:
    - protocol: TCP
      port: 80
      targetPort: 3000
```

Deploy the yaml file:

```bash
kubectl apply -f grafana-loadbalancer.yaml
```

## Find the access point

```bash
kubectl get all -o wide
```

Fine a line similar to this one:

```bash
service/aerospike-monitoring-grafana-service-loadbalancer   LoadBalancer   10.100.203.15   affbd394c1a66476d843ae866a710aa8-1529446045.us-west-1.elb.amazonaws.com   80:30076/TCP   21s
```

The FQDN, in this example `affbd394c1a66476d843ae866a710aa8-1529446045.us-west-1.elb.amazonaws.com`, is the domain used to access Grafana.

## Wait for the load balancer DNS to finish setting up

Install `dig` if the tool is not available: `apt-get update && apt-get -y install dnsutils`

Keep running the following command to see when the domain starts resolving to an IP: `dig affbd394c1a66476d843ae866a710aa8-1529446045.us-west-1.elb.amazonaws.com +short`. Once you see a result with an IP address, the mapping has been created. This may take 2 to 5 minutes.

## Access Grafana

The URL is `http://ELB_Domain`

For example `http://affbd394c1a66476d843ae866a710aa8-1529446045.us-west-1.elb.amazonaws.com`

When you access Grafana default user/pass is `admin/admin` and will have you change password on first login.

## Setup dashboards

Downloads documented [here](https://aerospike.com/docs/monitorstack/configure/configure-grafana#add-or-upgrade-dashboards). Below is command summary.

```bash
wget https://github.com/aerospike/aerospike-monitoring/archive/refs/tags/v3.0.0.tar.gz
tar -xvf v3.0.0.tar.gz
```

Once all the dashboards have been downloaded and unzipped, in Grafana, navigate on the left-side menu to `Dashboards->Import` and import each grafana json that was download via the wget command.

## Notes

If you want to access prometheus externally you can follow the same steps for a loadbalancer to targetPort: 9090 and change the selector to match the labels.

You can also access the endpoint by using kubectl to port-forward to all addresses and then access the container for the forwarded port: 

```bash
kubectl port-forward svc/aerospike-monitoring-stack-prometheus 9090:9090 --address='0.0.0.0'
```

Same premise can be done for exporter to view metrics for troubleshooting:

```bash
kubectl port-forward -naerospike aerocluster-1-0 9145:9145 --address='0.0.0.0'
```

* Also, make sure the sidecar has the env variables for credentials configured in main YAML as the exporter needs to authenticate if security is enabled for Aerospike Server
* By default, the prometheus will scrape containers named "exporter" so its best to keep the sidecars.ports.name as "exporter" unless you want to modify the Prometheus ConfigMap
