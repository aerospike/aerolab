# Specifying custom dashboards for the AMS stack

## Parameter usage

### Specify a custom list file of dashboards to install, instead of the default set:

`aerolab client create ams --dashboards dashboards.yaml`

### Specify a custom list file of dashboards to install on top of the default set:

`aerolab client create ams --dashboards +dashboards.yaml`

## File definition

The custom dashboards YAML file must contain the following:

```yaml
- destination: ""
  fromFile: ""
- destination: ""
  fromUrl: ""
...
```

Field definitions:

* destination - the destination location including file name, starting with `/var/lib/grafana/dashboards/`
* fromFile - a relative or absolute path to a file on a local machine aerolab is running from
* fromUrl - a full URL path to download the file from

NOTE: if a destination file already exists, it will be overwritten.

## Example file

Example YAML file telling aerolab to load three dashboards, two from local disk and one from a custom URL:

```yaml
- destination: "/var/lib/grafana/dashboards/mycustom1.json"
  fromUrl: "https://example.com/path/to/mycustom.json"
- destination: "/var/lib/grafana/dashboards/mycustom2.json"
  fromFile: "relative/path/to/file.json"
- destination: "/var/lib/grafana/dashboards/mycustom3.json"
  fromFile: "/home/rglonek/absolute/path/to/file.json"
```
