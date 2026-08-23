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

func TestFixtureMutationRouteIsExplicitAndAuthorized(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/fixture/edge-update", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("production fixture route status = %d, want 404", recorder.Code)
	}

	called := false
	server = newTestServerWithOptions(t, Options{
		Authorizer: staticAuthorizer{},
		FixtureMutation: func(context.Context) (any, error) {
			called = true
			return map[string]any{"sequence": 4}, nil
		},
	})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/fixture/edge-update", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || !called {
		t.Fatalf("fixture route status = %d called = %t", recorder.Code, called)
	}

	server = newTestServerWithOptions(t, Options{FixtureMutation: func(context.Context) (any, error) {
		return nil, nil
	}})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/fixture/edge-update", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized fixture route status = %d, want 401", recorder.Code)
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

func TestHistoryAPIsValidateQueriesAndDistinguishKnownEmpty(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	now := time.Now().UTC()
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "Alpha"}, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "Beta"}, LastEvidenceAt: now},
		{ID: "n_c", NodeIdentity: domain.NodeIdentity{StableNodeID: "c", Hostname: "Charlie"}, LastEvidenceAt: now},
	}}
	if err := server.app.Store.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	recordHistoryTraffic(t, server, "recent", "n_a--n_b", "n_a", "n_b", now.Add(-time.Minute))
	recordHistoryTraffic(t, server, "old", "n_b--n_c", "n_b", "n_c", now.Add(-8*24*time.Hour))

	for _, requestPath := range []string{
		"/api/v1/history/nodes",
		"/api/v1/history/edges?window=bad",
		"/api/v1/history/edges?window=1h&limit=101",
		"/api/v1/history/edges?window=1h&path=invalid",
		"/api/v1/history/edges?window=1h&cursor=invalid",
		"/api/v1/history/edges/n_a--n_b",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400: %s", requestPath, recorder.Code, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/history/nodes?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history nodes status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var nodes domain.HistoryNodes
	if err := json.NewDecoder(recorder.Body).Decode(&nodes); err != nil || len(nodes.Nodes) != 2 {
		t.Fatalf("history nodes = %#v, err=%v", nodes, err)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges?window=1h&path=direct&limit=1", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history edges status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var page domain.HistoryEdgePage
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil || len(page.Edges) != 1 {
		t.Fatalf("history page = %#v, err=%v", page, err)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_a--n_b?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recent detail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var history domain.EdgeHistory
	if err := json.NewDecoder(recorder.Body).Decode(&history); err != nil || history.EdgeID != "n_a--n_b" || len(history.Traffic) != 1 {
		t.Fatalf("recent history = %#v, err=%v", history, err)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_b--n_c?window=7d", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("known empty status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.NewDecoder(recorder.Body).Decode(&history); err != nil || len(history.Traffic) != 0 {
		t.Fatalf("known empty history = %#v, err=%v", history, err)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/missing?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown detail status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHistoryRequestCancellationIsNotAnInternalServerError(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	for _, path := range []string{
		"/api/v1/history/nodes?window=1h",
		"/api/v1/history/edges?window=1h",
		"/api/v1/history/edges/edge?window=1h",
	} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code == http.StatusInternalServerError {
			t.Errorf("GET %s reported client cancellation as 500", path)
		}
	}
}

func recordHistoryTraffic(t *testing.T, server *Server, reportID, edgeID, sourceID, targetID string, at time.Time) {
	t.Helper()
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: reportID, ReporterInstanceID: "history-fixture",
		Sequence: at.UnixNano(), CollectedAt: at, Kind: domain.ReportTrafficSample,
	}
	traffic := []domain.AcceptedTraffic{{
		EdgeID: edgeID, SourceID: sourceID, TargetID: targetID, ObserverID: sourceID,
		AToBBytes: 10, BToABytes: 2, ReceivedAt: at,
	}}
	transition := []domain.PathTransition{{EdgeID: edgeID, ObservedAt: at, Path: domain.PathObservation{Kind: domain.PathDirect}}}
	if _, err := server.app.Store.Record(context.Background(), report, at, nil, traffic, transition); err != nil {
		t.Fatal(err)
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
	return newTestServerWithOptions(t, Options{Authorizer: authorizer})
}

func newTestServerWithOptions(t *testing.T, options Options) *Server {
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
	return New(application, options)
}

type staticAuthorizer struct{}

func (staticAuthorizer) Authorize(context.Context, string) (string, error) {
	return "trusted", nil
}
