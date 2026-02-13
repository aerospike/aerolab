# WebUI Parameter Override Configuration

The AeroLab WebUI supports an external YAML configuration file that allows operators to customize how command parameters are displayed in the web interface. This is useful when deploying the WebUI for a team and you want to:

- Restrict certain fields to a predefined list of choices (e.g., allowed instance types or S3 bucket names)
- Change default values for parameters (e.g., default a boolean to `true` instead of `false`)
- Hide parameters that are not relevant to your deployment
- Mark parameters as required
- Override parameter descriptions with team-specific guidance

## Usage

Pass the config file path when starting the WebUI:

```bash
aerolab webui --config /path/to/webui-overrides.yaml
```

The overrides are applied at startup after the command tree is built. If a command path or parameter name in the config file does not match anything in the current version of AeroLab, a warning is logged but startup continues normally. This means the config file is loosely coupled to AeroLab versions and will not break on upgrades.

## Config File Format

The configuration file uses YAML format with a single top-level key `overrides`. Each entry maps a **command path** to a set of **parameter overrides**, keyed by the parameter's long flag name (the `--name` you see in `aerolab <command> --help`).

```yaml
overrides:
  <command-path>:
    <parameter-long-name>:
      <property>: <value>
```

### Command Paths

Command paths use forward slashes and match the WebUI API paths. Examples:

| CLI Command | Config Path |
|---|---|
| `aerolab cluster create` | `cluster/create` |
| `aerolab aerospike start` | `aerospike/start` |
| `aerolab agi create` | `agi/create` |
| `aerolab client create ams` | `client/create/ams` |
| `aerolab conf adjust` | `conf/adjust` |

### Parameter Names

Parameters are identified by their **long flag name** (case-insensitive). This is the name after `--` in the CLI help output. For example, if `aerolab cluster create --help` shows `--instance`, use `instance` in the config file.

### Supported Override Properties

| Property | Type | Description |
|---|---|---|
| `choices` | list of strings | Sets allowed values for the parameter. Turns a free-text input or tag input into a dropdown (single-select) or multi-select (for slice types). |
| `default` | string | Overrides the default value. For booleans, use `"true"` or `"false"`. |
| `hidden` | boolean | If `true`, hides the parameter from the web interface. |
| `required` | boolean | If `true`, marks the parameter as required in the web form. |
| `description` | string | Overrides the parameter's help text / description. |

## Examples

### Restricting Instance Types to Approved Values

Turn the free-text `--instance` field into a dropdown with only your approved instance types:

```yaml
overrides:
  cluster/create:
    instance:
      choices:
        - "t3.medium"
        - "t3.large"
        - "t3.xlarge"
        - "r5.xlarge"
      default: "t3.large"
```

### Restricting S3 Bucket Names

For AGI create, restrict the S3 bucket name to a predefined list:

```yaml
overrides:
  agi/create:
    s3-bucket-name:
      choices:
        - "prod-logs-bucket"
        - "staging-logs-bucket"
        - "dev-logs-bucket"
```

### Changing Boolean Defaults

Default the `--start` flag to `false` so clusters are not auto-started:

```yaml
overrides:
  cluster/create:
    start:
      default: "false"
```

Default `--https` to `true` for AGI:

```yaml
overrides:
  agi/create:
    use-ssl:
      default: "true"
```

### Hiding Parameters

Hide parameters that are not relevant to your deployment:

```yaml
overrides:
  client/create/ams:
    seed-node:
      hidden: true
  cluster/create:
    gcp-expire:
      hidden: true
```

### Overriding Descriptions

Add team-specific guidance to a parameter:

```yaml
overrides:
  cluster/create:
    name:
      description: "Cluster name. Use the format: team-project-env (e.g., data-analytics-dev)"
```

### Marking Parameters as Required

```yaml
overrides:
  cluster/create:
    aws-expire:
      required: true
      description: "Expiry is mandatory per company policy. Use format like 8h, 1d, 7d."
```

### Full Example

```yaml
overrides:
  # Cluster creation defaults
  cluster/create:
    instance:
      choices: ["t3.medium", "t3.large", "t3.xlarge", "r5.xlarge"]
      default: "t3.large"
    count:
      default: "3"
    start:
      default: "false"
    aws-expire:
      required: true
      default: "8h"
      description: "Expiry is mandatory. Use format like 8h, 1d, 7d."
    name:
      description: "Cluster name. Use format: team-project-env"

  # AGI creation
  agi/create:
    s3-bucket-name:
      choices: ["prod-logs", "staging-logs", "dev-logs"]

  # AMS client
  client/create/ams:
    seed-node:
      hidden: true

  # Hide GCP-specific params when using AWS
  cluster/add/firewall:
    gcp-zone:
      hidden: true
```

## Discovering Command Paths and Parameter Names

### Using the CLI

```bash
# List all top-level commands
aerolab --help

# See parameters for a specific command
aerolab cluster create --help
```

### Using the WebUI API

When the WebUI is running, you can query the command tree:

```bash
# Get the full command tree
curl http://localhost:8080/api/commands | jq '.children[].name'

# Get parameters for a specific command
curl http://localhost:8080/api/commands/cluster/create | jq '.parameters[] | {name: .name, long: .long, type: .type}'
```

## Notes

- Overrides are applied once at startup. Changes to the config file require a WebUI restart.
- Parameter name matching is case-insensitive.
- If a command path or parameter name in the config does not exist, a warning is logged but startup continues. This allows the config to remain compatible across AeroLab version upgrades.
- Setting `choices` on a `[]string`, `[]int`, or `[]float` parameter turns the tag-style input into a multi-select dropdown. Setting `choices` on a scalar parameter turns it into a single-select dropdown.
