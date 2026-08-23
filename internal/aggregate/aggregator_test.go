package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestReconcilesDirectionalRatesWithoutAddingObservers(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")

	applySample(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", 200, 100)
	applySample(t, aggregator, "reporter-b", 2, "b", "B", "a", "A", "inventory-b", 100, 200)

	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(topology.Edges))
	}
	edge := topology.Edges[0]
	if edge.AToBBytesPerSecond != 100 || edge.BToABytesPerSecond != 50 {
		t.Fatalf("rates = %.0f/%.0f, want 100/50", edge.AToBBytesPerSecond, edge.BToABytesPerSecond)
	}
	if len(edge.Observations) != 2 {
		t.Fatalf("got %d observations, want 2", len(edge.Observations))
	}
	if len(edge.Conflicts) != 0 {
		t.Fatalf("equivalent direct observations produced conflicts: %#v", edge.Conflicts)
	}
}

func TestDirectEndpointsFromOppositeObserversAreEquivalent(t *testing.T) {
	observations := []domain.ObservationProvenance{
		{ObserverID: "node:a", Path: domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "192.0.2.1:41641"}},
		{ObserverID: "node:b", Path: domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "192.0.2.2:41641"}},
	}
	path, conflicts := reconcilePaths(observations)
	if path.Kind != domain.PathDirect || len(conflicts) != 0 {
		t.Fatalf("path = %#v, conflicts = %#v", path, conflicts)
	}
}

func TestEquivalentDirectEndpointsDoNotCreatePathTransitions(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")

	first, err := aggregator.ApplyAt(sampleReport(
		"reporter-a", 2, "a", "A", "b", "B", "inventory-a",
		domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "192.0.2.2:41641"}, 100, 50,
	), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.PathTransitions) != 1 {
		t.Fatalf("initial sample created %d transitions, want 1", len(first.PathTransitions))
	}

	second, err := aggregator.ApplyAt(sampleReport(
		"reporter-b", 2, "b", "B", "a", "A", "inventory-b",
		domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "198.51.100.1:41641"}, 50, 100,
	), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.PathTransitions) != 0 {
		t.Fatalf("equivalent observer endpoint created transitions: %#v", second.PathTransitions)
	}

	now = now.Add(time.Second)
	changed, err := aggregator.ApplyAt(sampleReport(
		"reporter-a", 3, "a", "A", "b", "B", "inventory-a",
		domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"}, 100, 50,
	), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.PathTransitions) != 1 || changed.PathTransitions[0].Path.Kind != domain.PathDERP {
		t.Fatalf("real path change transitions = %#v, want one DERP event", changed.PathTransitions)
	}
}

func TestActivePeerRelayEdgeRetainsKnownRelayNode(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter-a", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: node("a", "A"), InventoryGeneration: "inventory-a",
			Peers: []domain.PeerObservation{
				{Peer: node("b", "B")},
				{Peer: node("relay-hz", "Relay-HZ")},
			},
		}},
	}
	if _, err := aggregator.ApplyAt(hello, now); err != nil {
		t.Fatal(err)
	}

	now = now.Add(4*time.Minute + time.Second)
	report := sampleReport(
		"reporter-a", 2, "a", "A", "b", "B", "inventory-a",
		domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-hz"}, 100, 50,
	)
	report.CollectedAt = now
	if _, err := aggregator.ApplyAt(report, now); err != nil {
		t.Fatal(err)
	}

	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 || topology.Edges[0].Path.PeerRelayStableNodeID != "relay-hz" {
		t.Fatalf("peer relay edge missing: %#v", topology.Edges)
	}
	for _, candidate := range topology.Nodes {
		if candidate.StableNodeID == "relay-hz" {
			if candidate.DisplayName() != "Relay-HZ" {
				t.Fatalf("relay display name = %q, want Relay-HZ", candidate.DisplayName())
			}
			return
		}
	}
	t.Fatalf("known relay node expired while its edge remained active: %#v", topology.Nodes)
}

