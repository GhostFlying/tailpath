package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

var ErrInvalidHistoryCursor = errors.New("invalid history cursor")

type historyEdgeRecord struct {
	edgeID, sourceID, targetID string
	firstTrafficAt             time.Time
	lastTrafficAt              time.Time
}

type historyIndex struct {
	redirects    map[string]string
	nodes        map[string]domain.NodeIdentity
	edgeAlias    map[string]string
	edgeReversed map[string]bool
	edges        map[string]*historyEdgeRecord
}

type storedTrafficPoint struct {
	edgeID      string
	sourceID    string
	targetID    string
	bucketStart time.Time
	aToBBytes   int64
	bToABytes   int64
}

type historyPathSet struct {
	anchor    *domain.PathEvent
	events    []domain.PathEvent
	pathKinds map[domain.PathKind]struct{}
}

func (s *SQLite) HistoryNodes(ctx context.Context, window domain.HistoryWindow, to time.Time) (domain.HistoryNodes, error) {
	if !window.Valid() {
		return domain.HistoryNodes{}, fmt.Errorf("invalid history window %q", window)
	}
	index, err := s.loadHistoryIndex(ctx)
	if err != nil {
		return domain.HistoryNodes{}, err
	}
	from := to.UTC().Add(-window.Duration())
	points, err := s.loadTrafficSummaryPoints(ctx, index, window, from, to.UTC())
	if err != nil {
		return domain.HistoryNodes{}, err
	}
	ids := make(map[string]struct{})
	for edgeID := range points {
		edge := index.edges[edgeID]
		if edge == nil {
			continue
		}
		ids[edge.sourceID] = struct{}{}
		ids[edge.targetID] = struct{}{}
	}
	result := domain.HistoryNodes{Nodes: make([]domain.HistoryNodeReference, 0, len(ids))}
	for id := range ids {
		result.Nodes = append(result.Nodes, historyNodeReference(id, index.nodes[id]))
	}
	sort.Slice(result.Nodes, func(i, j int) bool {
		left, right := strings.ToLower(result.Nodes[i].Label), strings.ToLower(result.Nodes[j].Label)
		if left == right {
			return result.Nodes[i].ID < result.Nodes[j].ID
		}
		return left < right
	})
	if len(result.Nodes) > 250 {
		result.Nodes = result.Nodes[:250]
	}
	return result, nil
}

func (s *SQLite) HistoryEdges(ctx context.Context, query domain.HistoryEdgeQuery, to time.Time) (domain.HistoryEdgePage, error) {
	if !query.Window.Valid() {
		return domain.HistoryEdgePage{}, fmt.Errorf("invalid history window %q", query.Window)
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return domain.HistoryEdgePage{}, fmt.Errorf("history limit must be between 1 and 100")
	}
	index, err := s.loadHistoryIndex(ctx)
	if err != nil {
		return domain.HistoryEdgePage{}, err
	}
	from := to.UTC().Add(-query.Window.Duration())
	points, err := s.loadTrafficSummaryPoints(ctx, index, query.Window, from, to.UTC())
	if err != nil {
		return domain.HistoryEdgePage{}, err
	}
	paths, err := s.loadPathSets(ctx, index, from, to.UTC())
	if err != nil {
		return domain.HistoryEdgePage{}, err
	}
	nodeID := resolveNodeID(index.redirects, query.NodeID)
	summaries := summarizeHistoryEdges(index, points, paths, from, to.UTC())
	filtered := summaries[:0]
	for _, summary := range summaries {
		if nodeID != "" && summary.Source.ID != nodeID && summary.Target.ID != nodeID {
			continue
		}
		if query.Path != "" && !containsPathKind(summary.Paths, query.Path) {
			continue
		}
		filtered = append(filtered, summary)
	}
	if query.Cursor != "" {
		cursor, err := decodeHistoryCursor(query.Cursor)
		if err != nil {
			return domain.HistoryEdgePage{}, err
		}
		start := 0
		for start < len(filtered) && !historySummaryAfterCursor(filtered[start], cursor) {
			start++
		}
		filtered = filtered[start:]
	}
	page := domain.HistoryEdgePage{Edges: filtered}
	if len(page.Edges) > query.Limit {
		page.Edges = page.Edges[:query.Limit]
		page.NextCursor = encodeHistoryCursor(page.Edges[len(page.Edges)-1])
	}
	return page, nil
}

