# AeroLab Web UI

The AeroLab Web UI provides a browser-based interface for managing AeroLab operations. It dynamically generates command forms from the REST API and provides real-time job monitoring with streaming logs.

## Quick Start

```bash
# Start the web UI server
aerolab webui

# With custom port
aerolab webui --listen :9090

# With authentication
aerolab webui --auth basic --basic-user admin --basic-pass secretpass

# With root path prefix (for reverse proxy)
aerolab webui --root-path /aerolab
```

Then open `http://localhost:8080` in your browser.

## Features

### Command Explorer

The left sidebar displays all available AeroLab commands in a tree structure:

- **Expandable navigation** - Click to expand/collapse command groups
- **Icons** - Visual indicators for different command types
- **Search-friendly** - Commands are organized hierarchically (cluster, client, aerospike, etc.)

### Dynamic Command Forms

When you select an executable command, the UI generates a form based on the command's parameter metadata:

| Parameter Type | UI Control |
|----------------|------------|
| `string` | Text input |
| `int`, `float` | Number input with validation |
| `bool` | Toggle switch |
| `duration` | Text input (e.g., "30s", "5m", "1h") |
| `[]string` | Multi-value tag input |
| `choices` | Dropdown select |
| `password` | Password input with show/hide toggle |
| `upload` | File picker |

**Simple/Advanced Mode**: Parameters marked with `simpleMode: false` are hidden by default. Click "Show Advanced Options" to reveal them.

**Parameter Groups**: Related parameters are grouped together under collapsible sections.

### Command Execution

1. Fill in the form parameters
2. Click **Execute** to run the command
3. You'll be redirected to the job view to monitor progress

Alternatively, click **Generate CLI** to see the equivalent command-line invocation without executing.

### Job Management

The Jobs page (`/jobs`) shows all your submitted jobs:

| Column | Description |
|--------|-------------|
| Status | Pending, Running, Completed, Failed, or Error |
| Command | The command path that was executed |
| User | Who submitted the job |
| Started | When the job began |
| Duration | How long the job has been running |

**Filtering Options**:
- Filter by status (dropdown)
- Show all users' jobs (checkbox)
- Manual refresh button

### Real-Time Log Viewer

Click **View** on any job to open the log viewer modal:

- **Live streaming** - Logs stream in real-time via Server-Sent Events (SSE)
- **ANSI color support** - Terminal colors are preserved using xterm.js
- **Auto-scroll** - Automatically scrolls to latest output (can be toggled)
- **Job cancellation** - Cancel running jobs with the Cancel button
- **Export options** - Copy logs to clipboard or download as a file

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `--listen, -l` | `:8080` | Address to listen on (host:port) |
| `--root-path` | `` | Root path prefix for all URLs (e.g., `/aerolab`) |
| `--https` | `false` | Enable HTTPS |
| `--cert` | | TLS certificate file (required if --https) |
| `--key` | | TLS key file (required if --https) |
| `--auth` | `none` | Authentication: `none`, `basic`, or `token` |
| `--basic-user` | `admin` | Basic auth username |
| `--basic-pass` | | Basic auth password |
| `--token-path` | | Path to token file (one per line) |
| `--cors-origins` | `*` | Allowed CORS origins |
| `--user-header` | | Header for user identification (e.g., `X-User`) |

## Reverse Proxy Setup

### With Root Path Prefix

When running behind a reverse proxy that maps AeroLab to a subpath:

```bash
aerolab webui --root-path /aerolab
```

**Nginx example**:
```nginx
location /aerolab/ {
    proxy_pass http://localhost:8080/aerolab/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_buffering off;  # Required for SSE
}
```

### With User Header

For authentication via reverse proxy:

```bash
aerolab webui --user-header X-Forwarded-User
```

The proxy should set the `X-Forwarded-User` header after authenticating users.

## Dark Mode

The UI supports both light and dark themes:

- Click the sun/moon icon in the top-right corner to toggle
- Preference is saved in browser localStorage
- Defaults to system preference

## Architecture

### Frontend Stack

- **React 18** - UI framework
- **TypeScript** - Type-safe JavaScript
- **Vite** - Build tool
- **TailwindCSS** - Utility-first CSS
- **React Query** - API state management
- **React Router** - Client-side routing
- **xterm.js** - Terminal emulation for log viewer
- **Lucide React** - Icons

### Build Process

The Web UI is built and embedded into the AeroLab binary:

```
web/webui/           # React source code
    ↓ (npm run build)
web/webui/dist/      # Built assets
    ↓ (cp -r)
src/pkg/webui/dist/  # Copied for embedding
    ↓ (go:embed)
aerolab binary       # Single binary with embedded UI
```

To rebuild the UI:

```bash
cd web
./build.sh
```

Or using go generate:

```bash
go generate ./src/pkg/webui/...
```

### Embedding

The UI is embedded using Go's `embed` package:

```go
//go:embed dist/*
var WebUIFS embed.FS
```

This allows the UI to be served directly from memory without extracting files to disk.

### API Integration

The frontend communicates with the backend via the REST API:

| Frontend Action | API Endpoint | Method |
|-----------------|--------------|--------|
| Load command tree | `/api/commands` | GET |
| Get command details | `/api/commands/{path}` | GET |
| Execute command | `/{command/path}` | PUT |
| Upload file | `/{command/path}` | POST (multipart) |
| Download file | `/{command/path}` | GET |
| List jobs | `/api/jobs` | GET |
| Get job details | `/api/jobs/{id}` | GET |
| Get job logs | `/api/jobs/{id}/logs` | GET |
| Stream job logs | `/api/jobs/{id}/logs/stream` | GET (SSE) |
| Cancel job | `/api/jobs/{id}` | DELETE |
| Generate CLI | `/api/generate-cli` | POST |

See [webui-rest.md](webui-rest.md) for complete API documentation.

### Runtime Configuration

The server injects configuration into the frontend at runtime:

```html
<script>
window.__AEROLAB_CONFIG__ = {
    rootPath: "/aerolab",  // or "" if no prefix
    version: "8.0.0"
};
</script>
```

This allows the same built assets to work with different configurations.

## Development

### Local Development

For frontend development with hot reload:

```bash
cd web/webui
npm install
npm run dev
```

This starts a Vite dev server on `http://localhost:5173` that proxies requests to `http://localhost:8080`:
- `/api/*` routes for REST API calls
- Command execution routes (`/cluster/*`, `/client/*`, etc.) for PUT/POST operations

Run the backend separately:

```bash
go run ./src/... webui
```

### Project Structure

```
web/webui/
├── src/
│   ├── api/           # API client and types
│   │   ├── client.ts  # Fetch wrapper with rootPath support
│   │   └── types.ts   # TypeScript interfaces
│   ├── components/    # React components
│   │   ├── Layout.tsx
│   │   ├── Sidebar.tsx
│   │   ├── CommandForm.tsx
│   │   ├── ParameterInput.tsx
│   │   ├── JobList.tsx
│   │   └── JobLogModal.tsx
│   ├── hooks/         # React Query hooks
│   │   ├── useCommands.ts
│   │   └── useJobs.ts
│   ├── pages/         # Page components
│   │   ├── CommandPage.tsx
│   │   └── JobsPage.tsx
│   ├── utils/         # Utilities
│   │   ├── config.ts  # Runtime config reader
│   │   ├── cn.ts      # Class name merger
│   │   ├── date.ts    # Date formatting
│   │   └── icons.ts   # Icon mapping
│   ├── App.tsx        # Main app with routing
│   ├── main.tsx       # Entry point
│   └── index.css      # Global styles
├── package.json
├── vite.config.ts
├── tailwind.config.js
└── tsconfig.json
```

## Troubleshooting

### UI Not Loading

1. Check that the server is running: `curl http://localhost:8080/api/health`
2. Check browser console for JavaScript errors
3. Ensure the UI was built: check that `src/pkg/webui/dist/` contains files

### Jobs Not Appearing

1. Verify you're viewing the correct user's jobs
2. Try enabling "Show all users" checkbox
3. Check the status filter isn't hiding jobs

### Log Streaming Not Working

1. SSE requires HTTP/1.1 - ensure your proxy supports it
2. Disable response buffering in your reverse proxy
3. Check browser console for connection errors
4. If logs appear duplicated, try refreshing the page

### Form Not Submitting

1. Check browser console for validation errors
2. Ensure required fields are filled
3. Verify the backend is accepting the request format

### Development Mode Issues

1. **Command execution returns 404**: Ensure the Vite dev server is configured to proxy command routes. The `vite.config.ts` should proxy both `/api` and command paths like `/cluster/*`.
2. **CORS errors**: Run the backend with `--cors-origins http://localhost:5173` during development.
3. **Authentication errors (401)**: When using basic auth, ensure credentials are passed. The browser will prompt for them automatically.