func TestRelaySessionCreatesEndpointEdgeWithRelayProvenance(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "relay", ReporterInstanceID: "relay-reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportRelaySessionUpdate,
		RelaySessions: []domain.RelaySessionObservation{{
			Relay: node("relay", "Relay-HZ"), Source: node("a", "A"), Target: node("b", "B"),
			SessionID: "session", VNI: 7,
			SourceToTargetBytes: 1200, TargetToSourceBytes: 400,
			SourceToTargetDelta: 1200, TargetToSourceDelta: 400,
			SampleDurationMS: 2000, LastActive: now,
		}},
	}
	result, err := aggregator.ApplyAt(report, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traffic) != 1 || result.Traffic[0].AToBBytes != 1200 || result.Traffic[0].BToABytes != 400 {
		t.Fatalf("relay directional traffic = %#v", result.Traffic)
	}

	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 {
		t.Fatalf("relay update created %d edges, want one endpoint edge", len(topology.Edges))
	}
	edge := topology.Edges[0]
	if edge.Path.Kind != domain.PathPeerRelay || edge.Path.PeerRelayStableNodeID != "relay" {
		t.Fatalf("relay path = %#v", edge.Path)
	}
	if edge.AToBBytesPerSecond != 600 || edge.BToABytesPerSecond != 200 {
		t.Fatalf("relay rates = %.0f/%.0f, want 600/200", edge.AToBBytesPerSecond, edge.BToABytesPerSecond)
	}
	if len(edge.Observations) != 1 {
		t.Fatalf("relay provenance = %#v", edge.Observations)
	}
	var relayID string
	for _, candidate := range topology.Nodes {
		if candidate.StableNodeID == "relay" {
			relayID = candidate.ID
		}
	}
	if relayID == "" || edge.Observations[0].ObserverID != relayID {
		t.Fatalf("relay observer identity = %q, nodes = %#v", edge.Observations[0].ObserverID, topology.Nodes)
	}
}

func TestPreservesPathConflictsAndPeerRelayWins(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")

	applySampleWithPath(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", domain.PathObservation{Kind: domain.PathDirect})
	applySampleWithPath(t, aggregator, "reporter-b", 2, "b", "B", "a", "A", "inventory-b", domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay"})

	edge := aggregator.Snapshot().Edges[0]
	if edge.Path.Kind != domain.PathPeerRelay {
		t.Fatalf("path = %q, want peer_relay", edge.Path.Kind)
	}
	if len(edge.Conflicts) != 1 || edge.Conflicts[0].Kind != domain.PathDirect {
		t.Fatalf("conflicts = %#v, want direct", edge.Conflicts)
	}
}

func TestHeartbeatDoesNotCreateOrRefreshEdge(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "heartbeat", ReporterInstanceID: "reporter-a",
		Sequence: 2, CollectedAt: now, Kind: domain.ReportObserverHeartbeat,
		Observers: []domain.ObserverReport{{Observer: node("a", "A"), InventoryGeneration: "inventory-a"}},
	}
	if _, err := aggregator.Apply(report); err != nil {
		t.Fatal(err)
	}
	if got := len(aggregator.Snapshot().Edges); got != 0 {
		t.Fatalf("heartbeat created %d edges", got)
	}
}

func TestEmptySnapshotUsesEmptyCollections(t *testing.T) {
	aggregator := newTestAggregator(func() time.Time {
		return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	})
	topology := aggregator.Snapshot()
	if topology.Nodes == nil || topology.Edges == nil || topology.Observers == nil {
		t.Fatalf("empty topology contains nil collections: %#v", topology)
	}
}

func TestNewestReceivedPathWinsAndStaleRateIsNotReused(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")
	applySampleWithPathAndDeltas(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a",
		domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay"}, 200, 100)

	now = now.Add(11 * time.Second)
	applySampleWithPathAndDeltas(t, aggregator, "reporter-b", 2, "b", "B", "a", "A", "inventory-b",
		domain.PathObservation{Kind: domain.PathDirect}, 80, 40)

	edge := aggregator.Snapshot().Edges[0]
	if edge.Path.Kind != domain.PathDirect {
		t.Fatalf("path = %q, want latest direct observation", edge.Path.Kind)
	}
	if edge.AToBBytesPerSecond != 20 || edge.BToABytesPerSecond != 40 {
		t.Fatalf("rates = %.0f/%.0f, want 20/40 without stale A rate", edge.AToBBytesPerSecond, edge.BToABytesPerSecond)
	}
	if len(edge.Conflicts) != 1 || edge.Conflicts[0].Kind != domain.PathPeerRelay {
		t.Fatalf("conflicts = %#v, want retained fresh peer relay provenance", edge.Conflicts)
	}
}

func TestRecentEdgeRatesAgeToZeroWithoutAnotherReport(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applySample(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", 200, 100)

	now = now.Add(11 * time.Second)
	edge := aggregator.Snapshot().Edges[0]
	if edge.State != domain.EdgeRecent || edge.AToBBytesPerSecond != 0 || edge.BToABytesPerSecond != 0 {
		t.Fatalf("recent edge retained current rate: %#v", edge)
	}
}

func TestUnknownInventoryAcceptsTrafficAndRequestsResync(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	report := sampleReport("reporter-a", 2, "a", "A", "b", "B", "unknown", domain.PathObservation{Kind: domain.PathDERP}, 100, 50)
	result, err := aggregator.ApplyAt(report, now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Receipt.Accepted || !result.Receipt.ResyncRequired {
		t.Fatalf("receipt = %#v, want accepted resync", result.Receipt)
	}
	if got := len(aggregator.Snapshot().Edges); got != 1 {
		t.Fatalf("accepted sample created %d edges, want 1", got)
	}
}

func TestInventoryReplacementWithdrawsOnlyObserverProvenance(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter-a", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: node("a", "A"), InventoryGeneration: "inventory-a",
			Peers: []domain.PeerObservation{{Peer: node("b", "B")}},
		}},
	}
	if _, err := aggregator.Apply(hello); err != nil {
		t.Fatal(err)
	}
	applySample(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", 100, 50)
	update := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "inventory-empty", ReporterInstanceID: "reporter-a", Sequence: 3,
		CollectedAt: now, Kind: domain.ReportInventoryUpdate,
		Observers: []domain.ObserverReport{{Observer: node("a", "A"), InventoryGeneration: "inventory-empty"}},
	}
	if _, err := aggregator.Apply(update); err != nil {
		t.Fatal(err)
	}
	edge := aggregator.Snapshot().Edges[0]
	if edge.Observations == nil || len(edge.Observations) != 0 || edge.Path.Kind != domain.PathDirect {
		t.Fatalf("withdrawn inventory edge = %#v", edge)
	}
	if got := len(aggregator.Snapshot().Nodes); got != 2 {
		t.Fatalf("inventory replacement globally removed node; got %d nodes", got)
	}
}

func TestOpaqueIdentityMergesAliasesAndNeverUsesHostname(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	first := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{DiscoKey: "disco-a", Hostname: "old"}, InventoryGeneration: "one",
		}},
	}
	if _, err := aggregator.Apply(first); err != nil {
		t.Fatal(err)
	}
	firstID := aggregator.Snapshot().Nodes[0].ID
	second := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "inventory", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: now, Kind: domain.ReportInventoryUpdate,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "stable-a", DiscoKey: "disco-a", Hostname: "new", DNSName: "friendly.tail.example."}, InventoryGeneration: "two",
		}},
	}
	if _, err := aggregator.Apply(second); err != nil {
		t.Fatal(err)
	}
	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 1 || topology.Nodes[0].ID != firstID || topology.Nodes[0].DisplayName() != "friendly" {
		t.Fatalf("identity was not reconciled in place: %#v", topology.Nodes)
	}

	hostnameOnly := first
	hostnameOnly.ReportID = "invalid"
	hostnameOnly.Observers[0].Observer = domain.NodeIdentity{Hostname: "same"}
	if _, err := aggregator.Apply(hostnameOnly); err == nil {
		t.Fatal("hostname-only identity was accepted")
	}
}

