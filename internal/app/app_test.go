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

func TestRuntimeStateSurvivesRawReportRetention(t *testing.T) {
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
	reports, err := database.RestoreReports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Report.Kind != domain.ReportObserverHeartbeat {
		t.Fatalf("retained reports = %#v, want only heartbeat", reports)
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
