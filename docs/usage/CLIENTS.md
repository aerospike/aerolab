# Deploy Clients with AeroLab

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
  aerolab [OPTIONS] client create [base | tools | help]

Available commands:
  base     simple base image
  tools    aerospike-tools
  ams      prometheus and grafana for AMS; for exporter see: cluster add exporter
  jupyter  launch a jupyter IDE client
  help     Print help
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
