package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestCollectorConfigPrecedence(t *testing.T) {
	environment := map[string]string{
		"TAILPATH_SERVER_URL": "http://environment:8080",
		"TAILPATH_SOCKET":     "/environment/tailscaled.sock",
	}
	getenv := func(key string) string { return environment[key] }

	tests := []struct {
		name       string
		arguments  []string
		getenv     func(string) string
		wantServer string
		wantSocket string
		wantCheck  bool
	}{
		{name: "built-in defaults", getenv: func(string) string { return "" }, wantServer: "http://tailpath:8080"},
		{name: "environment", getenv: getenv, wantServer: "http://environment:8080", wantSocket: "/environment/tailscaled.sock"},
		{name: "flags", arguments: []string{"--server=http://flag:8080", "--socket=/flag/tailscaled.sock", "--check"}, getenv: getenv, wantServer: "http://flag:8080", wantSocket: "/flag/tailscaled.sock", wantCheck: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCollectorConfig(test.arguments, test.getenv)
			if err != nil {
				t.Fatal(err)
			}
			if got.serverURL != test.wantServer || got.socket != test.wantSocket || got.check != test.wantCheck {
				t.Fatalf("config = %#v, want server=%q socket=%q check=%v", got, test.wantServer, test.wantSocket, test.wantCheck)
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
	if err := checkCollector(context.Background(), source, &output); err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 {
		t.Fatalf("Snapshot calls = %d, want one", source.calls)
	}
	var result collector.Diagnostic
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("check output is not JSON: %v", err)
	}
	if result.Self.StableNodeID != "self-id" || result.OS == "" || result.PeerCount != 2 {
		t.Fatalf("check result = %#v", result)
	}
}

type checkSource struct {
	diagnostic collector.Diagnostic
	calls      int
}

func (s *checkSource) Diagnostic(context.Context) (collector.Diagnostic, error) {
	s.calls++
	return s.diagnostic, nil
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

func TestServerRejectsScaleFixtureOutsideFixtureCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := runServer([]string{"--scale"}, logger, false)
	if err == nil || err.Error() != "scale fixture is only available with fixture-server" {
		t.Fatalf("runServer error = %v, want scale fixture validation", err)
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
