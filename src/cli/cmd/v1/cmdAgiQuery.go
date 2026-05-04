//go:build !noagi

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	flags "github.com/rglonek/go-flags"
)

// AgiQueryCmd talks to the AGI plugin's localhost-only debug endpoints
// (registered by pkg/agi/plugin/frontend_debug.go). It picks one of
// two transports:
//
//   - "local"  — direct net/http to 127.0.0.1:8851, no backend, no
//     SSH. Used when running on the AGI box itself (the marker file
//     /opt/aerolab-agi-exec is the canonical AGI-host signal,
//     established by template create and reused by checkUpgrade).
//   - "ssh"    — `instances.Exec` SSH'es to the AGI box and runs curl
//     against the same localhost endpoint. Used everywhere else.
//
// Auto-detection picks "local" when /opt/aerolab-agi-exec is present
// (we are on an AGI host) and "ssh" otherwise. --transport=local|ssh
// forces the choice for testing or proxied scenarios.
//
// Modes (mutually exclusive; exactly one must be selected):
//
//	--info             ⇒ GET  /debug/db/info
//	--list-sets        ⇒ GET  /debug/db/sets
//	--describe SET     ⇒ GET  /debug/db/sets/SET
//	--sample SET       ⇒ GET  /debug/db/sample?set=SET&limit=N
//	--get-set SET --get-key K ⇒ GET /debug/db/get?set=SET&key=K
//	--plan FILE        ⇒ POST /debug/db/query (JSON plan from FILE; '-' = stdin)
type AgiQueryCmd struct {
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name (ignored in --transport=local)" default:"agi"`
	// Mode flags. Bool flags are not mutually exclusive at the
	// flag-parser level; we enforce that in Execute() so we can
	// produce a helpful error.
	Info     bool   `long:"info" description:"Show DB path, storage version, set count and stats"`
	ListSets bool   `long:"list-sets" description:"List all sets and a one-line schema summary"`
	Describe string `long:"describe" description:"Print the full schema of one set" value-name:"SET"`
	Sample   string `long:"sample" description:"Stream the first --limit rows of a set" value-name:"SET"`
	GetSet   string `long:"get-set" description:"Set to point-read from (used with --get-key)" value-name:"SET"`
	GetKey   string `long:"get-key" description:"Primary key to point-read (used with --get-set)" value-name:"KEY"`
	Plan     string `long:"plan" description:"POST a query plan from a JSON file ('-' for stdin)" value-name:"FILE"`
	HashKey  string `long:"hash-key" description:"Compute the metrics-set XXH3-128 PK for 'cluster::/::node_id::/::log_line' and exit (no transport, no SSH)" value-name:"STRING"`

	// Common knobs.
	Limit     int    `long:"limit" description:"Row cap for --sample / --plan (server caps further)" default:"100"`
	Output    string `short:"o" long:"output" description:"Output format: table | json | json-indent | ndjson" default:"table"`
	Host      string `long:"plugin-host" description:"Plugin debug host inside the AGI instance" default:"127.0.0.1"`
	Port      int    `long:"plugin-port" description:"Plugin debug port inside the AGI instance" default:"8851"`
	Transport string `long:"transport" description:"Transport: auto | local | ssh (auto picks 'local' on an AGI box, otherwise 'ssh')" default:"auto"`

	// SSH knobs (mirroring agi attach). Ignored in --transport=local.
	ConnectTimeout time.Duration `short:"C" long:"connect-timeout" description:"SSH connect timeout (ssh transport only)" default:"10s"`
	SessionTimeout time.Duration `short:"S" long:"session-timeout" description:"SSH session timeout (ssh transport only)" default:"5m"`

	// HTTP timeout (used by the local transport; the ssh transport
	// inherits SessionTimeout instead).
	HTTPTimeout time.Duration `long:"http-timeout" description:"HTTP timeout for the local transport" default:"65s"`

	// Output redirection (rare; --output ndjson into a file is the
	// most common reason).
	Out flags.Filename `short:"O" long:"stdout" description:"Path to redirect stdout to"`

	Help AgiQueryCmdHelp `command:"help" subcommands-optional:"true" description:"Print help"`
}

// agiHostMarker is the well-known sentinel that template create
// stamps onto every AGI box. checkUpgrade consults it to decide
// whether to skip the auto-downgrade dance; we consult it here to
// decide whether to skip SSH and talk to the local plugin directly.
const agiHostMarker = "/opt/aerolab-agi-exec"

