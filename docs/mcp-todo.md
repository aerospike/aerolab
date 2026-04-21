# Aerolab MCP — Deferred Work

A small number of known issues in the MCP server were intentionally deferred after the April 2026 review. They are tracked here (rather than in commit messages) so the next person who touches the package has a single place to look.

Each entry lists the problem, the file/line pointer, and enough context to decide whether a fix is worth scheduling.

---

## #6 — Timing side-channel in `BearerMiddleware` length check

File: [src/pkg/mcp/auth.go](../src/pkg/mcp/auth.go) (≈lines 41–47).

`BearerMiddleware` compares the incoming token against the configured token using `subtle.ConstantTimeCompare`, which is good. However, it short-circuits with `if len(token) != len(expected)` before the constant-time compare. That early-exit is measurable and leaks the length of the configured token to a remote attacker over repeated probes.

The practical impact is small (token length is not secret material in most deployments) but the fix is trivial: pad both byte slices to a fixed upper bound (or always call `ConstantTimeCompare` and AND the length-equal result). Defer until we add native TLS (#25) so we're only ever fixing this once.

---

## #8 — No CORS and no per-session auth revalidation for streamable HTTP

File: [src/pkg/mcp/server.go](../src/pkg/mcp/server.go) (`serveHTTP`).

The streamable HTTP handler currently accepts requests from any Origin and only authenticates at connection time. Once a session is open, the bearer token is never re-checked. This is fine for localhost stdio bridges but a real concern when the endpoint is exposed publicly — a compromised client keeps privileged access indefinitely.

Fixes: add a minimal CORS policy (opt-in, default deny), and refactor `BearerMiddleware` into a per-request hook that the streamable handler can call on every message rather than only on the initial HTTP upgrade. Blocked on the MCP SDK exposing an appropriate extension point.

---

## #9 — `BuildArgv` cannot explicitly set a boolean flag to `false`

File: [src/pkg/mcp/runner.go](../src/pkg/mcp/runner.go) (≈lines 193–195).

When an agent passes `"verbose": false`, `BuildArgv` emits nothing, which is indistinguishable from "the flag was omitted". For aerolab flags whose default is `true`, this means `false` is silently lost and the subprocess runs with the default. Agents have no way to explicitly disable such a flag through the MCP surface.

The fix is to emit `--flag=false` when the value is a boolean `false` and the parameter's resolved default is `true`. That requires threading the default through `RunInput` (or consulting the command metadata), which is a non-trivial refactor. Until then, agents needing the `false` form must call `aerolab_execute_command` with raw positional args.

---

## #10 — `limitedBuffer.Write` returns `len(p)` on overflow, misreporting bytes written

File: [src/pkg/mcp/runner.go](../src/pkg/mcp/runner.go) (≈lines 280–295).

Per `io.Writer`'s contract, `Write` must return the number of bytes consumed. `limitedBuffer.Write` currently returns `len(p)` even when part of `p` was dropped because the capacity was reached, and it does not return an error. A caller that checks `n` expects to be able to trust it; the current behaviour quietly lies.

This is latent because our only consumer is `os/exec` which ignores the return value, but the shape of the bug means it will bite us the day we point a different writer at it. A one-line fix plus a test, but noisy to land in isolation, so scheduled with the next runner change.

---

## #25 — HTTP transport has no native TLS

File: [src/pkg/mcp/server.go](../src/pkg/mcp/server.go) (`serveHTTP`).

`aerolab mcp --transport=http` always listens with `http.Server` over plain TCP. This is intentional for v1 — operators are expected to terminate TLS at a reverse proxy — but it means the `--auth-token` bearer header travels in clear on the wire. Issue #5 added a startup warning to make that obvious, but a production deployment option is still missing.

Native TLS needs two new flags (`--tls-cert` / `--tls-key`), a loader that rejects weak cipher suites, and a test matrix covering HTTP/1.1 and HTTP/2. Not on the critical path for local-dev agents. Track alongside the eventual SSE/WebSocket transport upgrade.
