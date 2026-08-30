package devicesapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientListsOnlyApprovedDeviceFields(t *testing.T) {
	t.Parallel()

	var tokenRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/oauth/token":
			clientID, clientSecret, ok := request.BasicAuth()
			if !ok || clientID != "client-id" || clientSecret != "client-secret" {
				t.Fatalf("unexpected OAuth credentials: %q %q %v", clientID, clientSecret, ok)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := request.Form.Get("scope"); got != OAuthScope {
				t.Fatalf("scope = %q, want %q", got, OAuthScope)
			}
			tokenRequests.Add(1)
			writeJSON(t, writer, map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/v2/tailnet/-/devices":
			if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("authorization = %q", got)
			}
			if got := request.URL.Query().Get("fields"); got != "" {
				t.Fatalf("fields = %q, want default fields", got)
			}
			writeJSON(t, writer, map[string]any{"devices": []map[string]any{{
				"addresses":          []string{"100.64.0.1", "fd7a:115c:a1e0::1"},
				"name":               "workstation.example.ts.net",
				"id":                 "legacy-id-must-not-cross-boundary",
				"nodeId":             "node-stable-1",
				"tags":               []string{"tag:dev"},
				"hostname":           "workstation",
				"connectedToControl": false,
				"lastSeen":           "2026-08-31T08:00:00Z",
				"nodeKey":            "nodekey:abc",
				"os":                 "linux",
				"user":               "must-not-cross-boundary@example.com",
				"clientVersion":      "must-not-cross-boundary",
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client := New(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		BaseURL:      mustParseURL(t, server.URL),
	})
	devices, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tokenRequests.Load() != 1 {
		t.Fatalf("token requests = %d, want 1", tokenRequests.Load())
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	device := devices[0]
	if device.NodeID != "node-stable-1" || device.Name != "workstation.example.ts.net" ||
		device.Hostname != "workstation" || device.NodeKey != "nodekey:abc" || device.OS != "linux" {
		t.Fatalf("unexpected device: %#v", device)
	}
	if device.LastSeen == nil || !device.LastSeen.Equal(time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("last seen = %v", device.LastSeen)
	}
	if got := strings.Join(device.Addresses, ","); got != "100.64.0.1,fd7a:115c:a1e0::1" {
		t.Fatalf("addresses = %q", got)
	}
	if got := strings.Join(device.Tags, ","); got != "tag:dev" {
		t.Fatalf("tags = %q", got)
	}
}

func TestClientUsesFixedTimeoutAndRenewsOAuthToken(t *testing.T) {
	t.Parallel()

	var tokenRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/oauth/token":
			token := tokenRequests.Add(1)
			writeJSON(t, writer, map[string]any{
				"access_token": "token-" + string(rune('0'+token)),
				"token_type":   "Bearer",
				"expires_in":   1,
			})
		case "/api/v2/tailnet/example.test/devices":
			writeJSON(t, writer, map[string]any{"devices": []any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client := New(Config{
		Tailnet:      "example.test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		BaseURL:      mustParseURL(t, server.URL),
	})
	if client.upstream.HTTP.Timeout != RequestTimeout {
		t.Fatalf("timeout = %s, want %s", client.upstream.HTTP.Timeout, RequestTimeout)
	}
	for range 2 {
		if _, err := client.List(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if tokenRequests.Load() != 2 {
		t.Fatalf("token requests = %d, want 2 for immediately expiring tokens", tokenRequests.Load())
	}
}

func TestClientHonorsCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/oauth/token" {
			writeJSON(t, writer, map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	client := New(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		BaseURL:      mustParseURL(t, server.URL),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestClientSanitizesAPIError(t *testing.T) {
	t.Parallel()

	const marker = "private-upstream-message"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/oauth/token" {
			writeJSON(t, writer, map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		writer.WriteHeader(http.StatusForbidden)
		writeJSON(t, writer, map[string]any{"message": marker})
	}))
	t.Cleanup(server.Close)

	client := New(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		BaseURL:      mustParseURL(t, server.URL),
	})
	_, err := client.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("sanitized error contains upstream body: %q", err)
	}
	var requestError *RequestError
	if !errors.As(err, &requestError) || requestError.StatusCode != http.StatusForbidden {
		t.Fatalf("error = %#v, want status 403", err)
	}
	if requestError.Kind != ErrorForbidden {
		t.Fatalf("kind = %q, want %q", requestError.Kind, ErrorForbidden)
	}
	if errors.Unwrap(requestError) != nil {
		t.Fatalf("API error retained upstream cause: %#v", errors.Unwrap(requestError))
	}
}

func TestKindForStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   ErrorKind
	}{
		{status: http.StatusUnauthorized, want: ErrorUnauthorized},
		{status: http.StatusForbidden, want: ErrorForbidden},
		{status: http.StatusTooManyRequests, want: ErrorRateLimited},
		{status: http.StatusInternalServerError, want: ErrorUnavailable},
		{status: http.StatusBadRequest, want: ErrorInvalidResponse},
	}
	for _, test := range tests {
		if got := kindForStatus(test.status); got != test.want {
			t.Errorf("status %d = %q, want %q", test.status, got, test.want)
		}
	}
}

func mustParseURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
}
