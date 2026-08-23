package app

import (
	"context"
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
