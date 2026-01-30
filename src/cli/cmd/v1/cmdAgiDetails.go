package cmd

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

// AgiDetailsCmd shows detailed ingest progress for an AGI instance.
// This includes step-by-step progress, file counts, and error details.
//
// Usage:
//
//	aerolab agi details -n myagi
//	aerolab agi details -n myagi --watch
type AgiDetailsCmd struct {
	Name       TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name" default:"agi"`
	Output     string             `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq)" default:"table"`
	TableTheme string             `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	Watch      bool               `short:"w" long:"watch" description:"Watch mode - continuously update the output"`
	Interval   int                `short:"i" long:"interval" description:"Watch interval in seconds" default:"5"`
	Pager      bool               `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiDetailsOutput represents the detailed output structure for AGI details command.
type AgiDetailsOutput struct {
	Name     string              `json:"name"`
	Label    string              `json:"label"`
	Steps    AgiIngestSteps      `json:"steps"`
	Download AgiDownloadProgress `json:"download,omitempty"`
	Process  AgiProcessProgress  `json:"process,omitempty"`
	Errors   []AgiError          `json:"errors,omitempty"`
}

// AgiIngestSteps represents the ingest step completion status.
type AgiIngestSteps struct {
	Init                bool      `json:"init"`
	Download            bool      `json:"download"`
	Unpack              bool      `json:"unpack"`
	PreProcess          bool      `json:"preProcess"`
	ProcessLogs         bool      `json:"processLogs"`
	ProcessCollect      bool      `json:"processCollectInfo"`
	CriticalError       string    `json:"criticalError,omitempty"`
	InitStart           time.Time `json:"initStart,omitempty"`
	InitEnd             time.Time `json:"initEnd,omitempty"`
	DownloadStart       time.Time `json:"downloadStart,omitempty"`
	DownloadEnd         time.Time `json:"downloadEnd,omitempty"`
	UnpackStart         time.Time `json:"unpackStart,omitempty"`
	UnpackEnd           time.Time `json:"unpackEnd,omitempty"`
	PreProcessStart     time.Time `json:"preProcessStart,omitempty"`
	PreProcessEnd       time.Time `json:"preProcessEnd,omitempty"`
	ProcessStart        time.Time `json:"processStart,omitempty"`
	ProcessEnd          time.Time `json:"processEnd,omitempty"`
	ProcessCollectStart time.Time `json:"processCollectStart,omitempty"`
	ProcessCollectEnd   time.Time `json:"processCollectEnd,omitempty"`
}

// AgiDownloadProgress represents download progress details.
type AgiDownloadProgress struct {
	TotalFiles      int    `json:"totalFiles,omitempty"`
	CompletedFiles  int    `json:"completedFiles,omitempty"`
	TotalSize       string `json:"totalSize,omitempty"`
	CompletedSize   string `json:"completedSize,omitempty"`
	PercentComplete int    `json:"percentComplete,omitempty"`
}

// AgiProcessProgress represents processing progress details.
type AgiProcessProgress struct {
	TotalFiles      int    `json:"totalFiles,omitempty"`
	CompletedFiles  int    `json:"completedFiles,omitempty"`
	TotalSize       string `json:"totalSize,omitempty"`
	CompletedSize   string `json:"completedSize,omitempty"`
	PercentComplete int    `json:"percentComplete,omitempty"`
}

// AgiError represents an error from the ingest process.
type AgiError struct {
	Stage   string `json:"stage"`
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

// Execute implements the command execution for agi details.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiDetailsCmd) Execute(args []string) error {
	cmd := []string{"agi", "details"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.Watch {
		// Watch mode - continuously update
		for {
			// Clear screen (simple approach)
			fmt.Print("\033[H\033[2J")
			err = c.ShowDetails(system, system.Backend.GetInventory(), system.Logger, args, os.Stdout)
			if err != nil {
				system.Logger.Warn("Error getting details: %s", err)
			}
			fmt.Printf("\nRefreshing every %d seconds... (Ctrl+C to stop)\n", c.Interval)
			time.Sleep(time.Duration(c.Interval) * time.Second)
		}
	}

	err = c.ShowDetails(system, system.Backend.GetInventory(), system.Logger, args, os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ShowDetails shows detailed ingest progress for an AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//   - out: Output writer
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiDetailsCmd) ShowDetails(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, out *os.File) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "details"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Find AGI instance
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateRunning).Describe()

	if instances.Count() == 0 {
		// Check if instance exists but not running
		allInstances := inventory.Instances.WithTags(map[string]string{
			"aerolab.type": "agi",
		}).WithClusterName(c.Name.String()).Describe()
		if allInstances.Count() > 0 {
			return fmt.Errorf("AGI instance %s is not running (state: %s)", c.Name, allInstances[0].InstanceState.String())
		}
		return fmt.Errorf("AGI instance %s not found", c.Name)
	}

	inst := instances[0]

	// Build output
	output := AgiDetailsOutput{
		Name:   inst.ClusterName,
		Label:  decodeBase64Tag(inst.Tags["agiLabel"]),
		Errors: []AgiError{},
	}

	// Establish a single SFTP connection for all file reads
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		output.Steps.CriticalError = fmt.Sprintf("failed to get SFTP config: %v", err)
		return c.renderOutput(system, output, out)
	}

	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		output.Steps.CriticalError = fmt.Sprintf("failed to establish SFTP connection: %v", err)
		return c.renderOutput(system, output, out)
	}
	defer cli.Close()

	// Read all files using the single SFTP connection
	files := c.readAllFiles(cli)

	// Get steps status
	output.Steps = c.getStepsFromFiles(files)

	// Get download progress
	output.Download = c.getDownloadProgressFromFiles(files)

	// Get processing progress
	output.Process = c.getProcessProgressFromFiles(files)

	// Get errors
	output.Errors = c.getErrorsFromFiles(files)

	return c.renderOutput(system, output, out)
}

