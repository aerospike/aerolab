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
| `AEROLAB_NOERROR_ON_NOT_IMPLEMENTED` | true | If set to a non-empty value, aerolab will not return an error when a called function is not implemented in a particular backend |
| `AEROSPIKE_CLOUD_ENV` | dev | Set to `dev` to use development environment for Aerospike Cloud API endpoints |
| `AEROSPIKE_CLOUD_KEY` | API_KEY | Set the API key for Aerospike Cloud API |
| `AEROSPIKE_CLOUD_SECRET` | API_SECRET | Set the API secret for Aerospike Cloud API |
| `AEROLAB_DEBUG` | 1 | If set to 1, aerolab will print more detailed output and not terminate instances on certain errors |
| `AEROLAB_SIMPLE_MODE` | FILEPATH | Path to a simple mode config file that overrides which commands/parameters are visible in simple mode |
| `AEROLAB_FORCE_SIMPLE_MODE` | true | Enforce simple mode: blocked commands cannot be run, blocked parameters cannot be changed from defaults |

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

## AEROLAB_NOERROR_ON_NOT_IMPLEMENTED

Disable errors when functions are not implemented in a backend.

### Example

```bash
export AEROLAB_NOERROR_ON_NOT_IMPLEMENTED=true
aerolab cluster list
```

### Use Cases

- Graceful degradation when using backends with incomplete implementations
- Testing compatibility across multiple backends
- Allowing commands to continue despite unsupported operations

## AEROSPIKE_CLOUD_ENV

Set the environment for Aerospike Cloud API endpoints.

### Values

- `dev` - Use development environment endpoints

### Example

```bash
export AEROSPIKE_CLOUD_ENV=dev
aerolab cloud credentials list
```

### Use Cases

- Testing against Aerospike Cloud development environment
- Internal development and testing
- QA and validation workflows

## AEROLAB_SIMPLE_MODE

Point to a configuration file that overrides which commands and parameters are visible in simple mode. This allows administrators to customize the user experience by hiding or showing specific functionality.

### Config File Format

The file uses a simple line-based format:

- Lines starting with `#` are comments
- Empty lines are ignored
- Lines start with `+` (show) or `-` (hide)
- Paths use dots as separators, matching CLI command and parameter names
- `ALL` is a special keyword matching everything
- `*` at the end of a path matches all descendants
- Rules are evaluated top-to-bottom; last matching rule wins
- Matching is case-insensitive

### Path Naming

- **Commands** use the CLI command names (lowercase, hyphenated): `cluster`, `cluster.create`, `agi.add-auth-token`
- **Parameters** use the `--long` flag name appended to the command path: `cluster.create.name`, `cluster.create.count`

### Example Config File

```
# Hide everything first, then selectively enable
-ALL

# Allow cluster operations
+cluster
+cluster.create
+cluster.list
+cluster.destroy
+cluster.start
+cluster.stop

# Allow AGI with all subcommands and parameters
+agi
+agi.*

# But hide the add-auth-token subcommand
-agi.add-auth-token

# Show specific parameters for cluster create
+cluster.create.name
+cluster.create.count
+cluster.create.distro
+cluster.create.distro-version
+cluster.create.aerospike-version
```

### Example: Hide Only Advanced Options

```
# Start from defaults (everything shown), hide specific items
-attach
-volumes
-cluster.create.heartbeat-mode
-cluster.create.max-retries
```

### Example

```bash
export AEROLAB_SIMPLE_MODE=/etc/aerolab/simple-mode.conf
aerolab webui -l :8080
```

### Use Cases

- Customize the WebUI to show only relevant commands for specific teams
- Hide advanced or dangerous operations from inexperienced users
- Create role-based command visibility without code changes

## AEROLAB_FORCE_SIMPLE_MODE

When set to `true`, enforces simple mode restrictions at all levels:

- **CLI**: Blocked commands return an error; blocked parameters cannot be changed from their defaults
- **WebUI**: The simple mode toggle is locked on and cannot be disabled; blocked commands are rejected server-side
- **MCP server**: Blocked commands and blocked parameters are removed from the advertised tool list and input schemas, and any call that reaches them is rejected before the subprocess is forked
- Commands `config`, `webui`, `mcp`, `help`, `version`, `completion`, and `upgrade` are always allowed to prevent lockout

### Example

```bash
export AEROLAB_SIMPLE_MODE=/etc/aerolab/simple-mode.conf
export AEROLAB_FORCE_SIMPLE_MODE=true
aerolab cluster create  # works if allowed by config
aerolab attach shell    # ERROR: command 'attach.shell' is not available in simple mode
```

### Use Cases

- Enforce restricted command sets in production environments
- Prevent users from accessing hidden functionality via CLI
- Create locked-down shared environments

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

