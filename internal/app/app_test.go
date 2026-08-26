package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

func TestRuntimeStateSurvivesRawReportCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailpath.db")
	retention := 7 * 24 * time.Hour
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	database, err := store.Open(path, retention)
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(database, testOptions(func() time.Time { return now }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SubmitAt(context.Background(), helloReport(now), now); err != nil {
		t.Fatal(err)
	}

	now = now.Add(8 * 24 * time.Hour)
	if _, err := application.SubmitAt(context.Background(), heartbeatReport(now), now); err != nil {
		t.Fatal(err)
	}
	if err := application.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	reports, err := database.RestoreReports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 0 {
		t.Fatalf("checkpoint-covered reports remain: %#v", reports)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = store.Open(path, retention)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	application, err = New(database, testOptions(func() time.Time { return now }), nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := application.SubmitAt(context.Background(), trafficReport(now.Add(time.Second)), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Accepted || receipt.ResyncRequired {
		t.Fatalf("restored inventory receipt = %#v", receipt)
	}
	if got := len(application.Aggregator.Snapshot().Edges); got != 1 {
		t.Fatalf("restored topology has %d edges, want 1", got)
	}
}

func TestRestartReplaysReportsAfterCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailpath.db")
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	database, err := store.Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(database, testOptions(func() time.Time { return now }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SubmitAt(context.Background(), helloReport(now), now); err != nil {
		t.Fatal(err)
	}
	journaledAt := now.Add(500 * time.Millisecond)
	journaled := trafficReport(journaledAt)
	journaled.ReportID = "traffic-after-checkpoint"
	journaled.Sequence = 2
	if _, err := application.SubmitAt(context.Background(), journaled, journaledAt); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	later, err := database.RestoreReportsAfter(context.Background(), checkpoint.LastReportRowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(later) != 1 || later[0].Report.ReportID != journaled.ReportID {
		t.Fatalf("reports after checkpoint = %#v", later)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = store.Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	restarted, err := New(database, testOptions(func() time.Time { return journaledAt }), nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err = database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	later, err = database.RestoreReportsAfter(context.Background(), checkpoint.LastReportRowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(later) != 0 {
		t.Fatalf("replayed reports were not checkpointed: %#v", later)
	}
	if got := len(restarted.Aggregator.Snapshot().Edges); got != 1 {
		t.Fatalf("restarted topology has %d edges, want journaled edge", got)
	}
	next := heartbeatReport(now.Add(time.Second))
	next.ReportID = "heartbeat-after-replay"
	next.Sequence = 3
	receipt, err := restarted.SubmitAt(context.Background(), next, now.Add(time.Second))
	if err != nil || !receipt.Accepted || receipt.ResyncRequired {
		t.Fatalf("post-restart receipt = %#v, err=%v", receipt, err)
	}
}

func TestCanonicalAllocationForcesCheckpointBeforeJournalReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailpath.db")
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := now
	options := aggregate.Options{Now: func() time.Time { return current }}
	database, err := store.Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	hello := helloReport(now)
	hello.Observers[0].Peers = nil
	application, err := New(database, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SubmitAt(context.Background(), hello, now); err != nil {
		t.Fatal(err)
	}
	firstCheckpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	inventoryAt := now.Add(400 * time.Millisecond)
	inventory := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "inventory-new-peer", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: inventoryAt, Kind: domain.ReportInventoryUpdate,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory-2",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}}},
		}},
	}
	if _, err := application.SubmitAt(context.Background(), inventory, inventoryAt); err != nil {
		t.Fatal(err)
	}
	allocationCheckpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if allocationCheckpoint.LastReportRowID <= firstCheckpoint.LastReportRowID {
		t.Fatalf("canonical allocation remained journal-only: first=%#v allocation=%#v", firstCheckpoint, allocationCheckpoint)
	}

	trafficAt := now.Add(800 * time.Millisecond)
	traffic := trafficReport(trafficAt)
	traffic.ReportID = "traffic-after-allocation"
	traffic.Sequence = 3
	traffic.Observers[0].InventoryGeneration = "inventory-2"
	current = trafficAt
	if _, err := application.SubmitAt(context.Background(), traffic, trafficAt); err != nil {
		t.Fatal(err)
	}
	stillAllocation, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stillAllocation.LastReportRowID != allocationCheckpoint.LastReportRowID {
		t.Fatalf("ordinary sub-second traffic unexpectedly checkpointed: allocation=%#v traffic=%#v", allocationCheckpoint, stillAllocation)
	}
	before := application.Aggregator.Snapshot()
	if len(before.Edges) != 1 {
		t.Fatalf("topology before restart = %#v", before.Edges)
	}
	journaled, err := database.RestoreReportsAfter(context.Background(), allocationCheckpoint.LastReportRowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(journaled) != 1 || journaled[0].Report.ReportID != traffic.ReportID {
		t.Fatalf("journal after allocation checkpoint = %#v", journaled)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = store.Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	restarted, err := New(database, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	after := restarted.Aggregator.Snapshot()
	if len(after.Edges) != 1 || after.Edges[0].ID != before.Edges[0].ID ||
		after.Edges[0].Source != before.Edges[0].Source || after.Edges[0].Target != before.Edges[0].Target {
		t.Fatalf("canonical edge changed across replay: before=%#v after=%#v", before.Edges, after.Edges)
	}
	for stableID, beforeID := range topologyIDsByStableNodeID(before) {
		if got := topologyIDsByStableNodeID(after)[stableID]; got != beforeID {
			t.Fatalf("canonical node %q changed across replay: before=%q after=%q", stableID, beforeID, got)
		}
	}
	replayedCheckpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	journaled, err = database.RestoreReportsAfter(context.Background(), replayedCheckpoint.LastReportRowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(journaled) != 0 {
		t.Fatalf("startup replay was not checkpointed: %#v", journaled)
	}
}

func TestRelayHistorySurvivesRestartScopedMergeAndRollup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailpath.db")
	current := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	nextNode := 0
	options := aggregate.Options{
		Now: func() time.Time { return current },
		NewNodeID: func() string {
			nextNode++
			return fmt.Sprintf("n_%03d", nextNode)
		},
	}
	open := func() (*store.SQLite, *App) {
		t.Helper()
		database, err := store.Open(path, 7*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		application, err := New(database, options, nil)
		if err != nil {
			database.Close()
			t.Fatal(err)
		}
		return database, application
	}

	database, application := open()
	if _, err := application.SubmitAt(context.Background(), relayTrafficReport(current), current); err != nil {
		t.Fatal(err)
	}
	before := application.Aggregator.Snapshot()
	if len(before.Edges) != 1 {
		t.Fatalf("initial relay topology = %#v", before)
	}
	aID := topologyIDsByStableNodeID(before)["a"]
	placeholderID := before.Edges[0].Source
	if placeholderID == aID {
		placeholderID = before.Edges[0].Target
	}
	oldEdgeID := before.Edges[0].ID
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, application = open()
	restarted := application.Aggregator.Snapshot()
	if len(restarted.Edges) != 1 || restarted.Edges[0].ID != oldEdgeID {
		t.Fatalf("relay topology changed across checkpoint restart: %#v", restarted)
	}

	current = current.Add(time.Minute)
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "endpoint-hello", ReporterInstanceID: "endpoint-reporter", Sequence: 1,
		CollectedAt: current, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, InventoryGeneration: "endpoint-inventory",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}}},
		}},
	}
	if _, err := application.SubmitAt(context.Background(), hello, current); err != nil {
		t.Fatal(err)
	}
	vni := int64(7)
	sample := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "endpoint-traffic", ReporterInstanceID: "endpoint-reporter", Sequence: 2,
		CollectedAt: current, Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, InventoryGeneration: "endpoint-inventory",
			Peers: []domain.PeerObservation{{
				Peer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, TxDelta: 30, RxDelta: 10,
				SampleDurationMS: 2000, LastActive: current,
				Path: domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni},
			}},
		}},
	}
	if _, err := application.SubmitAt(context.Background(), sample, current); err != nil {
		t.Fatal(err)
	}
	afterMerge := application.Aggregator.Snapshot()
	bID := topologyIDsByStableNodeID(afterMerge)["b"]
	canonicalEdgeID, _, _ := domain.EdgeID(aID, bID)
	if len(afterMerge.Edges) != 1 || afterMerge.Edges[0].ID != canonicalEdgeID {
		t.Fatalf("scoped merge topology = %#v", afterMerge)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, application = open()
	defer database.Close()
	if topology := application.Aggregator.Snapshot(); len(topology.Edges) != 1 || topology.Edges[0].ID != canonicalEdgeID {
		t.Fatalf("merged relay topology changed across restart: %#v", topology)
	}
	current = current.Add(15 * time.Minute)
	if err := application.Maintain(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	nodes, err := application.Store.HistoryNodes(context.Background(), domain.History15Minutes, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes.Nodes) != 2 {
		t.Fatalf("redirected relay history nodes = %#v", nodes.Nodes)
	}
	for _, node := range nodes.Nodes {
		if node.IdentityStatus != domain.IdentityResolved {
			t.Fatalf("history node remained unresolved after merge: %#v", node)
		}
	}
	history, found, err := application.Store.EdgeHistoryWindow(context.Background(), oldEdgeID, domain.History15Minutes, current)
	if err != nil || !found {
		t.Fatalf("redirected relay history found=%v err=%v", found, err)
	}
	if history.EdgeID != canonicalEdgeID || history.PathAnchor == nil ||
		history.PathAnchor.Path.Kind != domain.PathPeerRelay || history.PathAnchor.Path.PeerRelayVNI == nil ||
		*history.PathAnchor.Path.PeerRelayVNI != vni || len(history.PathAnchor.Observations) != 1 ||
		history.PathAnchor.Observations[0].RelaySession == nil {
		t.Fatalf("retained relay anchor = %#v", history.PathAnchor)
	}
	var aToB, bToA int64
	for _, bucket := range history.Traffic {
		aToB += bucket.AToBBytes
		bToA += bucket.BToABytes
	}
	if aToB != 10 || bToA != 30 {
		t.Fatalf("window traffic after relay redirect = %d/%d, want 10/30", aToB, bToA)
	}
	if redirected := application.Aggregator.HistoryMetadata().Redirects[placeholderID]; redirected != bID {
		t.Fatalf("placeholder redirect = %q, want %q", redirected, bID)
	}
}

func TestDirectToRelayTransitionPersistsSanitizedProvenance(t *testing.T) {
	database, err := store.Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	application, err := New(database, testOptions(func() time.Time { return at }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := application.SubmitAt(context.Background(), helloReport(at), at); err != nil ||
		!receipt.Accepted || receipt.ResyncRequired {
		t.Fatalf("hello receipt = %#v, err=%v", receipt, err)
	}
	direct := trafficReport(at)
	direct.ReportID = "direct-traffic"
	direct.Sequence = 2
	if receipt, err := application.SubmitAt(context.Background(), direct, at); err != nil ||
		!receipt.Accepted || receipt.ResyncRequired {
		t.Fatalf("direct receipt = %#v, err=%v", receipt, err)
	}

	relayAt := at.Add(2 * time.Second)
	relay := relayTrafficReport(relayAt)
	relay.RelaySessions[0].Target = domain.RelaySessionClient{
		SessionClientID: "right",
		Identity:        &domain.NodeIdentity{StableNodeID: "b", Hostname: "B"},
	}
	if receipt, err := application.SubmitAt(context.Background(), relay, relayAt); err != nil ||
		!receipt.Accepted || receipt.ResyncRequired {
		t.Fatalf("relay receipt = %#v, err=%v", receipt, err)
	}
	topology := application.Aggregator.Snapshot()
	if len(topology.Edges) != 1 || topology.Edges[0].Path.Kind != domain.PathPeerRelay {
		t.Fatalf("transition topology = %#v", topology.Edges)
	}
	history, err := database.EdgeHistory(context.Background(), topology.Edges[0].ID, at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hasRelaySession := false
	if len(history.PathEvents) == 2 {
		for _, observation := range history.PathEvents[1].Observations {
			if observation.RelaySession != nil {
				hasRelaySession = true
			}
		}
	}
	if len(history.PathEvents) != 2 || history.PathEvents[0].Path.Kind != domain.PathDirect ||
		history.PathEvents[1].Path.Kind != domain.PathPeerRelay ||
		len(history.PathEvents[1].Observations) != 2 || !hasRelaySession {
		t.Fatalf("Direct-to-Relay history = %#v", history.PathEvents)
	}
	payload, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("Endpoint")) || bytes.Contains(payload, []byte("Disco")) {
		t.Fatalf("relay History retained underlay provenance: %s", payload)
	}
}

func TestCheckpointCadenceAndReceiveTimeRollback(t *testing.T) {
	database, err := store.Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	application, err := New(database, testOptions(func() time.Time { return now }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SubmitAt(context.Background(), helloReport(now), now); err != nil {
		t.Fatal(err)
	}
	first, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	journaledAt := now.Add(500 * time.Millisecond)
	journaled := heartbeatReport(journaledAt)
	if _, err := application.SubmitAt(context.Background(), journaled, journaledAt); err != nil {
		t.Fatal(err)
	}
	stillFirst, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stillFirst.LastReportRowID != first.LastReportRowID || !stillFirst.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("sub-second report advanced checkpoint: first=%#v next=%#v", first, stillFirst)
	}
	checkpointAt := now.Add(time.Second)
	checkpointed := heartbeatReport(checkpointAt)
	checkpointed.ReportID = "heartbeat-checkpointed"
	checkpointed.Sequence = 3
	if _, err := application.SubmitAt(context.Background(), checkpointed, checkpointAt); err != nil {
		t.Fatal(err)
	}
	second, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.LastReportRowID <= first.LastReportRowID || !second.UpdatedAt.Equal(checkpointAt) {
		t.Fatalf("one-second report did not advance checkpoint: first=%#v next=%#v", first, second)
	}
	rollbackAt := now.Add(-time.Second)
	rollback := heartbeatReport(rollbackAt)
	rollback.ReportID = "heartbeat-clock-rollback"
	rollback.Sequence = 4
	if _, err := application.SubmitAt(context.Background(), rollback, rollbackAt); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.LastReportRowID <= second.LastReportRowID || !rolledBack.UpdatedAt.Equal(rollbackAt) {
		t.Fatalf("receive-time rollback did not force checkpoint: previous=%#v next=%#v", second, rolledBack)
	}
}

func TestStoreFailureDoesNotPublishOrAdvanceMemory(t *testing.T) {
	database, err := store.Open(":memory:", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	application, err := New(database, testOptions(func() time.Time { return now }), nil)
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := application.Aggregator.Subscribe()
	defer unsubscribe()
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SubmitAt(context.Background(), helloReport(now), now); err == nil {
		t.Fatal("submit succeeded after storage was closed")
	}
	if got := len(application.Aggregator.Snapshot().Nodes); got != 0 {
		t.Fatalf("failed transaction published %d nodes", got)
	}
	select {
	case <-events:
		t.Fatal("failed transaction notified SSE subscribers")
	default:
	}
}

func testOptions(now func() time.Time) aggregate.Options {
	next := 0
	return aggregate.Options{
		Now: now,
		NewNodeID: func() string {
			next++
			return fmt.Sprintf("n_%03d", next)
		},
	}
}

func topologyIDsByStableNodeID(topology domain.Topology) map[string]string {
	ids := make(map[string]string, len(topology.Nodes))
	for _, node := range topology.Nodes {
		ids[node.StableNodeID] = node.ID
	}
	return ids
}

func helloReport(at time.Time) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}}},
		}},
	}
}

