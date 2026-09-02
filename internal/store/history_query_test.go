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

func TestHistoryQueriesHideSystemTelemetryUnlessExplicitlyIncluded(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_app", NodeIdentity: domain.NodeIdentity{StableNodeID: "app", Hostname: "App"}},
		{ID: "n_peer", NodeIdentity: domain.NodeIdentity{StableNodeID: "peer", Hostname: "Peer"}},
		{ID: "n_control", NodeIdentity: domain.NodeIdentity{StableNodeID: "control", Hostname: "Tailpath"}},
		{ID: "n_control_old", NodeIdentity: domain.NodeIdentity{DiscoKey: "old-control"}},
	}}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	for index, record := range []domain.AcceptedTraffic{
		{EdgeID: "n_app--n_peer", SourceID: "n_app", TargetID: "n_peer", ObserverID: "n_app", AToBBytes: 10, ReceivedAt: now.Add(-time.Minute)},
		{EdgeID: "n_app--n_control_old", SourceID: "n_app", TargetID: "n_control_old", ObserverID: "n_app", AToBBytes: 20, ReceivedAt: now.Add(-30 * time.Second)},
	} {
		if _, err := database.Record(context.Background(), sampleReport(record.ReceivedAt, record.ReceivedAt, fmt.Sprintf("system-%d", index)), record.ReceivedAt, nil, []domain.AcceptedTraffic{record}, nil); err != nil {
			t.Fatal(err)
		}
	}
	metadata.Redirects = map[string]string{"n_control_old": "n_control"}
	metadata.ControlNodeIDs = []string{"n_control"}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}

	nodes, err := database.HistoryNodes(context.Background(), domain.History15Minutes, now)
	if err != nil || len(nodes.Nodes) != 2 {
		t.Fatalf("default nodes = %#v, err=%v", nodes.Nodes, err)
	}
	page, err := database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{Window: domain.History15Minutes}, now)
	if err != nil || len(page.Edges) != 1 || page.Edges[0].SystemTelemetry {
		t.Fatalf("default page = %#v, err=%v", page, err)
	}
	if _, found, err := database.EdgeHistoryWindow(context.Background(), "n_app--n_control_old", domain.History15Minutes, now); err != nil || found {
		t.Fatalf("default system detail found=%v err=%v", found, err)
	}

	nodes, err = database.HistoryNodes(context.Background(), domain.History15Minutes, now, true)
	if err != nil || len(nodes.Nodes) != 3 {
		t.Fatalf("diagnostic nodes = %#v, err=%v", nodes.Nodes, err)
	}
	page, err = database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{Window: domain.History15Minutes, IncludeSystemTelemetry: true}, now)
	if err != nil || len(page.Edges) != 2 || !page.Edges[0].SystemTelemetry {
		t.Fatalf("diagnostic page = %#v, err=%v", page, err)
	}
	history, found, err := database.EdgeHistoryWindow(context.Background(), "n_app--n_control_old", domain.History15Minutes, now, true)
	if err != nil || !found || !history.SystemTelemetry || history.EdgeID != "n_app--n_control" {
		t.Fatalf("diagnostic detail = %#v, found=%v err=%v", history, found, err)
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
	wantLastTrafficAt := now.Add(-2 * time.Minute)
	if history.LastTrafficAt == nil || !history.LastTrafficAt.Equal(wantLastTrafficAt) {
		t.Fatalf("last traffic = %v, want %s", history.LastTrafficAt, wantLastTrafficAt)
	}

	empty, found, err := database.EdgeHistoryWindow(ctx, "n_b--n_c", domain.History7Days, now)
	if err != nil || !found || len(empty.Traffic) != 0 || len(empty.PathEvents) != 0 {
		t.Fatalf("known empty history = %#v, found=%v err=%v", empty, found, err)
	}
	if empty.LastTrafficAt != nil {
		t.Fatalf("known empty last traffic = %s, want nil", empty.LastTrafficAt)
	}
	anchorOnly, found, err := database.EdgeHistoryWindow(ctx, "n_a--n_b", domain.History1Hour, now.Add(2*time.Hour))
	if err != nil || !found {
		t.Fatalf("anchor-only found=%v err=%v", found, err)
	}
	if anchorOnly.Traffic == nil || anchorOnly.PathEvents == nil {
		t.Fatalf("anchor-only collections must be non-nil: %#v", anchorOnly)
	}
	if anchorOnly.PathAnchor == nil || anchorOnly.PathAnchor.Observations == nil {
		t.Fatalf("anchor-only provenance must be non-nil: %#v", anchorOnly.PathAnchor)
	}
	if _, found, err := database.EdgeHistoryWindow(ctx, "missing", domain.History1Hour, now); err != nil || found {
		t.Fatalf("unknown edge found=%v err=%v", found, err)
	}
}

