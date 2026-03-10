package api

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeEnvVar writes or updates a KEY=VALUE pair in the .env file and calls
// os.Setenv so the value takes effect immediately in the running process.
//
// The .env file path is determined by the SEMDRAGONS_ENV_FILE environment
// variable; when unset it falls back to ".env" in the working directory.
//
// The file is written atomically (temp file + rename) with mode 0600 so only
// the process owner can read the credentials.
func writeEnvVar(key, value string) error {
	if key == "" {
		return fmt.Errorf("env var key must not be empty")
	}

	envPath := envFilePath()

	// Read existing lines so we can do an in-place replacement when the key
	// already exists rather than creating duplicates.
	existing, err := readEnvLines(envPath)
	if err != nil {
		return fmt.Errorf("read .env file: %w", err)
	}

	prefix := key + "="
	found := false
	for i, line := range existing {
		if strings.HasPrefix(line, prefix) {
			existing[i] = prefix + value
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, prefix+value)
	}

	if err := writeEnvLines(envPath, existing); err != nil {
		return fmt.Errorf("write .env file: %w", err)
	}

	// Make the value available immediately without requiring a restart.
	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("os.Setenv %q: %w", key, err)
	}

	return nil
}

// envFilePath returns the path to the .env file. It honours the
// SEMDRAGONS_ENV_FILE environment variable and falls back to ".env".
func envFilePath() string {
	if p := os.Getenv("SEMDRAGONS_ENV_FILE"); p != "" {
		return p
	}
	return ".env"
}

// readEnvLines reads the .env file and returns its lines. Returns an empty
// slice when the file does not exist (first-run case).
func readEnvLines(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // path is controlled by the operator
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writeEnvLines writes lines to path atomically using a sibling temp file and
// rename. The file is created with mode 0600 (owner read/write only).
func writeEnvLines(path string, lines []string) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}

	// Write to a temp file in the same directory so the rename is atomic on
	// POSIX filesystems (same mount point).
	tmp, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure the temp file is removed if we fail before the rename.
	var committed bool
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	w := bufio.NewWriter(tmp)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("write line: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	committed = true
	return nil
}
