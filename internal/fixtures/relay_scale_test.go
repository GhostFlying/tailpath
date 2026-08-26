package fixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

func TestRelayScaleScenarioContract(t *testing.T) {
	scenario, err := NewRelayScaleScenario(DefaultRelayScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	if scenario.NodeCount() != 250 || scenario.SessionCount() != 1000 || scenario.RelayCount() != 8 {
		t.Fatalf("relay scale = %d nodes/%d sessions/%d relays, want 250/1000/8",
			scenario.NodeCount(), scenario.SessionCount(), scenario.RelayCount())
	}

	reports := scenario.Reports(time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), 1)
	if len(reports) != 8 {
		t.Fatalf("reports = %d, want 8", len(reports))
	}
	seenSessions := make(map[string]struct{}, scenario.SessionCount())
	for _, timed := range reports {
		if err := timed.Report.Validate(); err != nil {
			t.Fatalf("report %s: %v", timed.Report.ReportID, err)
		}
		for _, session := range timed.Report.RelaySessions {
			if _, duplicate := seenSessions[session.SessionID]; duplicate {
				t.Fatalf("duplicate session %q", session.SessionID)
			}
			seenSessions[session.SessionID] = struct{}{}
		}
	}
	if len(seenSessions) != scenario.SessionCount() {
		t.Fatalf("sessions = %d, want %d", len(seenSessions), scenario.SessionCount())
	}
}