// agiFileContents holds all files read from the AGI instance.
type agiFileContents struct {
	Steps        []byte
	Downloader   []byte
	Unpacker     []byte
	PreProcessor []byte
	LogProcessor []byte
	CfProcessor  []byte
}

// readAllFiles reads all necessary files using a single SFTP connection.
func (c *AgiDetailsCmd) readAllFiles(cli *sshexec.Sftp) agiFileContents {
	files := agiFileContents{}

	// Define all files to read
	fileList := []struct {
		path  string
		dest  *[]byte
		tryGz bool
	}{
		{"/opt/agi/ingest/steps.json", &files.Steps, false},
		{"/opt/agi/ingest/downloader.json", &files.Downloader, true},
		{"/opt/agi/ingest/unpacker.json", &files.Unpacker, true},
		{"/opt/agi/ingest/pre-processor.json", &files.PreProcessor, true},
		{"/opt/agi/ingest/log-processor.json", &files.LogProcessor, true},
		{"/opt/agi/ingest/cf-processor.json", &files.CfProcessor, true},
	}

	for _, f := range fileList {
		content := c.readFileWithClient(cli, f.path, f.tryGz)
		if content != nil {
			*f.dest = content
		}
	}

	return files
}

// readFileWithClient reads a file using an existing SFTP client.
// If tryGz is true and the plain file doesn't exist, it tries the .gz version.
func (c *AgiDetailsCmd) readFileWithClient(cli *sshexec.Sftp, path string, tryGz bool) []byte {
	// Try plain file first
	if cli.IsExists(path) {
		var buf bytes.Buffer
		err := cli.ReadFile(&sshexec.FileReader{
			SourcePath:  path,
			Destination: &buf,
		})
		if err == nil {
			return buf.Bytes()
		}
	}

	// Try gzipped version if requested
	if tryGz {
		gzPath := path + ".gz"
		if cli.IsExists(gzPath) {
			var buf bytes.Buffer
			err := cli.ReadFile(&sshexec.FileReader{
				SourcePath:  gzPath,
				Destination: &buf,
			})
			if err == nil {
				// Decompress gzip content
				reader, err := gzip.NewReader(&buf)
				if err != nil {
					return nil
				}
				defer reader.Close()
				decompressed, err := io.ReadAll(reader)
				if err != nil {
					return nil
				}
				return decompressed
			}
		}
	}

	return nil
}