// runningOnAGIHost returns true if this binary is running on the
// AGI box itself.
func runningOnAGIHost() bool {
	_, err := os.Stat(agiHostMarker)
	return err == nil
}

// AgiQueryCmdHelp prints inline help. It's a separate command so the
// parent help wiring sees a real Execute method on the help struct.
type AgiQueryCmdHelp struct{}

// Execute prints help text for the agi query command.
func (c *AgiQueryCmdHelp) Execute(args []string) error {
	PrintHelp(false, `Inspect the running AGI database via the plugin's debug endpoints.

This is a read-only debugging tool. Two transports are supported:

  - local : direct HTTP to 127.0.0.1:8851 (no SSH, no backend creds).
            Auto-selected when running on the AGI box itself
            (detected via the /opt/aerolab-agi-exec marker).
  - ssh   : 'curl' on the AGI box via SSH; auto-selected everywhere
            else. Use --transport=ssh|local|auto to override.

Examples (run from your laptop, transport=ssh auto):

  aerolab agi query --name=myagi --info
  aerolab agi query --name=myagi --list-sets
  aerolab agi query --name=myagi --describe metrics
  aerolab agi query --name=myagi --sample metrics --limit 5
  aerolab agi query --name=myagi --get-set metrics --get-key "cluster-a/node-1/...key..."

Examples (run on the AGI box itself, transport=local auto):

  aerolab agi query --info
  aerolab agi query --list-sets

Run a query plan from a file (or stdin):

  cat <<EOF | aerolab agi query --name=myagi --plan -
  {
    "set": "metrics",
    "between": {"col":"timestamp","lo":{"int":0},"hi":{"int":99999999999999}},
    "where":   {"and":[{"eq":{"col":"ClusterName","value":{"int":0}}}]},
    "project": ["timestamp","latency_p99"],
    "limit":   100
  }
  EOF

Compute the metrics-set XXH3-128 PK for a known triple (no transport
needed; useful when an operator has the (cluster, node, line) parts
and wants to point-read by --get-key):

  aerolab agi query --hash-key 'cluster-a::/::1_bb978a3b3565000::/::Apr 22 2026 00:00:25 GMT+0700: INFO ...'
`)
	return nil
}

