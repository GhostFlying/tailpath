package aggregate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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

func TestReconcilePathsEnrichesLatestPeerRelayFromRelayProvenance(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	vni := int64(7)
	observations := []domain.ObservationProvenance{
		{
			ObserverID: "relay",
			ReceivedAt: now,
			Path: domain.PathObservation{
				Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-stable", PeerRelayVNI: &vni,
			},
		},
		{
			ObserverID: "endpoint",
			ReceivedAt: now.Add(time.Second),
			Path:       domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayVNI: &vni},
		},
	}

	path, conflicts := reconcilePaths(observations)
	if path.Kind != domain.PathPeerRelay || path.PeerRelayStableNodeID != "relay-stable" ||
		path.PeerRelayVNI == nil || *path.PeerRelayVNI != vni {
		t.Fatalf("path = %#v, want enriched peer relay", path)
	}
	if len(conflicts) != 0 {
		t.Fatalf("equivalent relay provenance produced conflicts: %#v", conflicts)
	}
}

func TestReconcilePathsKeepsConflictingRelayIdentity(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	observations := []domain.ObservationProvenance{
		{
			ObserverID: "old-relay", ReceivedAt: now,
			Path: domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-old"},
		},
		{
			ObserverID: "new-relay", ReceivedAt: now.Add(time.Second),
			Path: domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-new"},
		},
		{
			ObserverID: "endpoint", ReceivedAt: now.Add(2 * time.Second),
			Path: domain.PathObservation{Kind: domain.PathPeerRelay},
		},
	}

	path, conflicts := reconcilePaths(observations)
	if path.PeerRelayStableNodeID != "relay-new" {
		t.Fatalf("path = %#v, want newest detailed relay", path)
	}
	if len(conflicts) != 1 || conflicts[0].PeerRelayStableNodeID != "relay-old" {
		t.Fatalf("conflicts = %#v, want older conflicting relay", conflicts)
	}
}

func TestCloneRuntimeStateIsIndependent(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	source := runtimeState{
		Reporters: map[string]*reporterState{"reporter": {
			LastSequence: 4,
			ReportIDs:    map[string]struct{}{"report": {}},
			ObserverIDs:  map[string]struct{}{"observer": {}},
			LegacyInventories: map[string]string{
				"legacy": "generation",
			},
			LegacyMemberships: map[string]map[string]struct{}{
				"legacy": {"peer": {}},
			},
		}},
		Observers: map[string]*observerRuntimeState{"observer": {
			OwnerReporterInstanceID: "reporter",
			InventoryGeneration:     "generation",
			Membership:              map[string]struct{}{"peer": {}},
		}},
		Nodes: map[string]*nodeState{"observer": {
			Identity: domain.NodeIdentity{StableNodeID: "observer", TailscaleIPs: []string{"100.64.0.1"}},
		}},
		Aliases:       map[string]string{"stable:observer": "observer"},
		AliasLastSeen: map[string]time.Time{"ip:100.64.0.1": now},
		Edges: map[string]*edgeState{"observer--peer": {
			ID: "observer--peer", Source: "observer", Target: "peer",
			Observations: map[string]edgeObservation{"observer": {ObserverID: "observer", TxRate: 1}},
		}},
		RelayScopes: map[string]*relayScopeState{"relay:7": {
			RelayID: "relay", VNI: 7, Sessions: map[string]*relaySessionState{"session": {
				Clients: map[string]string{"left": "observer"}, SourceNodeID: "observer",
			}},
		}},
	}
	clone := cloneRuntimeState(source)
	clone.Reporters["reporter"].LastSequence = 5
	clone.Reporters["reporter"].ReportIDs["other"] = struct{}{}
	clone.Reporters["reporter"].ObserverIDs["other"] = struct{}{}
	clone.Reporters["reporter"].LegacyInventories["legacy"] = "changed"
	clone.Reporters["reporter"].LegacyMemberships["legacy"]["other"] = struct{}{}
	clone.Observers["observer"].InventoryGeneration = "changed"
	clone.Observers["observer"].Membership["other"] = struct{}{}
	clone.Nodes["observer"].Identity.TailscaleIPs[0] = "100.64.0.2"
	clone.Aliases["stable:observer"] = "other"
	clone.AliasLastSeen["ip:100.64.0.1"] = now.Add(time.Hour)
	observation := clone.Edges["observer--peer"].Observations["observer"]
	observation.TxRate = 2
	clone.Edges["observer--peer"].Observations["observer"] = observation
	clone.RelayScopes["relay:7"].Sessions["session"].Clients["left"] = "other"

	reporter := source.Reporters["reporter"]
	if reporter.LastSequence != 4 || len(reporter.ReportIDs) != 1 || len(reporter.ObserverIDs) != 1 ||
		reporter.LegacyInventories["legacy"] != "generation" || len(reporter.LegacyMemberships["legacy"]) != 1 {
		t.Fatalf("reporter clone mutated source: %#v", reporter)
	}
	observer := source.Observers["observer"]
	if observer.InventoryGeneration != "generation" || len(observer.Membership) != 1 {
		t.Fatalf("observer clone mutated source: %#v", observer)
	}
	if source.Nodes["observer"].Identity.TailscaleIPs[0] != "100.64.0.1" ||
		source.Aliases["stable:observer"] != "observer" || !source.AliasLastSeen["ip:100.64.0.1"].Equal(now) ||
		source.Edges["observer--peer"].Observations["observer"].TxRate != 1 ||
		source.RelayScopes["relay:7"].Sessions["session"].Clients["left"] != "observer" {
		t.Fatal("node, alias, or edge clone mutated source")
	}
}