func TestEdgeHistoryWindowReturnsObserverAndRelayNodeReferences(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "Alpha"}, IdentityStatus: domain.IdentityResolved},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "Beta"}, IdentityStatus: domain.IdentityResolved},
		{ID: "n_observer", NodeIdentity: domain.NodeIdentity{StableNodeID: "observer", Hostname: "Observer"}, IdentityStatus: domain.IdentityResolved},
		{ID: "n_relay", NodeIdentity: domain.NodeIdentity{StableNodeID: "relay-known", Hostname: "Relay Hangzhou"}, IdentityStatus: domain.IdentityResolved},
	}}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	vni := int64(2120)
	knownRelay := domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-known", PeerRelayVNI: &vni}
	unknownRelay := domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-unknown"}
	transition := domain.PathTransition{
		EdgeID: "n_a--n_b", ObservedAt: now.Add(-time.Minute), Path: knownRelay,
		Conflicts:    []domain.PathObservation{unknownRelay},
		Observations: []domain.ObservationProvenance{{ObserverID: "n_observer", Path: knownRelay, CollectedAt: now, ReceivedAt: now}},
	}
	traffic := domain.AcceptedTraffic{EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_observer", AToBBytes: 10, ReceivedAt: now.Add(-time.Minute)}
	if _, err := database.Record(context.Background(), sampleReport(now.Add(-time.Minute), now, "related-nodes"), now, nil, []domain.AcceptedTraffic{traffic}, []domain.PathTransition{transition}); err != nil {
		t.Fatal(err)
	}
	history, found, err := database.EdgeHistoryWindow(context.Background(), "n_a--n_b", domain.History15Minutes, now)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	byStableID := make(map[string]domain.HistoryNodeReference)
	for _, node := range history.RelatedNodes {
		byStableID[node.StableNodeID] = node
	}
	if len(history.RelatedNodes) != 5 || byStableID["observer"].Label != "Observer" || byStableID["relay-known"].Label != "Relay Hangzhou" {
		t.Fatalf("related nodes = %#v", history.RelatedNodes)
	}
	if unresolved := byStableID["relay-unknown"]; unresolved.ID != "stable:relay-unknown" || unresolved.IdentityStatus != domain.IdentityPartial {
		t.Fatalf("unresolved relay = %#v", unresolved)
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

func TestEdgeHistoryWindowUsesExactTrafficTimeAcrossCoarseBuckets(t *testing.T) {
	for _, item := range []struct {
		name   string
		window domain.HistoryWindow
		age    time.Duration
	}{
		{name: "24 hour", window: domain.History24Hours, age: 2*time.Hour + 37*time.Second},
		{name: "7 day", window: domain.History7Days, age: 72*time.Hour + 17*time.Minute + 23*time.Second},
	} {
		t.Run(item.name, func(t *testing.T) {
			database, now := coverageHistoryDatabase(t)
			at := now.Add(-item.age)
			recordCoverageTraffic(t, database, at, 42)

			page, err := database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{Window: item.window}, now)
			if err != nil || len(page.Edges) != 1 {
				t.Fatalf("history list = %#v, err=%v", page, err)
			}
			history, found, err := database.EdgeHistoryWindow(context.Background(), "n_a--n_b", item.window, now)
			if err != nil || !found || len(history.Traffic) != 1 {
				t.Fatalf("history detail = %#v, found=%v err=%v", history, found, err)
			}
			if history.LastTrafficAt == nil || !history.LastTrafficAt.Equal(at) {
				t.Fatalf("last traffic = %v, want %s", history.LastTrafficAt, at)
			}
			if !page.Edges[0].LastTrafficAt.Equal(*history.LastTrafficAt) {
				t.Fatalf("list last traffic %s != detail %s", page.Edges[0].LastTrafficAt, history.LastTrafficAt)
			}
			if history.Traffic[0].BucketStart.Equal(*history.LastTrafficAt) {
				t.Fatalf("exact traffic time unexpectedly equals %s bucket start", item.window)
			}
		})
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

func TestRelayFallbackAndProvenanceSurviveSevenDayRetention(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 12, 5, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, IdentityStatus: domain.IdentityResolved, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, IdentityStatus: domain.IdentityResolved, LastEvidenceAt: now},
		{ID: "n_relay", NodeIdentity: domain.NodeIdentity{StableNodeID: "relay", Hostname: "Relay"}, IdentityStatus: domain.IdentityResolved, LastEvidenceAt: now},
	}}
	if err := database.SaveHistoryMetadata(ctx, metadata, now); err != nil {
		t.Fatal(err)
	}
	vni := int64(9)
	path := domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni}
	anchorAt := now.Add(-8 * 24 * time.Hour)
	transition := domain.PathTransition{
		EdgeID: "n_a--n_b", ObservedAt: anchorAt, Path: path,
		Observations: []domain.ObservationProvenance{{
			ObserverID: "n_relay", Path: path, CollectedAt: anchorAt, ReceivedAt: anchorAt,
			RelaySession: &domain.RelaySessionProvenance{
				SessionID: "retained-session", VNI: vni,
				SourceIdentityStatus: domain.IdentityResolved, TargetIdentityStatus: domain.IdentityResolved,
			},
		}},
	}
	if _, err := database.Record(ctx, sampleReport(anchorAt, anchorAt, "relay-anchor"), anchorAt, nil, nil, []domain.PathTransition{transition}); err != nil {
		t.Fatal(err)
	}
	trafficAt := now.Add(-6*24*time.Hour - 30*time.Minute)
	traffic := []domain.AcceptedTraffic{{
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_relay",
		AToBBytes: 321, BToABytes: 123, ReceivedAt: trafficAt,
	}}
	if _, err := database.Record(ctx, sampleReport(trafficAt, trafficAt, "relay-traffic"), trafficAt, nil, traffic, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Maintain(ctx, now); err != nil {
		t.Fatal(err)
	}
	history, found, err := database.EdgeHistoryWindow(ctx, "n_a--n_b", domain.History7Days, now)
	if err != nil || !found {
		t.Fatalf("seven-day relay history found=%v err=%v", found, err)
	}
	var aToB, bToA int64
	for _, bucket := range history.Traffic {
		aToB += bucket.AToBBytes
		bToA += bucket.BToABytes
	}
	if aToB != 321 || bToA != 123 {
		t.Fatalf("third-party relay rollup = %d/%d, want 321/123", aToB, bToA)
	}
	if history.PathAnchor == nil || history.PathAnchor.Path.PeerRelayVNI == nil ||
		*history.PathAnchor.Path.PeerRelayVNI != vni || len(history.PathAnchor.Observations) != 1 ||
		history.PathAnchor.Observations[0].RelaySession == nil ||
		history.PathAnchor.Observations[0].RelaySession.SessionID != "retained-session" {
		t.Fatalf("seven-day relay anchor = %#v", history.PathAnchor)
	}
}

