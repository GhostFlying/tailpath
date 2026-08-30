package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestOrdinaryCollectorSourceHidesRelayCapability(t *testing.T) {
	source := &checkSource{}
	wrapped := ordinaryCollectorSource{Source: source}
	if _, ok := any(wrapped).(exporter.RelaySource); ok {
		t.Fatal("relay-off wrapper still exposes RelaySource")
	}
}

func TestCollectorConfigPrecedence(t *testing.T) {
	environment := map[string]string{
		"TAILPATH_SERVER_URL":      "http://environment:8080",
		"TAILPATH_SOCKET":          "/environment/tailscaled.sock",
		"TAILPATH_RELAY_TELEMETRY": "off",
	}
	getenv := func(key string) string { return environment[key] }

	tests := []struct {
		name       string
		arguments  []string
		getenv     func(string) string
		wantServer string
		wantSocket string
		wantCheck  bool
		wantRelay  string
	}{
		{name: "built-in defaults", getenv: func(string) string { return "" }, wantServer: "http://tailpath:8080", wantRelay: "auto"},
		{name: "environment", getenv: getenv, wantServer: "http://environment:8080", wantSocket: "/environment/tailscaled.sock", wantRelay: "off"},
		{name: "flags", arguments: []string{"--server=http://flag:8080", "--socket=/flag/tailscaled.sock", "--relay-telemetry=auto", "--check"}, getenv: getenv, wantServer: "http://flag:8080", wantSocket: "/flag/tailscaled.sock", wantCheck: true, wantRelay: "auto"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCollectorConfig(test.arguments, test.getenv)
			if err != nil {
				t.Fatal(err)
			}
			if got.serverURL != test.wantServer || got.socket != test.wantSocket || got.check != test.wantCheck || got.relayTelemetry != test.wantRelay {
				t.Fatalf("config = %#v, want server=%q socket=%q check=%v relay=%q", got, test.wantServer, test.wantSocket, test.wantCheck, test.wantRelay)
			}
		})
	}
}

func TestCollectorCheckReadsOnePassiveSnapshot(t *testing.T) {
	source := &checkSource{diagnostic: collector.Diagnostic{
		Self:      domain.NodeIdentity{StableNodeID: "self-id", Hostname: "workstation"},
		OS:        "linux",
		PeerCount: 2,
	}}
	var output bytes.Buffer
	if err := checkCollector(context.Background(), source, source, "auto", &output); err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 {
		t.Fatalf("Snapshot calls = %d, want one", source.calls)
	}
	if source.relayCalls != 1 || bytes.Contains(output.Bytes(), []byte("private-session")) {
		t.Fatalf("relay check leaked session details or skipped capability read: calls=%d output=%s", source.relayCalls, output.Bytes())
	}
	var result collector.Diagnostic
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("check output is not JSON: %v", err)
	}
	if result.Self.StableNodeID != "self-id" || result.OS == "" || result.PeerCount != 2 ||
		result.RelayCapability != collector.RelayEnabled || result.RelayIdentity != collector.RelayIdentityAvailable ||
		!result.RelayEnabled || result.RelaySessionCount != 1 {
		t.Fatalf("check result = %#v", result)
	}
}

func TestCollectorCheckOffSkipsRelayLocalAPI(t *testing.T) {
	source := &checkSource{diagnostic: collector.Diagnostic{
		Self: domain.NodeIdentity{StableNodeID: "self-id"}, OS: "linux",
	}}
	var output bytes.Buffer
	if err := checkCollector(context.Background(), source, source, "off", &output); err != nil {
		t.Fatal(err)
	}
	if source.relayCalls != 0 {
		t.Fatalf("relay LocalAPI calls = %d, want zero", source.relayCalls)
	}
	var result collector.Diagnostic
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RelayCapability != collector.RelayOff || result.RelayEnabled || result.RelaySessionCount != 0 {
		t.Fatalf("off diagnostic = %#v", result)
	}
}

func TestCollectorConfigRejectsUnknownRelayMode(t *testing.T) {
	if _, err := parseCollectorConfig([]string{"--relay-telemetry=on"}, func(string) string { return "" }); err == nil {
		t.Fatal("unknown relay telemetry mode was accepted")
	}
}

