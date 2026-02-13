# AeroLab REST API (webui)

The `aerolab webui` command launches an **asynchronous** REST API server that exposes all AeroLab functionality via HTTP endpoints. Commands are executed as background jobs, allowing you to submit commands and monitor their progress without blocking.

## Quick Start

```bash
# Start the REST API server on default port 8080
aerolab webui

# Start with custom port
aerolab webui --listen :9090

# Start with basic authentication
aerolab webui --auth basic --basic-user admin --basic-pass secretpass

# Start with HTTPS
aerolab webui --https --cert server.crt --key server.key

# Start with user identification via header
aerolab webui --user-header X-User
```

## Command Options

| Option | Default | Description |
|--------|---------|-------------|
| `--listen, -l` | `:8080` | Address to listen on (host:port) |
| `--https` | `false` | Enable HTTPS |
| `--cert` | | Path to TLS certificate file (required if --https) |
| `--key` | | Path to TLS key file (required if --https) |
| `--auth` | `none` | Authentication type: `none`, `basic`, or `token` |
| `--basic-user` | `admin` | Basic auth username |
| `--basic-pass` | | Basic auth password (required if --auth=basic) |
| `--token-path` | | Path to file containing valid tokens, one per line (required if --auth=token) |
| `--cors-origins` | `*` | Comma-separated list of allowed CORS origins |
| `--read-timeout` | `300` | HTTP read timeout in seconds |
| `--write-timeout` | `300` | HTTP write timeout in seconds |
| `--user-header` | | Header name to extract user from (e.g., `X-User`, `X-Forwarded-User`) |
| `--cleanup-after` | `30d` | Auto-delete completed/failed jobs older than this (e.g., `30d`, `24h`, `168h`). Set to `0` to disable. |
| `--max-job-runtime` | `24h` | Maximum time a job can run before being killed (e.g., `1h`, `30m`). Set to `0` for no limit. |
| `--cleanup-interval` | `1h` | How often to run cleanup of old jobs |

## API Endpoints

### Exploration Endpoints

These endpoints allow you to discover available commands and their parameters.

#### GET /api/commands

Returns the complete command tree with all metadata.

```bash
curl http://localhost:8080/api/commands
```

**Response:**
```json
{
  "name": "aerolab",
  "path": "",
  "description": "AeroLab CLI",
  "hasChildren": true,
  "simpleMode": true,
  "children": [
    {
      "name": "cluster",
      "path": "cluster",
      "description": "Create and manage Aerospike clusters and nodes",
      "icon": "fas fa-database",
      "hasChildren": true,
      "simpleMode": true,
      "children": [...]
    }
  ]
}
```

#### GET /api/commands/{path}

Returns details for a specific command.

```bash
curl http://localhost:8080/api/commands/cluster/create
```

#### GET /api/health

Returns server health status.

```bash
curl http://localhost:8080/api/health
```

**Response:**
```json
{
  "status": "ok",
  "version": "8.0.0"
}
```

### Job Management Endpoints

The API executes commands asynchronously. When you submit a command, you receive a job ID immediately, and the command runs in the background.

#### PUT or POST /{command/path} - Submit Job

Submit a command for asynchronous execution.

**Query Parameters:**
| Parameter | Default | Description |
|-----------|---------|-------------|
| `dryRun` | `false` | If `true`, returns the equivalent CLI command without executing |
| `preferShort` | `false` | If `true` (with `dryRun=true`), use short flags (`-n`) instead of long flags (`--name`) |

```bash
# Create a 3-node cluster
curl -X PUT http://localhost:8080/cluster/create \
  -H "Content-Type: application/json" \
  -d '{
    "name": "testdc",
    "count": 3,
    "AerospikeVersion": "7.0.0.0"
  }'
```

