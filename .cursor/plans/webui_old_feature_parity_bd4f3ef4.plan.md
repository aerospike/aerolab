---
name: WebUI Old Feature Parity
overview: Add all missing old WebUI CLI flags to the new WebUI implementation, bridging the gap between the old feature set and the current v8 rewrite. This covers job concurrency/queuing, history limits, inventory refresh intervals, file browser controls, upload limits, per-user firewalls, AGI strict TLS, WebSocket proxy origin, force simple mode, and page title customization.
todos:
  - id: listen-default
    content: Update --listen default from :8080 to 127.0.0.1:3333 to match old behavior
    status: completed
  - id: webroot
    content: Rename --root-path to --webroot with default /
    status: completed
  - id: timeout
    content: Change --max-job-runtime default from 24h to 60m
    status: completed
  - id: job-concurrency
    content: Add --max-concurrent-job (5) and --max-queued-job (10) with semaphore-based job limiting
    status: completed
  - id: history
    content: Rename --cleanup-after to --history-expires (72h), add --show-max-history (100)
    status: completed
  - id: refresh-intervals
    content: Rename --inventory-poll-interval to --refresh-interval (30s), add --minimum-interval (10s)
    status: completed
  - id: server-ls
    content: Add --block-server-ls and --always-server-ls flags with access control in file browser handlers
    status: completed
  - id: upload-limits
    content: Add --max-upload-size-bytes (209715200) and --upload-temp-dir flags
    status: completed
  - id: unique-firewalls
    content: Add --unique-firewalls flag for per-username firewall names
    status: completed
  - id: agi-strict-tls
    content: Add --agi-strict-tls flag for AGI inventory TLS verification
    status: completed
  - id: ws-proxy-origin
    content: Add --ws-proxy-origin flag and implement proper WebSocket origin checking
    status: completed
  - id: force-simple-mode
    content: Add --force-simple-mode CLI flag that applies only to webui and subprocesses, not to the process launching webui itself
    status: completed
  - id: page-title
    content: Add --page-title flag (default AeroLab Web UI), inject into index.html and health endpoint
    status: completed
  - id: nobrowser
    content: Rename --no-browser to --nobrowser to match old naming
    status: completed
isProject: false
---

# WebUI Old Feature Parity Plan

Below is a feature-by-feature mapping from old flags to the current code, with what needs to change.

## 1. `--listen` (default update)

**Old:** default `127.0.0.1:3333`.
**Current:** Single `ListenAddr string` with default `:8080`.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Update default from `:8080` to `127.0.0.1:3333` to match old behavior

## 2. `--webroot` (web root path)

**Old:** `--webroot=` set the web root (default `/`).
**Current:** `--root-path` serves this purpose already.

**Changes:**

- Rename `RootPath` to `WebRoot` and change tag from `long:"root-path"` to `long:"webroot"` with default `/` to match old naming
- Update description to match old behavior
- Internal `c.rootPath` normalization stays the same

## 3. `--max-job-runtime` (command execution timeout)

**Old:** `--timeout=` absolute timeout for command execution (default `30m`).
**Current:** `--max-job-runtime` default `24h`.

**Changes:**

- Keep the flag name as `--max-job-runtime`, just change the default from `24h` to `60m`

## 4. `--max-concurrent-job` and `--max-queued-job`

**Old:** Limits on concurrent and queued jobs (defaults 5 and 10).
**Current:** No limits; jobs start immediately via `go c.executeJobAsync(job)`.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `MaxConcurrentJob int` flag (default 5) and `MaxQueuedJob int` flag (default 10)
- Store internal state: `jobSemaphore chan struct{}` (buffered to MaxConcurrentJob), `pendingJobs atomic.Int32`

**Changes in `[src/cli/cmd/v1/cmdWebUIJobs.go](src/cli/cmd/v1/cmdWebUIJobs.go)`:**

- Add `maxConcurrent` and `maxQueued` fields to `JobManager`
- In `executeCommand()`, check `pendingJobs + runningJobs < maxQueued + maxConcurrent`; reject with 429 if full
- Wrap `executeJobAsync` to acquire from semaphore before running, release after

## 5. `--history-expires` and `--show-max-history`

**Old:** `--history-expires=` (default `72h`), `--show-max-history=` (default `100`).
**Current:** `--cleanup-after` (default `30d`) handles expiry; no max history display limit.

**Changes:**

- Rename `CleanupAfter` to `HistoryExpires` with tag `long:"history-expires"` and default `72h`
- Add `ShowMaxHistory int` flag with tag `long:"show-max-history"` and default `100`
- In `handleJobsList` / `ListJobs`, after filtering, sort by creation time descending and truncate to `ShowMaxHistory` for completed jobs

## 6. `--refresh-interval` and `--minimum-interval`

**Old:** `--refresh-interval=` (default `30s`), `--minimum-interval=` (default `10s`).
**Current:** Single `--inventory-poll-interval` (default `5m`).

**Changes:**

- Rename `InventoryPollInterval` to `RefreshInterval` with tag `long:"refresh-interval"` and default `30s`
- Add `MinimumInterval string` flag with tag `long:"minimum-interval"` and default `10s`
- In the inventory polling logic, track `lastRefreshTime` and enforce `MinimumInterval` as a floor between refreshes (debounce)

