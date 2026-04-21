# Aerolab MCP Server

The `aerolab mcp` command runs a standalone [Model Context Protocol](https://modelcontextprotocol.io/) server that exposes every Aerolab CLI subcommand as an MCP tool. AI agents (Claude Desktop, Cursor, Codex, Continue, etc.) can then drive Aerolab without shell access — they see the same command tree you see on the CLI, with the same flags and the same help text.

## What you get

The server uses self-introspection over Aerolab's command tree — there is no hand-maintained tool registry — so every subcommand that works on the CLI is automatically available to the model. Two kinds of tools are registered:

1. **Generic explorer tools** (3 of them) for browsing and running arbitrary commands:
   - `aerolab_list_commands` — list every command path, optionally filtered by prefix. Each entry is tagged `[DESTRUCTIVE]` when appropriate.
   - `aerolab_describe_command` — show one command's description, CLI help, and JSON input schema.
   - `aerolab_execute_command` — run any command by path with a map of flag arguments.
2. **One specific tool per leaf subcommand**, auto-registered. For example `cluster create` becomes `aerolab_cluster_create`, with a strict JSON Schema derived from its `go-flags` struct tags.

Every tool call forks the Aerolab binary as a subprocess and returns merged `stdout + stderr` plus a structured metadata block (`exit_code`, `duration_ms`, `timed_out`, `truncated`, full `argv`).

## Transports

| Transport | When to use | Default address |
|-----------|-------------|-----------------|
| `stdio`   | Local agents that spawn the server as a child process (Claude Desktop, Cursor, Codex). This is the default. | stdin/stdout |
| `http`    | Remote agents or shared dev servers using [streamable HTTP](https://modelcontextprotocol.io/specification/server/transports/streamable-http). | `localhost:9190` |

## Flags

```text
aerolab mcp [OPTIONS]

  --transport=stdio|http           Transport to use (default: stdio)
  --addr=HOST:PORT                 HTTP listen address (default: localhost:9190)
  --auth-token=TOKEN               Optional bearer token required on HTTP requests
                                   (also read from $AEROLAB_MCP_AUTH_TOKEN)
  --profile=read-only|standard|admin
                                   Tool profile (default: standard)
  --binary=PATH                    aerolab binary to exec for tool calls
                                   (default: the currently running executable)
  --init-backend                   Initialize the configured backend on startup
                                   so dynamic choices (zones, instance types,
                                   VPCs) can be resolved
  --timeout=SECONDS                Default per-call subprocess timeout (default: 600)
  --max-output-bytes=N             Cap on captured stdout+stderr per call (default: 1048576)
  --session-timeout=DURATION       Idle timeout for HTTP streaming sessions (default: 30m)
```

## Profiles

Profiles control which tools an agent is allowed to call:

| Profile | Non-destructive commands | Destructive commands |
|---------|--------------------------|----------------------|
| `read-only` | allowed | rejected with a clear error |
| `standard` (default) | allowed | allowed only when the tool call includes `confirm: true` |
| `admin` | allowed | allowed without confirmation |

A command is "destructive" when:
- it is annotated with `InvWebForce=true` in the command tree (e.g. `cluster/create`, `cluster/destroy`, `cluster/grow`, `cluster/start`, `cluster/stop`, `client/destroy`, `volumes/delete`, …), or
- its leaf name matches one of: `destroy`, `delete`, `remove`, `terminate`, `stop`, `kill`, `wipe`, `reboot`, `restart`, `reset`, `purge`, `cold-start`.

Every auto-registered tool accepts two extra arguments:

- `confirm` (bool) — explicit confirmation for destructive commands.
- `timeout_sec` (int) — per-call timeout override. `0` disables the timeout.

## Quickstart — stdio (Cursor / Claude Desktop / Codex)

### Cursor

Put this in `~/.cursor/mcp.json` (or the project-local equivalent):

```json
{
  "mcpServers": {
    "aerolab": {
      "command": "/usr/local/bin/aerolab",
      "args": ["mcp", "--transport", "stdio", "--profile", "standard"]
    }
  }
}
```

### Claude Desktop

Put this in the Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "aerolab": {
      "command": "/usr/local/bin/aerolab",
      "args": ["mcp", "--transport", "stdio", "--profile", "read-only"]
    }
  }
}
```

### Codex / Continue / Generic

Any MCP client that launches a stdio child process works the same way — point it at the Aerolab binary with `mcp --transport stdio`.

### Sanity check (no client needed)

```bash
printf '%s\n%s\n%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | aerolab mcp --transport stdio
```

You should see a JSON response with all tools listed.

## Quickstart — HTTP

Start the server:

```bash
aerolab mcp --transport http --addr 0.0.0.0:9190 --profile standard
# or with bearer auth:
aerolab mcp --transport http --addr 0.0.0.0:9190 --auth-token "$(openssl rand -hex 32)"
```

Health check:

```bash
curl -sS http://localhost:9190/healthz
# -> ok
```

Point an HTTP-capable MCP client at `http://localhost:9190/mcp`. With bearer auth, clients must send `Authorization: Bearer <token>`.