**Response (202 Accepted):**
```json
{
  "jobId": "abc123XYZ-1706889600",
  "user": "admin",
  "commandPath": "cluster/create",
  "cliCommand": "aerolab cluster create --name=testdc --count=3 --AerospikeVersion=7.0.0.0",
  "status": "pending",
  "createdAt": "2026-02-02T12:00:00Z",
  "statusUrl": "http://localhost:8080/api/jobs/abc123XYZ-1706889600",
  "logsUrl": "http://localhost:8080/api/jobs/abc123XYZ-1706889600/logs",
  "logsStreamUrl": "http://localhost:8080/api/jobs/abc123XYZ-1706889600/logs/stream"
}
```

The job ID format is `{shortuuid}-{unix_timestamp}`, which includes the execution start time.

#### PUT or POST /{command/path}?dryRun=true - Dry Run (Preview CLI)

Preview the equivalent CLI command without executing it. This is useful for:
- Validating parameters before execution
- Generating CLI commands to run manually or in scripts
- Debugging parameter handling

```bash
# Preview the CLI command without executing
curl -X PUT "http://localhost:8080/cluster/create?dryRun=true" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "testdc",
    "count": 3,
    "AerospikeVersion": "7.0.0.0"
  }'

# Use short flags in output
curl -X PUT "http://localhost:8080/cluster/create?dryRun=true&preferShort=true" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "testdc",
    "count": 3
  }'
```

**Response (200 OK):**
```json
{
  "dryRun": true,
  "commandPath": "cluster/create",
  "cli": "aerolab cluster create --name=testdc --count=3 --aerospike-version=7.0.0.0",
  "parameters": {
    "name": "testdc",
    "count": 3,
    "AerospikeVersion": "7.0.0.0"
  }
}
```

The generated CLI command:
- Uses proper shell escaping for special characters (spaces, quotes, `$`, etc.)
- Omits parameters that match their default values
- Handles nested parameter groups with proper namespace prefixes
- Uses reflection to accurately reconstruct the command from struct definitions

#### GET /api/jobs - List Jobs

List jobs with optional filters.

```bash
# List all your jobs
curl http://localhost:8080/api/jobs

# Filter by status
curl "http://localhost:8080/api/jobs?status=running"
curl "http://localhost:8080/api/jobs?status=completed"
curl "http://localhost:8080/api/jobs?status=failed"

# List all users' jobs (admin)
curl "http://localhost:8080/api/jobs?all=true"
```

**Response:**
```json
{
  "jobs": [
    {
      "id": "abc123XYZ-1706889600",
      "user": "admin",
      "commandPath": "cluster/create",
      "parameters": {"name": "testdc", "count": 3},
      "cliCommand": "aerolab cluster create --name=testdc --count=3",
      "status": "running",
      "createdAt": "2026-02-02T12:00:00Z",
      "startedAt": "2026-02-02T12:00:01Z"
    }
  ],
  "count": 1
}
```

#### GET /api/jobs/{jobId} - Get Job Details

Get detailed status of a specific job.

```bash
curl http://localhost:8080/api/jobs/abc123XYZ-1706889600
```

**Response:**
```json
{
  "id": "abc123XYZ-1706889600",
  "user": "admin",
  "commandPath": "cluster/create",
  "parameters": {"name": "testdc", "count": 3},
  "cliCommand": "aerolab cluster create --name=testdc --count=3",
  "status": "completed",
  "createdAt": "2026-02-02T12:00:00Z",
  "startedAt": "2026-02-02T12:00:01Z",
  "completedAt": "2026-02-02T12:05:30Z",
  "refreshInventory": true,
  "pid": 12345,
  "exitCode": 0
}
```

Additional fields for subprocess execution:
- `pid` - Process ID of the subprocess (for running jobs)
- `exitCode` - Exit code of the subprocess (when completed)
- `cancelled` - Whether the job was cancelled by user
- `timedOut` - Whether the job was killed due to timeout

#### GET /api/jobs/{jobId}/logs - Get Job Logs (One-Shot)

Get the current log output for a job.

```bash
curl http://localhost:8080/api/jobs/abc123XYZ-1706889600/logs
```

**Response:**
```json
{
  "jobId": "abc123XYZ-1706889600",
  "status": "running",
  "logs": "[INFO] Starting command: cluster/create\n[INFO] Parameters: map[count:3 name:testdc]\n..."
}
```