## 7. `--block-server-ls` and `--always-server-ls` -- COMPLETED

Already implemented by the file browse/upload plan. Fields `BlockServerLs`, `AllowLsEverywhere` exist on `WebUICmd` with `allowServerBrowse(r)` guard on `/api/fs/*` endpoints.

## 8. `--max-upload-size-bytes` and `--upload-temp-dir` -- COMPLETED

Already implemented by the file browse/upload plan. Fields `MaxUploadSizeBytes` (default 209715200) and `UploadTempDir` (default `{aerolabRoot}/web.tmp`) exist on `WebUICmd` with `http.MaxBytesReader` enforcement in multipart handling.

## 9. `--unique-firewalls`

**Old:** Per-username firewalls for multi-user hosted mode.
**Current:** Not implemented.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `UniqueFirewalls bool` flag
- When enabled, before executing firewall-related commands (cluster/client/template add firewall), prepend the username to the firewall rule name
- Set `AEROLAB_UNIQUE_FIREWALLS=1` env var when launching job subprocesses so backend code can pick it up
- This will also require checking if the backend firewall commands already support a user-prefix mechanism, or if one needs to be added

## 10. `--agi-strict-tls`

**Old:** When performing AGI inventory lookup, expect valid certificates.
**Current:** AGI monitor has `--agi-monitor-strict-agi-tls` but no top-level flag for inventory.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `AgiStrictTls bool` flag with tag `long:"agi-strict-tls"`
- Pass this to the AGI inventory/status polling code (the `agiStatusCache` fetcher and any AGI detail lookups)
- When true, use default TLS verification; when false, use `InsecureSkipVerify`

## 11. `--ws-proxy-origin`

**Old:** Accept an additional Origin header for WebSocket connections when behind a proxy.
**Current:** `CheckOrigin` always returns `true`.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `WsProxyOrigin string` flag with tag `long:"ws-proxy-origin"`

**Changes in `[src/cli/cmd/v1/cmdWebUITerminal.go](src/cli/cmd/v1/cmdWebUITerminal.go)`:**

- Replace the `CheckOrigin: func(r *http.Request) bool { return true }` with a proper origin checker:
  - Accept if origin matches `r.Host`
  - Accept if origin matches `WsProxyOrigin` (if set)
  - Reject otherwise
- This improves security while supporting proxy setups

## 12. `--force-simple-mode`

**Old:** CLI flag to force simple mode.
**Current:** Only via `AEROLAB_FORCE_SIMPLE_MODE` env var.

**Key constraint:** The `--force-simple-mode` flag must NOT apply simple mode to the webui process itself (otherwise it could block webui from launching if webui is hidden in the simple mode config). It must only apply to the web UI frontend and to job subprocesses.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `ForceSimpleMode bool` flag with tag `long:"force-simple-mode"`
- Do NOT call `os.Setenv("AEROLAB_FORCE_SIMPLE_MODE", "true")` before `Initialize` -- that would block the webui command itself
- Instead, AFTER `Initialize` completes successfully:
  - If `ForceSimpleMode` is true and `c.simpleModeConfig == nil`, create a new `SimpleModeConfig{ForceEnabled: true}`
  - If `ForceSimpleMode` is true and `c.simpleModeConfig != nil`, set `c.simpleModeConfig.ForceEnabled = true`
  - Set `os.Setenv("AEROLAB_FORCE_SIMPLE_MODE", "true")` so that job subprocesses (spawned via `exec.Command`) inherit the env var and enforce simple mode
- This ensures the webui launch itself is not blocked, but all command executions through the UI are restricted

## 13. `--page-title`

**Old:** Custom page title (default `AeroLab Web UI`).
**Current:** Hardcoded `<title>AeroLab</title>` in `index.html`.

**Changes in `[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)`:**

- Add `PageTitle string` flag with tag `long:"page-title"` and default `AeroLab Web UI`
- In `serveIndexWithConfig()`, add `pageTitle` to `window.__AEROLAB_CONFIG__`
- Also replace `<title>AeroLab</title>` with `<title>{PageTitle}</title>` in the served HTML
- In `handleHealth()`, include `pageTitle` in the response

## 14. `--nobrowser` (naming alignment)

**Old:** `--nobrowser` (no hyphen).
**Current:** `--no-browser`.

**Changes:**

- Change tag from `long:"no-browser"` to `long:"nobrowser"` to match old naming

## Files to Modify

- `**[src/cli/cmd/v1/cmdWebUI.go](src/cli/cmd/v1/cmdWebUI.go)**` - Main struct, Execute method, health endpoint, index serving, server startup
- `**[src/cli/cmd/v1/cmdWebUIJobs.go](src/cli/cmd/v1/cmdWebUIJobs.go)**` - Job concurrency/queuing, history limits
- `**[src/cli/cmd/v1/cmdWebUIFileBrowser.go](src/cli/cmd/v1/cmdWebUIFileBrowser.go)**` - File browser access control
- `**[src/cli/cmd/v1/cmdWebUIHandlers.go](src/cli/cmd/v1/cmdWebUIHandlers.go)**` - Upload size limits
- `**[src/cli/cmd/v1/cmdWebUITerminal.go](src/cli/cmd/v1/cmdWebUITerminal.go)**` - WebSocket origin checking