// Execute is the flag-parser entry point. It resolves the transport
// before calling Initialize so we can skip backend init (which would
// fail with backend=none, the configuration AGI boxes ship with) on
// the local-transport path.
func (c *AgiQueryCmd) Execute(args []string) error {
	cmd := []string{"agi", "query"}

	// --hash-key is a pure local computation — no backend, no
	// transport, no HTTP. Short-circuit before Initialize so it
	// works on a bare laptop without an aerolab inventory and on
	// an AGI box without the plugin running. Validate that no
	// other mode was selected so behaviour is predictable.
	if c.HashKey != "" {
		if c.Info || c.ListSets || c.Describe != "" || c.Sample != "" ||
			c.GetSet != "" || c.GetKey != "" || c.Plan != "" {
			return fmt.Errorf("--hash-key is mutually exclusive with all other modes")
		}
		out := os.Stdout
		if c.Out != "" {
			f, err := os.OpenFile(string(c.Out), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			out = f
		}
		fmt.Fprintln(out, ingest.MetricsRowKeyFromString(c.HashKey))
		return nil
	}

	transport, err := c.resolveTransport()
	if err != nil {
		// We have no system yet, so log via fmt and exit with a
		// plain error; Error() requires a system handle.
		return err
	}

	// SSH transport needs the inventory to find the AGI box, so
	// the backend must be live. Local transport never touches the
	// inventory and would otherwise fail on AGI boxes (which run
	// with `aerolab config backend -t none`).
	initBackend := transport == transportSSH
	system, err := Initialize(&Init{InitBackend: initBackend, UpgradeCheck: true}, cmd, c)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s (transport=%s)", strings.Join(cmd, "."), transport)

	mode, err := c.selectedMode()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	out := os.Stdout
	if c.Out != "" {
		f, err := os.OpenFile(string(c.Out), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		defer f.Close()
		out = f
	}

	var inv *backends.Inventory
	if transport == transportSSH {
		inv = system.Backend.GetInventory()
	}
	if err := c.runMode(system, inv, transport, mode, out); err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// queryTransport tags the chosen transport. The string form is
// surfaced in logs so operators can confirm which path was taken.
type queryTransport string

const (
	transportLocal queryTransport = "local"
	transportSSH   queryTransport = "ssh"
)

// resolveTransport applies the auto-detection rules and validates the
// user-provided value. It is intentionally side-effect-free so it can
// run before Initialize.
func (c *AgiQueryCmd) resolveTransport() (queryTransport, error) {
	switch c.Transport {
	case "", "auto":
		if runningOnAGIHost() {
			return transportLocal, nil
		}
		return transportSSH, nil
	case "local":
		return transportLocal, nil
	case "ssh":
		return transportSSH, nil
	}
	return "", fmt.Errorf("invalid --transport %q (want auto | local | ssh)", c.Transport)
}

// queryMode tags the mutually-exclusive subcommands. The string form
// doubles as the kind passed to renderOutput so error messages stay
// stable.
type queryMode string

const (
	modeInfo     queryMode = "info"
	modeListSets queryMode = "list-sets"
	modeDescribe queryMode = "describe"
	modeSample   queryMode = "sample"
	modeGet      queryMode = "get"
	modePlan     queryMode = "plan"
)

// selectedMode validates that exactly one mode flag is set and
// returns the chosen mode, or an error describing the conflict.
// We do this client-side rather than reusing go-flags' "choice"
// semantics because each mode takes different argument shapes
// (Describe/Sample/GetSet take a value, Info/ListSets are bool).
func (c *AgiQueryCmd) selectedMode() (queryMode, error) {
	picked := []queryMode{}
	if c.Info {
		picked = append(picked, modeInfo)
	}
	if c.ListSets {
		picked = append(picked, modeListSets)
	}
	if c.Describe != "" {
		picked = append(picked, modeDescribe)
	}
	if c.Sample != "" {
		picked = append(picked, modeSample)
	}
	if c.GetSet != "" || c.GetKey != "" {
		if c.GetSet == "" || c.GetKey == "" {
			return "", fmt.Errorf("--get-set and --get-key must be specified together")
		}
		picked = append(picked, modeGet)
	}
	if c.Plan != "" {
		picked = append(picked, modePlan)
	}
	switch len(picked) {
	case 0:
		return "", fmt.Errorf("no mode selected; pass one of --info, --list-sets, --describe, --sample, --get-set/--get-key, or --plan (see 'agi query help')")
	case 1:
		return picked[0], nil
	default:
		return "", fmt.Errorf("multiple modes selected (%v); pick exactly one", picked)
	}
}

// runMode dispatches the chosen mode through the resolved transport
// and renders the response.
func (c *AgiQueryCmd) runMode(system *System, inventory *backends.Inventory, transport queryTransport, mode queryMode, out io.Writer) error {
	t, err := c.newTransport(system, inventory, transport)
	if err != nil {
		return err
	}
	host := fmt.Sprintf("http://%s:%d", c.Host, c.Port)
	switch mode {
	case modeInfo:
		body, err := t.Get(host + "/debug/db/info")
		if err != nil {
			return err
		}
		return renderJSONResponse(out, c.Output, body)
	case modeListSets:
		body, err := t.Get(host + "/debug/db/sets")
		if err != nil {
			return err
		}
		if c.Output == "table" {
			return renderSetsTable(out, body)
		}
		return renderJSONResponse(out, c.Output, body)
	case modeDescribe:
		body, err := t.Get(host + "/debug/db/sets/" + url.PathEscape(c.Describe))
		if err != nil {
			return err
		}
		if c.Output == "table" {
			return renderSchemaTable(out, body)
		}
		return renderJSONResponse(out, c.Output, body)
	case modeGet:
		u := fmt.Sprintf("%s/debug/db/get?set=%s&key=%s", host, url.QueryEscape(c.GetSet), url.QueryEscape(c.GetKey))
		body, err := t.Get(u)
		if err != nil {
			return err
		}
		return renderJSONResponse(out, c.Output, body)
	case modeSample:
		u := fmt.Sprintf("%s/debug/db/sample?set=%s&limit=%d", host, url.QueryEscape(c.Sample), c.Limit)
		body, err := t.Get(u)
		if err != nil {
			return err
		}
		return renderNDJSONResponse(out, c.Output, body)
	case modePlan:
		plan, err := loadPlan(c.Plan)
		if err != nil {
			return err
		}
		// Inject Limit if the plan didn't specify one and the
		// caller passed a non-default --limit. We do this by
		// patching the JSON object directly (cheap, robust to
		// fields we don't know about) rather than round-tripping
		// through db.WireQuery which would silently drop unknown
		// fields and confuse a forward-compat client.
		plan = injectLimitIfMissing(plan, c.Limit)
		body, err := t.Post(host+"/debug/db/query", "application/json", plan)
		if err != nil {
			return err
		}
		return renderNDJSONResponse(out, c.Output, body)
	}
	return fmt.Errorf("unknown mode %q (programmer error)", mode)
}

// --- Transport ---

// httpDoer is the minimal request/response surface a transport must
// expose. Both backends return raw bytes (NDJSON streams or single
// JSON documents) and let the renderers decide what to do with them.
type httpDoer interface {
	Get(url string) ([]byte, error)
	Post(url, contentType string, body []byte) ([]byte, error)
}

// newTransport constructs the right httpDoer for the chosen
// transport. The factory is on the command struct because both
// transports need access to the user-supplied flags (timeouts,
// cluster name, etc.).
func (c *AgiQueryCmd) newTransport(system *System, inv *backends.Inventory, t queryTransport) (httpDoer, error) {
	switch t {
	case transportLocal:
		return &localTransport{timeout: c.HTTPTimeout}, nil
	case transportSSH:
		return &sshTransport{cmd: c, system: system, inv: inv}, nil
	}
	return nil, fmt.Errorf("unknown transport %q", t)
}

// --- Local transport (direct net/http to 127.0.0.1) ---

// localTransport speaks HTTP directly to the plugin from the same
// host. It exists so an operator on the AGI box can run
// `aerolab agi query --info` without backend creds and without an
// SSH hop. The Dialer is wired explicitly so a non-loopback override
// (--plugin-host=10.x.x.x) still respects --http-timeout.
type localTransport struct {
	timeout time.Duration
}

func (l *localTransport) httpClient() *http.Client {
	timeout := l.timeout
	if timeout <= 0 {
		timeout = 65 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		},
	}
}

func (l *localTransport) Get(u string) ([]byte, error) {
	cli := l.httpClient()
	ctx, cancel := context.WithTimeout(context.Background(), cli.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return doHTTP(cli, req)
}

func (l *localTransport) Post(u, contentType string, body []byte) ([]byte, error) {
	cli := l.httpClient()
	ctx, cancel := context.WithTimeout(context.Background(), cli.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return doHTTP(cli, req)
}

// doHTTP returns the response body when status<400, or a structured
// error that includes the server's response body for any 4xx/5xx.
// The ssh transport gets a parallel structured error from curl's
// stderr; keeping the shape similar means renderJSONResponse's
// tryDecodeError path catches both.
func doHTTP(cli *http.Client, req *http.Request) ([]byte, error) {
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()
	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return nil, fmt.Errorf("http %s %s: read body: %w", req.Method, req.URL.String(), rerr)
	}
	if resp.StatusCode >= 400 {
		// The plugin always returns JSON {"error":"..."} on
		// failure; surface that text verbatim so the operator
		// sees the exact server-side message.
		if msg, ok := tryDecodeError(body); ok {
			return body, fmt.Errorf("http %d %s: %s", resp.StatusCode, req.URL.String(), msg)
		}
		return body, fmt.Errorf("http %d %s: %s", resp.StatusCode, req.URL.String(), strings.TrimSpace(string(body)))
	}
	return body, nil
}

// --- SSH transport (ssh root@agi -- curl ...) ---

// sshTransport uses the existing instances.Exec machinery to run
// curl on the AGI box. The instance is selected by cluster-name +
// type=agi + node=1 — multi-node AGIs do exist (template + replica)
// but the plugin only runs on node 1.
type sshTransport struct {
	cmd    *AgiQueryCmd
	system *System
	inv    *backends.Inventory
}

func (s *sshTransport) Get(u string) ([]byte, error) {
	args := []string{"curl", "-fsS", "--max-time", "65", u}
	return s.exec(args, nil)
}

func (s *sshTransport) Post(u, contentType string, body []byte) ([]byte, error) {
	args := []string{
		"curl", "-fsS", "--max-time", "65",
		"-H", "Content-Type: " + contentType,
		"-X", "POST",
		"--data-binary", "@-",
		u,
	}
	return s.exec(args, body)
}

// exec runs cmd[] on the AGI instance and returns the captured
// stdout. stderr is included in the error message on non-zero exit
// so the operator sees curl's diagnostics.
func (s *sshTransport) exec(cmd []string, stdin []byte) ([]byte, error) {
	inv := s.inv
	if inv == nil {
		inv = s.system.Backend.GetInventory()
	}
	instances := inv.Instances.WithState(backends.LifeCycleStateRunning).Describe()
	filter := InstancesListFilter{
		ClusterName: s.cmd.ClusterName.String(),
		NodeNo:      "1",
		Type:        "agi",
	}
	matched, err := filter.filter(instances, true)
	if err != nil {
		return nil, fmt.Errorf("locating AGI %q: %w", s.cmd.ClusterName.String(), err)
	}
	if matched.Count() == 0 {
		return nil, fmt.Errorf("no running AGI instance named %q (node 1)", s.cmd.ClusterName.String())
	}

	var stdinR io.ReadCloser
	if stdin != nil {
		stdinR = io.NopCloser(bytes.NewReader(stdin))
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	results := matched.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        cmd,
			Stdin:          stdinR,
			Stdout:         &stdoutBuf,
			Stderr:         &stderrBuf,
			SessionTimeout: s.cmd.SessionTimeout,
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  s.cmd.ConnectTimeout,
		ParallelThreads: 1,
	})
	if len(results) == 0 {
		return nil, fmt.Errorf("ssh: no exec result returned for AGI %q", s.cmd.ClusterName.String())
	}
	r := results[0]
	if r.Output.Err != nil {
		// curl --fail returns 22 when the server responds 4xx/5xx;
		// surface stderr (which carries the curl error message
		// AND the server's response body if any) so the operator
		// can see what the server actually said.
		errOut := strings.TrimSpace(string(r.Output.Stderr) + stderrBuf.String())
		if errOut == "" {
			errOut = strings.TrimSpace(string(r.Output.Stdout) + stdoutBuf.String())
		}
		return nil, fmt.Errorf("ssh exec to AGI %q: %w (stderr=%s)", s.cmd.ClusterName.String(), r.Output.Err, errOut)
	}
	// Some backends populate r.Output.Stdout; others stream into
	// the writer we passed. Combine both to be safe.
	out := append([]byte{}, r.Output.Stdout...)
	out = append(out, stdoutBuf.Bytes()...)
	return out, nil
}

// --- Renderers ---

// renderJSONResponse writes a single-document JSON response in one of
// the requested formats. Used by --info, --list-sets, --describe,
// --get.
func renderJSONResponse(out io.Writer, format string, body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return fmt.Errorf("empty response from server")
	}
	// If the server returned a structured error envelope, fail loudly.
	if errMsg, ok := tryDecodeError(body); ok {
		return fmt.Errorf("server error: %s", errMsg)
	}
	switch format {
	case "json", "table": // table falls through for endpoints we don't pretty-print
		_, err := out.Write(append(body, '\n'))
		return err
	case "json-indent":
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "ndjson":
		_, err := out.Write(append(body, '\n'))
		return err
	}
	return fmt.Errorf("unknown --output %q (want table | json | json-indent | ndjson)", format)
}

// renderNDJSONResponse handles streaming responses from --sample and
// --plan. The wire format is one JSON object per line, terminated by
// a final {"_meta":{...}} line — see streamRows in
// pkg/agi/plugin/frontend_debug.go.
func renderNDJSONResponse(out io.Writer, format string, body []byte) error {
	rows, meta, err := splitNDJSON(body)
	if err != nil {
		return err
	}
	if errMsg, ok := tryDecodeError(body); ok {
		return fmt.Errorf("server error: %s", errMsg)
	}
	switch format {
	case "ndjson":
		_, err := out.Write(append(bytes.TrimRight(body, "\n"), '\n'))
		return err
	case "json":
		// Rewrap rows as a single JSON array; meta becomes an
		// envelope-level field.
		wrap := struct {
			Rows []json.RawMessage `json:"rows"`
			Meta json.RawMessage   `json:"meta,omitempty"`
		}{Rows: rows, Meta: meta}
		return json.NewEncoder(out).Encode(wrap)
	case "json-indent":
		wrap := struct {
			Rows []json.RawMessage `json:"rows"`
			Meta json.RawMessage   `json:"meta,omitempty"`
		}{Rows: rows, Meta: meta}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(wrap)
	case "table", "":
		return renderRowsTable(out, rows, meta)
	}
	return fmt.Errorf("unknown --output %q", format)
}

// renderSetsTable formats the /debug/db/sets response.
func renderSetsTable(out io.Writer, body []byte) error {
	if errMsg, ok := tryDecodeError(body); ok {
		return fmt.Errorf("server error: %s", errMsg)
	}
	type setRow struct {
		Name       string `json:"name"`
		Columns    int    `json:"columns"`
		IndexedCol string `json:"indexedCol"`
	}
	var rows []setRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return fmt.Errorf("decode sets: %w", err)
	}
	t, err := printer.GetTableWriter("table", "default", nil, true, false)
	if err != nil && err != printer.ErrTerminalWidthUnknown {
		return err
	}
	tRows := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		idx := r.IndexedCol
		if idx == "" {
			idx = "(none)"
		}
		tRows = append(tRows, table.Row{r.Name, r.Columns, idx})
	}
	fmt.Fprintln(out, t.RenderTable(new("SETS"), table.Row{"Name", "Columns", "Indexed Column"}, tRows))
	return nil
}