func TestEdgeHistoryWindowUsesRetainedRawWhenRollupsAreMissing(t *testing.T) {
	database, now := coverageHistoryDatabase(t)
	recordCoverageTraffic(t, database, now.Add(-2*time.Hour), 42)

	assertHistoryListDetailTotals(t, database, domain.History6Hours, now, 42)
}

func TestEdgeHistoryWindowUsesCoverageWithoutTierOverlap(t *testing.T) {
	t.Run("lagging minute rollup", func(t *testing.T) {
		database, now := coverageHistoryDatabase(t)
		coveredAt := now.Add(-3 * time.Hour)
		uncoveredAt := now.Add(-90 * time.Minute)
		recordCoverageTraffic(t, database, coveredAt, 400)
		recordCoverageTraffic(t, database, uncoveredAt, 30)
		insertCoverageRollup(t, database, "traffic_rollup_minute", coveredAt, 40)
		insertCoverageRollup(t, database, "traffic_rollup_minute", uncoveredAt, 300)
		insertCoverageCursor(t, database, "minute", now.Add(-2*time.Hour))

		assertHistoryListDetailTotals(t, database, domain.History6Hours, now, 70)
	})

	t.Run("lagging hour and minute rollups", func(t *testing.T) {
		database, now := coverageHistoryDatabase(t)
		hourAt := now.Add(-6 * 24 * time.Hour)
		minuteFallbackAt := now.Add(-60 * time.Hour)
		minuteAt := now.Add(-3 * time.Hour)
		rawFallbackAt := now.Add(-90 * time.Minute)
		for _, sample := range []struct {
			at    time.Time
			bytes int64
		}{
			{hourAt, 1_000},
			{minuteFallbackAt, 2_000},
			{minuteAt, 3_000},
			{rawFallbackAt, 40},
		} {
			recordCoverageTraffic(t, database, sample.at, sample.bytes)
		}
		insertCoverageRollup(t, database, "traffic_rollup_hour", hourAt, 10)
		insertCoverageRollup(t, database, "traffic_rollup_hour", minuteFallbackAt, 200)
		insertCoverageRollup(t, database, "traffic_rollup_minute", minuteFallbackAt, 20)
		insertCoverageRollup(t, database, "traffic_rollup_minute", minuteAt, 30)
		insertCoverageRollup(t, database, "traffic_rollup_minute", rawFallbackAt, 400)
		insertCoverageCursor(t, database, "hour", now.Add(-72*time.Hour))
		insertCoverageCursor(t, database, "minute", now.Add(-2*time.Hour))

		assertHistoryListDetailTotals(t, database, domain.History7Days, now, 100)
	})
}

