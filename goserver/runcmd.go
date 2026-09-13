package beamcore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runCmd runs a system command without shell, with timeout.
// Returns (returncode, stdout, stderr); 127 = not found, 124 = timeout.
func runCmd(args []string, timeout time.Duration, lang string) (int, string, string) {
	return runCmdPkexec(args, timeout, lang, os.Getenv("HOTSPOT_PKEXEC") == "1")
}

// runCmdPkexec is the race-free variant: usePkexec is an explicit param,
// not a global env swap (os.Setenv is process-global and racy under
// parallel hotspot requests).
func runCmdPkexec(args []string, timeout time.Duration, lang string, usePkexec bool) (int, string, string) {
	if usePkexec && len(args) > 0 && args[0] == "nmcli" {
		if _, err := exec.LookPath("pkexec"); err == nil {
			args = append([]string{"pkexec"}, args...)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var outBuf, errBuf strings.Builder
	// Use combined capture via Output + Stderr pipe.
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return 124, "", tr(lang, "cmd_timeout", args[0])
	}
	if err != nil {
		var lookErr *exec.Error
		if errors.As(err, &lookErr) {
			return 127, "", tr(lang, "cmd_not_found", args[0])
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), outBuf.String(), errBuf.String()
		}
		return 1, outBuf.String(), tr(lang, "cmd_unexpected", err.Error())
	}
	return 0, outBuf.String(), errBuf.String()
}
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
func pkexecAvailable() bool {
	return hasCommand("pkexec")
}