#### DELETE /api/jobs/{jobId} - Cancel Job

Cancel a running job by sending SIGTERM (graceful) or SIGKILL (force) to the subprocess.

```bash
# Graceful cancellation (SIGTERM)
curl -X DELETE http://localhost:8080/api/jobs/abc123XYZ-1706889600

# Force kill (SIGKILL)
curl -X DELETE "http://localhost:8080/api/jobs/abc123XYZ-1706889600?force=true"
```

**Response:**
```json
{
  "status": "cancelled",
  "signal": "terminated",
  "force": false,
  "jobId": "abc123XYZ-1706889600",
  "message": "Sent terminated to process 12345"
}
```

**Error Responses:**
- `400 Bad Request` - Job is not running or has no PID
- `404 Not Found` - Job not found or process not found
- `410 Gone` - Process already completed

#### GET /api/jobs/{jobId}/logs/stream - Stream Job Logs (SSE)

Stream logs in real-time using Server-Sent Events (SSE) until the job completes.

```bash
curl -N http://localhost:8080/api/jobs/abc123XYZ-1706889600/logs/stream
```

**SSE Event Stream:**
```
event: status
data: running

data: [INFO] Starting command: cluster/create
data: [INFO] Creating instances...

data: [INFO] Instance creation complete

event: complete
data: {"status":"completed","error":""}
```

**JavaScript Example:**
```javascript
const eventSource = new EventSource('/api/jobs/abc123XYZ-1706889600/logs/stream');

eventSource.onmessage = (event) => {
  console.log('Log:', event.data);
};

eventSource.addEventListener('status', (event) => {
  console.log('Status:', event.data);
});

eventSource.addEventListener('complete', (event) => {
  const result = JSON.parse(event.data);
  console.log('Job completed with status:', result.status);
  eventSource.close();
});

eventSource.addEventListener('error', (event) => {
  console.error('Error:', event.data);
  eventSource.close();
});
```

### CLI Generation Endpoint

#### POST /api/generate-cli - Generate CLI Command

Generate the equivalent CLI command without executing it. This endpoint uses reflection to accurately reconstruct the command, properly handling default values, nested parameter groups, and shell escaping.

**Request Body:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `commandPath` | string | Yes | Command path (e.g., `cluster/create`) |
| `parameters` | object | No | Command parameters as key-value pairs |
| `preferShort` | boolean | No | If `true`, use short flags (`-n`) instead of long flags (`--name`). Default: `false` |

```bash
# Generate CLI with long flags (default)
curl -X POST http://localhost:8080/api/generate-cli \
  -H "Content-Type: application/json" \
  -d '{
    "commandPath": "cluster/create",
    "parameters": {
      "name": "testdc",
      "count": 3,
      "AerospikeVersion": "7.0.0.0"
    }
  }'

# Generate CLI with short flags where available
curl -X POST http://localhost:8080/api/generate-cli \
  -H "Content-Type: application/json" \
  -d '{
    "commandPath": "cluster/create",
    "parameters": {
      "name": "testdc",
      "count": 3
    },
    "preferShort": true
  }'
```

**Response:**
```json
{
  "cli": "aerolab cluster create --name=testdc --count=3 --aerospike-version=7.0.0.0"
}
```

**With `preferShort: true`:**
```json
{
  "cli": "aerolab cluster create -n=testdc -c=3"
}
```

**Features:**
- **Shell escaping**: Values with special characters are properly quoted (e.g., `'value with spaces'`)
- **Default omission**: Parameters matching their default values are omitted
- **Namespace handling**: Nested parameter groups include proper prefixes (e.g., `--aws-instance`)
- **Reflection-based**: Uses the actual command struct definitions for accuracy

### File Operations

File operations are handled via command paths that have parameters with `webType: "upload"` or `webType: "download"`. These special handlers stream files directly between HTTP and SFTP without temporary storage.

#### Download Files (Streaming)

