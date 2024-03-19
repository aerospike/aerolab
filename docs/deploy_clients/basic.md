# Launch a VS Code client VM with AeroLab

AeroLab supports launching two types of basic client machine with no extra software deployed.

## Vanilla

A vanilla container or instance gets deployed with no extra packages or software installed.

```
aerolab client create none -n somename
```

## Base

The base container or instance is used as the building block for all other client (aside from the vanilla one). It comes with a range of extra basic tools installed, including `top, netstat, sysstat, curl, wget, python3`, etc.

```
aerolab client create base -n somename
```

## Getting help

Run one of the following to get help on features, parameters and usage:

```
aerolab client create none help
aerolab client create base help
```
