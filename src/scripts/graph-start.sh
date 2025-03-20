# run graph
set -e
docker run -itd --label aerolab.client.type=graph --name=%s --restart=always %s -p8182:8182 -p9090:9090 -v %s:/opt/aerospike-graph/aerospike-graph.properties %s %s 
