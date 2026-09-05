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
	"github.com/GhostFlying/tailpath/internal/fixtures"
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

func TestFixtureLifecycleRouteIsExplicitAndAuthorized(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/fixture/observer-lifecycle", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("production fixture route status = %d, want 404", recorder.Code)
	}

	called := false
	server = newTestServerWithOptions(t, Options{
		Authorizer: staticAuthorizer{},
		FixtureLifecycle: func(context.Context) (any, error) {
			called = true
			return map[string]any{"state": "withdrawn"}, nil
		},
	})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/fixture/observer-lifecycle", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || !called {
		t.Fatalf("fixture route status = %d called = %t", recorder.Code, called)
	}
}

func TestReadAPIsRequireAuthorization(t *testing.T) {
	server := newTestServer(t, nil)
	for _, path := range []string{"/api/v1/capabilities", "/api/v1/topology", "/api/v1/devices", "/api/v1/events", "/api/v1/history/edges/edge"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, recorder.Code)
		}
	}
}

func TestCapabilitiesAdvertiseImplementedProtocolFeatures(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var capabilities domain.ServerCapabilities
	if err := json.NewDecoder(recorder.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if !capabilities.SupportsProtocol(domain.ProtocolVersion) ||
		!capabilities.SupportsFeature(domain.FeatureMultiObserver) ||
		!capabilities.SupportsFeature(domain.FeatureObserverWithdrawal) {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}
	if capabilities.SupportsFeature(domain.FeatureDeviceDirectory) {
		t.Fatalf("disabled directory advertised: %#v", capabilities)
	}

	server = newTestServerWithOptions(t, Options{Authorizer: staticAuthorizer{}, DeviceDirectoryEnabled: true})
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	server.Handler().ServeHTTP(recorder, request)
	if err := json.NewDecoder(recorder.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if !capabilities.SupportsFeature(domain.FeatureDeviceDirectory) {
		t.Fatalf("enabled directory missing from capabilities: %#v", capabilities)
	}
}

func TestDevicesAPIEncodesDisabledAndEmptyCollections(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("devices status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q, want no-store", got)
	}
	if got := recorder.Body.String(); got != "{\"sync\":{\"status\":\"disabled\",\"invalidAddressCount\":0},\"devices\":[]}\n" {
		t.Fatalf("disabled devices response = %s", got)
	}
	if err := server.app.UpdateDirectorySyncState(context.Background(), domain.DirectorySyncState{
		Status: domain.DirectorySyncSyncing,
	}); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil))
	if got := recorder.Body.String(); got != "{\"sync\":{\"status\":\"syncing\",\"invalidAddressCount\":0},\"devices\":[]}\n" {
		t.Fatalf("syncing devices response = %s", got)
	}
}