For commands that have a parameter with `webType: "download"`, a GET request streams files from cluster nodes as a tar.gz archive directly to the response.

**Query Parameters:**
- `cluster` or `name` (required): Cluster name
- `source` or `path` (required): Remote path to download
- `nodes` (optional): Comma-separated list of node numbers (e.g., `1,2,3` or `1-5`)

```bash
# Download /etc/aerospike from all nodes in cluster "mydc"
# The actual endpoint path depends on the command (e.g., files/download if such a command exists)
curl "http://localhost:8080/{command-path}?cluster=mydc&source=/etc/aerospike" \
  -o aerospike-config.tar.gz

# Download from specific nodes
curl "http://localhost:8080/{command-path}?cluster=mydc&nodes=1,2&source=/var/log/aerospike.log" \
  -o logs.tar.gz
```

The response is a `tar.gz` archive with files organized by node: `node-1/filename`, `node-2/filename`, etc.

#### Upload Files (Streaming)

For commands that have a parameter with `webType: "upload"`, a POST request with multipart form data streams files directly to cluster nodes via SFTP.

**Form Fields:**
- `cluster` or `name` (required): Cluster name
- `destination` or `path` (required): Remote destination path
- `nodes` (optional): Comma-separated list of node numbers
- `file` (required): The file to upload

```bash
# Upload a single file to all nodes
curl -X POST "http://localhost:8080/{command-path}?cluster=mydc&destination=/opt/myfile.txt" \
  -F "file=@localfile.txt"

# Upload a tar.gz archive (automatically extracted on remote)
curl -X POST "http://localhost:8080/{command-path}?cluster=mydc&destination=/opt/app" \
  -F "file=@app.tar.gz"
```

If the uploaded file has a `.tar.gz` or `.tgz` extension, it is automatically extracted to the destination directory on the remote nodes.

## User Identification

The API identifies users in the following order:

1. **Custom Header** (if `--user-header` is configured): Uses the value of the specified header
2. **Basic Auth Username** (if using basic auth): Uses the authenticated username
3. **System User**: Falls back to the current system user (standard aerolab owner)

This user is associated with all jobs created via the API.

## Job Status Values

| Status | Description |
|--------|-------------|
| `pending` | Job created, waiting to start |
| `running` | Job is currently executing |
| `completed` | Job finished successfully |
| `failed` | Job finished with an error |
| `error` | Job encountered a system error (e.g., failed to open log file) |

### Job Lifecycle Fields

When a job completes, additional fields may be present:

| Field | Description |
|-------|-------------|
| `pid` | Process ID of the subprocess |
| `exitCode` | Exit code of the subprocess (0 = success) |
| `cancelled` | `true` if job was cancelled by user |
| `timedOut` | `true` if job was killed due to timeout |

## Job Lifecycle Management

### Automatic Cleanup

By default, completed/failed jobs older than 30 days are automatically deleted. Configure with:

```bash
# Cleanup jobs older than 7 days, check every 30 minutes
aerolab webui --cleanup-after 7d --cleanup-interval 30m

# Disable automatic cleanup
aerolab webui --cleanup-after 0
```

### Job Timeout

By default, jobs are killed after 24 hours. Configure with:

```bash
# Kill jobs after 1 hour
aerolab webui --max-job-runtime 1h

# Disable timeout (jobs can run indefinitely)
aerolab webui --max-job-runtime 0
```

### Job Cancellation

Cancel a running job using the DELETE endpoint:

```bash
# Graceful cancellation (SIGTERM)
curl -X DELETE http://localhost:8080/api/jobs/abc123XYZ-1706889600

# Force kill (SIGKILL) - use if graceful doesn't work
curl -X DELETE "http://localhost:8080/api/jobs/abc123XYZ-1706889600?force=true"
```

## Job Storage

Jobs are stored in the AeroLab home directory:

```
~/.aerolab/restapi/commands/
├── {user}/
│   ├── {jobId}/
│   │   ├── command.json    # Job metadata
│   │   └── command.log     # Log output
│   └── {jobId}/
│       └── ...
└── {user}/
    └── ...
```

