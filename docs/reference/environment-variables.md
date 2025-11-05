# Environment Variables Reference

Aerolab supports several environment variables for configuration and behavior control.

## Available Environment Variables

| Variable | Values | Description |
|----------|--------|-------------|
| `AEROLAB_HOME` | FILEPATH | Override the default `~/.aerolab` home directory |
| `AEROLAB_LOG_LEVEL` | 0-6 | Set log level: 0=NONE, 1=CRITICAL, 2=ERROR, 3=WARN, 4=INFO, 5=DEBUG, 6=DETAIL |
| `AEROLAB_PROJECT` | PROJECTNAME | Set project name (Aerolab v8 has a notion of projects; setting this will make it work on resources other than in the 'default' aerolab project) |
| `AEROLAB_DISABLE_UPGRADE_CHECK` | true | If set to a non-empty value, aerolab will not check if upgrades are available |
| `AEROLAB_TELEMETRY_DISABLE` | true | If set to a non-empty value, telemetry will be disabled |
| `AEROLAB_CONFIG_FILE` | FILEPATH | If set, aerolab will read the given defaults config file instead of `$AEROLAB_HOME/conf` |
| `AEROLAB_NONINTERACTIVE` | true | If set to a non-empty value, aerolab will not ask for confirmation or choices at any point |
| `AEROSPIKE_CLOUD_VERSION` | v1, v2 | The version of the Aerospike Cloud API to use. If unset, will use the old `v2`. If set to `v1`, will use the new V1 API endpoints. |

## AEROLAB_HOME

Override the default home directory where Aerolab stores configuration and data.

### Default

`~/.aerolab`

### Example

```bash
export AEROLAB_HOME=/custom/path/aerolab
aerolab config backend -t docker
```

### Use Cases

- Use different configurations for different projects
- Run Aerolab in isolated environments
- Test configurations without affecting main setup

## AEROLAB_LOG_LEVEL

Set the log level for Aerolab output.

### Values

- `0` - NONE (no logging)
- `1` - CRITICAL
- `2` - ERROR
- `3` - WARN
- `4` - INFO (default)
- `5` - DEBUG
- `6` - DETAIL

### Example

```bash
export AEROLAB_LOG_LEVEL=5
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*'
```

### Use Cases

- Debug issues with detailed logging
- Reduce output for automated scripts
- Monitor specific log levels

## AEROLAB_PROJECT

Set the project name for resource isolation.

### Default

`default`

### Example

```bash
export AEROLAB_PROJECT=production
aerolab cluster create -c 2 -d ubuntu -i 24.04 -v '8.*'
```

### Use Cases

- Separate resources by environment (dev, staging, production)
- Isolate resources for different teams
- Organize resources by project

## AEROLAB_DISABLE_UPGRADE_CHECK

Disable automatic upgrade checks.

### Example

```bash
export AEROLAB_DISABLE_UPGRADE_CHECK=true
aerolab cluster list
```

### Use Cases

- Reduce network calls in automated environments
- Disable upgrade checks in CI/CD pipelines
- Speed up command execution

## AEROLAB_TELEMETRY_DISABLE

Disable telemetry collection.

### Example

```bash
export AEROLAB_TELEMETRY_DISABLE=true
aerolab cluster list
```

### Use Cases

- Privacy concerns
- Compliance requirements
- Reduce network calls

## AEROLAB_CONFIG_FILE

Specify a custom configuration file path.

### Default

`$AEROLAB_HOME/conf`

### Example

```bash
export AEROLAB_CONFIG_FILE=/custom/path/config.yaml
aerolab config backend -t docker
```

### Use Cases

- Use different configuration files for different projects
- Share configuration files across systems
- Test configuration changes

## AEROLAB_NONINTERACTIVE

Disable interactive prompts and confirmations.

### Example

```bash
export AEROLAB_NONINTERACTIVE=true
aerolab cluster destroy -n mydc --force
```

### Use Cases

- Automated scripts and CI/CD pipelines
- Prevent accidental confirmations
- Batch operations

## Common Usage Patterns

### Development Environment

```bash
export AEROLAB_HOME=~/aerolab-dev
export AEROLAB_PROJECT=development
export AEROLAB_LOG_LEVEL=5
export AEROLAB_TELEMETRY_DISABLE=true
```

### Production Environment

```bash
export AEROLAB_HOME=~/aerolab-prod
export AEROLAB_PROJECT=production
export AEROLAB_LOG_LEVEL=4
export AEROLAB_DISABLE_UPGRADE_CHECK=true
```

### CI/CD Pipeline

```bash
export AEROLAB_HOME=/tmp/aerolab
export AEROLAB_PROJECT=ci-test
export AEROLAB_LOG_LEVEL=3
export AEROLAB_NONINTERACTIVE=true
export AEROLAB_TELEMETRY_DISABLE=true
export AEROLAB_DISABLE_UPGRADE_CHECK=true
```

### Testing Environment

```bash
export AEROLAB_HOME=~/aerolab-test
export AEROLAB_PROJECT=testing
export AEROLAB_LOG_LEVEL=6
export AEROLAB_TELEMETRY_DISABLE=true
```

## Tips

1. **Project Isolation**: Use `AEROLAB_PROJECT` to separate resources by environment
2. **Debugging**: Set `AEROLAB_LOG_LEVEL=5` or `6` for detailed debugging
3. **Automation**: Use `AEROLAB_NONINTERACTIVE=true` in automated scripts
4. **Privacy**: Use `AEROLAB_TELEMETRY_DISABLE=true` to disable telemetry
5. **Configuration**: Use `AEROLAB_CONFIG_FILE` to use custom configuration files
6. **Home Directory**: Use `AEROLAB_HOME` to isolate Aerolab installations

## Persistent Configuration

To make environment variables persistent, add them to your shell profile:

```bash
# Add to ~/.bashrc or ~/.zshrc
export AEROLAB_HOME=~/aerolab
export AEROLAB_LOG_LEVEL=4
export AEROLAB_TELEMETRY_DISABLE=true
```

Or create a script for different environments:

```bash
#!/bin/bash
# ~/bin/aerolab-dev
export AEROLAB_HOME=~/aerolab-dev
export AEROLAB_PROJECT=development
export AEROLAB_LOG_LEVEL=5
exec aerolab "$@"
```