### Cursor over HTTP

```json
{
  "mcpServers": {
    "aerolab": {
      "url": "http://localhost:9190/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN_HERE" }
    }
  }
}
```

## Dynamic choices

Some Aerolab flags (instance types, zones, VPC IDs, …) have dynamic choice lists that are queried from the configured backend. The MCP server can pre-resolve these and advertise them as `enum` values in the JSON Schema **only if you start the server with `--init-backend`**. Without that flag, those parameters appear as plain strings — the model can still set them, but it won't see the available options.

```bash
aerolab mcp --transport stdio --init-backend
```

Backend initialization can be slow (AWS API calls, GCP discovery, etc.), so `--init-backend` is opt-in.

## Using the generic tools

Even without any per-leaf tool, an agent can drive all of Aerolab through three tools:

```json
// Step 1: discover
{"tool": "aerolab_list_commands", "arguments": {"prefix": "cluster"}}

// Step 2: inspect
{"tool": "aerolab_describe_command", "arguments": {"path": "cluster/create"}}

// Step 3: run
{
  "tool": "aerolab_execute_command",
  "arguments": {
    "path": "cluster/create",
    "args": { "name": "mydc", "count": 3, "distro": "ubuntu", "distro-version": "22.04" },
    "confirm": true
  }
}
```

Per-leaf tools are strictly preferred when the model knows which command it wants — they carry a full JSON Schema so the agent gets argument validation for free.

## Output format

Every tool call returns two things:

1. A Markdown text block summarising what happened, suitable for chat display.
2. A structured content block with machine-readable fields:

```json
{
  "path": "cluster/list",
  "argv": ["/usr/local/bin/aerolab", "cluster", "list", "--output", "json"],
  "exit_code": 0,
  "duration_ms": 812,
  "timed_out": false,
  "truncated": false,
  "output": "...\n"
}
```

## Safety model

- **No persistent state** — the MCP server is a thin multiplexer; every call re-execs the Aerolab binary with your existing config.
- **Bearer token for HTTP** — constant-time compared, so timing attacks are not a concern.
- **Destructive gating** — the `confirm` requirement forces the model to be explicit, which also makes logs easier to audit.
- **Bounded output** — `--max-output-bytes` prevents a chatty subcommand from blowing up the MCP response.
- **Bounded time** — `--timeout` / `timeout_sec` cap every call.

## Troubleshooting

- **Server prints "Warning: scriptlog cleanup failed"** — this is cosmetic; the MCP server does not depend on `/tmp/aerolab/`. It only happens on very first run.
- **Tool call returns `error: destructive command ... requires confirm=true`** — switch your agent profile to `admin` or call the tool again with `confirm: true`.
- **Dynamic choices show as plain strings** — start the server with `--init-backend` so backends can be queried at startup.
- **HTTP 401** — the server is configured with `--auth-token` (or `$AEROLAB_MCP_AUTH_TOKEN`) and the client did not send a matching `Authorization: Bearer …` header.
- **EOF / server exits immediately on stdio** — the client closed stdin. This is normal on shutdown; not an error.
