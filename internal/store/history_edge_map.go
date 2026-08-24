package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type physicalHistoryEdge struct {
	physicalID string
	sourceID   string
	targetID   string
	updatedAt  time.Time
}

func upsertHistoryEdgeMapping(
	ctx context.Context,
	tx *sql.Tx,
	physicalID, sourceID, targetID string,
	updatedAt time.Time,
) error {
	redirects, err := loadCanonicalRedirects(ctx, tx)
	if err != nil {
		return err
	}
	return writeHistoryEdgeMapping(ctx, tx, redirects, physicalHistoryEdge{
		physicalID: physicalID, sourceID: sourceID, targetID: targetID, updatedAt: updatedAt,
	})
}

func rebuildHistoryEdgeMap(ctx context.Context, tx *sql.Tx, updatedAt time.Time) error {
	redirects, err := loadCanonicalRedirects(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT edge_id, source_id, target_id, last_traffic_at FROM history_edges ORDER BY edge_id`)
	if err != nil {
		return err
	}
	var edges []physicalHistoryEdge
	for rows.Next() {
		var edge physicalHistoryEdge
		var rawUpdatedAt string
		if err := rows.Scan(&edge.physicalID, &edge.sourceID, &edge.targetID, &rawUpdatedAt); err != nil {
			rows.Close()
			return err
		}
		edge.updatedAt, err = time.Parse(time.RFC3339Nano, rawUpdatedAt)
		if err != nil {
			rows.Close()
			return err
		}
		if updatedAt.After(edge.updatedAt) {
			edge.updatedAt = updatedAt
		}
		edges = append(edges, edge)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM history_edge_map`); err != nil {
		return err
	}
	for _, edge := range edges {
		if err := writeHistoryEdgeMapping(ctx, tx, redirects, edge); err != nil {
			return err
		}
	}
	return nil
}

func writeHistoryEdgeMapping(ctx context.Context, tx *sql.Tx, redirects map[string]string, edge physicalHistoryEdge) error {
	sourceID := resolveNodeID(redirects, edge.sourceID)
	targetID := resolveNodeID(redirects, edge.targetID)
	if sourceID == targetID {
		_, err := tx.ExecContext(ctx, `DELETE FROM history_edge_map WHERE physical_edge_id = ?`, edge.physicalID)
		return err
	}
	logicalID, logicalSourceID, logicalTargetID := domain.EdgeID(sourceID, targetID)
	reversed := sourceID != logicalSourceID
	_, err := tx.ExecContext(ctx, `
		INSERT INTO history_edge_map(
		  physical_edge_id, logical_edge_id, logical_source_id, logical_target_id,
		  direction_reversed, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(physical_edge_id) DO UPDATE SET
		  logical_edge_id = excluded.logical_edge_id,
		  logical_source_id = excluded.logical_source_id,
		  logical_target_id = excluded.logical_target_id,
		  direction_reversed = excluded.direction_reversed,
		  updated_at = excluded.updated_at`,
		edge.physicalID, logicalID, logicalSourceID, logicalTargetID, reversed, formatTime(edge.updatedAt))
	return err
}

func loadCanonicalRedirects(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT from_node_id, to_node_id FROM canonical_redirects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	redirects := make(map[string]string)
	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			return nil, err
		}
		redirects[fromID] = toID
	}
	return redirects, rows.Err()
}
