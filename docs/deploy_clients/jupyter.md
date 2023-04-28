[Docs home](../../README.md)

# Deploy a Jupyter Notebook VM with AeroLab


Jupyter is a web-based interface for interactive testing and
development of code. It is intended for testing purposes only.

Supported Jupyter kernels are: `go,python,dotnet,java,node`

## Help pages

```
aerolab client create jupyter help
```

## Create a Jupyter client machine with all kernels

```
aerolab client create jupyter -n jupyter
```

## Create a Jupyter client machine with Go and Python kernels only

```
aerolab client create jupyter -n jupyter -k go,python
```

## Add dotnet kernel to existing Jupyter client

```
aerolab client create jupyter -n jupyter -k dotnet
```

## Connect

First get IP from:

```
aerolab client list
```

Then, in the browser, navigate to:

```
http://IP:8888
```

## Seed IPs

Use the `-s` switch  when creating a Jupyter client to make the creation process auto-fill the IP addresses in the example code with the cluster IP addresses for seeding.

If you haven't used the `-s` switch when creating a jupyter client, remember to adjust the seed IP address inside the jupyter GUI when editing your code, or you won't be able to connect.

## Connectivity notes on Docker Desktop

If you use Docker Desktop, you cannot access the containers directly from your host machine.
In this case, when running the `create` commands above, add the `-e 8888:8888` switch to expose port 8888 to the
host. Then access the Jupyter server in your browser using `http://127.0.0.1:8888`.

Example:

```
aerolab client create jupyter -n jupyter -e 8888:8888
```