func (s *SQLite) EdgeHistoryWindow(ctx context.Context, edgeID string, window domain.HistoryWindow, to time.Time) (domain.EdgeHistory, bool, error) {
	if !window.Valid() {
		return domain.EdgeHistory{}, false, fmt.Errorf("invalid history window %q", window)
	}
	index, err := s.loadHistoryIndex(ctx)
	if err != nil {
		return domain.EdgeHistory{}, false, err
	}
	canonicalID := index.edgeAlias[edgeID]
	if canonicalID == "" {
		canonicalID = edgeID
	}
	edge := index.edges[canonicalID]
	if edge == nil {
		return domain.EdgeHistory{}, false, nil
	}
	to = to.UTC()
	from := to.Add(-window.Duration())
	sourceEdgeIDs := originalEdgeIDs(index, canonicalID)
	points, err := s.loadTrafficPointsForEdges(ctx, index, window, from, to, sourceEdgeIDs)
	if err != nil {
		return domain.EdgeHistory{}, false, err
	}
	paths, err := s.loadPathSetsForEdges(ctx, index, from, to, sourceEdgeIDs)
	if err != nil {
		return domain.EdgeHistory{}, false, err
	}
	history := domain.EdgeHistory{
		EdgeID: canonicalID, Source: historyNodeReference(edge.sourceID, index.nodes[edge.sourceID]),
		Target: historyNodeReference(edge.targetID, index.nodes[edge.targetID]),
		From:   from, To: to, BucketDurationMS: window.Resolution().Milliseconds(),
		Traffic: []domain.TrafficBucket{}, PathEvents: []domain.PathEvent{},
	}
	for _, point := range points[canonicalID] {
		history.Traffic = append(history.Traffic, domain.TrafficBucket{
			BucketStart: point.bucketStart, AToBBytes: point.aToBBytes, BToABytes: point.bToABytes,
		})
	}
	if len(history.Traffic) > 200 {
		history.TrafficTruncated = true
		history.Traffic = history.Traffic[len(history.Traffic)-200:]
	}
	if pathSet := paths[canonicalID]; pathSet != nil {
		history.PathAnchor = pathSet.anchor
		history.PathEvents = pathSet.events
		if len(history.PathEvents) > 500 {
			history.PathEventsTruncated = true
			history.PathEvents = history.PathEvents[len(history.PathEvents)-500:]
		}
	}
	return history, true, nil
}

