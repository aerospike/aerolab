[Docs home](../../README.md)

## Sub-topics

[Launch a VS Code client VM with AeroLab](vscode.md)

[Deploy a Trino server](trino.md)

[Deploy a Jupyter Notebook VM with AeroLab](jupyter.md)

[Deploy the Elasticsearch connector for Aerospike](elasticsearch.md)

[Deploy a Rest Gateway](restgw.md)

# Deploying clients with AeroLab


## Client command

```
Usage:
  aerolab [OPTIONS] client [command]

Available commands:
  create     Create new client machines
  configure  (re)configure some clients, such as ams
  list       List client machine groups
  start      Start a client machine group
  stop       Stop a client machine group
  grow       Grow a client machine group
  destroy    Destroy client(s)
  attach     symlink to: attach client
  help       Print help
```

## Client create command - supported clients

```
Usage:
  aerolab [OPTIONS] client create [command]

Available commands:
  base           simple base image
  tools          aerospike-tools
  ams            prometheus and grafana for AMS; for exporter see: cluster add exporter
  jupyter        launch a jupyter IDE client
  vscode         launch a VSCode IDE client
  trino          launch a Trino server (use 'client attach trino' to get Trino shell)
  elasticsearch  deploy elasticsearch with the es connector for aerospike
  rest-gateway   deploy a rest-gateway client machine
```

## Get help for a given client type

```bash
aerolab client create base help
aerolab client create tools help
```

## Example

### Create three base client machines and grow the client group by three tools client machines

```bash
aerolab client create base -n myclients -c 3
aerolab client grow tools -n myclients -c 3
```

### List clients

```bash
aerolab client list
```

### Destroy clients

```bash
aerolab client destroy -n myclients -f
```
