# Launch a VS Code Client Machine

AeroLab supports installing a VS Code IDE in a browser, complete with Java, Go, Python and C# language clients and code examples.

## Install

### If your host can talk directly to your containers:

```bash
aerolab client create vscode -n vscode
```

### If you are using Docker Desktop instead:

```bash
aerolab client create vscode -n vscode -e 8080:8080
```

## Accessing the IDE through your browser

Language clients and example code is preloaded on the VS Code client machine.

### If your host can talk directly to your containers:

First get the IP of the client machine using `aerolab client list`.

Then visit the following URL in the browser: `http://<IP>:8080`

### If using Docker Desktop instead:

Visit `http://127.0.0.1:8080`

## Stopping and starting

AeroLab VS Code client can be stopped and started as needed, using:

```bash
aerolab client stop -n vscode
aerolab client start -n vscode
```

## Install, choosing only some language clients

```bash
aerolab client create vscode -n somename -k go,python
```

## See help to list supported language clients

```bash
aerolab client create vscode help
```
