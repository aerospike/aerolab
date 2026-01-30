// Package openbrowser provides cross-platform browser opening functionality.
package openbrowser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open attempts to open the URL in the default browser.
// It supports Linux, macOS (darwin), and Windows platforms.
//
// Parameters:
//   - url: The URL to open in the browser
//
// Returns:
//   - error: nil on success, or an error describing what failed
func Open(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
