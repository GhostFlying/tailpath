package fixtures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

func TestScaleScenarioContract(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	if scenario.NodeCount() != 250 || scenario.EdgeCount() != 1000 {
		t.Fatalf("scale = %d nodes/%d edges, want 250/1000", scenario.NodeCount(), scenario.EdgeCount())
	}

	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	reports := scenario.Reports(at)
	if len(reports) != 750 {
		t.Fatalf("reports = %d, want 750", len(reports))
	}
	pathCounts := map[domain.PathKind]int{}
	edgeObservers := map[string]int{}
	directedTraffic := 0
	for _, timed := range reports {
		if err := timed.Report.Validate(); err != nil {
			t.Fatalf("report %s: %v", timed.Report.ReportID, err)
		}
		if timed.Report.Kind != domain.ReportTrafficSample {
			continue
		}
		observer := timed.Report.Observers[0].Observer.StableNodeID
		for _, peer := range timed.Report.Observers[0].Peers {
			edgeID, _, _ := domain.EdgeID(observer, peer.Peer.StableNodeID)
			edgeObservers[edgeID]++
			pathCounts[peer.Path.Kind]++
			directedTraffic++
		}
	}
	if directedTraffic != 2000 || len(edgeObservers) != 1000 {
		t.Fatalf("traffic = %d directed/%d logical, want 2000/1000", directedTraffic, len(edgeObservers))
	}
	for edgeID, count := range edgeObservers {
		if count != 2 {
			t.Fatalf("edge %s has %d observations, want 2", edgeID, count)
		}
	}
	for _, kind := range []domain.PathKind{domain.PathDirect, domain.PathDERP, domain.PathPeerRelay, domain.PathUnknown} {
		if pathCounts[kind] != 500 {
			t.Fatalf("%s observations = %d, want 500", kind, pathCounts[kind])
		}
	}

	payload, err := json.Marshal(reports)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	const wantDigest = "10a2c9718a947aa4d4bd85cda044da98d0262a20a53a57dac999d4a3fd342e88"
	if got := hex.EncodeToString(digest[:]); got != wantDigest {
		t.Fatalf("digest = %s, want %s", got, wantDigest)
	}
}

func TestScaleScenarioAggregatesExpectedTopology(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	nextID := 0
	aggregator := aggregate.New(aggregate.Options{
		Now: func() time.Time { return at },
		NewNodeID: func() string {
			nextID++
			return scaleUUID(nextID)
		},
	})
	for _, timed := range scenario.Reports(at) {
		result, err := aggregator.ApplyAt(timed.Report, timed.ReceivedAt)
		if err != nil {
			t.Fatalf("apply %s: %v", timed.Report.ReportID, err)
		}
		if !result.Receipt.Accepted || result.Receipt.ResyncRequired {
			t.Fatalf("receipt for %s = %#v", timed.Report.ReportID, result.Receipt)
		}
	}

	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 250 || len(topology.Edges) != 1000 || len(topology.Observers) != 250 {
		t.Fatalf("topology = %d nodes/%d edges/%d observers", len(topology.Nodes), len(topology.Edges), len(topology.Observers))
	}
	stateCounts := map[domain.EdgeState]int{}
	pathCounts := map[domain.PathKind]int{}
	for _, edge := range topology.Edges {
		stateCounts[edge.State]++
		pathCounts[edge.Path.Kind]++
		if len(edge.Observations) != 2 {
			t.Fatalf("edge %s has %d observations, want 2", edge.ID, len(edge.Observations))
		}
	}
	if stateCounts[domain.EdgeActive] != 666 || stateCounts[domain.EdgeRecent] != 334 {
		t.Fatalf("states = %#v, want 666 active/334 recent", stateCounts)
	}
	for _, kind := range []domain.PathKind{domain.PathDirect, domain.PathDERP, domain.PathPeerRelay, domain.PathUnknown} {
		if pathCounts[kind] != 250 {
			t.Fatalf("%s edges = %d, want 250", kind, pathCounts[kind])
		}
	}
	skewed := 0
	for _, observer := range topology.Observers {
		if observer.ClockSkewed {
			skewed++
		}
	}
	if skewed != 9 {
		t.Fatalf("clock-skewed observers = %d, want 9", skewed)
	}
}

