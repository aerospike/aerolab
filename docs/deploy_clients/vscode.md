[Docs home](../../README.md)

# Launch a VS Code client VM with AeroLab


AeroLab supports installing a VS Code IDE in a browser, complete with Java, Go, Python and C# language clients and code examples.

## Install VS Code with Aerolab

### If your host talks directly to your containers or AWS instances

```bash
aerolab client create vscode -n vscode
```

### If using Docker Desktop

```bash
aerolab client create vscode -n vscode -e 8080:8080
```

## Accessing the IDE through the browser

### If your host talks directly to your containers or AWS instances

1. Get the IP of the client machine using `aerolab client list`.
2. Visit the following URL in the browser: `http://<IP>:8080`

### If using Docker Desktop

Visit `http://127.0.0.1:8080`

## Install, choosing only some language clients

```bash
aerolab client create vscode -n somename -k go,python
```

## See help to list supported language clients

```bash
aerolab client create vscode help
```
