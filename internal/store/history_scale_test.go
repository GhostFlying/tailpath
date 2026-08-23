package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestSevenDayHistoryDatabaseSize(t *testing.T) {
	if os.Getenv("TAILPATH_HISTORY_SCALE") != "1" {
		t.Skip("seven-day database fixture is workflow_dispatch only")
	}
	path := filepath.Join(t.TempDir(), "history-scale.db")
	database, err := Open(path, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	nodes := make([]domain.TopologyNode, 250)
	for index := range nodes {
		nodes[index] = domain.TopologyNode{
			ID: fmt.Sprintf("n_%03d", index),
			NodeIdentity: domain.NodeIdentity{
				StableNodeID: fmt.Sprintf("scale-%03d", index),
				Hostname:     fmt.Sprintf("scale-node-%03d", index),
				DNSName:      fmt.Sprintf("scale-node-%03d.example.ts.net.", index),
				OS:           "linux",
			},
			LastEvidenceAt: now,
		}
	}
	if err := database.SaveHistoryMetadata(context.Background(), domain.HistoryMetadata{Nodes: nodes}, now); err != nil {
		t.Fatal(err)
	}
	tx, err := database.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		scaleHistoryEdgeSQL(now),
		scaleRawTrafficSQL(now.Add(-time.Hour)),
		scaleMinuteTrafficSQL(now.Add(-48 * time.Hour)),
		scaleHourTrafficSQL(now.Add(-7 * 24 * time.Hour)),
		scalePathEventSQL(now.Add(-7 * 24 * time.Hour)),
	} {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	const maxBytes = int64(2 * 1024 * 1024 * 1024)
	t.Logf(`{"databaseBytes":%d,"nodes":250,"edges":1000,"rawRows":720000,"minuteRows":2880000,"hourRows":168000}`, info.Size())
	if info.Size() > maxBytes {
		t.Fatalf("seven-day history database = %d bytes, limit = %d", info.Size(), maxBytes)
	}
}

func scaleHistoryEdgeSQL(now time.Time) string {
	return fmt.Sprintf(`
WITH RECURSIVE edge(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM edge WHERE i<999)
INSERT INTO history_edges(edge_id, source_id, target_id, first_traffic_at, last_traffic_at)
SELECT
  printf('edge_%%04d', i),
  printf('n_%%03d', i %% 250),
  printf('n_%%03d', (i %% 250 + i / 250 + 1) %% 250),
  '%s', '%s'
FROM edge;`, formatTime(now.Add(-7*24*time.Hour)), formatTime(now))
}

func scaleRawTrafficSQL(start time.Time) string {
	return fmt.Sprintf(`
WITH RECURSIVE
edge(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM edge WHERE i<999),
bucket(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM bucket WHERE i<359),
observer(i) AS (VALUES(0),(1))
INSERT INTO traffic_buckets(edge_id, bucket_start, source_id, target_id, observer_id, tx_bytes, rx_bytes, a_to_b_bytes, b_to_a_bytes)
SELECT
  printf('edge_%%04d', edge.i),
  strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', '%s', printf('+%%d seconds', bucket.i * 10)),
  printf('n_%%03d', edge.i %% 250),
  printf('n_%%03d', (edge.i %% 250 + edge.i / 250 + 1) %% 250),
  CASE observer.i WHEN 0 THEN printf('n_%%03d', edge.i %% 250) ELSE printf('n_%%03d', (edge.i %% 250 + edge.i / 250 + 1) %% 250) END,
  0, 0, 4096 + observer.i, 2048 + observer.i
FROM edge CROSS JOIN bucket CROSS JOIN observer;`, formatTime(start))
}

func scaleMinuteTrafficSQL(start time.Time) string {
	return fmt.Sprintf(`
WITH RECURSIVE
edge(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM edge WHERE i<999),
hour(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM hour WHERE i<47),
minute(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM minute WHERE i<59)
INSERT INTO traffic_rollup_minute(edge_id, bucket_start, source_id, target_id, a_to_b_bytes, b_to_a_bytes)
SELECT
  printf('edge_%%04d', edge.i),
  strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', '%s', printf('+%%d minutes', hour.i * 60 + minute.i)),
  printf('n_%%03d', edge.i %% 250),
  printf('n_%%03d', (edge.i %% 250 + edge.i / 250 + 1) %% 250),
  245760, 122880
FROM edge CROSS JOIN hour CROSS JOIN minute;`, formatTime(start))
}

func scaleHourTrafficSQL(start time.Time) string {
	return fmt.Sprintf(`
WITH RECURSIVE
edge(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM edge WHERE i<999),
hour(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM hour WHERE i<167)
INSERT INTO traffic_rollup_hour(edge_id, bucket_start, source_id, target_id, a_to_b_bytes, b_to_a_bytes)
SELECT
  printf('edge_%%04d', edge.i),
  strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', '%s', printf('+%%d hours', hour.i)),
  printf('n_%%03d', edge.i %% 250),
  printf('n_%%03d', (edge.i %% 250 + edge.i / 250 + 1) %% 250),
  14745600, 7372800
FROM edge CROSS JOIN hour;`, formatTime(start))
}

func scalePathEventSQL(at time.Time) string {
	return fmt.Sprintf(`
WITH RECURSIVE edge(i) AS (VALUES(0) UNION ALL SELECT i+1 FROM edge WHERE i<999)
INSERT INTO path_events(edge_id, observed_at, path, observations)
SELECT printf('edge_%%04d', i), '%s', '{"kind":"direct"}', '[]' FROM edge;`, formatTime(at))
}