func TestScaleScenarioRefreshesBrowserRuntimeAfterSlowLoad(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	now := at.Add(30 * time.Second)
	aggregator := aggregate.New(aggregate.Options{Now: func() time.Time { return now }})
	for _, timed := range scenario.Reports(at) {
		if _, err := aggregator.ApplyAt(timed.Report, timed.ReceivedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := scenario.RefreshRuntime(aggregator, now, 4); err != nil {
		t.Fatal(err)
	}
	states := map[domain.EdgeState]int{}
	for _, edge := range aggregator.Snapshot().Edges {
		states[edge.State]++
	}
	if states[domain.EdgeActive] != 666 || states[domain.EdgeRecent] != 334 {
		t.Fatalf("states after refresh = %#v, want 666 active/334 recent", states)
	}
}

func TestScaleScenarioRejectsInvalidShape(t *testing.T) {
	for _, config := range []ScaleConfig{
		{NodeCount: 2, EdgeCount: 1},
		{NodeCount: 250, EdgeCount: 999},
		{NodeCount: 10, EdgeCount: 50},
	} {
		if _, err := NewScaleScenario(config); err == nil {
			t.Fatalf("NewScaleScenario(%#v) succeeded", config)
		}
	}
}

func TestScaleScenarioSteadyReportsCoverEveryEdgeBilaterally(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	hellos := scenario.HelloReports(at.Add(-time.Second))
	steady := scenario.SteadyReports(at, 2)
	if len(hellos) != 250 || len(steady) != 250 {
		t.Fatalf("reports = %d hello/%d steady, want 250/250", len(hellos), len(steady))
	}

	edgeObservations := map[string]int{}
	for node, report := range steady {
		if report.Sequence != 2 || report.Kind != domain.ReportTrafficSample {
			t.Fatalf("report %d has sequence %d and kind %s", node, report.Sequence, report.Kind)
		}
		if err := report.Validate(); err != nil {
			t.Fatalf("report %d: %v", node, err)
		}
		observer := report.Observers[0].Observer.StableNodeID
		for _, peer := range report.Observers[0].Peers {
			edgeID, _, _ := domain.EdgeID(observer, peer.Peer.StableNodeID)
			edgeObservations[edgeID]++
		}
	}
	if len(edgeObservations) != 1000 {
		t.Fatalf("logical edges = %d, want 1000", len(edgeObservations))
	}
	for edgeID, count := range edgeObservations {
		if count != 2 {
			t.Fatalf("edge %s has %d observations, want 2", edgeID, count)
		}
	}
}

func TestScaleScenarioExporterSnapshotsCoverEveryEdgeBilaterally(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	snapshots := scenario.ExporterSnapshots(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), 2)
	if len(snapshots) != DefaultScaleNodeCount {
		t.Fatalf("snapshots = %d, want %d", len(snapshots), DefaultScaleNodeCount)
	}
	edges := map[string]int{}
	for _, snapshot := range snapshots {
		if !snapshot.Observer.HasIdentity() || len(snapshot.Peers) != 8 {
			t.Fatalf("snapshot = %#v", snapshot)
		}
		for _, peer := range snapshot.Peers {
			edgeID, _, _ := domain.EdgeID(snapshot.Observer.StableNodeID, peer.Identity.StableNodeID)
			edges[edgeID]++
		}
	}
	if len(edges) != DefaultScaleEdgeCount {
		t.Fatalf("logical edges = %d, want %d", len(edges), DefaultScaleEdgeCount)
	}
	for edgeID, count := range edges {
		if count != 2 {
			t.Fatalf("edge %s has %d snapshots, want 2", edgeID, count)
		}
	}
}

func TestScaleScenarioEdgeMutationChangesOneEdgeRate(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	first := scenario.EdgeMutationReport(at, 1_000_001)
	second := scenario.EdgeMutationReport(at.Add(time.Second), 1_000_002)
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(first.Observers) != 1 || len(first.Observers[0].Peers) != 1 {
		t.Fatalf("mutation report = %#v", first)
	}
	if first.Observers[0].Peers[0].TxDelta == second.Observers[0].Peers[0].TxDelta {
		t.Fatal("successive mutation reports have the same visible rate")
	}
}

func TestScaleScenarioKeepsIndependentReporterSequencesAfterEdgeMutation(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	aggregator := aggregate.New(aggregate.Options{Now: func() time.Time { return at }})
	for _, timed := range scenario.Reports(at) {
		if _, err := aggregator.ApplyAt(timed.Report, timed.ReceivedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := scenario.RefreshRuntime(aggregator, at, 4); err != nil {
		t.Fatal(err)
	}
	sequences := make([]int64, scenario.NodeCount())
	for index := range sequences {
		sequences[index] = 4
	}
	observer := scenario.EdgeMutationObserver()
	sequences[observer]++
	mutation := scenario.EdgeMutationReport(at.Add(time.Second), sequences[observer])
	result, err := aggregator.ApplyAt(mutation, at.Add(time.Second))
	if err != nil || !result.Receipt.Accepted || result.Receipt.ResyncRequired {
		t.Fatalf("edge mutation result = %#v, err=%v", result.Receipt, err)
	}
	if err := scenario.RefreshRuntimeSequences(aggregator, at.Add(2*time.Second), sequences, -1); err != nil {
		t.Fatal(err)
	}
}

func TestScaleScenarioAppIngestAndRestart(t *testing.T) {
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	databasePath := filepath.Join(t.TempDir(), "scale.db")
	database, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	options := scaleAggregateOptions(at)
	application, err := app.New(database, options, nil)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := scenario.Load(context.Background(), application, at); err != nil {
		t.Fatal(err)
	}
	t.Logf("scale ingest elapsed=%s", time.Since(started))
	before := application.Aggregator.Snapshot()
	assertScaleTopology(t, before)

	seenPaths := map[domain.PathKind]bool{}
	for _, edge := range before.Edges {
		if seenPaths[edge.Path.Kind] {
			continue
		}
		history, err := database.EdgeHistory(context.Background(), edge.ID, at.Add(-time.Hour))
		if err != nil {
			t.Fatalf("history for %s: %v", edge.ID, err)
		}
		if len(history.Traffic) == 0 || len(history.PathEvents) == 0 {
			t.Fatalf("history for %s is incomplete: %#v", edge.ID, history)
		}
		seenPaths[edge.Path.Kind] = true
	}
	if len(seenPaths) != 4 {
		t.Fatalf("history paths = %#v, want all four kinds", seenPaths)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	restartedDatabase, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedDatabase.Close()
	restarted, err := app.New(restartedDatabase, scaleAggregateOptions(at), nil)
	if err != nil {
		t.Fatal(err)
	}
	after := restarted.Aggregator.Snapshot()
	assertScaleTopology(t, after)
	beforeDigest := topologyDigest(t, before)
	afterDigest := topologyDigest(t, after)
	if beforeDigest != afterDigest {
		t.Fatalf("topology digest changed across restart: before=%s after=%s", beforeDigest, afterDigest)
	}

	if info, err := os.Stat(databasePath); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("scale database bytes=%d", info.Size())
	}
}

func scaleAggregateOptions(at time.Time) aggregate.Options {
	nextID := 0
	return aggregate.Options{
		Now: func() time.Time { return at },
		NewNodeID: func() string {
			nextID++
			return fmt.Sprintf("n_%03d", nextID)
		},
	}
}

func assertScaleTopology(t *testing.T, topology domain.Topology) {
	t.Helper()
	if len(topology.Nodes) != DefaultScaleNodeCount || len(topology.Edges) != DefaultScaleEdgeCount ||
		len(topology.Observers) != DefaultScaleNodeCount {
		t.Fatalf("topology = %d nodes/%d edges/%d observers, want 250/1000/250",
			len(topology.Nodes), len(topology.Edges), len(topology.Observers))
	}
}

func topologyDigest(t *testing.T, topology domain.Topology) string {
	t.Helper()
	for index := range topology.Edges {
		edge := &topology.Edges[index]
		if edge.Path.Kind == domain.PathDirect {
			edge.Path.DirectEndpoint = ""
		}
		sort.Slice(edge.Observations, func(i, j int) bool {
			return edge.Observations[i].ObserverID < edge.Observations[j].ObserverID
		})
		sort.Slice(edge.Conflicts, func(i, j int) bool {
			left, _ := json.Marshal(edge.Conflicts[i])
			right, _ := json.Marshal(edge.Conflicts[j])
			return string(left) < string(right)
		})
	}
	payload, err := json.Marshal(topology)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