func TestReceiveTimeOwnsExpiryAndClockSkewIsExposed(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now.Add(8 * time.Hour), Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{Observer: node("a", "A"), InventoryGeneration: "inventory"}},
	}
	if _, err := aggregator.ApplyAt(report, now); err != nil {
		t.Fatal(err)
	}
	topology := aggregator.Snapshot()
	if !topology.Observers[0].ClockSkewed || !topology.Nodes[0].Online {
		t.Fatalf("clock skew or receive-time liveness missing: %#v", topology)
	}
	now = now.Add(2*time.Minute + time.Nanosecond)
	if aggregator.Snapshot().Observers[0].Online {
		t.Fatal("observer remained online beyond two heartbeat intervals")
	}
	now = now.Add(2 * time.Minute)
	if got := len(aggregator.Snapshot().Nodes); got != 0 {
		t.Fatalf("stale runtime nodes = %d, want hidden", got)
	}
}

func TestReporterRestartReclaimsPriorStateForObserver(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	_, err := aggregator.ApplyAt(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello-a", ReporterInstanceID: "reporter-a", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{
			{Observer: domain.NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory-a"},
			{Observer: domain.NodeIdentity{StableNodeID: "b", Hostname: "B"}, InventoryGeneration: "inventory-b"},
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregator.state.Reporters) != 1 {
		t.Fatalf("reporter state count = %d, want 1", len(aggregator.state.Reporters))
	}

	now = now.Add(time.Second)
	applyHello(t, aggregator, "reporter-b", 1, "a", "A", "inventory-a")
	if len(aggregator.state.Reporters) != 2 || aggregator.state.Reporters["reporter-b"] == nil {
		t.Fatalf("new reporter did not claim observer: %#v", aggregator.state.Reporters)
	}
	previous := aggregator.state.Reporters["reporter-a"]
	if previous == nil || len(previous.ObserverIDs) != 1 {
		t.Fatalf("unclaimed observer was not retained: %#v", previous)
	}
	if _, retained := previous.ObserverIDs[aggregator.state.Aliases["stable:b"]]; !retained {
		t.Fatalf("old reporter no longer owns observer B: %#v", previous.ObserverIDs)
	}
}

func TestIPAddressAliasExpiresWithNodeWindow(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyIdentityHello := func(reporter, reportID string, identity domain.NodeIdentity) {
		t.Helper()
		_, err := aggregator.ApplyAt(domain.ReportEnvelope{
			Version: domain.ProtocolVersion, ReportID: reportID, ReporterInstanceID: reporter, Sequence: 1,
			CollectedAt: now, Kind: domain.ReportObserverHello,
			Observers: []domain.ObserverReport{{Observer: identity, InventoryGeneration: reportID}},
		}, now)
		if err != nil {
			t.Fatal(err)
		}
	}

	applyIdentityHello("reporter-a", "hello-a", domain.NodeIdentity{
		StableNodeID: "stable-a", Hostname: "A", TailscaleIPs: []string{"100.64.0.1"},
	})
	firstID := aggregator.Snapshot().Nodes[0].ID

	now = now.Add(3 * time.Minute)
	applyIdentityHello("reporter-ip-current", "hello-current", domain.NodeIdentity{
		Hostname: "current-ip", TailscaleIPs: []string{"100.64.0.1"},
	})
	if currentID := aggregator.Snapshot().Nodes[0].ID; currentID != firstID {
		t.Fatalf("fresh IP alias resolved to %q, want %q", currentID, firstID)
	}

	now = now.Add(4*time.Minute + time.Second)
	applyIdentityHello("reporter-ip-reused", "hello-reused", domain.NodeIdentity{
		Hostname: "reused-ip", TailscaleIPs: []string{"100.64.0.1"},
	})
	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 1 || topology.Nodes[0].ID == firstID {
		t.Fatalf("expired IP alias reused old canonical node: %#v", topology.Nodes)
	}
	if addresses := aggregator.state.Nodes[firstID].Identity.TailscaleIPs; len(addresses) != 0 {
		t.Fatalf("expired canonical node retained stale addresses: %#v", addresses)
	}
}

func applyHello(t *testing.T, aggregator *Aggregator, reporter string, sequence int64, id, name, inventory string) {
	t.Helper()
	_, err := aggregator.Apply(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: reporter + "-hello", ReporterInstanceID: reporter,
		Sequence: sequence, CollectedAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{Observer: node(id, name), InventoryGeneration: inventory}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func applySample(t *testing.T, aggregator *Aggregator, reporter string, sequence int64, observerID, observerName, peerID, peerName, inventory string, tx, rx int64) {
	t.Helper()
	applySampleWithPathAndDeltas(t, aggregator, reporter, sequence, observerID, observerName, peerID, peerName, inventory, domain.PathObservation{Kind: domain.PathDirect}, tx, rx)
}

func applySampleWithPath(t *testing.T, aggregator *Aggregator, reporter string, sequence int64, observerID, observerName, peerID, peerName, inventory string, path domain.PathObservation) {
	t.Helper()
	applySampleWithPathAndDeltas(t, aggregator, reporter, sequence, observerID, observerName, peerID, peerName, inventory, path, 100, 50)
}

func applySampleWithPathAndDeltas(t *testing.T, aggregator *Aggregator, reporter string, sequence int64, observerID, observerName, peerID, peerName, inventory string, path domain.PathObservation, tx, rx int64) {
	t.Helper()
	_, err := aggregator.Apply(sampleReport(reporter, sequence, observerID, observerName, peerID, peerName, inventory, path, tx, rx))
	if err != nil {
		t.Fatal(err)
	}
}

func sampleReport(reporter string, sequence int64, observerID, observerName, peerID, peerName, inventory string, path domain.PathObservation, tx, rx int64) domain.ReportEnvelope {
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: fmt.Sprintf("%s-sample-%d", reporter, sequence), ReporterInstanceID: reporter,
		Sequence: sequence, CollectedAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: node(observerID, observerName), InventoryGeneration: inventory,
			Peers: []domain.PeerObservation{{Peer: node(peerID, peerName), TxDelta: tx, RxDelta: rx, SampleDurationMS: 2000, Path: path}},
		}},
	}
}

func newTestAggregator(now func() time.Time) *Aggregator {
	next := 0
	return New(Options{
		Now: now,
		NewNodeID: func() string {
			next++
			return fmt.Sprintf("n_%03d", next)
		},
	})
}

func node(id, hostname string) domain.NodeIdentity {
	return domain.NodeIdentity{StableNodeID: id, Hostname: hostname}
}