// renderSchemaTable formats the /debug/db/sets/{name} response.
func renderSchemaTable(out io.Writer, body []byte) error {
	if errMsg, ok := tryDecodeError(body); ok {
		return fmt.Errorf("server error: %s", errMsg)
	}
	type col struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Indexed bool   `json:"indexed"`
	}
	var schema struct {
		Name       string `json:"name"`
		IndexedCol string `json:"indexedCol"`
		Columns    []col  `json:"columns"`
	}
	if err := json.Unmarshal(body, &schema); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	t, err := printer.GetTableWriter("table", "default", nil, true, false)
	if err != nil && err != printer.ErrTerminalWidthUnknown {
		return err
	}
	fmt.Fprintf(out, "Set: %s\n", schema.Name)
	if schema.IndexedCol != "" {
		fmt.Fprintf(out, "Indexed column: %s\n", schema.IndexedCol)
	}
	tRows := make([]table.Row, 0, len(schema.Columns))
	for _, c := range schema.Columns {
		idx := ""
		if c.Indexed {
			idx = "yes"
		}
		tRows = append(tRows, table.Row{c.Name, c.Type, idx})
	}
	fmt.Fprintln(out, t.RenderTable(new("COLUMNS"), table.Row{"Name", "Type", "Indexed"}, tRows))
	return nil
}

