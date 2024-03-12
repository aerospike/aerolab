# run graph
set -e
docker run -itd --label aerolab.client.type=graph --name=%s --restart=always -p8182:8182 -p9090:9090 -e JAVA_OPTIONS="-Xmx%dm" -v %s:/opt/aerospike-graph/conf/aerospike-graph.properties %s %s 