func coverageHistoryDatabase(t *testing.T) (*SQLite, time.Time) {
	t.Helper()
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, LastEvidenceAt: now},
	}}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	return database, now
}

func recordCoverageTraffic(t *testing.T, database *SQLite, at time.Time, bytes int64) {
	t.Helper()
	traffic := []domain.AcceptedTraffic{{
		EdgeID: "n_a--n_b", SourceID: "n_a", TargetID: "n_b", ObserverID: "n_a",
		AToBBytes: bytes, ReceivedAt: at,
	}}
	if _, err := database.Record(context.Background(), sampleReport(at, at, formatTime(at)), at, nil, traffic, nil); err != nil {
		t.Fatal(err)
	}
}

func insertCoverageRollup(t *testing.T, database *SQLite, table string, at time.Time, bytes int64) {
	t.Helper()
	if _, err := database.db.Exec(`INSERT INTO `+table+`
		(edge_id, bucket_start, source_id, target_id, a_to_b_bytes, b_to_a_bytes)
		VALUES ('n_a--n_b', ?, 'n_a', 'n_b', ?, 0)`, formatTime(at), bytes); err != nil {
		t.Fatal(err)
	}
}

func insertCoverageCursor(t *testing.T, database *SQLite, name string, at time.Time) {
	t.Helper()
	if _, err := database.db.Exec(`INSERT INTO history_maintenance(name, value) VALUES (?, ?)`, name, formatTime(at)); err != nil {
		t.Fatal(err)
	}
}

