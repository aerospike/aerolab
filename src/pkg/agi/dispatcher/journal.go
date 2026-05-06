package dispatcher

import (
	"bufio"
	"context"
	"os/exec"
)

func tailJournal(ctx context.Context, unit string, write lineWriter) error {
	if unit == "" {
		unit = "aerospike.service"
	}
	cmd := exec.CommandContext(ctx, "journalctl", "-fn0", "-u", unit, "--output=cat")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	var offset int64
	for scanner.Scan() {
		line := scanner.Text()
		offset += int64(len(line) + 1)
		if err := write(line, offset); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
