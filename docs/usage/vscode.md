# VSCode client machine

AeroLab supports installing a full VSCode in browser, with multiple language support and basic example code.

## Install

### If your host can talk directly to your containers:

```
aerolab client create vscode -n vscode
```

### If using Docker Desktop instead:

```
aerolab client create vscode -n vscode -e 8080:8080
```

## Access

Example code for the supported languages is preloaded and provided.

### If your host can talk directly to your containers:

First get the IP of the client machine using `aerolab client list`.

Then visit the following URL in the browser: `http://IP:8080`

### If using Docker Desktop instead:

Visit `http://127.0.0.1:8080`

## Stopping and starting

AeroLab vscode client can be stopped and started as needed, using:

```
aerolab client stop -n vscode
aerolab client start -n vscode
```

## Install, choosing only some languages

```
aerolab client create vscode -n somename -k go,python
```

## See help to list supported languages

```
aerolab client create vscode help
```
