package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/GhostFlying/tailpath/internal/domain"
)

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS reports (
    report_id TEXT PRIMARY KEY,
    reporter_instance_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    collected_at TEXT NOT NULL,
    received_at TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    payload BLOB NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS nodes (
    node_id TEXT PRIMARY KEY,
    identity BLOB NOT NULL,
    observable INTEGER NOT NULL,
    last_seen TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS latest_observations (
    edge_id TEXT NOT NULL,
    observer_id TEXT NOT NULL,
    peer_id TEXT NOT NULL,
    path BLOB NOT NULL,
    collected_at TEXT NOT NULL,
    PRIMARY KEY(edge_id, observer_id)
);

CREATE TABLE IF NOT EXISTS traffic_buckets (
    edge_id TEXT NOT NULL,
    bucket_start TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    observer_id TEXT NOT NULL,
    tx_bytes INTEGER NOT NULL,
    rx_bytes INTEGER NOT NULL,
    a_to_b_bytes INTEGER NOT NULL,
    b_to_a_bytes INTEGER NOT NULL,
    PRIMARY KEY(edge_id, bucket_start, observer_id)
);

CREATE TABLE IF NOT EXISTS path_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    edge_id TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    path BLOB NOT NULL,
    observations BLOB NOT NULL DEFAULT '[]'
);
`

type SQLite struct {
	db        *sql.DB
	retention time.Duration
}

type StoredReport struct {
	Report     domain.ReportEnvelope
	ReceivedAt time.Time
}

func Open(path string, retention time.Duration) (*SQLite, error) {
	if path == "" {
		path = "tailpath.db"
	}
	if retention == 0 {
		retention = 7 * 24 * time.Hour
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &SQLite{db: db, retention: retention}, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	if err := ensureColumn(db, "reports", "received_at", "TEXT"); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE reports SET received_at = collected_at WHERE received_at IS NULL OR received_at = ''`); err != nil {
		return err
	}
	if err := ensureColumn(db, "path_events", "observations", "BLOB"); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE path_events SET observations = '[]' WHERE observations IS NULL`); err != nil {
		return err
	}
	if err := ensureColumn(db, "traffic_buckets", "a_to_b_bytes", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "traffic_buckets", "b_to_a_bytes", "INTEGER"); err != nil {
		return err
	}
	if _, err := db.Exec(`
        UPDATE traffic_buckets SET
          a_to_b_bytes = CASE WHEN observer_id = source_id THEN tx_bytes ELSE rx_bytes END,
          b_to_a_bytes = CASE WHEN observer_id = target_id THEN tx_bytes ELSE rx_bytes END
        WHERE a_to_b_bytes IS NULL OR b_to_a_bytes IS NULL`); err != nil {
		return err
	}
	_, err := db.Exec(`
        CREATE INDEX IF NOT EXISTS reports_received_at ON reports(received_at);
        CREATE INDEX IF NOT EXISTS path_events_edge_time ON path_events(edge_id, observed_at);
    `)
	return err
}

func ensureColumn(db *sql.DB, table, column, declaration string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var sequence int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + declaration)
	return err
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) Retention() time.Duration {
	return s.retention
}

func (s *SQLite) Record(
	ctx context.Context,
	report domain.ReportEnvelope,
	receivedAt time.Time,
	runtimeState []byte,
	traffic []domain.AcceptedTraffic,
	transitions []domain.PathTransition,
) (bool, error) {
	payload, err := json.Marshal(report)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
        INSERT OR IGNORE INTO reports(report_id, reporter_instance_id, sequence, collected_at, received_at, kind, payload)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		report.ReportID, report.ReporterInstanceID, report.Sequence, formatTime(report.CollectedAt),
		formatTime(receivedAt), report.Kind, payload)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO runtime_state(singleton, payload, updated_at) VALUES (1, ?, ?)
        ON CONFLICT(singleton) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		runtimeState, formatTime(receivedAt)); err != nil {
		return false, err
	}
	for _, record := range traffic {
		if err := recordTraffic(ctx, tx, record); err != nil {
			return false, err
		}
	}
	for _, transition := range transitions {
		if err := recordPathTransition(ctx, tx, transition); err != nil {
			return false, err
		}
	}

	cutoff := formatTime(receivedAt.Add(-s.retention))
	for _, statement := range []string{
		"DELETE FROM reports WHERE received_at < ?",
		"DELETE FROM traffic_buckets WHERE bucket_start < ?",
		"DELETE FROM path_events WHERE observed_at < ?",
	} {
		if _, err := tx.ExecContext(ctx, statement, cutoff); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func recordTraffic(ctx context.Context, tx *sql.Tx, record domain.AcceptedTraffic) error {
	bucket := record.ReceivedAt.Truncate(10 * time.Second)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO traffic_buckets(
		  edge_id, bucket_start, source_id, target_id, observer_id,
		  tx_bytes, rx_bytes, a_to_b_bytes, b_to_a_bytes)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?)
		ON CONFLICT(edge_id, bucket_start, observer_id) DO UPDATE SET
		  a_to_b_bytes = a_to_b_bytes + excluded.a_to_b_bytes,
		  b_to_a_bytes = b_to_a_bytes + excluded.b_to_a_bytes`,
		record.EdgeID, formatTime(bucket), record.SourceID, record.TargetID, record.ObserverID,
		record.AToBBytes, record.BToABytes)
	return err
}

