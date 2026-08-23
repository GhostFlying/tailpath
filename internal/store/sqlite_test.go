package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestRecordIsIdempotentAndBuildsLogicalHistory(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 22, 12, 0, 3, 0, time.UTC)
	report := sampleReport(at, at, "sample")
	traffic := []domain.AcceptedTraffic{{
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_a",
		AToBBytes: 120, BToABytes: 40, ReceivedAt: at,
	}, {
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_relay",
		AToBBytes: 30, BToABytes: 10, ReceivedAt: at,
	}}
	provenance := domain.ObservationProvenance{
		ObserverID: "n_a", Path: domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"},
		CollectedAt: at, ReceivedAt: at,
	}
	transitions := []domain.PathTransition{{
		EdgeID: "n_a--n_b", ObservedAt: at, Path: provenance.Path,
		Observations: []domain.ObservationProvenance{provenance},
	}}
	inserted, err := database.Record(context.Background(), report, at, []byte(`{"state":1}`), traffic, transitions)
	if err != nil || !inserted {
		t.Fatalf("first record inserted=%v, err=%v", inserted, err)
	}
	inserted, err = database.Record(context.Background(), report, at, []byte(`{"state":2}`), traffic, transitions)
	if err != nil || inserted {
		t.Fatalf("duplicate record inserted=%v, err=%v", inserted, err)
	}
	history, err := database.EdgeHistory(context.Background(), "n_a--n_b", at.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Traffic) != 1 || history.Traffic[0].AToBBytes != 120 || history.Traffic[0].BToABytes != 40 {
		t.Fatalf("unexpected traffic history: %#v", history.Traffic)
	}
	if len(history.PathEvents) != 1 || history.PathEvents[0].Path.DERPRegion != "hkg" || len(history.PathEvents[0].Observations) != 1 {
		t.Fatalf("unexpected logical path history: %#v", history.PathEvents)
	}
	state, err := database.RestoreState(context.Background())
	if err != nil || string(state) != `{"state":1}` {
		t.Fatalf("runtime state = %s, err=%v", state, err)
	}
}

func TestRelayTrafficProvidesHistoryWhenEndpointsAreUnobservable(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 22, 12, 0, 3, 0, time.UTC)
	report := sampleReport(at, at, "relay-sample")
	traffic := []domain.AcceptedTraffic{{
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_relay",
		AToBBytes: 30, BToABytes: 10, ReceivedAt: at,
	}}
	if _, err := database.Record(context.Background(), report, at, []byte(`{}`), traffic, nil); err != nil {
		t.Fatal(err)
	}
	history, err := database.EdgeHistory(context.Background(), "n_a--n_b", at.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Traffic) != 1 || history.Traffic[0].AToBBytes != 30 || history.Traffic[0].BToABytes != 10 {
		t.Fatalf("relay traffic history = %#v, want 30/10", history.Traffic)
	}
}

func TestRetentionUsesServerReceiveTime(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	received := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	wrongClientTime := received.Add(30 * 24 * time.Hour)
	for index, collected := range []time.Time{wrongClientTime, received} {
		report := sampleReport(collected, received, string(rune('a'+index)))
		if _, err := database.Record(context.Background(), report, received.Add(time.Duration(index)*time.Minute), []byte(`{}`), nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	reports, err := database.RestoreReports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("client clock deleted reports; got %d", len(reports))
	}
}

func TestOpenMigratesDraftSchemaReceiveTimeAndPathProvenance(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
        CREATE TABLE reports (
          report_id TEXT PRIMARY KEY, reporter_instance_id TEXT NOT NULL,
          sequence INTEGER NOT NULL, collected_at TEXT NOT NULL, kind TEXT NOT NULL, payload BLOB NOT NULL
        );
		CREATE TABLE path_events (
		  id INTEGER PRIMARY KEY AUTOINCREMENT, edge_id TEXT NOT NULL,
		  observed_at TEXT NOT NULL, path BLOB NOT NULL
		);
		CREATE TABLE traffic_buckets (
		  edge_id TEXT NOT NULL, bucket_start TEXT NOT NULL,
		  source_id TEXT NOT NULL, target_id TEXT NOT NULL, observer_id TEXT NOT NULL,
		  tx_bytes INTEGER NOT NULL, rx_bytes INTEGER NOT NULL,
		  PRIMARY KEY(edge_id, bucket_start, observer_id)
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	report := sampleReport(at, at, "legacy")
	payload, _ := json.Marshal(report)
	pathPayload, _ := json.Marshal(domain.PathObservation{Kind: domain.PathDirect})
	if _, err := raw.Exec(`INSERT INTO reports VALUES (?, ?, ?, ?, ?, ?)`, "legacy", "reporter", report.Sequence, formatTime(at), report.Kind, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO path_events(edge_id, observed_at, path) VALUES (?, ?, ?)`, "n_a--n_b", formatTime(at), pathPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO traffic_buckets VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n_a--n_b", formatTime(at), "n_a", "n_b", "n_a", 120, 40); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	reports, err := database.RestoreReports(context.Background())
	if err != nil || len(reports) != 1 || !reports[0].ReceivedAt.Equal(at) {
		t.Fatalf("migrated reports = %#v, err=%v", reports, err)
	}
	history, err := database.EdgeHistory(context.Background(), "n_a--n_b", at.Add(-time.Minute))
	if err != nil || len(history.PathEvents) != 1 || history.PathEvents[0].Observations == nil ||
		len(history.Traffic) != 1 || history.Traffic[0].AToBBytes != 120 || history.Traffic[0].BToABytes != 40 {
		t.Fatalf("migrated path history = %#v, err=%v", history, err)
	}
}

func sampleReport(collectedAt, lastActive time.Time, reportID string) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: reportID, ReporterInstanceID: "reporter", Sequence: int64(collectedAt.UnixNano()),
		CollectedAt: collectedAt, Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory",
			Peers: []domain.PeerObservation{{
				Peer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, TxDelta: 120, RxDelta: 40,
				SampleDurationMS: 2000, LastActive: lastActive, Path: domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"},
			}},
		}},
	}
}