func assertHistoryListDetailTotals(t *testing.T, database *SQLite, window domain.HistoryWindow, now time.Time, want int64) {
	t.Helper()
	page, err := database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{Window: window}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) != 1 || page.Edges[0].AToBBytes != want || page.Edges[0].BToABytes != 0 {
		t.Fatalf("history list = %#v, want one edge with %d/0 bytes", page.Edges, want)
	}
	history, found, err := database.EdgeHistoryWindow(context.Background(), "n_a--n_b", window, now)
	if err != nil || !found {
		t.Fatalf("history detail found=%v err=%v", found, err)
	}
	var aToB, bToA int64
	for _, point := range history.Traffic {
		aToB += point.AToBBytes
		bToA += point.BToABytes
	}
	if aToB != want || bToA != 0 {
		t.Fatalf("history detail totals = %d/%d, want %d/0; traffic=%#v", aToB, bToA, want, history.Traffic)
	}
}

func TestHistoryEdgeMapDeduplicatesAliasesAndPreservesDirection(t *testing.T) {
	database, err := Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
	metadata := domain.HistoryMetadata{Nodes: []domain.TopologyNode{
		{ID: "n_a", NodeIdentity: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, LastEvidenceAt: now},
		{ID: "n_b", NodeIdentity: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, LastEvidenceAt: now},
		{ID: "n_old", NodeIdentity: domain.NodeIdentity{DiscoKey: "old"}, LastEvidenceAt: now},
	}}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now); err != nil {
		t.Fatal(err)
	}
	record := func(reportID, edgeID, sourceID, targetID string, aToB, bToA int64) {
		t.Helper()
		traffic := []domain.AcceptedTraffic{{
			EdgeID: edgeID, SourceID: sourceID, TargetID: targetID, ObserverID: sourceID,
			AToBBytes: aToB, BToABytes: bToA, ReceivedAt: now,
		}}
		if _, err := database.Record(context.Background(), sampleReport(now, now, reportID), now, nil, traffic, nil); err != nil {
			t.Fatal(err)
		}
	}
	record("before-merge", "n_b--n_old", "n_b", "n_old", 100, 20)
	record("after-merge", "n_a--n_b", "n_a", "n_b", 30, 90)
	metadata.Redirects = map[string]string{"n_old": "n_a"}
	if err := database.SaveHistoryMetadata(context.Background(), metadata, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	page, err := database.HistoryEdges(context.Background(), domain.HistoryEdgeQuery{Window: domain.History15Minutes}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) != 1 || page.Edges[0].EdgeID != "n_a--n_b" ||
		page.Edges[0].AToBBytes != 30 || page.Edges[0].BToABytes != 100 {
		t.Fatalf("logical alias summary = %#v, want one 30/100 edge", page.Edges)
	}
	history, found, err := database.EdgeHistoryWindow(context.Background(), "n_b--n_old", domain.History15Minutes, now.Add(time.Minute))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(history.Traffic) != 1 || history.Traffic[0].AToBBytes != 30 || history.Traffic[0].BToABytes != 100 {
		t.Fatalf("logical alias detail = %#v, want one 30/100 bucket", history.Traffic)
	}
	var logicalID string
	var reversed bool
	if err := database.db.QueryRow(`SELECT logical_edge_id, direction_reversed FROM history_edge_map WHERE physical_edge_id = 'n_b--n_old'`).Scan(&logicalID, &reversed); err != nil {
		t.Fatal(err)
	}
	if logicalID != "n_a--n_b" || !reversed {
		t.Fatalf("physical mapping = logical %q reversed=%v", logicalID, reversed)
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
	if _, err := database.db.Exec(`INSERT INTO history_edges(edge_id, source_id, target_id, first_traffic_at, last_traffic_at) VALUES ('n_b--n_c', 'n_b', 'n_c', ?, ?)`, formatTime(old), formatTime(old)); err != nil {
		t.Fatal(err)
	}
	tx, err := database.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rebuildHistoryEdgeMap(context.Background(), tx, now); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return database, now
}