func TestReplaceWithTransfersStateAndKeepsSubscribers(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	clone, err := aggregator.Clone()
	if err != nil {
		t.Fatal(err)
	}
	applyHello(t, clone, "reporter-a", 1, "a", "A", "inventory-a")
	events, unsubscribe := aggregator.Subscribe()
	defer unsubscribe()
	if err := aggregator.ReplaceWith(clone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	default:
		t.Fatal("state transfer did not notify existing subscriber")
	}
	if got := len(aggregator.Snapshot().Nodes); got != 1 {
		t.Fatalf("transferred state has %d nodes, want 1", got)
	}
	if got := len(clone.Snapshot().Nodes); got != 0 {
		t.Fatalf("candidate retained %d transferred nodes", got)
	}
}

func TestReporterDeduplicationWindowStaysBounded(t *testing.T) {
	aggregator := newTestAggregator(time.Now)
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	applyHello(t, aggregator, "reporter", 1, "node-a", "A", "inventory")
	for sequence := int64(2); sequence <= reportIDWindowSize+3; sequence++ {
		report := sampleReport(
			"reporter", sequence, "node-a", "A", "node-b", "B", "inventory",
			domain.PathObservation{Kind: domain.PathDirect}, 100, 50,
		)
		report.CollectedAt = at.Add(time.Duration(sequence) * time.Second)
		if _, err := aggregator.ApplyAt(report, at.Add(time.Duration(sequence)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	aggregator.mu.RLock()
	reporter := aggregator.state.Reporters["reporter"]
	got := len(reporter.ReportIDs)
	aggregator.mu.RUnlock()
	if got > reportIDWindowSize {
		t.Fatalf("retained report IDs = %d, want at most %d", got, reportIDWindowSize)
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
			Relay:     node("relay", "Relay-HZ"),
			Source:    domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A")), Endpoint: "192.0.2.10:41641"},
			Target:    domain.RelaySessionClient{SessionClientID: "right", Identity: identity(node("b", "B")), Endpoint: "[2001:db8::10]:41641"},
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
	if edge.Path.PeerRelayVNI == nil || *edge.Path.PeerRelayVNI != 7 ||
		edge.Observations[0].RelaySession == nil || edge.Observations[0].RelaySession.VNI != 7 {
		t.Fatalf("relay session provenance = %#v", edge.Observations)
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
	checkpoint, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(checkpoint), "192.0.2.10") || strings.Contains(string(checkpoint), "2001:db8") {
		t.Fatalf("relay underlay endpoint entered checkpoint: %s", checkpoint)
	}
}

func TestRelaySessionMarksControlIdentityAsSystemTelemetry(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	next := 0
	aggregator := New(Options{
		Now:            func() time.Time { return now },
		ControlNodeIDs: []string{"server"},
		NewNodeID: func() string {
			next++
			return fmt.Sprintf("generated-%d", next)
		},
	})
	result, err := aggregator.ApplyAt(relayReport(
		1, "control-session", 7,
		domain.RelaySessionClient{SessionClientID: "server", Identity: identity(node("server", "Server"))},
		domain.RelaySessionClient{SessionClientID: "client", Identity: identity(node("client", "Client"))},
		1200, 400,
	), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traffic) != 1 {
		t.Fatalf("control traffic records = %d, want one retained internal record", len(result.Traffic))
	}
	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 || !topology.Edges[0].SystemTelemetry {
		t.Fatalf("control edge classification = %#v", topology.Edges)
	}
}

func TestSystemTelemetrySurvivesOrdinaryObservationCanonicalMergeAndRestore(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	ids := []string{"n_observer", "z_control", "a_alias"}
	aggregator := New(Options{
		Now:            func() time.Time { return now.Add(2 * time.Second) },
		ControlNodeIDs: []string{"server"},
		NewNodeID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
	control := domain.NodeIdentity{StableNodeID: "server", DiscoKey: "disco-control"}
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer"}, InventoryGeneration: "g1",
			Peers: []domain.PeerObservation{
				{Peer: control},
				{Peer: domain.NodeIdentity{NodeKey: "node-alias"}},
			},
		}},
	}
	if _, err := aggregator.ApplyAt(hello, now); err != nil {
		t.Fatal(err)
	}
	traffic := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "traffic", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: now.Add(time.Second), Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer"}, InventoryGeneration: "g1",
			Peers: []domain.PeerObservation{{
				Peer: control, TxDelta: 10, RxDelta: 5, SampleDurationMS: 1000,
				LastActive: now.Add(time.Second), Path: domain.PathObservation{Kind: domain.PathDirect},
			}},
		}},
	}
	result, err := aggregator.ApplyAt(traffic, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traffic) != 1 || len(aggregator.Snapshot().Edges) != 1 ||
		!aggregator.Snapshot().Edges[0].SystemTelemetry {
		t.Fatalf("ordinary control traffic was not retained and classified: result=%#v topology=%#v", result, aggregator.Snapshot())
	}

	merge := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "merge", ReporterInstanceID: "reporter", Sequence: 3,
		CollectedAt: now.Add(2 * time.Second), Kind: domain.ReportInventoryUpdate,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer"}, InventoryGeneration: "g2",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{
				StableNodeID: "renamed", DiscoKey: "disco-control", NodeKey: "node-alias",
			}}},
		}},
	}
	if result, err = aggregator.ApplyAt(merge, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	} else if !result.CanonicalStateChanged {
		t.Fatal("control alias merge was not exposed as canonical state change")
	}
	merged := aggregator.Snapshot()
	if len(merged.Edges) != 1 || !merged.Edges[0].SystemTelemetry {
		t.Fatalf("canonical merge lost control classification: %#v", merged.Edges)
	}
	payload, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte(`"systemTelemetry":true`)) {
		t.Fatalf("checkpoint omitted control classification: %s", payload)
	}
	restored := New(Options{Now: func() time.Time { return now.Add(2 * time.Second) }})
	if err := restored.RestoreState(payload); err != nil {
		t.Fatal(err)
	}
	if topology := restored.Snapshot(); len(topology.Edges) != 1 || !topology.Edges[0].SystemTelemetry {
		t.Fatalf("restored topology lost control classification: %#v", topology.Edges)
	}
}

