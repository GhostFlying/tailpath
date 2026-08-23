package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

func TestReportIngestRequiresAuthorization(t *testing.T) {
	server := newTestServer(t, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader([]byte("{}")))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestReadAPIsRequireAuthorization(t *testing.T) {
	server := newTestServer(t, nil)
	for _, path := range []string{"/api/v1/topology", "/api/v1/events", "/api/v1/history/edges/edge"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, recorder.Code)
		}
	}
}

func TestReportIngestAndTopology(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: time.Now(), Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "node", Hostname: "Node"}, InventoryGeneration: "inventory",
		}},
	}
	body, _ := json.Marshal(report)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	request.RemoteAddr = "100.64.0.1:1234"
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("ingest status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("topology status = %d", recorder.Code)
	}
	var topology domain.Topology
	if err := json.NewDecoder(recorder.Body).Decode(&topology); err != nil {
		t.Fatal(err)
	}
	if len(topology.Nodes) != 1 || topology.Nodes[0].Hostname != "Node" {
		t.Fatalf("unexpected topology: %#v", topology)
	}
}

func TestRelaySessionIngestPreservesThirdPartyProvenance(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	at := time.Now().UTC()
	report := domain.ReportEnvelope{
		Version:  domain.ProtocolVersion,
		ReportID: "00000000-0000-4000-8000-000000000001", ReporterInstanceID: "00000000-0000-4000-8000-000000000002",
		Sequence: 1, CollectedAt: at, Kind: domain.ReportRelaySessionUpdate,
		RelaySessions: []domain.RelaySessionObservation{{
			Relay:     domain.NodeIdentity{StableNodeID: "relay", Hostname: "Relay-HZ"},
			Source:    domain.NodeIdentity{StableNodeID: "a", Hostname: "A"},
			Target:    domain.NodeIdentity{StableNodeID: "b", Hostname: "B"},
			SessionID: "session", VNI: 7,
			SourceToTargetBytes: 1200, TargetToSourceBytes: 400,
			SourceToTargetDelta: 1200, TargetToSourceDelta: 400,
			SampleDurationMS: 2000, LastActive: at,
		}},
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	request.RemoteAddr = "100.64.0.1:1234"
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("relay ingest status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("topology status = %d", recorder.Code)
	}
	var topology domain.Topology
	if err := json.NewDecoder(recorder.Body).Decode(&topology); err != nil {
		t.Fatal(err)
	}
	if len(topology.Edges) != 1 || topology.Edges[0].Path.Kind != domain.PathPeerRelay || len(topology.Edges[0].Observations) != 1 {
		t.Fatalf("relay topology = %#v", topology)
	}
	var relayID string
	for _, node := range topology.Nodes {
		if node.StableNodeID == "relay" {
			relayID = node.ID
		}
	}
	if relayID == "" || topology.Edges[0].Observations[0].ObserverID != relayID {
		t.Fatalf("relay provenance = %#v, relay ID = %q", topology.Edges[0].Observations, relayID)
	}
}

func TestCoalesceInvalidationsKeepsOneEventPerWindowAndAFollowUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan struct{}, 100)
	output := coalesceInvalidations(ctx, input, 20*time.Millisecond)
	for range 100 {
		input <- struct{}{}
	}
	select {
	case <-output:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("burst did not produce a topology invalidation")
	}
	select {
	case <-output:
		t.Fatal("one burst produced more than one invalidation window")
	case <-time.After(30 * time.Millisecond):
	}
	input <- struct{}{}
	select {
	case <-output:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("later burst did not produce a follow-up invalidation")
	}
	cancel()
	select {
	case _, ok := <-output:
		if ok {
			t.Fatal("coalescer emitted after cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("coalescer did not close after cancellation")
	}
}

func newTestServer(t *testing.T, authorizer Authorizer) *Server {
	t.Helper()
	database, err := store.Open(":memory:", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	application, err := app.New(database, aggregate.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(application, Options{Authorizer: authorizer})
}

type staticAuthorizer struct{}

func (staticAuthorizer) Authorize(context.Context, string) (string, error) {
	return "trusted", nil
}
