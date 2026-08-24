package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

const (
	rawTrafficRetention    = time.Hour
	minuteTrafficRetention = 48 * time.Hour
	hourTrafficRetention   = 7 * 24 * time.Hour
	minuteRollupGrace      = 2 * time.Minute
)

type logicalTraffic struct {
	edgeID, sourceID, targetID string
	aToBBytes, bToABytes       int64
}

func rollupHistory(ctx context.Context, tx *sql.Tx, now time.Time) error {
	now = now.UTC()
	minuteEnd := now.Add(-minuteRollupGrace).Truncate(time.Minute)
	if err := rollupIntervals(ctx, tx, "minute", "traffic_buckets", "traffic_rollup_minute", time.Minute, minuteEnd, true); err != nil {
		return err
	}
	minuteCoverage, ok, err := maintenanceCursor(ctx, tx, "minute")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("minute rollup completed without a coverage cursor")
	}
	hourEnd := earlierTime(now.Truncate(time.Hour), minuteCoverage.Truncate(time.Hour))
	if err := rollupIntervals(ctx, tx, "hour", "traffic_rollup_minute", "traffic_rollup_hour", time.Hour, hourEnd, false); err != nil {
		return err
	}
	hourCoverage, ok, err := maintenanceCursor(ctx, tx, "hour")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("hour rollup completed without a coverage cursor")
	}
	if err := deleteCoveredTraffic(ctx, tx, now, minuteCoverage, hourCoverage); err != nil {
		return err
	}
	return maintainPathAnchors(ctx, tx, now.Add(-hourTrafficRetention))
}

func deleteCoveredTraffic(ctx context.Context, tx *sql.Tx, now, minuteCoverage, hourCoverage time.Time) error {
	for _, deletion := range []struct {
		table  string
		cutoff time.Time
	}{
		{"traffic_buckets", earlierTime(now.Add(-rawTrafficRetention), minuteCoverage)},
		{"traffic_rollup_minute", earlierTime(now.Add(-minuteTrafficRetention), hourCoverage)},
		{"traffic_rollup_hour", now.Add(-hourTrafficRetention)},
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+deletion.table+" WHERE bucket_start < ?", formatTime(deletion.cutoff)); err != nil {
			return err
		}
	}
	return nil
}

func earlierTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func rollupIntervals(
	ctx context.Context,
	tx *sql.Tx,
	cursorName, sourceTable, targetTable string,
	interval time.Duration,
	end time.Time,
	deduplicateObservers bool,
) error {
	start, ok, err := maintenanceCursor(ctx, tx, cursorName)
	if err != nil {
		return err
	}
	if !ok {
		var rawStart sql.NullString
		if err := tx.QueryRowContext(ctx, "SELECT MIN(bucket_start) FROM "+sourceTable).Scan(&rawStart); err != nil {
			return err
		}
		if !rawStart.Valid {
			return saveMaintenanceCursor(ctx, tx, cursorName, end)
		}
		start, err = time.Parse(time.RFC3339Nano, rawStart.String)
		if err != nil {
			return err
		}
		start = start.UTC().Truncate(interval)
		if !start.Before(end) {
			return saveMaintenanceCursor(ctx, tx, cursorName, end)
		}
	}
	for start.Before(end) {
		next := start.Add(interval)
		var records []logicalTraffic
		if deduplicateObservers {
			records, err = logicalRawTraffic(ctx, tx, start, next)
		} else {
			records, err = summedLogicalTraffic(ctx, tx, sourceTable, start, next)
		}
		if err != nil {
			return err
		}
		for _, record := range records {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO `+targetTable+`(edge_id, bucket_start, source_id, target_id, a_to_b_bytes, b_to_a_bytes)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(edge_id, bucket_start) DO UPDATE SET
				  source_id = excluded.source_id, target_id = excluded.target_id,
				  a_to_b_bytes = excluded.a_to_b_bytes, b_to_a_bytes = excluded.b_to_a_bytes`,
				record.edgeID, formatTime(start), record.sourceID, record.targetID,
				record.aToBBytes, record.bToABytes); err != nil {
				return err
			}
		}
		if err := saveMaintenanceCursor(ctx, tx, cursorName, next); err != nil {
			return err
		}
		start = next
	}
	return nil
}

func logicalRawTraffic(ctx context.Context, tx *sql.Tx, start, end time.Time) ([]logicalTraffic, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT edge_id, MIN(source_id), MIN(target_id),
		  COALESCE(
		    SUM(CASE WHEN observer_id = source_id THEN a_to_b_bytes END),
		    SUM(CASE WHEN observer_id = target_id THEN a_to_b_bytes END),
		    MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN a_to_b_bytes END), 0),
		  COALESCE(
		    SUM(CASE WHEN observer_id = target_id THEN b_to_a_bytes END),
		    SUM(CASE WHEN observer_id = source_id THEN b_to_a_bytes END),
		    MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN b_to_a_bytes END), 0)
		FROM traffic_buckets WHERE bucket_start >= ? AND bucket_start < ?
		GROUP BY edge_id, bucket_start ORDER BY edge_id, bucket_start`, formatTime(start), formatTime(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byEdge := make(map[string]*logicalTraffic)
	for rows.Next() {
		var record logicalTraffic
		if err := rows.Scan(&record.edgeID, &record.sourceID, &record.targetID, &record.aToBBytes, &record.bToABytes); err != nil {
			return nil, err
		}
		if current := byEdge[record.edgeID]; current != nil {
			current.aToBBytes += record.aToBBytes
			current.bToABytes += record.bToABytes
		} else {
			copy := record
			byEdge[record.edgeID] = &copy
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedLogicalTraffic(byEdge), nil
}

func summedLogicalTraffic(ctx context.Context, tx *sql.Tx, table string, start, end time.Time) ([]logicalTraffic, error) {
	rows, err := tx.QueryContext(ctx, `SELECT edge_id, MIN(source_id), MIN(target_id), SUM(a_to_b_bytes), SUM(b_to_a_bytes)
		FROM `+table+` WHERE bucket_start >= ? AND bucket_start < ? GROUP BY edge_id ORDER BY edge_id`,
		formatTime(start), formatTime(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []logicalTraffic
	for rows.Next() {
		var record logicalTraffic
		if err := rows.Scan(&record.edgeID, &record.sourceID, &record.targetID, &record.aToBBytes, &record.bToABytes); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func sortedLogicalTraffic(records map[string]*logicalTraffic) []logicalTraffic {
	result := make([]logicalTraffic, 0, len(records))
	for _, record := range records {
		result = append(result, *record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].edgeID < result[j].edgeID })
	return result
}

func maintenanceCursor(ctx context.Context, tx *sql.Tx, name string) (time.Time, bool, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value FROM history_maintenance WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	return value, true, err
}

func saveMaintenanceCursor(ctx context.Context, tx *sql.Tx, name string, value time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO history_maintenance(name, value) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value`, name, formatTime(value))
	return err
}

func maintainPathAnchors(ctx context.Context, tx *sql.Tx, cutoff time.Time) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM path_events
		WHERE observed_at < ? AND id NOT IN (
		  SELECT anchor.id FROM path_events AS anchor
		  JOIN history_edge_map AS anchor_map ON anchor_map.physical_edge_id = anchor.edge_id
		  WHERE EXISTS (
		      SELECT 1 FROM history_edges AS retained_edge
		      JOIN history_edge_map AS retained_map
		        ON retained_map.physical_edge_id = retained_edge.edge_id
		      WHERE retained_map.logical_edge_id = anchor_map.logical_edge_id
		        AND retained_edge.last_traffic_at >= ?
		    )
		    AND anchor.id = (
		      SELECT previous.id FROM path_events AS previous
		      JOIN history_edge_map AS previous_map
		        ON previous_map.physical_edge_id = previous.edge_id
		      WHERE previous_map.logical_edge_id = anchor_map.logical_edge_id
		        AND previous.observed_at < ?
		      ORDER BY previous.observed_at DESC, previous.id DESC LIMIT 1
		    )
		)`, formatTime(cutoff), formatTime(cutoff), formatTime(cutoff))
	return err
}