func TestRelayScaleScenarioAggregatesThirdPartyEdges(t *testing.T) {
	scenario, err := NewRelayScaleScenario(DefaultRelayScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	aggregator := aggregate.New(aggregate.Options{Now: func() time.Time { return at }})
	for _, timed := range scenario.Reports(at, 1) {
		result, err := aggregator.ApplyAt(timed.Report, timed.ReceivedAt)
		if err != nil {
			t.Fatalf("apply %s: %v", timed.Report.ReportID, err)
		}
		if !result.Receipt.Accepted || result.Receipt.ResyncRequired {
			t.Fatalf("receipt for %s = %#v", timed.Report.ReportID, result.Receipt)
		}
	}

	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 250 || len(topology.Edges) != 1000 || len(topology.Observers) != 8 {
		t.Fatalf("topology = %d nodes/%d edges/%d observers, want 250/1000/8",
			len(topology.Nodes), len(topology.Edges), len(topology.Observers))
	}
	for _, edge := range topology.Edges {
		if edge.Path.Kind != domain.PathPeerRelay || edge.Path.PeerRelayVNI == nil {
			t.Fatalf("edge %s path = %#v", edge.ID, edge.Path)
		}
		if len(edge.Observations) != 1 || edge.Observations[0].RelaySession == nil {
			t.Fatalf("edge %s observations = %#v", edge.ID, edge.Observations)
		}
		if edge.AToBBytesPerSecond <= 0 && edge.BToABytesPerSecond <= 0 {
			t.Fatalf("edge %s has no traffic rate", edge.ID)
		}
	}
	payload, err := json.Marshal(topology)
	if err != nil {
		t.Fatal(err)
	}
	if containsRelayScaleCanary(payload) {
		t.Fatal("topology exposed relay underlay endpoint or disco canary")
	}
}

func TestRelayScaleScenarioRejectsInvalidShape(t *testing.T) {
	for _, config := range []RelayScaleConfig{
		{NodeCount: 3, SessionCount: 1, RelayCount: 2},
		{NodeCount: 10, SessionCount: 1, RelayCount: 0},
		{NodeCount: 10, SessionCount: 29, RelayCount: 2},
	} {
		if _, err := NewRelayScaleScenario(config); err == nil {
			t.Fatalf("NewRelayScaleScenario(%#v) succeeded", config)
		}
	}
}

func TestRelayScaleScenarioPersistsSanitizedHistoryAcrossRestart(t *testing.T) {
	scenario, err := NewRelayScaleScenario(DefaultRelayScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	databasePath := filepath.Join(t.TempDir(), "relay-scale.db")
	database, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	application, err := app.New(database, scaleAggregateOptions(at), logger)
	if err != nil {
		t.Fatal(err)
	}

	for _, timed := range scenario.Reports(at, 1) {
		receipt, err := application.SubmitAt(context.Background(), timed.Report, timed.ReceivedAt)
		if err != nil {
			t.Fatalf("submit %s: %v", timed.Report.ReportID, err)
		}
		if !receipt.Accepted || receipt.ResyncRequired {
			t.Fatalf("receipt for %s = %#v", timed.Report.ReportID, receipt)
		}
	}
	before := application.Aggregator.Snapshot()
	assertRelayScaleTopology(t, before)
	assertRelayScaleTrafficIsNotSummed(t, scenario, before)
	assertRelayScaleExportSanitized(t, database, before, at)
	assertRelayScaleFilesSanitized(t, databasePath)
	if containsRelayScaleCanary(logs.Bytes()) {
		t.Fatal("successful relay ingest logged endpoint or disco canary")
	}
	beforeDigest := topologyDigest(t, before)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	restartedDatabase, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedDatabase.Close()
	restarted, err := app.New(restartedDatabase, scaleAggregateOptions(at), logger)
	if err != nil {
		t.Fatal(err)
	}
	after := restarted.Aggregator.Snapshot()
	assertRelayScaleTopology(t, after)
	if afterDigest := topologyDigest(t, after); afterDigest != beforeDigest {
		t.Fatalf("topology digest changed across restart: before=%s after=%s", beforeDigest, afterDigest)
	}
	assertRelayScaleExportSanitized(t, restartedDatabase, after, at)
	assertRelayScaleFilesSanitized(t, databasePath)
}

func assertRelayScaleTopology(t *testing.T, topology domain.Topology) {
	t.Helper()
	if len(topology.Nodes) != 250 || len(topology.Edges) != 1000 || len(topology.Observers) != 8 {
		t.Fatalf("topology = %d nodes/%d edges/%d observers, want 250/1000/8",
			len(topology.Nodes), len(topology.Edges), len(topology.Observers))
	}
}

func assertRelayScaleTrafficIsNotSummed(t *testing.T, scenario *RelayScaleScenario, topology domain.Topology) {
	t.Helper()
	var expectedBytes int64
	for _, session := range scenario.sessions {
		expectedBytes += session.sourceToTarget + session.targetToSource
	}
	var actualBytesPerSecond float64
	for _, edge := range topology.Edges {
		actualBytesPerSecond += edge.AToBBytesPerSecond + edge.BToABytesPerSecond
	}
	if actualBytes := int64(actualBytesPerSecond * 2); actualBytes != expectedBytes {
		t.Fatalf("logical traffic bytes = %d, want relay counter total %d", actualBytes, expectedBytes)
	}
}

func assertRelayScaleExportSanitized(t *testing.T, database *store.SQLite, topology domain.Topology, at time.Time) {
	t.Helper()
	payload, err := json.Marshal(topology)
	if err != nil {
		t.Fatal(err)
	}
	if containsRelayScaleCanary(payload) {
		t.Fatal("topology export contains endpoint or disco canary")
	}
	history, err := database.EdgeHistory(context.Background(), topology.Edges[0].ID, at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Traffic) == 0 || len(history.PathEvents) == 0 ||
		history.PathEvents[0].Path.Kind != domain.PathPeerRelay {
		t.Fatalf("relay history is incomplete: %#v", history)
	}
	payload, err = json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if containsRelayScaleCanary(payload) {
		t.Fatal("History export contains endpoint or disco canary")
	}
}

func assertRelayScaleFilesSanitized(t *testing.T, databasePath string) {
	t.Helper()
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		payload, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		if containsRelayScaleCanary(payload) {
			t.Fatalf("%s contains endpoint or disco canary", filepath.Base(path))
		}
	}
}

func containsRelayScaleCanary(payload []byte) bool {
	return bytes.Contains(payload, []byte(RelayScaleEndpointCanary)) ||
		bytes.Contains(payload, []byte("2001:db8::")) ||
		bytes.Contains(payload, []byte(RelayScaleDiscoCanary))
}
