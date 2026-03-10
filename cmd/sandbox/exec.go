package main

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"
)

// execCommand runs a shell command in the given directory with timeout enforcement.
// Returns stdout, stderr, exit code, and whether the command timed out.
func execCommand(ctx context.Context, workDir, command string, timeout time.Duration, maxOutput int) ExecResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir

	// Minimal environment — toolchain paths only.
	cmd.Env = []string{
		"PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/go/bin:/root/.cargo/bin",
		"HOME=/home/sandbox",
		"GOPATH=/go",
		"GOMODCACHE=/go/pkg/mod",
		"NODE_PATH=/usr/local/lib/node_modules",
		"JAVA_HOME=/usr/lib/jvm/java-21-openjdk-amd64",
	}

	// Set process group so we can kill the entire tree on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := &cappedWriter{max: maxOutput}
	stderr := &cappedWriter{max: maxOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	var exitCode int
	timedOut := ctx.Err() == context.DeadlineExceeded

	if timedOut {
		// Kill the process group to clean up any child processes.
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // best-effort cleanup
		}
		exitCode = -1
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return ExecResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		TimedOut: timedOut,
	}
}

// cappedWriter captures output up to a maximum number of bytes.
// Once the cap is reached, additional writes are silently discarded.
type cappedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // Discard silently.
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}

func (w *cappedWriter) String() string {
	return w.buf.String()
}