func TestRelaySessionNormalizesIdentityStatusWithCanonicalDirection(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "target-reporter", 1, "target", "Target", "inventory-target")
	report := relayReport(1, "session", 7,
		domain.RelaySessionClient{SessionClientID: "left", DiscoShort: "short-left"},
		domain.RelaySessionClient{SessionClientID: "right", Identity: identity(node("target", "Target"))},
		200, 80,
	)
	if _, err := aggregator.ApplyAt(report, now); err != nil {
		t.Fatal(err)
	}
	edge := topologyEdgeWithRelaySession(t, aggregator.Snapshot())
	provenance := relayProvenance(t, edge)
	if provenance.SourceIdentityStatus != domain.IdentityResolved ||
		provenance.TargetIdentityStatus != domain.IdentityPartial {
		t.Fatalf("canonical identity status = %#v, want resolved/partial", provenance)
	}
	if edge.AToBBytesPerSecond != 40 || edge.BToABytesPerSecond != 100 {
		t.Fatalf("canonical relay rates = %.0f/%.0f, want 40/100", edge.AToBBytesPerSecond, edge.BToABytesPerSecond)
	}
}

func TestRelaySessionRejectsCanonicalSelfEdge(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	shared := identity(node("same", "Same"))
	report := relayReport(1, "session", 7,
		domain.RelaySessionClient{SessionClientID: "left", Identity: shared},
		domain.RelaySessionClient{SessionClientID: "right", Identity: shared},
		200, 80,
	)
	result, err := aggregator.ApplyAt(report, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traffic) != 0 || len(aggregator.Snapshot().Edges) != 0 {
		t.Fatalf("ambiguous relay identity created self-edge: result=%#v topology=%#v",
			result, aggregator.Snapshot())
	}
}

