package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

// CLIConfig holds parsed CLI flags.
type CLIConfig struct {
	ConfigPath      string
	LogLevel        string
	LogFormat       string
	ShutdownTimeout time.Duration
	ShowVersion     bool
	ShowHelp        bool
	Validate        bool
	Debug           bool
	DebugPort       int
}

func parseFlags() *CLIConfig {
	cfg := &CLIConfig{}

	flag.StringVar(&cfg.ConfigPath, "config",
		getEnv("SEMDRAGONS_CONFIG", "config/semdragons.json"),
		"Path to configuration file (env: SEMDRAGONS_CONFIG)")

	flag.StringVar(&cfg.ConfigPath, "c",
		getEnv("SEMDRAGONS_CONFIG", "config/semdragons.json"),
		"Path to configuration file (shorthand)")

	flag.StringVar(&cfg.LogLevel, "log-level",
		getEnv("SEMDRAGONS_LOG_LEVEL", "info"),
		"Log level: debug, info, warn, error (env: SEMDRAGONS_LOG_LEVEL)")

	flag.StringVar(&cfg.LogFormat, "log-format",
		getEnv("SEMDRAGONS_LOG_FORMAT", "json"),
		"Log format: json, text (env: SEMDRAGONS_LOG_FORMAT)")

	flag.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout",
		30*time.Second,
		"Graceful shutdown timeout")

	flag.BoolVar(&cfg.Debug, "debug",
		getEnvBool("SEMDRAGONS_DEBUG", false),
		"Enable debug mode with pprof (env: SEMDRAGONS_DEBUG)")

	flag.IntVar(&cfg.DebugPort, "debug-port",
		getEnvInt("SEMDRAGONS_DEBUG_PORT", 6060),
		"Debug pprof server port, 0 to disable (env: SEMDRAGONS_DEBUG_PORT)")

	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version and exit")
	flag.BoolVar(&cfg.ShowHelp, "help", false, "Show help and exit")
	flag.BoolVar(&cfg.ShowHelp, "h", false, "Show help and exit (shorthand)")
	flag.BoolVar(&cfg.Validate, "validate", false, "Validate config and exit")

	flag.Parse()

	return cfg
}

func validateFlags(cfg *CLIConfig) error {
	if cfg.ShowVersion || cfg.ShowHelp {
		return nil
	}

	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		return fmt.Errorf("config file not found: %s", cfg.ConfigPath)
	}

	validLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLevels, cfg.LogLevel) {
		return fmt.Errorf("invalid log level: %s", cfg.LogLevel)
	}

	return nil
}

func printHelp() {
	_, _ = fmt.Fprintf(os.Stderr, `%s - Agentic Workflow Coordination Framework

Usage: %s [options]

Options:
`, appName, os.Args[0])
	flag.PrintDefaults()
	_, _ = fmt.Fprintf(os.Stderr, `
Examples:
  # Run with default config
  %s

  # Run with custom config
  %s --config=/path/to/config.yaml

  # Run with debug logging
  %s --log-level=debug --log-format=text

  # Validate config
  %s --validate

Environment Variables:
  SEMDRAGONS_CONFIG      Config file path
  SEMDRAGONS_LOG_LEVEL   Log level
  SEMDRAGONS_LOG_FORMAT  Log format
  SEMDRAGONS_NATS_URLS   NATS server URLs (comma-separated)
  SEMSTREAMS_NATS_URLS   NATS server URLs (fallback)
  SEMDRAGONS_DEBUG       Enable pprof debug server
  SEMDRAGONS_DEBUG_PORT  pprof server port (default: 6060)
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return fallback
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