func heartbeatReport(at time.Time) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "heartbeat", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: at, Kind: domain.ReportObserverHeartbeat,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory",
		}},
	}
}

func trafficReport(at time.Time) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "traffic", ReporterInstanceID: "reporter", Sequence: 3,
		CollectedAt: at, Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory",
			Peers: []domain.PeerObservation{{
				Peer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, TxDelta: 100,
				SampleDurationMS: 2000, Path: domain.PathObservation{Kind: domain.PathDirect}, LastActive: at,
			}},
		}},
	}
}

func relayTrafficReport(at time.Time) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "relay-traffic", ReporterInstanceID: "relay-reporter", Sequence: 1,
		CollectedAt: at, Kind: domain.ReportRelaySessionUpdate,
		RelaySessions: []domain.RelaySessionObservation{{
			Relay: domain.NodeIdentity{StableNodeID: "relay", Hostname: "Relay"},
			Source: domain.RelaySessionClient{
				SessionClientID: "left", Identity: &domain.NodeIdentity{StableNodeID: "a", Hostname: "A"},
			},
			Target:    domain.RelaySessionClient{SessionClientID: "right", DiscoShort: "short-right"},
			SessionID: "session", VNI: 7, SourceToTargetBytes: 200, TargetToSourceBytes: 80,
			SourceToTargetDelta: 200, TargetToSourceDelta: 80, SampleDurationMS: 2000, LastActive: at,
		}},
	}
}
