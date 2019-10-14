# aeromon.docker

In conjunction with Rob Glonek's aerolab, this project allows you to quickly build a container based system allowing you to showcase our monitoring solution built using Prometheus / Grafana and Khosrow Afroozeh's aeromon.

## Pre-requisites

Docker desktop version 18.09 or higher. This in order to allow you to clone private github repositories ( citrusleaf in our case ) as part of the build. See [this medium article](https://medium.com/@tonistiigi/build-secrets-and-ssh-forwarding-in-docker-18-09-ae8161d066) for more detail.

citrusleaf credentials for enterprise downloads

An ssh key in your .ssh folder giving you read access to citrusleaf repositories. ssh-agent needs to be running so that private repos can be build during the docker build phase. The build scripts will try and take care of that, flagging problems if ssh to github not available for you.

## Aerolab Quick Start

You will need to set up a cluster using aerolab. Quick start is 

1. Get aerolab - ```git clone https://github.com/citrusleaf/opstools && cd opstools/Aero-Lab_Quick_Cluster_Spinup```
2. Make sure aerolab binary is in your path - ```export PATH=$PATH:`pwd`/bin```
3. Clone this project (aeromon.docker) and cd inside
4. Create 'standard cluster' by cd-ing to ```standard-aerospike-for-aerolab``` and running ```./make-cluster.sh```. You will need the citrusleaf password for www.aerospike.com downloads set correctly in credentials.conf
5. You can verify your cluster is available via ```aerolab aql -n standard -l 1``` - aql into 1st node in 'standard' cluster
6. ```aerolab cluster-list``` to see cluster detail 

Note aerolab provides a wealth of functionality - including the ability to set up with arbitrary configuration / size / aerospike version etc. This just to get you started

## Aeromon Start

Run ```./aeromon-setup.sh CLUSTER_NAME``` e.g. ```./aeromon-setup.sh standard``` if using 'standard' cluster

Verify with ```docker container list | grep aeromon```. Sample output

```
2954df7daa68        grafana/grafana:6.3.2       ... 0.0.0.0:3000->3000/tcp   aeromondocker_grafana_1
99caac8cb7ce        prom/prometheus:v2.11.1     ... 0.0.0.0:9090->9090/tcp   aeromondocker_prometheus_1
b562028e58c9        prom/alertmanager           ... 0.0.0.0:9093->9093/tcp   aeromondocker_alertmanager_1
ad1835044611        aerospike/aeromon           ...                          aero-standard_3-aeromon-probe
d8609e4a3811        aerospike/aeromon           ...                          aero-standard_2-aeromon-probe
cd054310f0b1        aerospike/aeromon           ...                          aero-standard_1-aeromon-probe
```

The setup script will build an aeromon image and start it with the correct config ( each aerospike host in the aerolab cluster will have a corresponding aeromon probe )

It will start up Prometheus, configuring the above probes as the targets. 

View the Grafana Dashboard on http://localhost:3000 (admin/pass)

Prometheus on http://localhost:9090. http://localhost:9090/targets to see setup is correct.

The startup script makes best efforts to verify ssh is set up correctly.

## Demonstration

Insert some data to your cluster ...

1. Build client ```aerolab make-client --language=java --name=aeromon_demo_client``` to make a java client
2. Get the IP of a cluster node ```aerolab cluster-list | grep -e "^aero-standard_1 " | awk '{print $3}'```
3. Log in to the demo client ```docker exec -it aeromon_demo_client bash```
4. ```cd /root/java/aerospike-client-java-*/benchmarks```
5. Run a benchmark using namespace device / host ip as per step 3. Alternatively copy the canned script ```standard-aerospike-for-aerolab/as-demo-benchmark-rw.sh``` into the client container : ```docker container cp SCRIPT_LOCATION CONTAINER_NAME:PATH_TO_BENCHMARK_DIRECTORY```
6. Enjoy the results from http://localhost:3000 - credentials admin/pass

## Teardown

```./aeromon-teardown.sh CLUSTER_NAME``` e.g. ```./aeromon-teardown.sh standard``` removes probes / Prometheus / Grafana containers

Remove your cluster using ```aerolab cluster-stop -n CLUSTER_NAME``` followed by ```aerolab cluster-destroy -n CLUSTER_NAME```

Stop your demo client ```docker container stop CLIENT NAME``` as found from docker container list

## Notes

The 'docker' directory is a copy of the same directory in citrusleaf/aeromon. It should probably be periodically updated. 

## References

aeromon - https://github.com/citrusleaf/aeromon

aerolab - https://github.com/citrusleaf/opstools/tree/master/Aero-Lab_Quick_Cluster_Spinup