func recordPathTransition(ctx context.Context, tx *sql.Tx, transition domain.PathTransition) error {
	path, err := json.Marshal(transition.Path)
	if err != nil {
		return err
	}
	observations, err := json.Marshal(transition.Observations)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO path_events(edge_id, observed_at, path, observations) VALUES (?, ?, ?, ?)`,
		transition.EdgeID, formatTime(transition.ObservedAt), path, observations)
	return err
}

func (s *SQLite) EdgeHistory(ctx context.Context, edgeID string, since time.Time) (domain.EdgeHistory, error) {
	history := domain.EdgeHistory{EdgeID: edgeID, Traffic: []domain.TrafficBucket{}, PathEvents: []domain.PathEvent{}}
	rows, err := s.db.QueryContext(ctx, `
		SELECT bucket_start,
		  COALESCE(
		    SUM(CASE WHEN observer_id = source_id THEN a_to_b_bytes END),
		    SUM(CASE WHEN observer_id = target_id THEN a_to_b_bytes END),
		    MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN a_to_b_bytes END), 0),
		  COALESCE(
		    SUM(CASE WHEN observer_id = target_id THEN b_to_a_bytes END),
		    SUM(CASE WHEN observer_id = source_id THEN b_to_a_bytes END),
		    MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN b_to_a_bytes END), 0)
        FROM traffic_buckets WHERE edge_id = ? AND bucket_start >= ?
        GROUP BY edge_id, bucket_start ORDER BY bucket_start`,
		edgeID, formatTime(since))
	if err != nil {
		return history, err
	}
	for rows.Next() {
		var rawTime string
		var bucket domain.TrafficBucket
		if err := rows.Scan(&rawTime, &bucket.AToBBytes, &bucket.BToABytes); err != nil {
			rows.Close()
			return history, err
		}
		bucket.BucketStart, err = time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			rows.Close()
			return history, err
		}
		history.Traffic = append(history.Traffic, bucket)
	}
	if err := rows.Close(); err != nil {
		return history, err
	}

	rows, err = s.db.QueryContext(ctx, `
        SELECT observed_at, path, observations FROM path_events
        WHERE edge_id = ? AND observed_at >= ? ORDER BY observed_at`, edgeID, formatTime(since))
	if err != nil {
		return history, err
	}
	defer rows.Close()
	for rows.Next() {
		var rawTime string
		var rawPath, rawObservations []byte
		var event domain.PathEvent
		if err := rows.Scan(&rawTime, &rawPath, &rawObservations); err != nil {
			return history, err
		}
		event.ObservedAt, err = time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return history, err
		}
		if err := json.Unmarshal(rawPath, &event.Path); err != nil {
			return history, err
		}
		if err := json.Unmarshal(rawObservations, &event.Observations); err != nil {
			return history, err
		}
		history.PathEvents = append(history.PathEvents, event)
	}
	return history, rows.Err()
}

func (s *SQLite) RestoreState(ctx context.Context) ([]byte, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM runtime_state WHERE singleton = 1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return payload, err
}

func (s *SQLite) SaveState(ctx context.Context, payload []byte, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO runtime_state(singleton, payload, updated_at) VALUES (1, ?, ?)
        ON CONFLICT(singleton) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		payload, formatTime(updatedAt))
	return err
}

func (s *SQLite) RestoreReports(ctx context.Context) ([]StoredReport, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT payload, received_at FROM reports ORDER BY received_at, reporter_instance_id, sequence`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []StoredReport
	for rows.Next() {
		var payload []byte
		var rawReceivedAt string
		if err := rows.Scan(&payload, &rawReceivedAt); err != nil {
			return nil, err
		}
		var stored StoredReport
		if err := json.Unmarshal(payload, &stored.Report); err != nil {
			return nil, err
		}
		stored.ReceivedAt, err = time.Parse(time.RFC3339Nano, rawReceivedAt)
		if err != nil {
			return nil, err
		}
		reports = append(reports, stored)
	}
	return reports, rows.Err()
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
