#### Note on backends

There are 2 possibilities, aerolab is asked to run on mac, or on a linux machine (either remotely via ssh or on the machine itself). Depending on the destination system, it will have a different backend default:
* if deploying on mac: docker
* if deployying on linux: lxc

These can be changed with a switch (or config file), as follows.

#### Quick start

Just install docker on mac, run it, and run the aerolab mac binary. It will auto-work with docker on mac with no VMs or other installation required.

#### Running aerolab on a remote linux machine

// TODO

#### Running aerolab on a remote linux machine and selecting docker as backend

// TODO

#### Running locally on a linux machine

// TODO

#### Using config files to always run on remote machine and select a backend

// TODO
