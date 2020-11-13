# TODO

## small fixes

* custom docker images to allow for really fast deployments on docker
* add option to attach to docker host, or see dmesg
  * docker run -it --rm --privileged --pid=host debian nsenter -t 1 -m -u -n -i dmesg

## TODO for next major release (3.x)

* webInterface
* cleanup return codes and error handling + error lang messages
* correct accordingly with go-lint
