package store

import (
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 2

type migration func(*sql.Tx) error

var migrations = []migration{
	migrateDraftSchema,
	migrateBoundedHistory,
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;`); err != nil {
		return err
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > len(migrations) {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	for version < len(migrations) {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := migrations[version](tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		version++
	}
	return nil
}

func migrateDraftSchema(tx *sql.Tx) error {
	if _, err := tx.Exec(`
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
    updated_at TEXT NOT NULL,
    last_report_rowid INTEGER NOT NULL DEFAULT 0
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
);`); err != nil {
		return err
	}
	for _, column := range []struct {
		table, name, declaration string
	}{
		{"reports", "received_at", "TEXT"},
		{"runtime_state", "last_report_rowid", "INTEGER NOT NULL DEFAULT 0"},
		{"path_events", "observations", "BLOB"},
		{"traffic_buckets", "a_to_b_bytes", "INTEGER"},
		{"traffic_buckets", "b_to_a_bytes", "INTEGER"},
	} {
		if err := ensureColumn(tx, column.table, column.name, column.declaration); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
UPDATE reports SET received_at = collected_at WHERE received_at IS NULL OR received_at = '';
UPDATE path_events SET observations = '[]' WHERE observations IS NULL;
UPDATE traffic_buckets SET
  a_to_b_bytes = CASE WHEN observer_id = source_id THEN tx_bytes ELSE rx_bytes END,
  b_to_a_bytes = CASE WHEN observer_id = target_id THEN tx_bytes ELSE rx_bytes END
WHERE a_to_b_bytes IS NULL OR b_to_a_bytes IS NULL;
CREATE INDEX IF NOT EXISTS reports_received_at ON reports(received_at);
CREATE INDEX IF NOT EXISTS path_events_edge_time ON path_events(edge_id, observed_at);`); err != nil {
		return err
	}
	return nil
}

func migrateBoundedHistory(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS history_edges (
    edge_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    first_traffic_at TEXT NOT NULL,
    last_traffic_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS canonical_redirects (
    from_node_id TEXT PRIMARY KEY,
    to_node_id TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (from_node_id != to_node_id)
);
CREATE TABLE IF NOT EXISTS traffic_rollup_minute (
    edge_id TEXT NOT NULL,
    bucket_start TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    a_to_b_bytes INTEGER NOT NULL,
    b_to_a_bytes INTEGER NOT NULL,
    PRIMARY KEY(edge_id, bucket_start)
);
CREATE TABLE IF NOT EXISTS traffic_rollup_hour (
    edge_id TEXT NOT NULL,
    bucket_start TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    a_to_b_bytes INTEGER NOT NULL,
    b_to_a_bytes INTEGER NOT NULL,
    PRIMARY KEY(edge_id, bucket_start)
);
CREATE TABLE IF NOT EXISTS history_maintenance (
    name TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT OR IGNORE INTO history_edges(edge_id, source_id, target_id, first_traffic_at, last_traffic_at)
SELECT edge_id, MIN(source_id), MIN(target_id), MIN(bucket_start), MAX(bucket_start)
FROM traffic_buckets GROUP BY edge_id;
CREATE INDEX IF NOT EXISTS traffic_buckets_time_edge ON traffic_buckets(bucket_start, edge_id);
CREATE INDEX IF NOT EXISTS traffic_rollup_minute_time_edge ON traffic_rollup_minute(bucket_start, edge_id);
CREATE INDEX IF NOT EXISTS traffic_rollup_minute_edge_time ON traffic_rollup_minute(edge_id, bucket_start);
CREATE INDEX IF NOT EXISTS traffic_rollup_hour_time_edge ON traffic_rollup_hour(bucket_start, edge_id);
CREATE INDEX IF NOT EXISTS traffic_rollup_hour_edge_time ON traffic_rollup_hour(edge_id, bucket_start);
CREATE INDEX IF NOT EXISTS history_edges_last_edge ON history_edges(last_traffic_at DESC, edge_id);
CREATE INDEX IF NOT EXISTS history_edges_source_last ON history_edges(source_id, last_traffic_at DESC);
CREATE INDEX IF NOT EXISTS history_edges_target_last ON history_edges(target_id, last_traffic_at DESC);
CREATE INDEX IF NOT EXISTS canonical_redirects_target ON canonical_redirects(to_node_id);
CREATE INDEX IF NOT EXISTS path_events_time_edge ON path_events(observed_at, edge_id);`)
	return err
}

func ensureColumn(tx *sql.Tx, table, column, declaration string) error {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
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
	_, err = tx.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + declaration)
	return err
}
