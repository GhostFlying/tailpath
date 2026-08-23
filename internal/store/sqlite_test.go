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

func TestInMemoryDatabaseSurvivesPooledConnectionReplacement(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if database.anchor == nil {
		t.Fatal("in-memory database has no lifetime anchor")
	}
	database.db.SetConnMaxLifetime(time.Nanosecond)
	time.Sleep(time.Millisecond)

	var version int
	if err := database.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == 0 {
		t.Fatal("schema disappeared after replacing the pooled connection")
	}
}

func TestInMemoryDatabaseKeepsConcurrentConnectionsBesideAnchor(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	connections := make([]*sql.Conn, 0, 7)
	defer func() {
		for _, connection := range connections {
			connection.Close()
		}
	}()
	for range 7 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		connection, err := database.db.Conn(ctx)
		cancel()
		if err != nil {
			t.Fatalf("acquire concurrent in-memory connection %d: %v", len(connections)+1, err)
		}
		connections = append(connections, connection)
	}
}

func TestOpenConfiguresEverySQLiteConnectionForRuntimeWorkload(t *testing.T) {
	database, err := Open(":memory:", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for _, check := range []struct {
		pragma string
		want   int
	}{
		{pragma: "foreign_keys", want: 1},
		{pragma: "synchronous", want: 1},
		{pragma: "temp_store", want: 2},
	} {
		var got int
		if err := database.db.QueryRow("PRAGMA " + check.pragma).Scan(&got); err != nil {
			t.Fatalf("read PRAGMA %s: %v", check.pragma, err)
		}
		if got != check.want {
			t.Fatalf("PRAGMA %s = %d, want %d", check.pragma, got, check.want)
		}
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

func TestRecordPersistsHistoryEdgeAndCheckpointMetadata(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{
		Nodes: []domain.TopologyNode{{
			NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "Node A", OS: "linux"},
			ID:           "n_a", Observable: true, LastEvidenceAt: at,
		}},
		Redirects: map[string]string{"n_old": "n_a"},
	}
	traffic := []domain.AcceptedTraffic{{
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_a",
		AToBBytes: 10, BToABytes: 2, ReceivedAt: at,
	}}
	inserted, err := database.RecordWithMetadata(context.Background(), sampleReport(at, at, "metadata"), at, []byte(`{}`), traffic, nil, &metadata)
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	var sourceID, targetID, lastTrafficAt string
	if err := database.db.QueryRow(`SELECT source_id, target_id, last_traffic_at FROM history_edges WHERE edge_id = ?`, "n_a--n_b").Scan(&sourceID, &targetID, &lastTrafficAt); err != nil {
		t.Fatal(err)
	}
	if sourceID != "n_a" || targetID != "n_b" || lastTrafficAt != formatTime(at) {
		t.Fatalf("history edge = %q %q %q", sourceID, targetID, lastTrafficAt)
	}
	var identity []byte
	if err := database.db.QueryRow(`SELECT identity FROM nodes WHERE node_id = ?`, "n_a").Scan(&identity); err != nil {
		t.Fatal(err)
	}
	var stored domain.NodeIdentity
	if err := json.Unmarshal(identity, &stored); err != nil || stored.OS != "linux" {
		t.Fatalf("stored identity = %#v, err=%v", stored, err)
	}
	var redirect string
	if err := database.db.QueryRow(`SELECT to_node_id FROM canonical_redirects WHERE from_node_id = ?`, "n_old").Scan(&redirect); err != nil || redirect != "n_a" {
		t.Fatalf("redirect = %q, err=%v", redirect, err)
	}
}

func TestMaintainRollsUpDeduplicatedDirectionalTraffic(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	start := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	record := func(id, observer string, at time.Time, aToB, bToA int64) {
		t.Helper()
		traffic := []domain.AcceptedTraffic{{
			EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: observer,
			AToBBytes: aToB, BToABytes: bToA, ReceivedAt: at,
		}}
		if _, err := database.Record(context.Background(), sampleReport(at, at, id), at, nil, traffic, nil); err != nil {
			t.Fatal(err)
		}
	}
	record("source", "n_a", start.Add(3*time.Second), 120, 40)
	record("target", "n_b", start.Add(4*time.Second), 90, 60)
	record("relay-ignored", "n_relay", start.Add(5*time.Second), 500, 500)
	record("relay-fallback", "n_relay", start.Add(13*time.Second), 30, 10)

	if err := database.Maintain(context.Background(), start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertRollup := func(table string, wantAToB, wantBToA int64) {
		t.Helper()
		var aToB, bToA int64
		if err := database.db.QueryRow(`SELECT a_to_b_bytes, b_to_a_bytes FROM `+table+` WHERE edge_id = ?`, "n_a--n_b").Scan(&aToB, &bToA); err != nil {
			t.Fatal(err)
		}
		if aToB != wantAToB || bToA != wantBToA {
			t.Fatalf("%s traffic = %d/%d, want %d/%d", table, aToB, bToA, wantAToB, wantBToA)
		}
	}
	assertRollup("traffic_rollup_minute", 150, 70)
	if err := database.Maintain(context.Background(), start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertRollup("traffic_rollup_minute", 150, 70)
	if err := database.Maintain(context.Background(), start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertRollup("traffic_rollup_hour", 150, 70)
}

func TestMaintainKeepsOnlyRequiredPathAnchor(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)
	path, _ := json.Marshal(domain.PathObservation{Kind: domain.PathDirect})
	for _, at := range []time.Time{cutoff.Add(-2 * time.Hour), cutoff.Add(-time.Hour), cutoff.Add(time.Hour)} {
		if _, err := database.db.Exec(`INSERT INTO path_events(edge_id, observed_at, path, observations) VALUES (?, ?, ?, '[]')`, "retained", formatTime(at), path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.db.Exec(`INSERT INTO path_events(edge_id, observed_at, path, observations) VALUES (?, ?, ?, '[]')`, "expired", formatTime(cutoff.Add(-time.Hour)), path); err != nil {
		t.Fatal(err)
	}
	for edgeID, lastTraffic := range map[string]time.Time{"retained": now, "expired": cutoff.Add(-time.Hour)} {
		if _, err := database.db.Exec(`INSERT INTO history_edges VALUES (?, 'a', 'b', ?, ?)`, edgeID, formatTime(lastTraffic), formatTime(lastTraffic)); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	rows, err := database.db.Query(`SELECT edge_id, observed_at FROM path_events ORDER BY edge_id, observed_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var retained []string
	for rows.Next() {
		var edgeID, observedAt string
		if err := rows.Scan(&edgeID, &observedAt); err != nil {
			t.Fatal(err)
		}
		if edgeID == "expired" {
			t.Fatal("expired edge retained a path anchor")
		}
		retained = append(retained, observedAt)
	}
	want := []string{formatTime(cutoff.Add(-time.Hour)), formatTime(cutoff.Add(time.Hour))}
	if len(retained) != len(want) || retained[0] != want[0] || retained[1] != want[1] {
		t.Fatalf("retained path events = %#v, want %#v", retained, want)
	}
}

func TestMaintainAppliesTierRetentionBoundaries(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if _, err := database.db.Exec(`INSERT INTO history_maintenance VALUES ('minute', ?), ('hour', ?)`, formatTime(now.Truncate(time.Minute)), formatTime(now.Truncate(time.Hour))); err != nil {
		t.Fatal(err)
	}
	for table, retention := range map[string]time.Duration{
		"traffic_buckets": rawTrafficRetention, "traffic_rollup_minute": minuteTrafficRetention,
		"traffic_rollup_hour": hourTrafficRetention,
	} {
		columns := "edge_id, bucket_start, source_id, target_id, a_to_b_bytes, b_to_a_bytes"
		values := "?, ?, 'a', 'b', 1, 1"
		if table == "traffic_buckets" {
			columns = "edge_id, bucket_start, source_id, target_id, observer_id, tx_bytes, rx_bytes, a_to_b_bytes, b_to_a_bytes"
			values = "?, ?, 'a', 'b', 'a', 0, 0, 1, 1"
		}
		for _, item := range []struct {
			id string
			at time.Time
		}{{"expired", now.Add(-retention).Add(-time.Nanosecond)}, {"boundary", now.Add(-retention)}} {
			if _, err := database.db.Exec(`INSERT INTO `+table+`(`+columns+`) VALUES (`+values+`)`, item.id, formatTime(item.at)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := database.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"traffic_buckets", "traffic_rollup_minute", "traffic_rollup_hour"} {
		var count int
		var edgeID string
		if err := database.db.QueryRow(`SELECT COUNT(*), MIN(edge_id) FROM `+table).Scan(&count, &edgeID); err != nil {
			t.Fatal(err)
		}
		if count != 1 || edgeID != "boundary" {
			t.Fatalf("%s retained count=%d edge=%q", table, count, edgeID)
		}
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

func TestCheckpointCursorRestoresOnlyLaterReports(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	first := sampleReport(at, at, "first")
	second := sampleReport(at.Add(time.Second), at.Add(time.Second), "second")
	if _, err := database.Record(context.Background(), first, at, []byte(`{"checkpoint":1}`), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Record(context.Background(), second, at.Add(time.Second), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(checkpoint.Payload) != `{"checkpoint":1}` || checkpoint.LastReportRowID < 1 || !checkpoint.UpdatedAt.Equal(at) {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	reports, err := database.RestoreReportsAfter(context.Background(), checkpoint.LastReportRowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Report.ReportID != "second" || reports[0].RowID <= checkpoint.LastReportRowID {
		t.Fatalf("later reports = %#v", reports)
	}
}

func TestMaintainDeletesOnlyCheckpointCoveredReports(t *testing.T) {
	database, err := Open(":memory:", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if _, err := database.Record(context.Background(), sampleReport(at, at, "checkpointed"), at, []byte(`{}`), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Record(context.Background(), sampleReport(at.Add(time.Second), at.Add(time.Second), "journaled"), at.Add(time.Second), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Maintain(context.Background(), at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	reports, err := database.RestoreReports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Report.ReportID != "journaled" {
		t.Fatalf("reports after maintenance = %#v", reports)
	}
	if _, err := database.Record(context.Background(), sampleReport(at.Add(2*time.Second), at.Add(2*time.Second), "next-checkpoint"), at.Add(2*time.Second), []byte(`{"next":true}`), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Maintain(context.Background(), at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	reports, err = database.RestoreReports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 0 {
		t.Fatalf("checkpoint-covered reports remain: %#v", reports)
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
	var version int
	if err := database.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != currentSchemaVersion {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
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

func TestOpenNumberedMigrationsAreIdempotent(t *testing.T) {
	path := t.TempDir() + "/numbered.db"
	for attempt := range 2 {
		database, err := Open(path, 7*24*time.Hour)
		if err != nil {
			t.Fatalf("open attempt %d: %v", attempt, err)
		}
		var version int
		if err := database.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
			database.Close()
			t.Fatal(err)
		}
		if version != currentSchemaVersion {
			database.Close()
			t.Fatalf("schema version = %d, want %d", version, currentSchemaVersion)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenRejectsFutureSchemaVersion(t *testing.T) {
	path := t.TempDir() + "/future.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if database, err := Open(path, 7*24*time.Hour); err == nil {
		database.Close()
		t.Fatal("future schema version was accepted")
	}
}

func TestOpenMigratesLegacyRuntimeCheckpointCursor(t *testing.T) {
	path := t.TempDir() + "/legacy-checkpoint.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if _, err := raw.Exec(`
        CREATE TABLE runtime_state (
          singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
          payload BLOB NOT NULL,
          updated_at TEXT NOT NULL
        );
        INSERT INTO runtime_state(singleton, payload, updated_at) VALUES (1, '{"legacy":true}', ?)
    `, formatTime(at)); err != nil {
		raw.Close()
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
	checkpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(checkpoint.Payload) != `{"legacy":true}` || checkpoint.LastReportRowID != 0 || !checkpoint.UpdatedAt.Equal(at) {
		t.Fatalf("migrated checkpoint = %#v", checkpoint)
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