func TestRelaySequenceGapRequestsResyncWithoutDroppingSession(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	clientA := domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A"))}
	clientB := domain.RelaySessionClient{SessionClientID: "right", Identity: identity(node("b", "B"))}
	if result, err := aggregator.ApplyAt(relayReport(1, "session", 7, clientA, clientB, 20, 10), now); err != nil ||
		!result.Receipt.Accepted || result.Receipt.ResyncRequired {
		t.Fatalf("initial relay receipt = %#v, err=%v", result.Receipt, err)
	}

	now = now.Add(2 * time.Second)
	result, err := aggregator.ApplyAt(relayReport(3, "session", 7, clientA, clientB, 30, 15), now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Receipt.Accepted || !result.Receipt.ResyncRequired || !result.Changed {
		t.Fatalf("gap relay result = %#v", result)
	}
	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 || topology.Edges[0].Path.Kind != domain.PathPeerRelay ||
		topology.Edges[0].AToBBytesPerSecond+topology.Edges[0].BToABytesPerSecond <= 0 {
		t.Fatalf("gap relay topology = %#v", topology)
	}
}

func TestAnonymousRelaySessionKeepsScopedIdentityAcrossReportsAndRestart(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	first := relayReport(1, "session", 7,
		domain.RelaySessionClient{SessionClientID: "left", DiscoShort: "disco-left", Endpoint: "192.0.2.10:41641"},
		domain.RelaySessionClient{SessionClientID: "right", Endpoint: "192.0.2.11:41641"},
		120, 40,
	)
	if _, err := aggregator.ApplyAt(first, now); err != nil {
		t.Fatal(err)
	}
	initial := aggregator.Snapshot()
	if len(initial.Edges) != 1 || len(initial.Nodes) != 3 {
		t.Fatalf("initial anonymous topology = %#v", initial)
	}
	initialEdgeID := initial.Edges[0].ID

	now = now.Add(2 * time.Second)
	second := relayReport(2, "session", 7,
		domain.RelaySessionClient{SessionClientID: "left", DiscoShort: "changed-hint", Endpoint: "198.51.100.10:41641"},
		domain.RelaySessionClient{SessionClientID: "right", Endpoint: "198.51.100.11:41641"},
		60, 20,
	)
	second.ReportID = "relay-2"
	second.CollectedAt = now
	if _, err := aggregator.ApplyAt(second, now); err != nil {
		t.Fatal(err)
	}
	if topology := aggregator.Snapshot(); len(topology.Edges) != 1 || topology.Edges[0].ID != initialEdgeID || len(topology.Nodes) != 3 {
		t.Fatalf("anonymous session changed canonical identity: %#v", topology)
	}
	for alias := range aggregator.state.Aliases {
		if strings.Contains(alias, "session") || strings.Contains(alias, "disco-left") || strings.Contains(alias, "192.0.2") {
			t.Fatalf("relay-scoped evidence entered global aliases: %q", alias)
		}
	}

	checkpoint, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(checkpoint, []byte("192.0.2")) || bytes.Contains(checkpoint, []byte("198.51.100")) ||
		bytes.Contains(checkpoint, []byte("disco-left")) || bytes.Contains(checkpoint, []byte("changed-hint")) {
		t.Fatalf("ephemeral relay evidence entered checkpoint: %s", checkpoint)
	}
	restored := newTestAggregator(func() time.Time { return now })
	if err := restored.RestoreState(checkpoint); err != nil {
		t.Fatal(err)
	}
	if topology := restored.Snapshot(); len(topology.Edges) != 1 || topology.Edges[0].ID != initialEdgeID || len(topology.Nodes) != 3 {
		t.Fatalf("restored anonymous topology = %#v", topology)
	}
}

func TestRelayScopeInfersOnlyMissingSideFromEndpointPair(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	vni := int64(7)
	applySampleWithPath(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", domain.PathObservation{
		Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
	})
	report := relayReport(1, "session", vni,
		domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A"))},
		domain.RelaySessionClient{SessionClientID: "right", DiscoShort: "short-b"},
		200, 80,
	)
	if _, err := aggregator.ApplyAt(report, now); err != nil {
		t.Fatal(err)
	}
	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 {
		t.Fatalf("inferred relay topology has %d edges: %#v", len(topology.Edges), topology.Edges)
	}
	aID := topologyNodeID(t, topology, "a")
	bID := topologyNodeID(t, topology, "b")
	if topology.Edges[0].ID != edgeIDFor(aID, bID) {
		t.Fatalf("relay edge = %q, want canonical endpoint pair %q", topology.Edges[0].ID, edgeIDFor(aID, bID))
	}
	provenance := relayProvenance(t, topology.Edges[0])
	if provenance.SourceIdentityStatus != domain.IdentityResolved || provenance.TargetIdentityStatus != domain.IdentityResolved {
		t.Fatalf("inferred identity status = %#v", provenance)
	}
}