func (s *SQLite) loadHistoryIndex(ctx context.Context) (historyIndex, error) {
	index := historyIndex{
		redirects: make(map[string]string), nodes: make(map[string]domain.NodeIdentity),
		edgeAlias: make(map[string]string), edgeReversed: make(map[string]bool),
		edges: make(map[string]*historyEdgeRecord),
	}
	rows, err := s.db.QueryContext(ctx, `SELECT from_node_id, to_node_id FROM canonical_redirects`)
	if err != nil {
		return index, err
	}
	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			rows.Close()
			return index, err
		}
		index.redirects[fromID] = toID
	}
	if err := rows.Close(); err != nil {
		return index, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT node_id, identity FROM nodes`)
	if err != nil {
		return index, err
	}
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			rows.Close()
			return index, err
		}
		var identity domain.NodeIdentity
		if err := json.Unmarshal(payload, &identity); err != nil {
			rows.Close()
			return index, err
		}
		resolvedID := resolveNodeID(index.redirects, id)
		if _, exists := index.nodes[resolvedID]; !exists || resolvedID == id {
			index.nodes[resolvedID] = identity
		}
	}
	if err := rows.Close(); err != nil {
		return index, err
	}
	rows, err = s.db.QueryContext(ctx, `
		SELECT mapping.physical_edge_id, mapping.logical_edge_id,
		  mapping.logical_source_id, mapping.logical_target_id, mapping.direction_reversed,
		  edge.first_traffic_at, edge.last_traffic_at
		FROM history_edge_map AS mapping
		JOIN history_edges AS edge ON edge.edge_id = mapping.physical_edge_id`)
	if err != nil {
		return index, err
	}
	defer rows.Close()
	for rows.Next() {
		var originalID, canonicalID, sourceID, targetID, rawFirst, rawLast string
		var reversed bool
		if err := rows.Scan(&originalID, &canonicalID, &sourceID, &targetID, &reversed, &rawFirst, &rawLast); err != nil {
			return index, err
		}
		index.edgeAlias[originalID] = canonicalID
		index.edgeAlias[canonicalID] = canonicalID
		index.edgeReversed[originalID] = reversed
		first, err := time.Parse(time.RFC3339Nano, rawFirst)
		if err != nil {
			return index, err
		}
		last, err := time.Parse(time.RFC3339Nano, rawLast)
		if err != nil {
			return index, err
		}
		edge := index.edges[canonicalID]
		if edge == nil {
			edge = &historyEdgeRecord{edgeID: canonicalID, sourceID: sourceID, targetID: targetID, firstTrafficAt: first, lastTrafficAt: last}
			index.edges[canonicalID] = edge
		} else {
			if first.Before(edge.firstTrafficAt) {
				edge.firstTrafficAt = first
			}
			if last.After(edge.lastTrafficAt) {
				edge.lastTrafficAt = last
			}
		}
	}
	return index, rows.Err()
}

func (s *SQLite) loadTrafficPoints(ctx context.Context, index historyIndex, window domain.HistoryWindow, from, to time.Time) (map[string][]storedTrafficPoint, error) {
	return s.loadTrafficPointsForEdges(ctx, index, window, from, to, nil)
}

func (s *SQLite) loadTrafficPointsForEdges(ctx context.Context, index historyIndex, window domain.HistoryWindow, from, to time.Time, edgeIDs []string) (map[string][]storedTrafficPoint, error) {
	segments, err := s.detailTrafficSegments(ctx, window, from, to)
	if err != nil {
		return nil, err
	}
	bySourceBucket := make(map[string]storedTrafficPoint)
	for _, segment := range segments {
		points, err := s.queryTrafficLayer(ctx, segment.table, segment.from, segment.to, edgeIDs)
		if err != nil {
			return nil, err
		}
		for _, point := range points {
			canonicalID := index.edgeAlias[point.edgeID]
			if canonicalID == "" {
				continue
			}
			canonicalEdge := index.edges[canonicalID]
			if canonicalEdge != nil && index.edgeReversed[point.edgeID] {
				point.aToBBytes, point.bToABytes = point.bToABytes, point.aToBBytes
			}
			point.edgeID = canonicalID
			key := canonicalID + "\x00" + formatTime(point.bucketStart)
			current := bySourceBucket[key]
			if point.aToBBytes > current.aToBBytes {
				current.aToBBytes = point.aToBBytes
			}
			if point.bToABytes > current.bToABytes {
				current.bToABytes = point.bToABytes
			}
			current.edgeID = canonicalID
			current.bucketStart = point.bucketStart
			bySourceBucket[key] = current
		}
	}
	resolution := window.Resolution()
	grouped := make(map[string]map[time.Time]*storedTrafficPoint)
	for _, point := range bySourceBucket {
		if point.bucketStart.Before(from) || !point.bucketStart.Before(to) {
			continue
		}
		offset := point.bucketStart.Sub(from)
		bucket := from.Add(offset / resolution * resolution)
		if grouped[point.edgeID] == nil {
			grouped[point.edgeID] = make(map[time.Time]*storedTrafficPoint)
		}
		current := grouped[point.edgeID][bucket]
		if current == nil {
			current = &storedTrafficPoint{edgeID: point.edgeID, bucketStart: bucket}
			grouped[point.edgeID][bucket] = current
		}
		current.aToBBytes += point.aToBBytes
		current.bToABytes += point.bToABytes
	}
	result := make(map[string][]storedTrafficPoint, len(grouped))
	for edgeID, buckets := range grouped {
		for _, point := range buckets {
			result[edgeID] = append(result[edgeID], *point)
		}
		sort.Slice(result[edgeID], func(i, j int) bool {
			return result[edgeID][i].bucketStart.Before(result[edgeID][j].bucketStart)
		})
	}
	return result, nil
}

func (s *SQLite) loadTrafficSummaryPoints(ctx context.Context, index historyIndex, window domain.HistoryWindow, from, to time.Time) (map[string][]storedTrafficPoint, error) {
	result := make(map[string][]storedTrafficPoint)
	segments, err := s.summaryTrafficSegments(ctx, from, to)
	if err != nil {
		return nil, err
	}
	for _, segment := range segments {
		points, err := s.queryTrafficSummaryLayer(ctx, segment.table, segment.from, segment.to)
		if err != nil {
			return nil, err
		}
		for _, point := range points {
			canonicalID := point.edgeID
			if canonicalID == "" {
				continue
			}
			current := storedTrafficPoint{edgeID: canonicalID}
			if existing := result[canonicalID]; len(existing) != 0 {
				current = existing[0]
			}
			current.aToBBytes += point.aToBBytes
			current.bToABytes += point.bToABytes
			if point.bucketStart.After(current.bucketStart) {
				current.bucketStart = point.bucketStart
			}
			result[canonicalID] = []storedTrafficPoint{current}
		}
	}
	return result, nil
}

func (s *SQLite) summaryTrafficSegments(ctx context.Context, from, to time.Time) ([]trafficSegment, error) {
	hourEnd, err := s.maintenanceCoverage(ctx, "hour", from, to)
	if err != nil {
		return nil, err
	}
	minuteEnd, err := s.maintenanceCoverage(ctx, "minute", hourEnd, to)
	if err != nil {
		return nil, err
	}
	segments := make([]trafficSegment, 0, 3)
	if from.Before(hourEnd) {
		segments = append(segments, trafficSegment{"traffic_rollup_hour", from, hourEnd})
	}
	if hourEnd.Before(minuteEnd) {
		segments = append(segments, trafficSegment{"traffic_rollup_minute", hourEnd, minuteEnd})
	}
	if minuteEnd.Before(to) {
		segments = append(segments, trafficSegment{"traffic_buckets", minuteEnd, to})
	}
	return segments, nil
}

func (s *SQLite) maintenanceCoverage(ctx context.Context, name string, lower, upper time.Time) (time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM history_maintenance WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return lower, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	coverage, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	if coverage.Before(lower) {
		return lower, nil
	}
	if coverage.After(upper) {
		return upper, nil
	}
	return coverage, nil
}

type trafficSegment struct {
	table    string
	from, to time.Time
}

func (s *SQLite) detailTrafficSegments(ctx context.Context, window domain.HistoryWindow, from, to time.Time) ([]trafficSegment, error) {
	rawStart := laterTime(from, to.Add(-rawTrafficRetention))
	if window == domain.History15Minutes || window == domain.History1Hour {
		return []trafficSegment{{"traffic_buckets", from, to}}, nil
	}

	minuteFrom := from
	segments := make([]trafficSegment, 0, 3)
	if window == domain.History7Days {
		minuteStart := laterTime(from, to.Add(-minuteTrafficRetention))
		hourEnd, err := s.maintenanceCoverage(ctx, "hour", from, minuteStart)
		if err != nil {
			return nil, err
		}
		if from.Before(hourEnd) {
			segments = append(segments, trafficSegment{"traffic_rollup_hour", from, hourEnd})
		}
		minuteFrom = hourEnd
	}

	minuteEnd, err := s.maintenanceCoverage(ctx, "minute", minuteFrom, rawStart)
	if err != nil {
		return nil, err
	}
	if minuteFrom.Before(minuteEnd) {
		segments = append(segments, trafficSegment{"traffic_rollup_minute", minuteFrom, minuteEnd})
	}
	if minuteEnd.Before(to) {
		segments = append(segments, trafficSegment{"traffic_buckets", minuteEnd, to})
	}
	return segments, nil
}

func (s *SQLite) queryTrafficLayer(ctx context.Context, table string, from, to time.Time, edgeIDs []string) ([]storedTrafficPoint, error) {
	filter, args := edgeFilter(edgeIDs, from, to)
	query := `SELECT edge_id, source_id, target_id, bucket_start, a_to_b_bytes, b_to_a_bytes FROM ` + table + ` WHERE bucket_start >= ? AND bucket_start < ?` + filter + ` ORDER BY edge_id, bucket_start`
	if table == "traffic_buckets" {
		query = `SELECT edge_id, MIN(source_id), MIN(target_id), bucket_start,
		  COALESCE(SUM(CASE WHEN observer_id = source_id THEN a_to_b_bytes END), SUM(CASE WHEN observer_id = target_id THEN a_to_b_bytes END), MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN a_to_b_bytes END), 0),
		  COALESCE(SUM(CASE WHEN observer_id = target_id THEN b_to_a_bytes END), SUM(CASE WHEN observer_id = source_id THEN b_to_a_bytes END), MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN b_to_a_bytes END), 0)
		FROM traffic_buckets WHERE bucket_start >= ? AND bucket_start < ?` + filter + ` GROUP BY edge_id, bucket_start ORDER BY edge_id, bucket_start`
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []storedTrafficPoint
	for rows.Next() {
		var point storedTrafficPoint
		var rawTime string
		if err := rows.Scan(&point.edgeID, &point.sourceID, &point.targetID, &rawTime, &point.aToBBytes, &point.bToABytes); err != nil {
			return nil, err
		}
		point.bucketStart, err = time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *SQLite) queryTrafficSummaryLayer(ctx context.Context, table string, from, to time.Time) ([]storedTrafficPoint, error) {
	query := `SELECT logical_edge_id, MIN(logical_source_id), MIN(logical_target_id),
		  MAX(bucket_start), SUM(a_to_b_bytes), SUM(b_to_a_bytes)
		FROM (
		  SELECT mapping.logical_edge_id, mapping.logical_source_id, mapping.logical_target_id,
		    traffic.bucket_start,
		    MAX(CASE WHEN mapping.direction_reversed = 1 THEN traffic.b_to_a_bytes ELSE traffic.a_to_b_bytes END) AS a_to_b_bytes,
		    MAX(CASE WHEN mapping.direction_reversed = 1 THEN traffic.a_to_b_bytes ELSE traffic.b_to_a_bytes END) AS b_to_a_bytes
		  FROM ` + table + ` AS traffic
		  JOIN history_edge_map AS mapping ON mapping.physical_edge_id = traffic.edge_id
		  WHERE traffic.bucket_start >= ? AND traffic.bucket_start < ?
		  GROUP BY mapping.logical_edge_id, traffic.bucket_start
		) GROUP BY logical_edge_id`
	if table == "traffic_buckets" {
		query = `WITH physical AS (
		  SELECT edge_id, bucket_start,
		    COALESCE(SUM(CASE WHEN observer_id = source_id THEN a_to_b_bytes END), SUM(CASE WHEN observer_id = target_id THEN a_to_b_bytes END), MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN a_to_b_bytes END), 0) AS a_to_b_bytes,
		    COALESCE(SUM(CASE WHEN observer_id = target_id THEN b_to_a_bytes END), SUM(CASE WHEN observer_id = source_id THEN b_to_a_bytes END), MAX(CASE WHEN observer_id != source_id AND observer_id != target_id THEN b_to_a_bytes END), 0) AS b_to_a_bytes
		  FROM traffic_buckets WHERE bucket_start >= ? AND bucket_start < ? GROUP BY edge_id, bucket_start
		), logical AS (
		  SELECT mapping.logical_edge_id, mapping.logical_source_id, mapping.logical_target_id,
		    physical.bucket_start,
		    MAX(CASE WHEN mapping.direction_reversed = 1 THEN physical.b_to_a_bytes ELSE physical.a_to_b_bytes END) AS a_to_b_bytes,
		    MAX(CASE WHEN mapping.direction_reversed = 1 THEN physical.a_to_b_bytes ELSE physical.b_to_a_bytes END) AS b_to_a_bytes
		  FROM physical JOIN history_edge_map AS mapping ON mapping.physical_edge_id = physical.edge_id
		  GROUP BY mapping.logical_edge_id, physical.bucket_start
		)
		SELECT logical_edge_id, MIN(logical_source_id), MIN(logical_target_id),
		  MAX(bucket_start), SUM(a_to_b_bytes), SUM(b_to_a_bytes)
		FROM logical GROUP BY logical_edge_id`
	}
	rows, err := s.db.QueryContext(ctx, query, formatTime(from), formatTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []storedTrafficPoint
	for rows.Next() {
		var point storedTrafficPoint
		var rawTime string
		if err := rows.Scan(&point.edgeID, &point.sourceID, &point.targetID, &rawTime, &point.aToBBytes, &point.bToABytes); err != nil {
			return nil, err
		}
		point.bucketStart, err = time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func edgeFilter(edgeIDs []string, from, to time.Time) (string, []any) {
	args := []any{formatTime(from), formatTime(to)}
	if len(edgeIDs) == 0 {
		return "", args
	}
	placeholders := make([]string, len(edgeIDs))
	for index, edgeID := range edgeIDs {
		placeholders[index] = "?"
		args = append(args, edgeID)
	}
	return " AND edge_id IN (" + strings.Join(placeholders, ",") + ")", args
}

func (s *SQLite) loadPathSets(ctx context.Context, index historyIndex, from, to time.Time) (map[string]*historyPathSet, error) {
	return s.loadPathSetsForEdges(ctx, index, from, to, nil)
}

func (s *SQLite) loadPathSetsForEdges(ctx context.Context, index historyIndex, from, to time.Time, edgeIDs []string) (map[string]*historyPathSet, error) {
	query := `SELECT edge_id, observed_at, path, observations FROM path_events WHERE observed_at < ?`
	args := []any{formatTime(to)}
	if len(edgeIDs) != 0 {
		placeholders := make([]string, len(edgeIDs))
		for index, edgeID := range edgeIDs {
			placeholders[index] = "?"
			args = append(args, edgeID)
		}
		query += " AND edge_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY observed_at, id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*historyPathSet)
	for rows.Next() {
		var originalID, rawTime string
		var rawPath, rawObservations []byte
		if err := rows.Scan(&originalID, &rawTime, &rawPath, &rawObservations); err != nil {
			return nil, err
		}
		edgeID := index.edgeAlias[originalID]
		if edgeID == "" {
			continue
		}
		var event domain.PathEvent
		var err error
		event.ObservedAt, err = time.Parse(time.RFC3339Nano, rawTime)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawPath, &event.Path); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawObservations, &event.Observations); err != nil {
			return nil, err
		}
		for observationIndex := range event.Observations {
			event.Observations[observationIndex].ObserverID = resolveNodeID(index.redirects, event.Observations[observationIndex].ObserverID)
		}
		set := result[edgeID]
		if set == nil {
			set = &historyPathSet{pathKinds: make(map[domain.PathKind]struct{})}
			result[edgeID] = set
		}
		if event.ObservedAt.Before(from) {
			copy := event
			set.anchor = &copy
			continue
		}
		set.events = append(set.events, event)
		set.pathKinds[event.Path.Kind] = struct{}{}
	}
	for _, set := range result {
		if set.anchor != nil {
			set.pathKinds[set.anchor.Path.Kind] = struct{}{}
		}
	}
	return result, rows.Err()
}

func originalEdgeIDs(index historyIndex, canonicalID string) []string {
	result := make([]string, 0, 1)
	for edgeID, resolvedID := range index.edgeAlias {
		if resolvedID == canonicalID {
			result = append(result, edgeID)
		}
	}
	sort.Strings(result)
	return result
}

func summarizeHistoryEdges(index historyIndex, points map[string][]storedTrafficPoint, paths map[string]*historyPathSet, from, to time.Time) []domain.HistoryEdgeSummary {
	result := make([]domain.HistoryEdgeSummary, 0, len(points))
	for edgeID, traffic := range points {
		edge := index.edges[edgeID]
		if edge == nil || len(traffic) == 0 {
			continue
		}
		summary := domain.HistoryEdgeSummary{
			EdgeID: edgeID, Source: historyNodeReference(edge.sourceID, index.nodes[edge.sourceID]),
			Target:        historyNodeReference(edge.targetID, index.nodes[edge.targetID]),
			LastTrafficAt: traffic[len(traffic)-1].bucketStart,
			Paths:         []domain.PathKind{},
		}
		if !edge.lastTrafficAt.Before(from) && edge.lastTrafficAt.Before(to) {
			summary.LastTrafficAt = edge.lastTrafficAt
		}
		for _, point := range traffic {
			summary.AToBBytes += point.aToBBytes
			summary.BToABytes += point.bToABytes
		}
		if set := paths[edgeID]; set != nil {
			for path := range set.pathKinds {
				summary.Paths = append(summary.Paths, path)
			}
			sort.Slice(summary.Paths, func(i, j int) bool { return summary.Paths[i] < summary.Paths[j] })
		}
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastTrafficAt.Equal(result[j].LastTrafficAt) {
			return result[i].EdgeID < result[j].EdgeID
		}
		return result[i].LastTrafficAt.After(result[j].LastTrafficAt)
	})
	return result
}

func historyNodeReference(id string, identity domain.NodeIdentity) domain.HistoryNodeReference {
	label := identity.DisplayName()
	if label == "unknown" || label == "" {
		label = id
	}
	return domain.HistoryNodeReference{ID: id, Label: label, Hostname: identity.Hostname, DNSName: identity.DNSName, OS: identity.OS}
}

func resolveNodeID(redirects map[string]string, id string) string {
	if id == "" {
		return ""
	}
	visited := make(map[string]struct{})
	for redirects[id] != "" {
		if _, cycle := visited[id]; cycle {
			terminal := id
			for candidate := range visited {
				if candidate < terminal {
					terminal = candidate
				}
			}
			return terminal
		}
		visited[id] = struct{}{}
		id = redirects[id]
	}
	return id
}

func containsPathKind(paths []domain.PathKind, candidate domain.PathKind) bool {
	for _, path := range paths {
		if path == candidate {
			return true
		}
	}
	return false
}

type historyCursor struct {
	LastTrafficAt time.Time `json:"t"`
	EdgeID        string    `json:"e"`
}

func encodeHistoryCursor(summary domain.HistoryEdgeSummary) string {
	payload, _ := json.Marshal(historyCursor{LastTrafficAt: summary.LastTrafficAt, EdgeID: summary.EdgeID})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeHistoryCursor(value string) (historyCursor, error) {
	var cursor historyCursor
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(payload, &cursor) != nil || cursor.LastTrafficAt.IsZero() || cursor.EdgeID == "" {
		return cursor, ErrInvalidHistoryCursor
	}
	return cursor, nil
}

func historySummaryAfterCursor(summary domain.HistoryEdgeSummary, cursor historyCursor) bool {
	return summary.LastTrafficAt.Before(cursor.LastTrafficAt) ||
		(summary.LastTrafficAt.Equal(cursor.LastTrafficAt) && summary.EdgeID > cursor.EdgeID)
}

func laterTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}
