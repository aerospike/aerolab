# Deploying clients with aerolab

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

```
aerolab client create base help
aerolab client create tools help
```

## Example

### Create 3 base client machines and grow client group by 3 tools client machines

```
aerolab client create base -n myclients -c 3
aerolab client grow tools -n myclients -c 3
```

### List clients

```
aerolab client list
```

### Destroy clients

```
aerolab client destroy -n myclients -f
```