func TestEndpointPairReconcilesEarlierAnonymousRelayClient(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	report := relayReport(1, "session", 7,
		domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A"))},
		domain.RelaySessionClient{SessionClientID: "right", DiscoShort: "short-b"},
		200, 80,
	)
	if _, err := aggregator.ApplyAt(report, now); err != nil {
		t.Fatal(err)
	}
	before := aggregator.Snapshot()
	if len(before.Edges) != 1 {
		t.Fatalf("initial relay topology = %#v", before)
	}
	aID := topologyNodeID(t, before, "a")
	placeholderID := before.Edges[0].Target
	if placeholderID == aID {
		placeholderID = before.Edges[0].Source
	}

	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")
	vni := int64(7)
	applySampleWithPath(t, aggregator, "reporter-b", 2, "b", "B", "a", "A", "inventory-b", domain.PathObservation{
		Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
	})
	after := aggregator.Snapshot()
	bID := topologyNodeID(t, after, "b")
	if after.Edges[0].ID != edgeIDFor(aID, bID) || aggregator.state.Redirects[placeholderID] != bID {
		t.Fatalf("delayed reconciliation edge/redirect = %#v / %#v", after.Edges, aggregator.state.Redirects)
	}
}

func TestRelayScopeDoesNotInferFromVNIAloneOrConflictingPairs(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	vni := int64(7)

	t.Run("vni alone", func(t *testing.T) {
		aggregator := newTestAggregator(func() time.Time { return now })
		applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
		applySampleWithPath(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", domain.PathObservation{
			Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
		})
		if _, err := aggregator.ApplyAt(relayReport(1, "anonymous", vni,
			domain.RelaySessionClient{SessionClientID: "left"},
			domain.RelaySessionClient{SessionClientID: "right"}, 20, 10), now); err != nil {
			t.Fatal(err)
		}
		if got := len(aggregator.Snapshot().Edges); got != 2 {
			t.Fatalf("VNI-only evidence guessed endpoint pair; got %d edges", got)
		}
	})

	t.Run("conflicting pair", func(t *testing.T) {
		aggregator := newTestAggregator(func() time.Time { return now })
		applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
		applySampleWithPath(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", domain.PathObservation{
			Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
		})
		applyHello(t, aggregator, "reporter-c", 1, "c", "C", "inventory-c")
		applySampleWithPath(t, aggregator, "reporter-c", 2, "c", "C", "d", "D", "inventory-c", domain.PathObservation{
			Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
		})
		if _, err := aggregator.ApplyAt(relayReport(1, "conflict", vni,
			domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A"))},
			domain.RelaySessionClient{SessionClientID: "right", DiscoShort: "short"}, 20, 10), now); err != nil {
			t.Fatal(err)
		}
		topology := aggregator.Snapshot()
		if got := len(topology.Edges); got != 3 {
			t.Fatalf("conflicting scope guessed a merge; got %d edges: %#v", got, topology.Edges)
		}
		provenance := relayProvenance(t, topologyEdgeWithRelaySession(t, topology))
		if provenance.TargetIdentityStatus != domain.IdentityConflict {
			t.Fatalf("conflicting relay provenance = %#v", provenance)
		}
	})
}

func TestEndpointTrafficOutranksRelayFallbackWithoutSumming(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	vni := int64(7)
	applySampleWithPathAndDeltas(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", domain.PathObservation{
		Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni,
	}, 100, 50)
	if _, err := aggregator.ApplyAt(relayReport(1, "session", vni,
		domain.RelaySessionClient{SessionClientID: "left", Identity: identity(node("a", "A"))},
		domain.RelaySessionClient{SessionClientID: "right", Identity: identity(node("b", "B"))},
		10000, 5000,
	), now); err != nil {
		t.Fatal(err)
	}
	edge := aggregator.Snapshot().Edges[0]
	if edge.AToBBytesPerSecond != 50 || edge.BToABytesPerSecond != 25 {
		t.Fatalf("endpoint and relay rates were summed or relay won: %.0f/%.0f", edge.AToBBytesPerSecond, edge.BToABytesPerSecond)
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

func TestPlatformMetadataRefreshesWithoutChangingCanonicalNode(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello-linux", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer:            domain.NodeIdentity{StableNodeID: "a", Hostname: "A", OS: "linux"},
			InventoryGeneration: "linux-inventory",
		}},
	}
	if _, err := aggregator.Apply(hello); err != nil {
		t.Fatal(err)
	}
	canonicalID := aggregator.state.Aliases["stable:a"]
	update := hello
	update.ReportID = "inventory-macos"
	update.Sequence = 2
	update.Kind = domain.ReportInventoryUpdate
	update.Observers[0].Observer.OS = "macos"
	update.Observers[0].InventoryGeneration = "macos-inventory"
	if _, err := aggregator.Apply(update); err != nil {
		t.Fatal(err)
	}
	if got := aggregator.state.Aliases["stable:a"]; got != canonicalID {
		t.Fatalf("OS refresh changed canonical node from %q to %q", canonicalID, got)
	}
	if len(aggregator.state.Nodes) != 1 {
		t.Fatalf("OS refresh created %d canonical nodes", len(aggregator.state.Nodes))
	}
	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 1 || topology.Nodes[0].OS != "macos" {
		t.Fatalf("topology platform = %#v, want refreshed macos", topology.Nodes)
	}
}

