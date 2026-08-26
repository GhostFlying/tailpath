package tailscaleadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"tailscale.com/client/local"
)

func TestRelayFixtureTransportAllowsOnlyPassiveStatusReads(t *testing.T) {
	transport := &relayFixtureTransport{t: t}
	client := &local.Client{Transport: transport, OmitAuth: true}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://local-tailscaled.sock/localapi/v0/debug-peer-relay-sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DoLocalRequest(request)
	if err != nil {
		t.Fatalf("read relay sessions: %v", err)
	}
	response.Body.Close()

	request, err = http.NewRequestWithContext(context.Background(), http.MethodPost,
		"http://local-tailscaled.sock/localapi/v0/debug?action=peer-disco-keys", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.DoLocalRequest(request)
	if err != nil {
		t.Fatalf("read peer disco keys: %v", err)
	}
	response.Body.Close()

	transport.assertPassive(t)
}

func TestRelayFixtureTransportRepresentsUnsupportedAPI(t *testing.T) {
	transport := &relayFixtureTransport{t: t, relayStatusCode: http.StatusNotFound}
	client := &local.Client{Transport: transport, OmitAuth: true}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://local-tailscaled.sock/localapi/v0/debug-peer-relay-sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DoLocalRequest(request)
	if err != nil {
		t.Fatalf("read unsupported relay endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestRelayFixtureTransportRejectsNonPassiveRoutes(t *testing.T) {
	transport := &relayFixtureTransport{t: t}
	client := &local.Client{Transport: transport, OmitAuth: true}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"http://local-tailscaled.sock/localapi/v0/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DoLocalRequest(request)
	if err != nil {
		t.Fatalf("exercise rejected route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusMethodNotAllowed)
	}
	if fmt.Sprint(transport.unexpected) != "[POST /localapi/v0/ping]" {
		t.Fatalf("unexpected request log = %v", transport.unexpected)
	}
}

type relayFixtureTransport struct {
	t               *testing.T
	relayStatusCode int
	requests        []string
	unexpected      []string
}

func (transport *relayFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Helper()
	requestKey := request.Method + " " + request.URL.RequestURI()
	transport.requests = append(transport.requests, requestKey)

	var payload []byte
	switch requestKey {
	case "GET /localapi/v0/debug-peer-relay-sessions":
		if transport.relayStatusCode != 0 && transport.relayStatusCode != http.StatusOK {
			return fixtureResponse(request, transport.relayStatusCode, []byte("relay API unsupported")), nil
		}
		payload = readRelayFixture(transport.t, "active.json")
	case "POST /localapi/v0/debug?action=peer-disco-keys":
		payload = readRelayFixture(transport.t, "peer-disco-keys.json")
	default:
		transport.unexpected = append(transport.unexpected, requestKey)
		return fixtureResponse(request, http.StatusMethodNotAllowed, []byte("passive fixture rejected request")), nil
	}
	return fixtureResponse(request, http.StatusOK, payload), nil
}

func (transport *relayFixtureTransport) assertPassive(t *testing.T) {
	t.Helper()
	if len(transport.unexpected) != 0 {
		t.Fatalf("unexpected LocalAPI requests: %v", transport.unexpected)
	}
	want := []string{
		"GET /localapi/v0/debug-peer-relay-sessions",
		"POST /localapi/v0/debug?action=peer-disco-keys",
	}
	if fmt.Sprint(transport.requests) != fmt.Sprint(want) {
		t.Fatalf("LocalAPI requests = %v, want %v", transport.requests, want)
	}
}

func fixtureResponse(request *http.Request, statusCode int, payload []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(payload)),
		Request:    request,
	}
}