The `command.json` file contains:
- Job ID, user, command path, parameters
- Generated CLI command equivalent
- Status (running/completed/error/failed)
- Timestamps: createdAt, startedAt, completedAt
- Error message (if any)

## Authentication

### No Authentication (default)

```bash
aerolab webui --auth none
```

### Basic Authentication

```bash
aerolab webui --auth basic --basic-user admin --basic-pass mypassword
```

```bash
curl -u admin:mypassword http://localhost:8080/api/commands
```

### Token Authentication

Create a tokens file with one token per line:

```bash
echo "my-secure-token-at-least-64-characters-long-for-security" > /path/to/tokens.txt
```

```bash
aerolab webui --auth token --token-path /path/to/tokens.txt
```

```bash
curl -H "X-Auth-Token: my-secure-token-at-least-64-characters-long-for-security" \
  http://localhost:8080/api/commands
```

## Parameter Metadata

The API returns rich metadata about each parameter for building dynamic UIs:

| Field | Description |
|-------|-------------|
| `name` | Parameter name (from `long` tag or field name) |
| `fieldName` | Go struct field name |
| `short` | Short flag (e.g., `-n`) |
| `long` | Long flag (e.g., `--name`) |
| `description` | Help text |
| `type` | Data type: `string`, `int`, `bool`, `float`, `duration`, `[]string`, etc. |
| `default` | Default value |
| `required` | Whether the field is required in web UI |
| `webType` | Special input type: `password`, `upload`, `download` |
| `choices` | Array of valid choices |
| `hidden` | Hidden from CLI help |
| `webHidden` | Hidden from web UI |
| `simpleMode` | Show in simple mode (defaults to true) |
| `group` | Parameter group name |
| `isSlice` | Whether the parameter accepts multiple values |
| `isPositional` | Whether it's a positional argument |

### Command Metadata

| Field | Description |
|-------|-------------|
| `name` | Command name |
| `path` | Full command path (e.g., `cluster/create`) |
| `description` | Command description |
| `icon` | FontAwesome icon class |
| `hidden` | Hidden from CLI help |
| `webHidden` | Hidden from web UI |
| `simpleMode` | Show in simple mode |
| `hasChildren` | Whether this command has subcommands |
| `invWebForce` | Whether executing this command should trigger inventory refresh |
| `children` | Array of child commands |
| `parameters` | Array of command parameters |

## How It Works

### Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ REST Client │────▶│  HTTP Server     │────▶│ Job Manager     │
└─────────────┘     └──────────────────┘     └─────────────────┘
      │                     │                        │
      │                     │                        ▼
      │              ┌──────┴──────┐         ┌─────────────────┐
      │              │             │         │ Subprocess      │
      │              ▼             ▼         │ Launcher        │
      │         Submit        Query/        └────────┬────────┘
      │          Job          Stream                 │
      │              │             │                 ▼
      └──────── jobId ◀───── job status    ┌─────────────────────┐
                                           │  aerolab webui exec │
                                           │  (subprocess)       │
                                           │                     │
                                           │  - Reads params     │
                                           │    from stdin       │
                                           │  - Stdout/stderr    │
                                           │    to log file      │
                                           │  - Killable via     │
                                           │    SIGTERM/SIGKILL  │
                                           └─────────────────────┘
