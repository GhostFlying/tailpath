package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type config struct {
	serverURL      string
	stateDir       string
	hostnamePrefix string
	authKey        string
	controlURL     string
	lifecycleDemo  bool
	lifecycleStep  time.Duration
	workloadDemo   bool
}

func parseConfig(arguments []string, getenv func(string) string) (config, error) {
	flags := flag.NewFlagSet("tsnet-multi", flag.ContinueOnError)
	serverURL := flags.String("server", environmentDefault(getenv, "TAILPATH_SERVER_URL", "http://tailpath:8080"),
		"Tailpath server URL on the Tailnet")
	stateDir := flags.String("state-dir", environmentDefault(getenv, "TAILPATH_STATE_DIR", "tailpath-tsnet-example"),
		"directory for persisted tsnet identities")
	hostnamePrefix := flags.String("hostname-prefix", environmentDefault(getenv, "TAILPATH_HOSTNAME_PREFIX", "tailpath-example"),
		"hostname prefix for reporter and runtime identities")
	authKey := flags.String("auth-key", getenv("TS_AUTHKEY"), "reusable Tailscale auth key or file:path")
	controlURL := flags.String("control-url", getenv("TS_CONTROL_URL"), "optional Tailscale control URL")
	lifecycleDemo := flags.Bool("lifecycle-demo", false, "remove and restart the third runtime once")
	lifecycleStep := flags.Duration("lifecycle-step", 30*time.Second, "delay between lifecycle demo transitions")
	workloadDemo := flags.Bool("workload-demo", false, "run ordinary HTTP traffic from runtime A to runtime B")
	if err := flags.Parse(arguments); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, errors.New("tsnet-multi does not accept positional arguments")
	}
	if strings.TrimSpace(*serverURL) == "" {
		return config{}, errors.New("Tailpath server URL is required")
	}
	if strings.TrimSpace(*stateDir) == "" {
		return config{}, errors.New("state directory is required")
	}
	if strings.TrimSpace(*hostnamePrefix) == "" {
		return config{}, errors.New("hostname prefix is required")
	}
	if *lifecycleStep < time.Second {
		return config{}, errors.New("lifecycle step must be at least one second")
	}
	resolvedKey, err := resolveSecret(*authKey)
	if err != nil {
		return config{}, fmt.Errorf("read auth key: %w", err)
	}
	return config{
		serverURL: strings.TrimSpace(*serverURL), stateDir: filepath.Clean(*stateDir),
		hostnamePrefix: strings.TrimSpace(*hostnamePrefix), authKey: resolvedKey,
		controlURL: strings.TrimSpace(*controlURL), lifecycleDemo: *lifecycleDemo,
		lifecycleStep: *lifecycleStep, workloadDemo: *workloadDemo,
	}, nil
}

func environmentDefault(getenv func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

func resolveSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "file:") {
		return value, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(value, "file:"))
	if path == "" {
		return "", errors.New("auth key file path is empty")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(contents)), nil
}
