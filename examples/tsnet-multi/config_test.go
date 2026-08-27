package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigUsesEnvironmentAndFlags(t *testing.T) {
	environment := map[string]string{
		"TAILPATH_SERVER_URL":      "http://environment:8080",
		"TAILPATH_STATE_DIR":       "/environment/state",
		"TAILPATH_HOSTNAME_PREFIX": "environment",
		"TS_AUTHKEY":               "tskey-environment",
		"TS_CONTROL_URL":           "https://control.example",
	}
	getenv := func(key string) string { return environment[key] }
	config, err := parseConfig([]string{
		"--server=http://flag:8080", "--state-dir=/flag/state", "--hostname-prefix=flag",
		"--auth-key=tskey-flag", "--lifecycle-demo", "--lifecycle-step=2s", "--workload-demo",
	}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if config.serverURL != "http://flag:8080" || config.stateDir != filepath.Clean("/flag/state") ||
		config.hostnamePrefix != "flag" || config.authKey != "tskey-flag" ||
		config.controlURL != "https://control.example" || !config.lifecycleDemo ||
		config.lifecycleStep != 2*time.Second || !config.workloadDemo {
		t.Fatalf("config = %#v", config)
	}
}

func TestConfigReadsAuthKeyFileWithoutRetainingPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-key")
	if err := os.WriteFile(path, []byte("  tskey-private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := parseConfig([]string{"--auth-key=file:" + path}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if config.authKey != "tskey-private" || strings.Contains(config.authKey, "file:") {
		t.Fatalf("resolved auth key = %q", config.authKey)
	}
}

func TestConfigRejectsUnsafeLifecycleAndMissingValues(t *testing.T) {
	for _, arguments := range [][]string{
		{"--lifecycle-step=100ms"},
		{"--server="},
		{"--state-dir="},
		{"--hostname-prefix="},
		{"unexpected"},
		{"--auth-key=file:"},
	} {
		if _, err := parseConfig(arguments, func(string) string { return "" }); err == nil {
			t.Fatalf("arguments %#v were accepted", arguments)
		}
	}
}