```

Jobs are executed as subprocesses using a hidden `aerolab webui exec` command. This architecture provides:

1. **Complete log capture** - All stdout/stderr is captured to the log file
2. **Job cancellation** - Running jobs can be cancelled via `DELETE /api/jobs/{jobId}`
3. **Timeout enforcement** - Jobs are automatically killed after `--max-job-runtime`
4. **Process isolation** - Each job runs in its own process

### Async Execution Flow

1. **Submit**: Client sends PUT/POST request with JSON parameters
2. **Create Job**: Server creates job record with unique ID (`shortuuid-timestamp`)
3. **Return Immediately**: Server returns 202 Accepted with job details and URLs
4. **Background Execution**: Command runs in a goroutine, logging to file
5. **Monitor**: Client polls job status or streams logs via SSE
6. **Complete**: Job status updates to `completed` or `failed`

### Non-Interactive Mode

The REST API automatically sets `AEROLAB_NONINTERACTIVE=1` at startup. This ensures:
- Commands never prompt for user input
- Destructive operations require explicit `force` parameters
- No hanging on confirmation prompts

### Dynamic Command Discovery

The API uses Go reflection to walk the `Commands` struct and extract:
1. All commands and subcommands (from `command` struct tags)
2. Parameter definitions (from `short`, `long`, `description`, `default` tags)
3. Web UI metadata (from `webicon`, `webhidden`, `simplemode`, `webchoice`, etc.)

## Examples

### Python Client

```python
import requests
import time
import json

BASE_URL = "http://localhost:8080"
AUTH = ("admin", "password")

# Submit a job
resp = requests.put(
    f"{BASE_URL}/cluster/create",
    auth=AUTH,
    json={"name": "mydc", "count": 3}
)
job = resp.json()
job_id = job["jobId"]
print(f"Job submitted: {job_id}")
print(f"CLI equivalent: {job['cliCommand']}")

# Poll for completion
while True:
    resp = requests.get(f"{BASE_URL}/api/jobs/{job_id}", auth=AUTH)
    status = resp.json()
    print(f"Status: {status['status']}")
    
    if status["status"] in ["completed", "failed", "error"]:
        break
    time.sleep(5)

# Get logs
resp = requests.get(f"{BASE_URL}/api/jobs/{job_id}/logs", auth=AUTH)
print(resp.json()["logs"])

# Cancel a running job (graceful)
resp = requests.delete(f"{BASE_URL}/api/jobs/{job_id}", auth=AUTH)
print(f"Cancelled: {resp.json()}")

# Force kill a running job
resp = requests.delete(f"{BASE_URL}/api/jobs/{job_id}?force=true", auth=AUTH)
print(f"Force killed: {resp.json()}")
```

### JavaScript Client with SSE

```javascript
const BASE_URL = 'http://localhost:8080';

async function createCluster(name, count) {
  // Submit job
  const resp = await fetch(`${BASE_URL}/cluster/create`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, count })
  });
  const job = await resp.json();
  console.log('Job submitted:', job.jobId);
  console.log('CLI:', job.cliCommand);
  
  // Stream logs
  return new Promise((resolve, reject) => {
    const es = new EventSource(`${BASE_URL}/api/jobs/${job.jobId}/logs/stream`);
    
    es.onmessage = (e) => console.log(e.data);
    
    es.addEventListener('complete', (e) => {
      const result = JSON.parse(e.data);
      es.close();
      if (result.status === 'completed') {
        resolve(result);
      } else {
        reject(new Error(result.error));
      }
    });
    
    es.onerror = () => {
      es.close();
      reject(new Error('Stream error'));
    };
  });
}

// Usage
createCluster('testdc', 3)
  .then(() => console.log('Cluster created!'))
  .catch(err => console.error('Failed:', err));
```

### Curl Workflow

```bash
# 1. Submit job
JOB_ID=$(curl -s -X PUT http://localhost:8080/cluster/create \
  -H "Content-Type: application/json" \
  -d '{"name":"testdc","count":3}' | jq -r '.jobId')

echo "Job ID: $JOB_ID"

# 2. Check status
curl -s "http://localhost:8080/api/jobs/$JOB_ID" | jq '.status'

# 3. Get logs
curl -s "http://localhost:8080/api/jobs/$JOB_ID/logs" | jq -r '.logs'

# 4. Stream logs (blocking until complete)
curl -N "http://localhost:8080/api/jobs/$JOB_ID/logs/stream"

# 5. Cancel a running job (optional)
curl -X DELETE "http://localhost:8080/api/jobs/$JOB_ID"

