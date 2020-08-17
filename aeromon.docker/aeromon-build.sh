export DOCKER_BUILDKIT=1

docker build --ssh default -t aerospike/aeromon .
