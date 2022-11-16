# VSCode client machine

AeroLab supports installing a full VSCode in browser, with multiple language support and basic example code.

## Install

```
aerolab client create vscode -n vscode
```

## Access

First get the IP of the client machine using `aerolab client list`.

Then visit the following URL in the browser: `http://IP:8080`

Example code for the supported languages is preloaded and provided.

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