func TestDevicesAPIExposesDirectoryAndRuntimeAsSeparateDimensions(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	server := newTestServerWithOptions(t, Options{Authorizer: staticAuthorizer{}, DeviceDirectoryEnabled: true})
	_, err := server.app.SubmitAt(context.Background(), domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{
				StableNodeID: "stable-runtime", NodeKey: "nodekey:private-runtime",
				DNSName: "runtime.example.ts.net", Hostname: "runtime-host", OS: "linux",
				TailscaleIPs: []string{"100.64.0.1"},
			},
			InventoryGeneration: "inventory",
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	lastSeen := now.Add(-time.Hour)
	directoryAt := now.Add(time.Minute)
	_, err = server.app.ApplyDirectorySnapshotAt(context.Background(), domain.DirectorySnapshot{
		CollectedAt: directoryAt,
		Devices: []domain.DirectoryDevice{
			{
				StableNodeID: "stable-runtime", NodeKey: "nodekey:private-directory",
				DNSName: "catalog.example.ts.net", Hostname: "catalog-host", OS: "macos",
				TailscaleIPs: []string{"fd7a:115c:a1e0::1", "100.64.0.2"}, Tags: []string{"tag:dev"},
				ConnectedToControl: true, LastSeen: &lastSeen,
			},
			{
				StableNodeID: "stable-directory-only", DNSName: "alpha.example.ts.net",
				TailscaleIPs: []string{}, Tags: []string{}, LastSeen: &lastSeen,
			},
		},
	}, healthyDirectoryState(directoryAt), directoryAt)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("devices status = %d: %s", recorder.Code, recorder.Body.String())
	}
	payload := recorder.Body.Bytes()
	if bytes.Contains(payload, []byte("nodekey:")) {
		t.Fatalf("devices response leaked identity evidence: %s", payload)
	}
	var raw struct {
		Devices []map[string]any `json:"devices"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	for _, device := range raw.Devices {
		if device["stableNodeId"] == "stable-runtime" {
			if _, exists := device["lastSeen"]; exists {
				t.Fatalf("control-connected device exposed lastSeen: %#v", device)
			}
		}
	}
	var response deviceDirectoryResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if response.Sync.Status != domain.DirectorySyncHealthy || len(response.Devices) != 2 {
		t.Fatalf("devices response = %#v", response)
	}
	if response.Devices[0].StableNodeID != "stable-directory-only" || response.Devices[1].StableNodeID != "stable-runtime" {
		t.Fatalf("device sort = %#v", response.Devices)
	}
	item := response.Devices[1]
	if item.Runtime == nil || item.Runtime.Platform != "linux" || !item.Runtime.Observable || !item.Runtime.Online ||
		item.Platform != "macos" || item.LastSeen != nil || len(item.Conflicts) != 4 {
		t.Fatalf("directory/runtime dimensions = %#v", item)
	}

	topologyRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(topologyRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil))
	if bytes.Contains(topologyRecorder.Body.Bytes(), []byte("nodekey:private-directory")) {
		t.Fatalf("topology directory enrichment leaked NodeKey: %s", topologyRecorder.Body.String())
	}
	var topology domain.Topology
	if err := json.NewDecoder(topologyRecorder.Body).Decode(&topology); err != nil {
		t.Fatal(err)
	}
	if len(topology.Nodes) != 1 || topology.Nodes[0].DNSName != "catalog.example.ts.net" || topology.Nodes[0].Directory == nil {
		t.Fatalf("enriched topology = %#v", topology)
	}
}

func TestDevicesAPIKeepsLastGoodWhenSyncIsStale(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	server := newTestServerWithOptions(t, Options{Authorizer: staticAuthorizer{}, DeviceDirectoryEnabled: true})
	_, err := server.app.ApplyDirectorySnapshotAt(context.Background(), domain.DirectorySnapshot{
		CollectedAt: now,
		Devices:     []domain.DirectoryDevice{{StableNodeID: "stable-device", Hostname: "catalog-device"}},
	}, healthyDirectoryState(now), now)
	if err != nil {
		t.Fatal(err)
	}
	attempt := now.Add(5 * time.Minute)
	retry := attempt.Add(30 * time.Second)
	if err := server.app.UpdateDirectorySyncStateAt(context.Background(), domain.DirectorySyncState{
		Status: domain.DirectorySyncStale, LastAttemptAt: &attempt, LastSuccessAt: &now,
		NextRetryAt: &retry, ErrorCode: domain.DirectoryErrorRateLimited,
	}, attempt); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil))
	var response deviceDirectoryResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Sync.Status != domain.DirectorySyncStale || response.Sync.ErrorCode != domain.DirectoryErrorRateLimited ||
		len(response.Devices) != 1 || response.Devices[0].Hostname != "catalog-device" {
		t.Fatalf("stale directory = %#v", response)
	}
}

func TestDevicesAPIReturnsFull250DeviceFixtureWithoutChangingLive(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	server := newTestServerWithOptions(t, Options{Authorizer: staticAuthorizer{}, DeviceDirectoryEnabled: true})
	if _, err := server.app.ApplyDirectorySnapshotAt(
		context.Background(), fixtures.DeviceDirectorySnapshot(now, fixtures.DefaultDirectoryDeviceCount),
		healthyDirectoryState(now), now,
	); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil))
	var response deviceDirectoryResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Devices) != fixtures.DefaultDirectoryDeviceCount {
		t.Fatalf("fixture devices = %d", len(response.Devices))
	}
	if topology := server.app.Aggregator.Snapshot(); len(topology.Nodes) != 0 || len(topology.Edges) != 0 {
		t.Fatalf("directory fixture entered Live: nodes=%d edges=%d", len(topology.Nodes), len(topology.Edges))
	}
}

func healthyDirectoryState(at time.Time) domain.DirectorySyncState {
	return domain.DirectorySyncState{Status: domain.DirectorySyncHealthy, LastAttemptAt: &at, LastSuccessAt: &at}
}

func TestLegacySingleObserverReportIngestsWithoutCapabilityNegotiation(t *testing.T) {
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
			Source:    domain.RelaySessionClient{SessionClientID: "left", Identity: identityPtr(domain.NodeIdentity{StableNodeID: "a", Hostname: "A"})},
			Target:    domain.RelaySessionClient{SessionClientID: "right", Identity: identityPtr(domain.NodeIdentity{StableNodeID: "b", Hostname: "B"})},
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

func identityPtr(value domain.NodeIdentity) *domain.NodeIdentity {
	return &value
}

func TestHistoryAPIsValidateQueriesAndDistinguishKnownEmpty(t *testing.T) {
	server := newTestServer(t, staticAuthorizer{})
	now := time.Now().UTC()
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "Alpha"}, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "Beta"}, LastEvidenceAt: now},
		{ID: "n_c", NodeIdentity: domain.NodeIdentity{StableNodeID: "c", Hostname: "Charlie"}, LastEvidenceAt: now},
		{ID: "n_control", NodeIdentity: domain.NodeIdentity{StableNodeID: "control", Hostname: "Tailpath"}, LastEvidenceAt: now},
	}}
	if err := server.app.Store.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	recordHistoryTraffic(t, server, "recent", "n_a--n_b", "n_a", "n_b", now.Add(-time.Minute))
	recordHistoryTraffic(t, server, "anchor-only", "n_a--n_c", "n_a", "n_c", now.Add(-2*time.Hour))
	recordHistoryTraffic(t, server, "old", "n_b--n_c", "n_b", "n_c", now.Add(-8*24*time.Hour))
	recordHistoryTraffic(t, server, "system", "n_a--n_control", "n_a", "n_control", now.Add(-30*time.Second))
	metadata.ControlNodeIDs = []string{"n_control"}
	if err := server.app.Store.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}

	for _, requestPath := range []string{
		"/api/v1/history/nodes",
		"/api/v1/history/edges?window=bad",
		"/api/v1/history/edges?window=1h&limit=101",
		"/api/v1/history/edges?window=1h&path=invalid",
		"/api/v1/history/edges?window=1h&cursor=invalid",
		"/api/v1/history/edges?window=1h&includeSystemTelemetry=maybe",
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
	if err := json.NewDecoder(recorder.Body).Decode(&history); err != nil || history.EdgeID != "n_a--n_b" || len(history.Traffic) != 1 || history.LastTrafficAt == nil || !history.LastTrafficAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("recent history = %#v, err=%v", history, err)
	}
	if history.PathEvents == nil || len(history.PathEvents) != 1 || history.PathEvents[0].Observations == nil {
		t.Fatalf("recent history collections = %#v", history)
	}
	if history.RelatedNodes == nil || history.PathEvents[0].Conflicts == nil {
		t.Fatalf("required history collections = %#v", history)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_a--n_c?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("anchor-only status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var rawHistory map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&rawHistory); err != nil {
		t.Fatal(err)
	}
	pathEvents, pathEventsOK := rawHistory["pathEvents"].([]any)
	traffic, trafficOK := rawHistory["traffic"].([]any)
	pathAnchor, anchorOK := rawHistory["pathAnchor"].(map[string]any)
	observations, observationsOK := pathAnchor["observations"].([]any)
	if !pathEventsOK || len(pathEvents) != 0 || !trafficOK || len(traffic) != 0 || !anchorOK || !observationsOK || len(observations) != 0 {
		t.Fatalf("anchor-only JSON collections = %#v", rawHistory)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_b--n_c?window=7d", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("known empty status = %d: %s", recorder.Code, recorder.Body.String())
	}
	history = domain.EdgeHistory{}
	if err := json.NewDecoder(recorder.Body).Decode(&history); err != nil || len(history.Traffic) != 0 || history.LastTrafficAt != nil {
		t.Fatalf("known empty history = %#v, err=%v", history, err)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/missing?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown detail status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_a--n_control?window=1h", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("default system detail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/history/edges/n_a--n_control?window=1h&includeSystemTelemetry=true", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("diagnostic system detail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.NewDecoder(recorder.Body).Decode(&history); err != nil || !history.SystemTelemetry {
		t.Fatalf("diagnostic system history = %#v, err=%v", history, err)
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

func TestCoalesceInvalidationsEmitsLeadingEventAndOneRateLimitedFollowUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan struct{}, 100)
	interval := 50 * time.Millisecond
	output := coalesceInvalidations(ctx, input, interval)
	started := time.Now()
	input <- struct{}{}
	select {
	case <-output:
		if elapsed := time.Since(started); elapsed >= interval {
			t.Fatalf("leading invalidation took %s", elapsed)
		}
	case <-time.After(interval):
		t.Fatal("leading topology invalidation was delayed by a full window")
	}
	for range 99 {
		input <- struct{}{}
	}
	select {
	case <-output:
		t.Fatal("burst follow-up was not rate limited")
	case <-time.After(interval / 2):
	}
	select {
	case <-output:
	case <-time.After(2 * interval):
		t.Fatal("burst did not produce one trailing invalidation")
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
