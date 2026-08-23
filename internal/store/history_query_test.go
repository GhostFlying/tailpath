package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestHistoryQueriesResolveRedirectsFilterAndPaginate(t *testing.T) {
	database, now := seededHistoryDatabase(t)
	ctx := context.Background()

	nodes, err := database.HistoryNodes(ctx, domain.History15Minutes, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes.Nodes) != 3 || nodes.Nodes[0].Label != "Alpha" {
		t.Fatalf("history nodes = %#v", nodes.Nodes)
	}

	page, err := database.HistoryEdges(ctx, domain.HistoryEdgeQuery{Window: domain.History15Minutes, Limit: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) != 1 || page.Edges[0].EdgeID != "n_a--n_c" || page.NextCursor == "" {
		t.Fatalf("first page = %#v", page)
	}
	page, err = database.HistoryEdges(ctx, domain.HistoryEdgeQuery{Window: domain.History15Minutes, Limit: 1, Cursor: page.NextCursor}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) != 1 || page.Edges[0].EdgeID != "n_a--n_b" || page.NextCursor != "" {
		t.Fatalf("second page = %#v", page)
	}
	if page.Edges[0].AToBBytes != 13 || page.Edges[0].BToABytes != 11 {
		t.Fatalf("redirected directional totals = %d/%d", page.Edges[0].AToBBytes, page.Edges[0].BToABytes)
	}

	filtered, err := database.HistoryEdges(ctx, domain.HistoryEdgeQuery{
		Window: domain.History15Minutes, NodeID: "n_old", Path: domain.PathDERP, Limit: 50,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Edges) != 1 || filtered.Edges[0].EdgeID != "n_a--n_b" {
		t.Fatalf("redirect/path filtered edges = %#v", filtered.Edges)
	}
	if _, err := database.HistoryEdges(ctx, domain.HistoryEdgeQuery{Window: domain.History15Minutes, Cursor: "not-a-cursor", Limit: 50}, now); err != ErrInvalidHistoryCursor {
		t.Fatalf("invalid cursor err = %v", err)
	}
}

func TestEdgeHistoryWindowReturnsAnchorBoundsAndKnownEmpty(t *testing.T) {
	database, now := seededHistoryDatabase(t)
	ctx := context.Background()
	history, found, err := database.EdgeHistoryWindow(ctx, "n_b--n_old", domain.History15Minutes, now)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if history.EdgeID != "n_a--n_b" || history.Source.Label != "Alpha" || history.Target.Label != "Beta" {
		t.Fatalf("history identity = %#v", history)
	}
	if history.BucketDurationMS != (10*time.Second).Milliseconds() || !history.From.Equal(now.Add(-15*time.Minute)) || !history.To.Equal(now) {
		t.Fatalf("history bounds = %#v", history)
	}
	if history.PathAnchor == nil || history.PathAnchor.Path.Kind != domain.PathUnknown {
		t.Fatalf("path anchor = %#v", history.PathAnchor)
	}
	if len(history.PathEvents) != 2 || history.PathEvents[0].Path.Kind != domain.PathDirect || history.PathEvents[1].Path.Kind != domain.PathDERP {
		t.Fatalf("path events = %#v", history.PathEvents)
	}
	var aToB, bToA int64
	for _, point := range history.Traffic {
		aToB += point.AToBBytes
		bToA += point.BToABytes
	}
	if aToB != 13 || bToA != 11 {
		t.Fatalf("detail traffic = %d/%d", aToB, bToA)
	}

	empty, found, err := database.EdgeHistoryWindow(ctx, "n_b--n_c", domain.History7Days, now)
	if err != nil || !found || len(empty.Traffic) != 0 || len(empty.PathEvents) != 0 {
		t.Fatalf("known empty history = %#v, found=%v err=%v", empty, found, err)
	}
	if _, found, err := database.EdgeHistoryWindow(ctx, "missing", domain.History1Hour, now); err != nil || found {
		t.Fatalf("unknown edge found=%v err=%v", found, err)
	}
}

func TestEdgeHistoryWindowCapsPathTransitions(t *testing.T) {
	database, now := seededHistoryDatabase(t)
	path, _ := json.Marshal(domain.PathObservation{Kind: domain.PathDirect})
	for index := range 501 {
		at := now.Add(-10 * time.Minute).Add(time.Duration(index) * time.Millisecond)
		if _, err := database.db.Exec(`INSERT INTO path_events(edge_id, observed_at, path, observations) VALUES ('n_a--n_b', ?, ?, '[]')`, formatTime(at), path); err != nil {
			t.Fatal(err)
		}
	}
	history, found, err := database.EdgeHistoryWindow(context.Background(), "n_a--n_b", domain.History15Minutes, now)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(history.PathEvents) != 500 || !history.PathEventsTruncated {
		t.Fatalf("path cap count=%d truncated=%v", len(history.PathEvents), history.PathEventsTruncated)
	}
}

func TestHistoryWindowResolutionContract(t *testing.T) {
	for _, item := range []struct {
		window     domain.HistoryWindow
		duration   time.Duration
		resolution time.Duration
	}{
		{domain.History15Minutes, 15 * time.Minute, 10 * time.Second},
		{domain.History1Hour, time.Hour, 30 * time.Second},
		{domain.History6Hours, 6 * time.Hour, 3 * time.Minute},
		{domain.History24Hours, 24 * time.Hour, 12 * time.Minute},
		{domain.History7Days, 7 * 24 * time.Hour, time.Hour},
	} {
		if !item.window.Valid() || item.window.Duration() != item.duration || item.window.Resolution() != item.resolution {
			t.Fatalf("window %q = %s/%s", item.window, item.window.Duration(), item.window.Resolution())
		}
	}
	if domain.HistoryWindow("invalid").Valid() {
		t.Fatal("invalid history window accepted")
	}
}

func TestHistorySummaryUsesCompletedRollupsWithoutDoubleCountingRaw(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 24, 12, 2, 30, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, LastEvidenceAt: now},
	}}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	for index, sample := range []struct {
		at    time.Time
		bytes int64
	}{
		{at: now.Add(-2*time.Minute - 20*time.Second), bytes: 10},
		{at: now.Add(-time.Minute - 20*time.Second), bytes: 20},
		{at: now.Add(-20 * time.Second), bytes: 30},
	} {
		traffic := []domain.AcceptedTraffic{{
			EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_a",
			AToBBytes: sample.bytes, ReceivedAt: sample.at,
		}}
		if _, err := database.Record(context.Background(), sampleReport(sample.at, sample.at, fmt.Sprintf("summary-%d", index)), sample.at, nil, traffic, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	page, err := database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{
		Window: domain.History15Minutes,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) != 1 || page.Edges[0].AToBBytes != 60 {
		t.Fatalf("history summary = %#v, want one edge with 60 bytes", page.Edges)
	}
}

func seededHistoryDatabase(t *testing.T) (*SQLite, time.Time) {
	t.Helper()
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{
		Nodes: []domain.TopologyNode{
			{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "Alpha", OS: "linux"}, LastEvidenceAt: now},
			{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "Beta", OS: "macos"}, LastEvidenceAt: now},
			{ID: "n_c", NodeIdentity: domain.NodeIdentity{StableNodeID: "c", Hostname: "Charlie"}, LastEvidenceAt: now},
			{ID: "n_old", NodeIdentity: domain.NodeIdentity{DiscoKey: "old"}, LastEvidenceAt: now.Add(-time.Hour)},
		},
		Redirects: map[string]string{"n_old": "n_a"},
	}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	record := func(id, edgeID, sourceID, targetID string, at time.Time, aToB, bToA int64, pathKind domain.PathKind) {
		t.Helper()
		traffic := []domain.AcceptedTraffic{{
			EdgeID: edgeID, SourceID: sourceID, TargetID: targetID, ObserverID: sourceID,
			AToBBytes: aToB, BToABytes: bToA, ReceivedAt: at,
		}}
		transition := domain.PathTransition{EdgeID: edgeID, ObservedAt: at.Add(-time.Minute), Path: domain.PathObservation{Kind: pathKind}}
		if _, err := database.Record(context.Background(), sampleReport(at, at, id), at, nil, traffic, []domain.PathTransition{transition}); err != nil {
			t.Fatal(err)
		}
	}
	unknownPath, _ := json.Marshal(domain.PathObservation{Kind: domain.PathUnknown})
	if _, err := database.db.Exec(`INSERT INTO path_events(edge_id, observed_at, path, observations) VALUES ('n_b--n_old', ?, ?, '[]')`, formatTime(now.Add(-20*time.Minute)), unknownPath); err != nil {
		t.Fatal(err)
	}
	record("pre-merge", "n_b--n_old", "n_b", "n_old", now.Add(-5*time.Minute), 7, 3, domain.PathDirect)
	record("post-merge", "n_a--n_b", "n_a", "n_b", now.Add(-2*time.Minute), 10, 4, domain.PathDERP)
	record("newest", "n_a--n_c", "n_a", "n_c", now.Add(-time.Minute), 5, 1, domain.PathDirect)
	old := now.Add(-8 * 24 * time.Hour)
	if _, err := database.db.Exec(`INSERT INTO history_edges VALUES ('n_b--n_c', 'n_b', 'n_c', ?, ?)`, formatTime(old), formatTime(old)); err != nil {
		t.Fatal(err)
	}
	return database, now
}
