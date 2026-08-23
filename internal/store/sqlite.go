package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type SQLite struct {
	db        *sql.DB
	anchor    *sql.Conn
	retention time.Duration
}

var memoryDatabaseSequence atomic.Uint64

type StoredReport struct {
	RowID      int64
	Report     domain.ReportEnvelope
	ReceivedAt time.Time
}

type RuntimeCheckpoint struct {
	Payload         []byte
	LastReportRowID int64
	UpdatedAt       time.Time
}

func Open(path string, retention time.Duration) (*SQLite, error) {
	if path == "" {
		path = "tailpath.db"
	}
	if retention == 0 {
		retention = 7 * 24 * time.Hour
	}
	memory := path == ":memory:"
	if memory {
		path = fmt.Sprintf("file:tailpath-memory-%d?mode=memory&cache=shared", memoryDatabaseSequence.Add(1))
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	maxConnections := 1
	var anchor *sql.Conn
	if memory {
		// Keep the lifetime anchor plus enough working connections for concurrent
		// fixture API reads, writes, and canceled-request cleanup.
		maxConnections = 8
		anchor, err = db.Conn(context.Background())
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("anchor in-memory database: %w", err)
		}
	}
	db.SetMaxOpenConns(maxConnections)
	if err := migrate(db); err != nil {
		if anchor != nil {
			anchor.Close()
		}
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &SQLite{db: db, anchor: anchor, retention: retention}, nil
}

func (s *SQLite) Close() error {
	if s.anchor != nil {
		if err := s.anchor.Close(); err != nil {
			return err
		}
	}
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
	return s.RecordWithMetadata(ctx, report, receivedAt, runtimeState, traffic, transitions, nil)
}

func (s *SQLite) RecordWithMetadata(
	ctx context.Context,
	report domain.ReportEnvelope,
	receivedAt time.Time,
	runtimeState []byte,
	traffic []domain.AcceptedTraffic,
	transitions []domain.PathTransition,
	metadata *domain.HistoryMetadata,
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
	reportRowID, err := result.LastInsertId()
	if err != nil {
		return false, err
	}
	if runtimeState != nil {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO runtime_state(singleton, payload, updated_at, last_report_rowid) VALUES (1, ?, ?, ?)
            ON CONFLICT(singleton) DO UPDATE SET
              payload = excluded.payload,
              updated_at = excluded.updated_at,
              last_report_rowid = excluded.last_report_rowid`,
			runtimeState, formatTime(receivedAt), reportRowID); err != nil {
			return false, err
		}
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
	if metadata != nil {
		if err := recordHistoryMetadata(ctx, tx, *metadata, receivedAt); err != nil {
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO traffic_buckets(
		  edge_id, bucket_start, source_id, target_id, observer_id,
		  tx_bytes, rx_bytes, a_to_b_bytes, b_to_a_bytes)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?)
		ON CONFLICT(edge_id, bucket_start, observer_id) DO UPDATE SET
		  a_to_b_bytes = a_to_b_bytes + excluded.a_to_b_bytes,
		  b_to_a_bytes = b_to_a_bytes + excluded.b_to_a_bytes`,
		record.EdgeID, formatTime(bucket), record.SourceID, record.TargetID, record.ObserverID,
		record.AToBBytes, record.BToABytes); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO history_edges(edge_id, source_id, target_id, first_traffic_at, last_traffic_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(edge_id) DO UPDATE SET
		  first_traffic_at = MIN(first_traffic_at, excluded.first_traffic_at),
		  last_traffic_at = MAX(last_traffic_at, excluded.last_traffic_at)`,
		record.EdgeID, record.SourceID, record.TargetID, formatTime(record.ReceivedAt), formatTime(record.ReceivedAt))
	return err
}

func recordHistoryMetadata(ctx context.Context, tx *sql.Tx, metadata domain.HistoryMetadata, updatedAt time.Time) error {
	for _, node := range metadata.Nodes {
		identity, err := json.Marshal(node.NodeIdentity)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO nodes(node_id, identity, observable, last_seen) VALUES (?, ?, ?, ?)
			ON CONFLICT(node_id) DO UPDATE SET identity = excluded.identity,
			  observable = excluded.observable, last_seen = MAX(last_seen, excluded.last_seen)`,
			node.ID, identity, node.Observable, formatTime(node.LastEvidenceAt)); err != nil {
			return err
		}
	}
	for fromID, toID := range metadata.Redirects {
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO canonical_redirects(from_node_id, to_node_id, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(from_node_id) DO UPDATE SET to_node_id = excluded.to_node_id,
			  updated_at = excluded.updated_at`, fromID, toID, formatTime(updatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) SaveHistoryMetadata(ctx context.Context, metadata domain.HistoryMetadata, updatedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordHistoryMetadata(ctx, tx, metadata, updatedAt); err != nil {
		return err
	}
	return tx.Commit()
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
	checkpoint, err := s.RestoreCheckpoint(ctx)
	return checkpoint.Payload, err
}

func (s *SQLite) RestoreCheckpoint(ctx context.Context) (RuntimeCheckpoint, error) {
	var checkpoint RuntimeCheckpoint
	var rawUpdatedAt string
	err := s.db.QueryRowContext(ctx, `
        SELECT payload, last_report_rowid, updated_at FROM runtime_state WHERE singleton = 1`,
	).Scan(&checkpoint.Payload, &checkpoint.LastReportRowID, &rawUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return checkpoint, nil
	}
	if err != nil {
		return checkpoint, err
	}
	checkpoint.UpdatedAt, err = time.Parse(time.RFC3339Nano, rawUpdatedAt)
	return checkpoint, err
}

func (s *SQLite) SaveState(ctx context.Context, payload []byte, updatedAt time.Time) error {
	var lastReportRowID int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(rowid), 0) FROM reports`).Scan(&lastReportRowID); err != nil {
		return err
	}
	return s.SaveCheckpoint(ctx, payload, lastReportRowID, updatedAt)
}

func (s *SQLite) SaveCheckpoint(ctx context.Context, payload []byte, lastReportRowID int64, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO runtime_state(singleton, payload, updated_at, last_report_rowid) VALUES (1, ?, ?, ?)
        ON CONFLICT(singleton) DO UPDATE SET
          payload = excluded.payload,
          updated_at = excluded.updated_at,
          last_report_rowid = excluded.last_report_rowid`,
		payload, formatTime(updatedAt), lastReportRowID)
	return err
}

func (s *SQLite) RestoreReports(ctx context.Context) ([]StoredReport, error) {
	return s.RestoreReportsAfter(ctx, 0)
}

func (s *SQLite) RestoreReportsAfter(ctx context.Context, lastReportRowID int64) ([]StoredReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rowid, payload, received_at FROM reports WHERE rowid > ? ORDER BY rowid`, lastReportRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []StoredReport
	for rows.Next() {
		var stored StoredReport
		var payload []byte
		var rawReceivedAt string
		if err := rows.Scan(&stored.RowID, &payload, &rawReceivedAt); err != nil {
			return nil, err
		}
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

func (s *SQLite) Maintain(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var checkpointRowID int64
	err = tx.QueryRowContext(ctx, `SELECT last_report_rowid FROM runtime_state WHERE singleton = 1`).Scan(&checkpointRowID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if checkpointRowID > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM reports WHERE rowid <= ?`, checkpointRowID); err != nil {
			return err
		}
	}
	cutoff := formatTime(now.UTC().Add(-s.retention))
	for _, statement := range []string{
		"DELETE FROM traffic_buckets WHERE bucket_start < ?",
		"DELETE FROM path_events WHERE observed_at < ?",
	} {
		if _, err := tx.ExecContext(ctx, statement, cutoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