func TestCanonicalMergeRecordsDurableRedirect(t *testing.T) {
	at := time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)
	ids := []string{"n_observer", "n_disco", "n_key"}
	aggregator := New(Options{NewNodeID: func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}})
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer"}, InventoryGeneration: "g1",
			Peers: []domain.PeerObservation{
				{Peer: domain.NodeIdentity{DiscoKey: "disco-b"}, Path: domain.PathObservation{Kind: domain.PathUnknown}, LastActive: at},
				{Peer: domain.NodeIdentity{NodeKey: "node-key-b"}, Path: domain.PathObservation{Kind: domain.PathUnknown}, LastActive: at},
			},
		}},
	}
	if _, err := aggregator.ApplyAt(hello, at); err != nil {
		t.Fatal(err)
	}
	merge := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "merge", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: at.Add(time.Second), Kind: domain.ReportTrafficSample,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer"}, InventoryGeneration: "g1",
			Peers: []domain.PeerObservation{{
				Peer:    domain.NodeIdentity{DiscoKey: "disco-b", NodeKey: "node-key-b"},
				TxDelta: 1, SampleDurationMS: 1000, LastActive: at.Add(time.Second),
				Path: domain.PathObservation{Kind: domain.PathDirect},
			}},
		}},
	}
	result, err := aggregator.ApplyAt(merge, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanonicalStateChanged {
		t.Fatal("canonical merge was not exposed to the checkpoint policy")
	}
	metadata := aggregator.HistoryMetadata()
	if metadata.Redirects["n_key"] != "n_disco" {
		t.Fatalf("redirects = %#v, want n_key -> n_disco", metadata.Redirects)
	}
	if len(metadata.Nodes) != 2 {
		t.Fatalf("nodes after merge = %#v", metadata.Nodes)
	}
	payload, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	restored := New(Options{})
	if err := restored.RestoreState(payload); err != nil {
		t.Fatal(err)
	}
	if restored.HistoryMetadata().Redirects["n_key"] != "n_disco" {
		t.Fatalf("restored redirects = %#v", restored.HistoryMetadata().Redirects)
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

func TestObserverWithdrawalIsImmediateIdempotentAndFenced(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-old", 1, "a", "A", "inventory")
	applySample(t, aggregator, "reporter-old", 2, "a", "A", "b", "B", "inventory", 200, 100)

	before := aggregator.Snapshot()
	if len(before.Edges) != 1 || before.Edges[0].State != domain.EdgeActive || !before.Observers[0].Online {
		t.Fatalf("pre-withdraw topology = %#v", before)
	}

	now = now.Add(time.Second)
	withdrawal := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "withdraw", ReporterInstanceID: "reporter-old", Sequence: 3,
		CollectedAt: now, Kind: domain.ReportObserverWithdrawal,
		Observers: []domain.ObserverReport{{Observer: node("a", "A"), InventoryGeneration: "inventory"}},
	}
	result, err := aggregator.ApplyAt(withdrawal, now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Receipt.Accepted || !result.CheckpointRequired || len(result.Traffic) != 0 || len(result.PathTransitions) != 0 {
		t.Fatalf("withdraw result = %#v", result)
	}
	after := aggregator.Snapshot()
	if len(after.Edges) != 1 || after.Edges[0].State != domain.EdgeRecent ||
		after.Edges[0].AToBBytesPerSecond != 0 || after.Edges[0].BToABytesPerSecond != 0 ||
		len(after.Edges[0].Observations) != 0 || after.Observers[0].Online {
		t.Fatalf("post-withdraw topology = %#v", after)
	}
	observerID := aggregator.state.Aliases["stable:a"]
	observer := aggregator.state.Observers[observerID]
	if observer.OwnerReporterInstanceID != "" || !observer.WithdrawnAt.Equal(now) || len(observer.Membership) != 0 {
		t.Fatalf("withdrawn observer state = %#v", observer)
	}
	edge := aggregator.state.Edges[after.Edges[0].ID]
	if edge.Observations[observerID].WithdrawnAt.IsZero() {
		t.Fatalf("withdrawal provenance was not retained: %#v", edge.Observations)
	}

	unknown := withdrawal
	unknown.ReportID = "unknown-withdraw"
	unknown.Sequence = 4
	unknown.Observers[0].Observer = node("unknown", "Unknown")
	result, err = aggregator.ApplyAt(unknown, now.Add(time.Second))
	if err != nil || !result.Receipt.Accepted || result.CheckpointRequired {
		t.Fatalf("unknown withdrawal result = %#v, err=%v", result, err)
	}
	if _, exists := aggregator.state.Aliases["stable:unknown"]; exists {
		t.Fatal("unknown withdrawal created a canonical node")
	}

	now = now.Add(2 * time.Second)
	applyHello(t, aggregator, "reporter-new", 1, "a", "A", "inventory")
	reclaimed := aggregator.Snapshot()
	if !reclaimed.Observers[0].Online || reclaimed.Edges[0].State != domain.EdgeRecent {
		t.Fatalf("reclaimed topology revived traffic or stayed offline: %#v", reclaimed)
	}
	stale := withdrawal
	stale.ReportID = "stale-withdraw"
	stale.Sequence = 5
	result, err = aggregator.ApplyAt(stale, now.Add(time.Second))
	if err != nil || !result.Receipt.Accepted || result.CheckpointRequired {
		t.Fatalf("stale withdrawal result = %#v, err=%v", result, err)
	}
	if owner := aggregator.state.Observers[observerID].OwnerReporterInstanceID; owner != "reporter-new" {
		t.Fatalf("stale withdrawal changed owner to %q", owner)
	}
}

func TestObserverWithdrawalKeepsEdgeActiveWithOtherFreshProvenance(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	applyHello(t, aggregator, "reporter-a", 1, "a", "A", "inventory-a")
	applyHello(t, aggregator, "reporter-b", 1, "b", "B", "inventory-b")
	applySample(t, aggregator, "reporter-a", 2, "a", "A", "b", "B", "inventory-a", 200, 100)
	applySample(t, aggregator, "reporter-b", 2, "b", "B", "a", "A", "inventory-b", 100, 200)

	now = now.Add(time.Second)
	_, err := aggregator.ApplyAt(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "withdraw-a", ReporterInstanceID: "reporter-a", Sequence: 3,
		CollectedAt: now, Kind: domain.ReportObserverWithdrawal,
		Observers: []domain.ObserverReport{{Observer: node("a", "A"), InventoryGeneration: "inventory-a"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	topology := aggregator.Snapshot()
	if len(topology.Edges) != 1 || topology.Edges[0].State != domain.EdgeActive || len(topology.Edges[0].Observations) != 1 {
		t.Fatalf("remaining fresh provenance did not keep edge active: %#v", topology.Edges)
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
	observerA := aggregator.state.Observers[aggregator.state.Aliases["stable:a"]]
	if observerA == nil || observerA.OwnerReporterInstanceID != "reporter-b" {
		t.Fatalf("observer A owner = %#v, want reporter-b", observerA)
	}
}

func TestReporterRestartReplacesObserverInventoryAndFencesOldSession(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	aggregator := newTestAggregator(func() time.Time { return now })
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "old-hello", ReporterInstanceID: "reporter-old", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: node("a", "A"), InventoryGeneration: "old-inventory",
			Peers: []domain.PeerObservation{{Peer: node("b", "B")}, {Peer: node("c", "C")}},
		}},
	}
	if _, err := aggregator.ApplyAt(hello, now); err != nil {
		t.Fatal(err)
	}
	applySample(t, aggregator, "reporter-old", 2, "a", "A", "c", "C", "old-inventory", 100, 50)

	now = now.Add(time.Second)
	restartedHello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "new-hello", ReporterInstanceID: "reporter-new", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: node("a", "A"), InventoryGeneration: "new-inventory",
			Peers: []domain.PeerObservation{{Peer: node("b", "B")}},
		}},
	}
	if _, err := aggregator.ApplyAt(restartedHello, now); err != nil {
		t.Fatal(err)
	}

	observerID := aggregator.state.Aliases["stable:a"]
	peerBID := aggregator.state.Aliases["stable:b"]
	peerCID := aggregator.state.Aliases["stable:c"]
	observer := aggregator.state.Observers[observerID]
	if observer == nil || observer.OwnerReporterInstanceID != "reporter-new" ||
		observer.InventoryGeneration != "new-inventory" {
		t.Fatalf("restarted observer state = %#v", observer)
	}
	if _, retained := observer.Membership[peerBID]; !retained || len(observer.Membership) != 1 {
		t.Fatalf("restarted membership = %#v, want only B", observer.Membership)
	}
	edgeID, _, _ := domain.EdgeID(observerID, peerCID)
	if edge := aggregator.state.Edges[edgeID]; edge == nil || len(edge.Observations) != 0 {
		t.Fatalf("removed peer provenance was retained: %#v", edge)
	}
	if aggregator.state.Reporters["reporter-old"] != nil {
		t.Fatalf("old reporter state was retained: %#v", aggregator.state.Reporters["reporter-old"])
	}

	lastReport := aggregator.state.Nodes[observerID].LastReport
	now = now.Add(time.Second)
	delayed := sampleReport(
		"reporter-old", 3, "a", "A", "c", "C", "old-inventory",
		domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"}, 100, 50,
	)
	delayed.CollectedAt = now
	result, err := aggregator.ApplyAt(delayed, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Accepted || !result.Receipt.ResyncRequired || result.Changed {
		t.Fatalf("delayed old-session receipt = %#v, changed=%t", result.Receipt, result.Changed)
	}
	if !aggregator.state.Nodes[observerID].LastReport.Equal(lastReport) {
		t.Fatal("delayed old-session sample refreshed observer liveness")
	}
	if aggregator.state.Reporters["reporter-old"] != nil {
		t.Fatal("rejected old session recreated durable reporter state")
	}
	if edge := aggregator.state.Edges[edgeID]; len(edge.Observations) != 0 {
		t.Fatalf("delayed old-session sample restored provenance: %#v", edge.Observations)
	}
}

func TestRestoreMigratesReporterOwnedInventoryToObserverState(t *testing.T) {
	legacy := runtimeState{
		Reporters: map[string]*reporterState{
			"reporter": {
				LastSequence: 2,
				ReportIDs:    map[string]struct{}{"sample": {}},
				ObserverIDs:  map[string]struct{}{"n_a": {}},
				LegacyInventories: map[string]string{
					"n_a": "inventory",
				},
				LegacyMemberships: map[string]map[string]struct{}{
					"n_a": {"n_b": {}},
				},
			},
		},
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	aggregator := newTestAggregator(time.Now)
	if err := aggregator.RestoreState(payload); err != nil {
		t.Fatal(err)
	}
	observer := aggregator.state.Observers["n_a"]
	if observer == nil || observer.OwnerReporterInstanceID != "reporter" ||
		observer.InventoryGeneration != "inventory" {
		t.Fatalf("migrated observer state = %#v", observer)
	}
	if _, exists := observer.Membership["n_b"]; !exists || len(observer.Membership) != 1 {
		t.Fatalf("migrated membership = %#v", observer.Membership)
	}
	migrated, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(migrated, []byte(`"inventories"`)) || bytes.Contains(migrated, []byte(`"memberships"`)) {
		t.Fatalf("migrated state retained reporter-owned inventory: %s", migrated)
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

func relayReport(
	sequence int64,
	sessionID string,
	vni int64,
	source, target domain.RelaySessionClient,
	sourceToTarget, targetToSource int64,
) domain.ReportEnvelope {
	collectedAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	return domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: fmt.Sprintf("relay-%d", sequence),
		ReporterInstanceID: "relay-reporter", Sequence: sequence, CollectedAt: collectedAt,
		Kind: domain.ReportRelaySessionUpdate,
		RelaySessions: []domain.RelaySessionObservation{{
			Relay: node("relay", "Relay"), Source: source, Target: target,
			SessionID: sessionID, VNI: vni,
			SourceToTargetBytes: sourceToTarget, TargetToSourceBytes: targetToSource,
			SourceToTargetDelta: sourceToTarget, TargetToSourceDelta: targetToSource,
			SampleDurationMS: 2000, LastActive: collectedAt,
		}},
	}
}

func topologyNodeID(t *testing.T, topology domain.Topology, stableID string) string {
	t.Helper()
	for _, candidate := range topology.Nodes {
		if candidate.StableNodeID == stableID {
			return candidate.ID
		}
	}
	t.Fatalf("topology has no node with stable ID %q: %#v", stableID, topology.Nodes)
	return ""
}

func edgeIDFor(leftID, rightID string) string {
	edgeID, _, _ := domain.EdgeID(leftID, rightID)
	return edgeID
}

func topologyEdgeWithRelaySession(t *testing.T, topology domain.Topology) domain.TopologyEdge {
	t.Helper()
	for _, edge := range topology.Edges {
		for _, observation := range edge.Observations {
			if observation.RelaySession != nil {
				return edge
			}
		}
	}
	t.Fatalf("topology has no relay session edge: %#v", topology.Edges)
	return domain.TopologyEdge{}
}

func relayProvenance(t *testing.T, edge domain.TopologyEdge) *domain.RelaySessionProvenance {
	t.Helper()
	for _, observation := range edge.Observations {
		if observation.RelaySession != nil {
			return observation.RelaySession
		}
	}
	t.Fatalf("edge has no relay provenance: %#v", edge)
	return nil
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

func identity(value domain.NodeIdentity) *domain.NodeIdentity {
	return &value
}