// renderRowsTable formats NDJSON row records as a table. The columns
// are derived from the union of all keys actually present in the rows
// — the schema is sparse, so any given record may omit columns.
func renderRowsTable(out io.Writer, rows []json.RawMessage, meta json.RawMessage) error {
	type rowRec struct {
		Key string                 `json:"key"`
		Row map[string]interface{} `json:"row"`
	}
	parsed := make([]rowRec, 0, len(rows))
	colSet := map[string]struct{}{}
	for _, raw := range rows {
		var r rowRec
		// UseNumber so int64 timestamps (and any other 64-bit
		// counters) survive decode without being truncated to
		// float64. Without this, /debug/db/sample on the metrics
		// set prints things like 1.776816025e+12 for ms-precision
		// timestamps; formatWireValue's json.Number branch keeps
		// the exact wire bytes.
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&r); err != nil {
			return fmt.Errorf("decode row: %w", err)
		}
		parsed = append(parsed, r)
		for k := range r.Row {
			colSet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	header := table.Row{"key"}
	for _, c := range cols {
		header = append(header, c)
	}
	tRows := make([]table.Row, 0, len(parsed))
	for _, r := range parsed {
		row := table.Row{r.Key}
		for _, c := range cols {
			row = append(row, formatWireValue(r.Row[c]))
		}
		tRows = append(tRows, row)
	}
	t, err := printer.GetTableWriter("table", "default", nil, true, false)
	if err != nil && err != printer.ErrTerminalWidthUnknown {
		return err
	}
	fmt.Fprintln(out, t.RenderTable(new("ROWS"), header, tRows))

	if len(meta) > 0 {
		var m struct {
			Rows      int    `json:"rowsReturned"`
			Truncated bool   `json:"truncated"`
			Duration  string `json:"durationMs"`
			Error     string `json:"error,omitempty"`
		}
		if err := json.Unmarshal(meta, &m); err == nil {
			fmt.Fprintf(out, "rows=%d truncated=%t durationMs=%s\n", m.Rows, m.Truncated, m.Duration)
			if m.Error != "" {
				fmt.Fprintf(out, "ERROR: %s\n", m.Error)
			}
		}
	}
	return nil
}

// formatWireValue renders a wire-form Value (a single-key tagged
// union) as a short string. The codec always emits exactly one of
// int|float|str|bytes|bool, so the first non-nil value wins.
//
// Numbers come out of UseNumber()-mode decoding as json.Number (the
// exact literal the server emitted). We render them in plain
// decimal form so 13-digit ms timestamps and 8-digit counters never
// appear as 1.776816e+12 or 2.6e+07.
func formatWireValue(v interface{}) string {
	if v == nil {
		return ""
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return formatScalar(v)
	}
	for k, val := range m {
		switch k {
		case "int":
			// Server-side int64 → json.Number on the wire;
			// the literal already lacks any exponent, so
			// returning .String() gives a clean integer.
			if n, ok := val.(json.Number); ok {
				return n.String()
			}
			return formatScalar(val)
		case "float":
			// Server-side float64 → json.Number, which may
			// or may not carry an exponent depending on the
			// magnitude. Re-format through Float64 so the
			// table column is consistent across rows.
			if n, ok := val.(json.Number); ok {
				if f, err := n.Float64(); err == nil {
					return strconv.FormatFloat(f, 'f', -1, 64)
				}
				return n.String()
			}
			return formatScalar(val)
		case "bool":
			return formatScalar(val)
		case "str":
			s, _ := val.(string)
			return s
		case "bytes":
			s, _ := val.(string)
			return "b64:" + s
		}
	}
	return ""
}

// formatScalar handles the residual cases where a value reached us
// without UseNumber() (e.g. raw json.Unmarshal somewhere in the
// pipeline). It applies the same "no scientific notation" rule.
func formatScalar(v interface{}) string {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	}
	return fmt.Sprintf("%v", v)
}