// getStepsFromFiles retrieves the ingest steps status from pre-read files.
func (c *AgiDetailsCmd) getStepsFromFiles(files agiFileContents) AgiIngestSteps {
	steps := AgiIngestSteps{}

	if files.Steps == nil {
		steps.CriticalError = "could not read steps.json via SFTP"
		return steps
	}

	var rawSteps struct {
		Init                        bool      `json:"Init"`
		Download                    bool      `json:"Download"`
		Unpack                      bool      `json:"Unpack"`
		PreProcess                  bool      `json:"PreProcess"`
		ProcessLogs                 bool      `json:"ProcessLogs"`
		ProcessCollectInfo          bool      `json:"ProcessCollectInfo"`
		CriticalError               string    `json:"CriticalError"`
		InitStartTime               time.Time `json:"InitStartTime"`
		InitEndTime                 time.Time `json:"InitEndTime"`
		DownloadStartTime           time.Time `json:"DownloadStartTime"`
		DownloadEndTime             time.Time `json:"DownloadEndTime"`
		UnpackStartTime             time.Time `json:"UnpackStartTime"`
		UnpackEndTime               time.Time `json:"UnpackEndTime"`
		PreProcessStartTime         time.Time `json:"PreProcessStartTime"`
		PreProcessEndTime           time.Time `json:"PreProcessEndTime"`
		ProcessLogsStartTime        time.Time `json:"ProcessLogsStartTime"`
		ProcessLogsEndTime          time.Time `json:"ProcessLogsEndTime"`
		ProcessCollectInfoStartTime time.Time `json:"ProcessCollectInfoStartTime"`
		ProcessCollectInfoEndTime   time.Time `json:"ProcessCollectInfoEndTime"`
	}
	if err := json.Unmarshal(files.Steps, &rawSteps); err != nil {
		steps.CriticalError = fmt.Sprintf("json parse error: %v (content: %s)", err, string(files.Steps))
		return steps
	}

	steps.Init = rawSteps.Init
	steps.Download = rawSteps.Download
	steps.Unpack = rawSteps.Unpack
	steps.PreProcess = rawSteps.PreProcess
	steps.ProcessLogs = rawSteps.ProcessLogs
	steps.ProcessCollect = rawSteps.ProcessCollectInfo
	steps.CriticalError = rawSteps.CriticalError
	steps.InitStart = rawSteps.InitStartTime
	steps.InitEnd = rawSteps.InitEndTime
	steps.DownloadStart = rawSteps.DownloadStartTime
	steps.DownloadEnd = rawSteps.DownloadEndTime
	steps.UnpackStart = rawSteps.UnpackStartTime
	steps.UnpackEnd = rawSteps.UnpackEndTime
	steps.PreProcessStart = rawSteps.PreProcessStartTime
	steps.PreProcessEnd = rawSteps.PreProcessEndTime
	steps.ProcessStart = rawSteps.ProcessLogsStartTime
	steps.ProcessEnd = rawSteps.ProcessLogsEndTime
	steps.ProcessCollectStart = rawSteps.ProcessCollectInfoStartTime
	steps.ProcessCollectEnd = rawSteps.ProcessCollectInfoEndTime

	return steps
}

// getDownloadProgressFromFiles retrieves download progress from pre-read files.
func (c *AgiDetailsCmd) getDownloadProgressFromFiles(files agiFileContents) AgiDownloadProgress {
	progress := AgiDownloadProgress{}

	if files.Downloader == nil {
		return progress
	}

	var rawProgress struct {
		TotalSize      int64 `json:"TotalSize"`
		CompletedSize  int64 `json:"CompletedSize"`
		TotalFiles     int   `json:"TotalFiles"`
		CompletedFiles int   `json:"CompletedFiles"`
	}
	if err := json.Unmarshal(files.Downloader, &rawProgress); err == nil {
		progress.TotalFiles = rawProgress.TotalFiles
		progress.CompletedFiles = rawProgress.CompletedFiles
		progress.TotalSize = formatBytes(rawProgress.TotalSize)
		progress.CompletedSize = formatBytes(rawProgress.CompletedSize)
		if rawProgress.TotalSize > 0 {
			progress.PercentComplete = int(rawProgress.CompletedSize * 100 / rawProgress.TotalSize)
		}
	}

	return progress
}

// getProcessProgressFromFiles retrieves processing progress from pre-read files.
func (c *AgiDetailsCmd) getProcessProgressFromFiles(files agiFileContents) AgiProcessProgress {
	progress := AgiProcessProgress{}

	if files.LogProcessor == nil {
		return progress
	}

	var rawProgress struct {
		TotalSize      int64 `json:"TotalSize"`
		CompletedSize  int64 `json:"CompletedSize"`
		TotalFiles     int   `json:"TotalFiles"`
		CompletedFiles int   `json:"CompletedFiles"`
	}
	if err := json.Unmarshal(files.LogProcessor, &rawProgress); err == nil {
		progress.TotalFiles = rawProgress.TotalFiles
		progress.CompletedFiles = rawProgress.CompletedFiles
		progress.TotalSize = formatBytes(rawProgress.TotalSize)
		progress.CompletedSize = formatBytes(rawProgress.CompletedSize)
		if rawProgress.TotalSize > 0 {
			progress.PercentComplete = int(rawProgress.CompletedSize * 100 / rawProgress.TotalSize)
		}
	}

	return progress
}

