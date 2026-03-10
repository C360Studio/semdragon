package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteEnvVar_CreateFile verifies that writeEnvVar creates a new .env file
// when none exists and writes the key=value pair with mode 0600.
func TestWriteEnvVar_CreateFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	if err := writeEnvVar("TEST_KEY_ALPHA", "secret123"); err != nil {
		t.Fatalf("writeEnvVar: %v", err)
	}

	// File should exist with correct content.
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(data), "TEST_KEY_ALPHA=secret123") {
		t.Errorf("expected TEST_KEY_ALPHA=secret123 in file, got:\n%s", string(data))
	}

	// File mode must be 0600 (owner read/write only).
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}
}

// TestWriteEnvVar_UpdateExistingKey verifies that writing to an existing key
// replaces its line rather than appending a duplicate.
func TestWriteEnvVar_UpdateExistingKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	// Write initial value.
	if err := writeEnvVar("MY_KEY", "value1"); err != nil {
		t.Fatalf("first writeEnvVar: %v", err)
	}

	// Overwrite.
	if err := writeEnvVar("MY_KEY", "value2"); err != nil {
		t.Fatalf("second writeEnvVar: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "MY_KEY=value1") {
		t.Error("old value should have been replaced but was found in file")
	}
	if !strings.Contains(content, "MY_KEY=value2") {
		t.Errorf("expected MY_KEY=value2 in file, got:\n%s", content)
	}

	// Exactly one occurrence.
	count := strings.Count(content, "MY_KEY=")
	if count != 1 {
		t.Errorf("expected exactly 1 MY_KEY= line, got %d in:\n%s", count, content)
	}
}

// TestWriteEnvVar_PreservesOtherKeys verifies that updating one key does not
// remove unrelated keys in the same file.
func TestWriteEnvVar_PreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	if err := writeEnvVar("KEY_ONE", "aaa"); err != nil {
		t.Fatalf("writeEnvVar KEY_ONE: %v", err)
	}
	if err := writeEnvVar("KEY_TWO", "bbb"); err != nil {
		t.Fatalf("writeEnvVar KEY_TWO: %v", err)
	}
	if err := writeEnvVar("KEY_ONE", "zzz"); err != nil {
		t.Fatalf("writeEnvVar KEY_ONE update: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "KEY_ONE=zzz") {
		t.Errorf("expected KEY_ONE=zzz, got:\n%s", content)
	}
	if !strings.Contains(content, "KEY_TWO=bbb") {
		t.Errorf("expected KEY_TWO=bbb to be preserved, got:\n%s", content)
	}
}

// TestWriteEnvVar_SetsProcessEnv verifies that os.Getenv reflects the written
// value immediately after the call returns.
func TestWriteEnvVar_SetsProcessEnv(t *testing.T) {
	// Not parallel — touches os.Setenv.
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	const key = "SEMDRAGONS_ENVFILE_TEST_IMMEDIATE"
	os.Unsetenv(key) //nolint:errcheck

	if err := writeEnvVar(key, "live-value"); err != nil {
		t.Fatalf("writeEnvVar: %v", err)
	}

	if got := os.Getenv(key); got != "live-value" {
		t.Errorf("os.Getenv(%q) = %q, want %q", key, got, "live-value")
	}
}

// TestWriteEnvVar_EmptyKey verifies that an empty key is rejected.
func TestWriteEnvVar_EmptyKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	if err := writeEnvVar("", "value"); err == nil {
		t.Error("expected error for empty key, got nil")
	}
}

// TestSearchAPIKeyEnvVar verifies the env var naming convention for search providers.
func TestSearchAPIKeyEnvVar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider string
		want     string
	}{
		{"brave", "BRAVE_SEARCH_API_KEY"},
		{"", "BRAVE_SEARCH_API_KEY"},   // unknown falls back to brave
		{"other", "BRAVE_SEARCH_API_KEY"}, // unknown falls back to brave
	}

	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			got := searchAPIKeyEnvVar(tc.provider)
			if got != tc.want {
				t.Errorf("searchAPIKeyEnvVar(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}