func TestDevicesServerConfigPrecedenceAndSecretFile(t *testing.T) {
	directory := t.TempDir()
	environmentSecret := filepath.Join(directory, "environment-secret")
	flagSecret := filepath.Join(directory, "flag-secret")
	if err := os.WriteFile(environmentSecret, []byte(" environment-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flagSecret, []byte("flag-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"TAILPATH_DEVICES_OAUTH_CLIENT_ID":          "environment-client",
		"TAILPATH_DEVICES_OAUTH_CLIENT_SECRET_FILE": environmentSecret,
		"TAILPATH_DEVICES_TAILNET":                  "environment.example",
	}
	getenv := func(key string) string { return environment[key] }

	disabled, err := parseDevicesServerConfig(nil, func(string) string { return "" }, os.ReadFile)
	if err != nil || disabled.enabled || disabled.tailnet != "-" || disabled.clientSecret != "" {
		t.Fatalf("disabled config = %#v, err=%v", disabled, err)
	}
	fromEnvironment, err := parseDevicesServerConfig(nil, getenv, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	if !fromEnvironment.enabled || fromEnvironment.clientID != "environment-client" ||
		fromEnvironment.clientSecret != "environment-value" || fromEnvironment.tailnet != "environment.example" {
		t.Fatalf("environment config = %#v", fromEnvironment)
	}
	fromFlags, err := parseDevicesServerConfig([]string{
		"--devices-oauth-client-id=flag-client",
		"--devices-oauth-client-secret-file=" + flagSecret,
		"--devices-tailnet=flag.example",
	}, getenv, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	if !fromFlags.enabled || fromFlags.clientID != "flag-client" || fromFlags.clientSecret != "flag-value" ||
		fromFlags.tailnet != "flag.example" {
		t.Fatalf("flag config = %#v", fromFlags)
	}
}

func TestDevicesServerConfigRejectsPartialAndInvalidSecrets(t *testing.T) {
	directory := t.TempDir()
	emptySecret := filepath.Join(directory, "empty")
	if err := os.WriteFile(emptySecret, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		arguments []string
		readFile  func(string) ([]byte, error)
	}{
		{name: "client only", arguments: []string{"--devices-oauth-client-id=client"}},
		{name: "secret only", arguments: []string{"--devices-oauth-client-secret-file=" + emptySecret}},
		{name: "empty secret", arguments: []string{"--devices-oauth-client-id=client", "--devices-oauth-client-secret-file=" + emptySecret}},
		{
			name: "unreadable secret", arguments: []string{"--devices-oauth-client-id=client", "--devices-oauth-client-secret-file=/private/secret"},
			readFile: func(string) ([]byte, error) { return nil, errors.New("permission denied") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			readFile := test.readFile
			if readFile == nil {
				readFile = os.ReadFile
			}
			if config, err := parseDevicesServerConfig(test.arguments, func(string) string { return "" }, readFile); err == nil {
				t.Fatalf("invalid config accepted: %#v", config)
			}
		})
	}
}

type checkSource struct {
	diagnostic collector.Diagnostic
	calls      int
	relayCalls int
	relayError error
}

func (s *checkSource) Snapshot(context.Context) (exporter.Snapshot, error) {
	return exporter.Snapshot{}, nil
}

func (s *checkSource) Diagnostic(context.Context) (collector.Diagnostic, error) {
	s.calls++
	return s.diagnostic, nil
}

func (s *checkSource) PeerRelaySnapshot(context.Context) (collector.RelaySnapshot, error) {
	s.relayCalls++
	return collector.RelaySnapshot{
		Capability:       collector.RelayEnabled,
		IdentityEvidence: collector.RelayIdentityAvailable,
		Sessions:         []collector.RelaySessionSnapshot{{SessionID: "private-session"}},
	}, s.relayError
}

func TestResolveAuthKey(t *testing.T) {
	directory := t.TempDir()
	keyPath := filepath.Join(directory, "authkey")
	if err := os.WriteFile(keyPath, []byte("  tskey-test-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "direct", value: " tskey-direct ", want: "tskey-direct"},
		{name: "file", value: "file:" + keyPath, want: "tskey-test-value"},
		{name: "empty direct", want: ""},
		{name: "empty path", value: "file:", wantErr: true},
		{name: "missing file", value: "file:" + filepath.Join(directory, "missing"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveAuthKey(test.value)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("resolveAuthKey() = %q, err=%v, want %q, err=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestValidateTailscaledListenAddress(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		unsafe  bool
		want    string
		wantErr bool
	}{
		{name: "default", host: "", want: "100.64.0.1:8080"},
		{name: "own tailscale IP", host: "100.64.0.1", want: "100.64.0.1:8080"},
		{name: "broad rejected", host: "0.0.0.0", wantErr: true},
		{name: "LAN rejected", host: "192.0.2.10", wantErr: true},
		{name: "explicit unsafe", host: "0.0.0.0", unsafe: true, want: "0.0.0.0:8080"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateTailscaledListenAddress(test.host, "8080", []string{"100.64.0.1", "fd7a::1"}, test.unsafe)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("address = %q, err=%v, want %q, err=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestServerRejectsHeartbeatOutsideSupportedRange(t *testing.T) {
	for _, value := range []string{"9s", "10m1s"} {
		t.Run(value, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			err := runServer([]string{"--heartbeat-interval=" + value}, logger, false)
			if err == nil || err.Error() != "heartbeat interval must be between 10s and 10m" {
				t.Fatalf("runServer error = %v, want heartbeat range validation", err)
			}
		})
	}
}

func TestServerRejectsScaleFixtureOutsideFixtureCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := runServer([]string{"--scale"}, logger, false)
	if err == nil || err.Error() != "scale fixture is only available with fixture-server" {
		t.Fatalf("runServer error = %v, want scale fixture validation", err)
	}
}

func TestServerRejectsRelayScaleFixtureOutsideFixtureCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := runServer([]string{"--relay-scale"}, logger, false)
	if err == nil || err.Error() != "relay scale fixture is only available with fixture-server" {
		t.Fatalf("runServer error = %v, want relay scale fixture validation", err)
	}
}

func TestServerRejectsDevicesFixtureOutsideFixtureCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := runServer([]string{"--devices"}, logger, false)
	if err == nil || err.Error() != "devices fixture is only available with fixture-server" {
		t.Fatalf("runServer error = %v, want devices fixture validation", err)
	}
}

func TestFixtureServerRejectsMultipleFixtureModes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := runServer([]string{"--scale", "--relay-scale"}, logger, true)
	if err == nil || err.Error() != "scale, relay-scale, and empty fixtures are mutually exclusive" {
		t.Fatalf("runServer error = %v, want fixture mode validation", err)
	}
}

func TestServeCancelsStreamingRequestsBeforeShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requestStarted := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
	})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serve(ctx, listener, handler, "127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	requestDone := make(chan struct{})
	go func() {
		response, _ := http.Get("http://" + listener.Addr().String())
		if response != nil {
			response.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming request did not reach server")
	}

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown waited for streaming request")
	}
	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming client did not exit")
	}
}