// getErrorsFromFiles retrieves errors from pre-read files.
func (c *AgiDetailsCmd) getErrorsFromFiles(files agiFileContents) []AgiError {
	errors := []AgiError{}

	// Check for errors in different progress files
	stages := []struct {
		name    string
		content []byte
	}{
		{"download", files.Downloader},
		{"unpack", files.Unpacker},
		{"preprocess", files.PreProcessor},
		{"process", files.LogProcessor},
		{"collectinfo", files.CfProcessor},
	}

	for _, stage := range stages {
		if stage.content != nil {
			var rawProgress struct {
				Errors []struct {
					File    string `json:"File"`
					Message string `json:"Error"`
				} `json:"Errors"`
			}
			if err := json.Unmarshal(stage.content, &rawProgress); err == nil {
				for _, e := range rawProgress.Errors {
					errors = append(errors, AgiError{
						Stage:   stage.name,
						File:    e.File,
						Message: e.Message,
					})
				}
			}
		}
	}

	return errors
}

// formatBytes formats bytes to human-readable format.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// renderOutput renders the details output in the requested format.
func (c *AgiDetailsCmd) renderOutput(system *System, output AgiDetailsOutput, out *os.File) error {
	var page *pager.Pager
	var err error

	if c.Pager && !c.Watch {
		page, err = pager.New(out)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
	}

	switch c.Output {
	case "jq":
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		cmd.Stdout = out
		cmd.Stderr = out
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(output)
			w.Close()
		}()
		return cmd.Run()

	case "json":
		return json.NewEncoder(out).Encode(output)

	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(output)

	case "text":
		fmt.Fprintf(out, "AGI Instance: %s\n", output.Name)
		fmt.Fprintf(out, "Label: %s\n\n", output.Label)

		fmt.Fprintln(out, "Ingest Steps:")
		fmt.Fprintf(out, "  Init: %s\n", boolToStatus(output.Steps.Init))
		fmt.Fprintf(out, "  Download: %s\n", boolToStatus(output.Steps.Download))
		fmt.Fprintf(out, "  Unpack: %s\n", boolToStatus(output.Steps.Unpack))
		fmt.Fprintf(out, "  Pre-Process: %s\n", boolToStatus(output.Steps.PreProcess))
		fmt.Fprintf(out, "  Process Logs: %s\n", boolToStatus(output.Steps.ProcessLogs))
		fmt.Fprintf(out, "  Process CollectInfo: %s\n", boolToStatus(output.Steps.ProcessCollect))

		if output.Steps.CriticalError != "" {
			fmt.Fprintf(out, "\nCRITICAL ERROR: %s\n", output.Steps.CriticalError)
		}

		if output.Download.TotalFiles > 0 {
			fmt.Fprintf(out, "\nDownload Progress: %d%% (%d/%d files, %s/%s)\n",
				output.Download.PercentComplete, output.Download.CompletedFiles, output.Download.TotalFiles,
				output.Download.CompletedSize, output.Download.TotalSize)
		}

		if output.Process.TotalFiles > 0 {
			fmt.Fprintf(out, "\nProcessing Progress: %d%% (%d/%d files, %s/%s)\n",
				output.Process.PercentComplete, output.Process.CompletedFiles, output.Process.TotalFiles,
				output.Process.CompletedSize, output.Process.TotalSize)
		}

		if len(output.Errors) > 0 {
			fmt.Fprintf(out, "\nErrors (%d):\n", len(output.Errors))
			for _, e := range output.Errors {
				if e.File != "" {
					fmt.Fprintf(out, "  [%s] %s: %s\n", e.Stage, e.File, e.Message)
				} else {
					fmt.Fprintf(out, "  [%s] %s\n", e.Stage, e.Message)
				}
			}
		}
		return nil

	default: // table
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, nil, page == nil || !page.HasColors(), page != nil)
		if err != nil && err != printer.ErrTerminalWidthUnknown {
			return err
		}

		// Instance info
		fmt.Fprintf(out, "AGI Instance: %s\n", output.Name)
		fmt.Fprintf(out, "Label: %s\n\n", output.Label)

		// Steps table
		stepsHeader := table.Row{"Step", "Status", "Duration"}
		stepsRows := []table.Row{}

		steps := []struct {
			name  string
			done  bool
			start time.Time
			end   time.Time
		}{
			{"Init", output.Steps.Init, output.Steps.InitStart, output.Steps.InitEnd},
			{"Download", output.Steps.Download, output.Steps.DownloadStart, output.Steps.DownloadEnd},
			{"Unpack", output.Steps.Unpack, output.Steps.UnpackStart, output.Steps.UnpackEnd},
			{"Pre-Process", output.Steps.PreProcess, output.Steps.PreProcessStart, output.Steps.PreProcessEnd},
			{"Process Logs", output.Steps.ProcessLogs, output.Steps.ProcessStart, output.Steps.ProcessEnd},
			{"Process CollectInfo", output.Steps.ProcessCollect, output.Steps.ProcessCollectStart, output.Steps.ProcessCollectEnd},
		}

		for _, step := range steps {
			status := "Pending"
			if step.done {
				status = "Complete"
				if t != nil {
					status = t.ColorHiWhite.Sprint("Complete")
				}
			}

			duration := "-"
			if !step.start.IsZero() && !step.end.IsZero() {
				duration = step.end.Sub(step.start).Truncate(time.Second).String()
			} else if !step.start.IsZero() {
				duration = time.Since(step.start).Truncate(time.Second).String() + " (running)"
			}

			stepsRows = append(stepsRows, table.Row{step.name, status, duration})
		}

		fmt.Fprintln(out, t.RenderTable(printer.String("INGEST STEPS"), stepsHeader, stepsRows))

		// Critical error
		if output.Steps.CriticalError != "" {
			fmt.Fprintln(out, "")
			if t != nil {
				fmt.Fprintf(out, "%s: %s\n", t.ColorErr.Sprint("CRITICAL ERROR"), output.Steps.CriticalError)
			} else {
				fmt.Fprintf(out, "CRITICAL ERROR: %s\n", output.Steps.CriticalError)
			}
		}

		// Progress summary
		if output.Download.TotalFiles > 0 || output.Process.TotalFiles > 0 {
			fmt.Fprintln(out, "")
			progressHeader := table.Row{"Stage", "Files", "Size", "Progress"}
			progressRows := []table.Row{}

			if output.Download.TotalFiles > 0 {
				progressRows = append(progressRows, table.Row{
					"Download",
					fmt.Sprintf("%d/%d", output.Download.CompletedFiles, output.Download.TotalFiles),
					fmt.Sprintf("%s/%s", output.Download.CompletedSize, output.Download.TotalSize),
					fmt.Sprintf("%d%%", output.Download.PercentComplete),
				})
			}

			if output.Process.TotalFiles > 0 {
				progressRows = append(progressRows, table.Row{
					"Processing",
					fmt.Sprintf("%d/%d", output.Process.CompletedFiles, output.Process.TotalFiles),
					fmt.Sprintf("%s/%s", output.Process.CompletedSize, output.Process.TotalSize),
					fmt.Sprintf("%d%%", output.Process.PercentComplete),
				})
			}

			fmt.Fprintln(out, t.RenderTable(printer.String("PROGRESS"), progressHeader, progressRows))
		}

		// Errors summary
		if len(output.Errors) > 0 {
			fmt.Fprintln(out, "")
			errHeader := table.Row{"Stage", "File", "Error"}
			errRows := []table.Row{}

			// Limit to first 10 errors in table view
			maxErrors := 10
			if len(output.Errors) < maxErrors {
				maxErrors = len(output.Errors)
			}

			for i := 0; i < maxErrors; i++ {
				e := output.Errors[i]
				errRows = append(errRows, table.Row{e.Stage, e.File, e.Message})
			}

			title := fmt.Sprintf("ERRORS (%d total)", len(output.Errors))
			if len(output.Errors) > maxErrors {
				title += fmt.Sprintf(" - showing first %d", maxErrors)
			}
			fmt.Fprintln(out, t.RenderTable(printer.String(title), errHeader, errRows))
		}

		fmt.Fprintln(out, "")
		return nil
	}
}

// boolToStatus converts a boolean to a status string.
func boolToStatus(b bool) string {
	if b {
		return "Complete"
	}
	return "Pending"
}

// Ensure imports are used
var _ = gzip.Reader{}
var _ io.Reader