# 6. Force kill a job (if graceful doesn't work)
curl -X DELETE "http://localhost:8080/api/jobs/$JOB_ID?force=true"
```

### Generate CLI Without Executing

There are two ways to generate a CLI command without executing it:

**Method 1: Using /api/generate-cli endpoint**

```bash
# With long flags (default)
curl -X POST http://localhost:8080/api/generate-cli \
  -H "Content-Type: application/json" \
  -d '{
    "commandPath": "cluster/create",
    "parameters": {
      "name": "prod",
      "count": 5,
      "AerospikeVersion": "7.0.0.0"
    }
  }' | jq -r '.cli'

# Output: aerolab cluster create --name=prod --count=5 --aerospike-version=7.0.0.0

# With short flags
curl -X POST http://localhost:8080/api/generate-cli \
  -H "Content-Type: application/json" \
  -d '{
    "commandPath": "cluster/create",
    "parameters": {"name": "prod", "count": 5},
    "preferShort": true
  }' | jq -r '.cli'

# Output: aerolab cluster create -n=prod -c=5
```

**Method 2: Using dryRun query parameter on any command endpoint**

```bash
# Dry run on the command endpoint directly
curl -X PUT "http://localhost:8080/cluster/create?dryRun=true" \
  -H "Content-Type: application/json" \
  -d '{"name": "prod", "count": 5}' | jq

# Output:
# {
#   "dryRun": true,
#   "commandPath": "cluster/create",
#   "cli": "aerolab cluster create --name=prod --count=5",
#   "parameters": {"name": "prod", "count": 5}
# }

# With short flags
curl -X PUT "http://localhost:8080/cluster/create?dryRun=true&preferShort=true" \
  -H "Content-Type: application/json" \
  -d '{"name": "prod", "count": 5}' | jq -r '.cli'

# Output: aerolab cluster create -n=prod -c=5
```

**Shell escaping example:**

```bash
# Values with special characters are properly escaped
curl -X POST http://localhost:8080/api/generate-cli \
  -H "Content-Type: application/json" \
  -d '{
    "commandPath": "cluster/create",
    "parameters": {
      "name": "my cluster",
      "owner": "O'\''Brien"
    }
  }' | jq -r '.cli'

# Output: aerolab cluster create --name='my cluster' --owner='O'\''Brien'
```

## Implementation Details

The REST API is implemented in these files:

| File | Purpose |
|------|---------|
| `cmdWebUI.go` | Main command struct, HTTP server setup, handlers, authentication, subprocess launcher, dry-run support |
| `cmdWebUIExec.go` | Hidden `webui exec` command for subprocess execution |
| `cmdWebUIJobs.go` | Job struct, JobManager, file storage, basic CLI generation, cleanup |
| `cmdWebUIReflect.go` | Reflection engine for command tree building and execution |
| `cmdWebUIHandlers.go` | HTTP handlers for file upload/download streaming |
| `cmdWebUIOpenAPI.go` | OpenAPI specification generation |
| `helpers.go` | CLI reconstruction (`ReconstructCommandLine`), shell escaping utilities |

### Key Components

1. **JobManager** - Manages job lifecycle, persistence, cleanup, and log files
2. **BuildCommandTree()** - Uses reflection to build command metadata tree
3. **executeJobAsync()** - Launches jobs as subprocesses with timeout and cancellation support
4. **WebUIExecCmd** - Hidden command that executes commands in subprocess mode
5. **SSE Streaming** - Real-time log streaming using Server-Sent Events
6. **CleanupOldJobs()** - Automatic cleanup of old completed/failed jobs
7. **ReconstructCommandLine()** - Reflection-based CLI command generation with proper shell escaping
8. **generateCLIWithReflection()** - Combines parameter application with CLI reconstruction

## Security Considerations

1. **Authentication** - Always use `--auth basic` or `--auth token` in production
2. **HTTPS** - Use `--https` with valid certificates for encrypted connections
3. **CORS** - Restrict `--cors-origins` to specific domains in production
4. **Network** - Bind to specific interfaces (e.g., `--listen 127.0.0.1:8080`)
5. **Tokens** - Use tokens at least 64 characters long for security
6. **User Header** - Only trust `--user-header` behind authenticated proxies
