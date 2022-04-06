# Deploying Python Clients

Notes:
  * at the end of `runme.sh run`, a useful list of commands and IPs is printed to access the client
  * run `runme.sh get` to get the useful list again :)


## First clone this repo

```
Via Web
% git clone https://github.com/citrusleaf/aerolab.git

Via SSH
% git clone git@github.com:citrusleaf/aerolab.git
```

## Enter this directory

```
% cd aerolab/scripts/aerolab-pythonclient
```

## Get help

```
% ./runme.sh
```

## Create new PythonClient Node

```
% ./runme.sh run
```

## Destroy

```
% ./runme.sh destroy
```

## Usage

```
% ./runme.sh 

Usage: ./runme.sh start|stop|destroy|run|get

  run     - create and start Client Node
  start   - start an existing, stopped, Client Node
  stop    - stop a running Client Node, without destroying it
  get     - get the IPs of Client Node
  destroy - stop and destroy the Client Node
```

## Advanced

To access the Python client container, do the following :-
```
% ./runme.sh get
```
This will return both the client IP and the container name.

Then run the following :-
```
% docker exec -it <containername> /bin/bash
```
This will get you into the container. Once there, in /root/clients you will see the Python code.<br>

If this has been pre-configured with the aerolab-buildenv system there will be a directory called withserverip<br>
This directory will have the python pre-configured with your Clusters IP Address, if not then just change the CLUSTERIP in the code to point to your seed node.