// --- helpers ---

// loadPlan reads a query plan from path; "-" means stdin.
func loadPlan(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// injectLimitIfMissing adds {"limit":N} to plan if it doesn't already
// have a top-level "limit" key. Operates on the JSON object directly
// because the WireQuery struct has DisallowUnknownFields enabled
// server-side; we don't want to drop fields we don't know about.
func injectLimitIfMissing(plan []byte, limit int) []byte {
	if limit <= 0 {
		return plan
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(plan, &probe); err != nil {
		// Let the server return its 400; nothing to inject into.
		return plan
	}
	if _, has := probe["limit"]; has {
		return plan
	}
	probe["limit"] = json.RawMessage(fmt.Sprintf("%d", limit))
	out, err := json.Marshal(probe)
	if err != nil {
		return plan
	}
	return out
}

// splitNDJSON parses the streaming response. Lines that contain
// "_meta" are returned via the meta result; everything else is a row.
// The server always emits exactly one trailing _meta line, but a
// disconnected mid-stream curl might leave none — callers handle
// the empty-meta case.
func splitNDJSON(body []byte) (rows []json.RawMessage, meta json.RawMessage, err error) {
	for _, line := range bytes.Split(bytes.TrimRight(body, "\n"), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, nil, fmt.Errorf("ndjson decode: %w (line=%q)", err, string(line))
		}
		if m, ok := probe["_meta"]; ok {
			meta = m
			continue
		}
		rows = append(rows, append(json.RawMessage(nil), line...))
	}
	return rows, meta, nil
}

// tryDecodeError peeks at the response body for the {"error":"..."}
// envelope written by writeError on the server. Returns the message
// and ok=true on a match. On any parse error it returns ok=false so
// callers fall through to their normal happy-path decoding.
func tryDecodeError(body []byte) (string, bool) {
	body = bytes.TrimSpace(body)
	if !bytes.HasPrefix(body, []byte("{")) {
		return "", false
	}
	var probe struct {
		Error string `json:"error"`
		// presence-check fields used to filter false positives: a
		// successful /debug/db/info response also starts with a
		// JSON object but never carries "error" as the only key.
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", false
	}
	if probe.Error == "" {
		return "", false
	}
	return probe.Error, true
}
