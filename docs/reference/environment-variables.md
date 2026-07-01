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
| `AEROLAB_SKIP_NAT_CHECK` | 1 | GCP only. Bypass the Cloud NAT pre-check that blocks `--gcp-nopublic-ip` creates when no Cloud NAT covers the target subnet. Set when egress is provided through VPN, VPC peering, an internal proxy, or another mechanism aerolab cannot detect via `compute.routers.list` |
| `AEROLAB_ARTIFACTS_URL` | URL | Alternative source for Aerospike server install artifacts. Set to a JFrog Artifactory base URL (e.g. `https://<org>.jfrog.io`) to fetch pre-release/dev builds via the JFrog API, or to a plain HTTP mirror of `download.aerospike.com` |
| `AEROLAB_ARTIFACTS_AUTH` | CREDENTIALS | Credentials for `AEROLAB_ARTIFACTS_URL` when it points at JFrog. Accepts a bearer token, a `Bearer ...`/`Basic ...` header value, a JFrog API key, or `user:pass` |

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

## AEROLAB_SKIP_NAT_CHECK

GCP only. When the operator opts out of public IPs (`config backend --gcp-nopublic-ip` or `--gcp-disable-public-ip` on a create), aerolab queries Cloud Routers in the target region and aborts the create if no Cloud NAT covers the chosen subnet. Without egress, the install script would hang for minutes on `apt-get` / `curl` from `download.aerospike.com` before timing out.

Set this variable to bypass the check. The create proceeds regardless of NAT state.

### Values

- `1`, `true`, or `TRUE` — bypass the check
- unset / any other value — check is enforced

### Example

```bash
export AEROLAB_SKIP_NAT_CHECK=1
aerolab cluster create -n iaptest -c 2 --gcp-disable-public-ip ...
```

### Use Cases

- Egress is provided through Cloud VPN, VPC peering through a transit VPC, an internal HTTP proxy, or a hand-rolled NAT VM — none of which `compute.routers.list` can see
- The caller's service account lacks `compute.routers.list` permission and you accept the risk that NAT may be missing (the check itself soft-fails on API errors and lets the create proceed in this case, but the env var is the explicit way to silence it)
- Air-gapped environments where image-baked dependencies make outbound internet unnecessary

## AEROLAB_ARTIFACTS_URL

Point Aerolab at an alternative source for Aerospike server install artifacts instead of the public `download.aerospike.com`.

Two modes are supported, selected automatically from the URL:

- **JFrog Artifactory** — when the host ends in `.jfrog.io`, Aerolab talks to the JFrog REST/AQL API to resolve *build numbers* (including pre-release / dev builds) rather than public release versions. Downloads are `.rpm` / `.deb` packages, fetched to the operator's machine and uploaded to instances over SFTP (the auth token never reaches the target VM).
- **Plain HTTP mirror** — any other URL is treated as a mirror of the `download.aerospike.com/artifacts` HTML directory structure.

This affects `cluster create`, `template create`, `aerospike upgrade`, `installer list-versions`, and `installer download`.

### JFrog version selection

In JFrog mode, `-v` takes a build number rather than a release version. `latest` is not supported. The canonical build entry always carries an `-artifacts` suffix; Aerolab appends it automatically, so `-v 8.1.3.0-28-g302194ebc` resolves to `8.1.3.0-28-g302194ebc-artifacts`. Append `:c`, `:f`, or `:e` to force community/federal/enterprise (a plain trailing `c`/`f` is not used, since JFrog build numbers can end in a git SHA).

Use `installer list-versions` to enumerate the available build numbers.

### Optional companion variable

- `AEROLAB_ARTIFACTS_BUILD_NAME` — the JFrog build name to query. Defaults to `aerospike-server`.

### Example

```bash
export AEROLAB_ARTIFACTS_URL=https://myorg.jfrog.io
export AEROLAB_ARTIFACTS_AUTH="Bearer <token>"
aerolab installer list-versions -v 8.1.3
aerolab cluster create -n dev -c 1 -d ubuntu -i 24.04 -v 8.1.3.0-28-g302194ebc
```

### Use Cases

- Install and test pre-release / CI dev builds not published on the public download site
- Pull artifacts from an internal mirror in air-gapped or egress-restricted environments

## AEROLAB_ARTIFACTS_AUTH

Credentials for `AEROLAB_ARTIFACTS_URL` when it points at a JFrog Artifactory instance. The format is auto-detected:

- `Bearer ...` or `Basic ...` — used verbatim as the `Authorization` header
- a JWT (starts with `eyJ`) — sent as a `Bearer` token
- a JFrog reference / API key (e.g. `AKC...`, `cmVm...`) — sent as the `X-JFrog-Art-Api` header
- `user:pass` — encoded into a `Basic` `Authorization` header
- anything else — treated as a bearer token

The value is redacted (`[REDACTED]`) in `aerolab config env-vars` output and never written to logs or uploaded to instances.

### Example

```bash
export AEROLAB_ARTIFACTS_URL=https://myorg.jfrog.io
export AEROLAB_ARTIFACTS_AUTH="Bearer <token>"
# or
export AEROLAB_ARTIFACTS_AUTH="myuser:mypassword"
```

### Use Cases

- Authenticate to a private JFrog repository hosting Aerospike dev builds

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

