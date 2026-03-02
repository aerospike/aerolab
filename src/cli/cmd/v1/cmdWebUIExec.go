package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
)

// WebUIExecCmd is a hidden command that executes commands in subprocess mode.
// It reads command path and parameters from stdin as JSON, executes the command,
// and outputs all logs to stdout/stderr for the parent process to capture.
// This command should never be called directly by users.
type WebUIExecCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// webUIExecInput is the JSON input format for the exec command
type webUIExecInput struct {
	CommandPath string         `json:"commandPath"`
	Parameters  map[string]any `json:"parameters"`
}

func (c *WebUIExecCmd) Execute(args []string) error {
	// Set non-interactive mode
	os.Setenv("AEROLAB_NONINTERACTIVE", "1")

	// Read JSON from stdin
	var input webUIExecInput
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to read input from stdin: %s\n", err) //nolint:errcheck
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Validate input
	if input.CommandPath == "" {
		fmt.Fprintf(os.Stderr, "[ERROR] commandPath is required\n") //nolint:errcheck
		return fmt.Errorf("commandPath is required")
	}

	// Initialize system for command execution.
	// Use InitBackend: false so a broken saved backend config (e.g. invalid region)
	// does not prevent the subprocess from starting. Each command's own Execute
	// method calls Initialize with InitBackend: true when it needs the backend.
	cmd := []string{"webui", "exec"}
	system, err := Initialize(&Init{InitBackend: false}, cmd, c, args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to initialize system: %s\n", err) //nolint:errcheck
		return err
	}

	// Create a log writer that writes to stdout
	logWriter := &execLogWriter{out: os.Stdout}

	// Execute the command using the existing reflection-based execution
	err = ExecuteCommandByPathDirect(system, input.CommandPath, input.Parameters, logWriter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Command failed: %s\n", err) //nolint:errcheck
		return err
	}

	return nil
}

// execLogWriter wraps an io.Writer for command output
type execLogWriter struct {
	out io.Writer
}

func (w *execLogWriter) Write(p []byte) (n int, err error) {
	return w.out.Write(p)
}

// ExecuteCommandByPathDirect executes a command by its path with given parameters,
// writing all output to the provided io.Writer. This is used for subprocess execution.
func ExecuteCommandByPathDirect(system *System, path string, params map[string]any, logWriter io.Writer) error {
	// Enforce simple mode restrictions
	if system.SimpleModeConfig != nil && system.SimpleModeConfig.ForceEnabled {
		dotPath := SimpleModePathFromSlash(path)
		if err := system.SimpleModeConfig.CheckCommandAllowed(dotPath); err != nil {
			if logWriter != nil {
				fmt.Fprintf(logWriter, "[ERROR] %s\n", err) //nolint:errcheck
			}
			return err
		}
	}

	// Find the command struct type by path
	cmdVal, err := getCommandValueByPath(system.Opts, path)
	if err != nil {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "[ERROR] Failed to find command: %s\n", err) //nolint:errcheck
		}
		return err
	}

	// Create a new instance of the command
	cmdType := cmdVal.Type()
	newCmd := reflect.New(cmdType).Elem()

	// Copy defaults from the original
	newCmd.Set(cmdVal)

	// Track which parameters were explicitly provided by the frontend.
	// Commands like config/backend need this to distinguish user-provided
	// values from saved config defaults (analogous to CLI's os.Args checking).
	if params != nil {
		paramKeys := make([]string, 0, len(params))
		for k := range params {
			paramKeys = append(paramKeys, k)
		}
		os.Setenv("AEROLAB_WEBUI_EXEC_PARAMS", strings.Join(paramKeys, ","))
	} else {
		os.Setenv("AEROLAB_WEBUI_EXEC_PARAMS", "")
	}
	defer os.Unsetenv("AEROLAB_WEBUI_EXEC_PARAMS")

	// Apply parameters from the request
	if params != nil {
		if err := applyParameters(newCmd, params); err != nil {
			if logWriter != nil {
				fmt.Fprintf(logWriter, "[ERROR] Failed to apply parameters: %s\n", err) //nolint:errcheck
			}
			return fmt.Errorf("failed to apply parameters: %w", err)
		}
	}

	// Get the Execute method
	cmdPtr := newCmd.Addr()
	executeMethod := cmdPtr.MethodByName("Execute")
	if !executeMethod.IsValid() {
		errMsg := fmt.Sprintf("command %s does not have an Execute method", path)
		if logWriter != nil {
			fmt.Fprintf(logWriter, "[ERROR] %s\n", errMsg) //nolint:errcheck
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Write start message
	if logWriter != nil {
		fmt.Fprintf(logWriter, "[INFO] Starting command: %s\n", path) //nolint:errcheck
		fmt.Fprintf(logWriter, "[INFO] Parameters: %v\n", params)       //nolint:errcheck
		fmt.Fprintf(logWriter, "[INFO] %s\n", strings.Repeat("-", 50)) //nolint:errcheck
	}

	// Call Execute with empty args
	execArgs := []string{}
	results := executeMethod.Call([]reflect.Value{reflect.ValueOf(execArgs)})

	// Check for error
	if len(results) > 0 && !results[0].IsNil() {
		err := results[0].Interface().(error)
		return err
	}

	if logWriter != nil {
		fmt.Fprintf(logWriter, "[INFO] Command completed successfully\n") //nolint:errcheck
	}

	return nil
}
