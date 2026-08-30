package aggregate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestDirectoryOnlyDeviceNeverAppearsInLive(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_directory")
	result, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices: []domain.DirectoryDevice{{
			StableNodeID: "stable-directory", NodeKey: "nodekey:directory-only", DNSName: "catalog-only.example.ts.net",
			TailscaleIPs: []string{"100.64.0.20"},
		}},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.CanonicalStateChanged {
		t.Fatalf("apply result = %#v", result)
	}
	directory := aggregator.DeviceDirectory()
	if len(directory.Devices) != 1 || directory.Devices[0].ID != "n_directory" || directory.Devices[0].Runtime != nil {
		t.Fatalf("directory = %#v", directory)
	}
	topology := aggregator.Snapshot()
	if len(topology.Nodes) != 0 || len(topology.Edges) != 0 || len(topology.Observers) != 0 {
		t.Fatalf("directory-only device entered Live: %#v", topology)
	}
	if _, exists := aggregator.state.Aliases["ip:100.64.0.20"]; exists {
		t.Fatal("directory display IP entered runtime alias index")
	}
	if _, exists := aggregator.state.Aliases["node-key:nodekey:directory-only"]; exists {
		t.Fatal("directory-only NodeKey entered runtime alias index")
	}
}

func TestDirectoryEnrichesPlaceholderWithoutReplacingRuntimeEvidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_observer", "n_peer")
	hello := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer", Hostname: "observer"}, InventoryGeneration: "g1",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{
				NodeKey: "nodekey:peer", DNSName: "runtime.example.ts.net", Hostname: "runtime-host",
				OS: "linux", TailscaleIPs: []string{"100.64.0.1"},
			}}},
		}},
	}
	if _, err := aggregator.ApplyAt(hello, now); err != nil {
		t.Fatal(err)
	}
	peerID := aggregator.state.Aliases["node-key:nodekey:peer"]
	directoryAt := now.Add(time.Minute)
	_, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: directoryAt,
		Devices: []domain.DirectoryDevice{{
			StableNodeID: "stable-peer", NodeKey: "nodekey:peer",
			DNSName: "directory.example.ts.net", Hostname: "directory-host", OS: "macos",
			TailscaleIPs: []string{"100.64.0.2"}, Tags: []string{"tag:catalog"},
		}},
	}, healthyDirectorySync(directoryAt))
	if err != nil {
		t.Fatal(err)
	}
	if got := aggregator.state.Aliases["stable:stable-peer"]; got != peerID {
		t.Fatalf("directory StableNodeID = %q, want placeholder %q", got, peerID)
	}
	if len(aggregator.state.Nodes) != 2 {
		t.Fatalf("directory enrichment created a third node: %#v", aggregator.state.Nodes)
	}
	if _, exists := aggregator.state.Aliases["ip:100.64.0.2"]; exists {
		t.Fatal("directory IP entered alias index")
	}

	node := findTopologyNode(t, aggregator.Snapshot(), "stable-peer")
	if node.DNSName != "directory.example.ts.net" || node.Hostname != "directory-host" ||
		node.OS != "macos" || len(node.TailscaleIPs) != 1 || node.TailscaleIPs[0] != "100.64.0.2" {
		t.Fatalf("effective presentation = %#v", node)
	}
	if node.Observable || node.Online || node.Directory == nil || len(node.Directory.Conflicts) != 4 {
		t.Fatalf("runtime authority or conflicts = %#v", node)
	}
	directory := aggregator.DeviceDirectory()
	entry := directory.Devices[0]
	if entry.Runtime == nil || entry.Runtime.Identity.DNSName != "runtime.example.ts.net" || entry.Runtime.Observable {
		t.Fatalf("runtime evidence = %#v", entry.Runtime)
	}
}

func TestSuccessfulDirectoryReplacementFallsBackToRuntimePresentation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_runtime")
	applyHelloWithIdentity(t, aggregator, "reporter", domain.NodeIdentity{
		StableNodeID: "stable-runtime", DNSName: "runtime.example.ts.net", Hostname: "runtime", OS: "linux",
	})
	_, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices: []domain.DirectoryDevice{{
			StableNodeID: "stable-runtime", DNSName: "directory.example.ts.net", Hostname: "directory", OS: "macos",
		}},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	if got := findTopologyNode(t, aggregator.Snapshot(), "stable-runtime").DNSName; got != "directory.example.ts.net" {
		t.Fatalf("directory DNS = %q", got)
	}
	later := now.Add(5 * time.Minute)
	if _, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{CollectedAt: later}, healthyDirectorySync(later)); err != nil {
		t.Fatal(err)
	}
	if len(aggregator.DeviceDirectory().Devices) != 0 {
		t.Fatalf("missing device retained in current directory: %#v", aggregator.DeviceDirectory())
	}
	node := findTopologyNode(t, aggregator.Snapshot(), "stable-runtime")
	if node.DNSName != "runtime.example.ts.net" || node.Directory != nil {
		t.Fatalf("runtime fallback = %#v", node)
	}
	if aggregator.state.Aliases["stable:stable-runtime"] != "n_runtime" {
		t.Fatal("successful deletion discarded canonical identity")
	}
}

func TestDirectoryNodeKeyCannotMergeDifferentStableNodes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_runtime", "n_directory")
	applyHelloWithIdentity(t, aggregator, "reporter-a", domain.NodeIdentity{
		StableNodeID: "stable-runtime", NodeKey: "nodekey:shared", Hostname: "runtime",
	})
	_, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices: []domain.DirectoryDevice{{
			StableNodeID: "stable-directory", NodeKey: "nodekey:shared", Hostname: "directory",
		}},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	runtimeID := aggregator.state.Aliases["stable:stable-runtime"]
	directoryID := aggregator.state.Aliases["stable:stable-directory"]
	if runtimeID == directoryID || len(aggregator.state.Nodes) != 2 {
		t.Fatalf("conflicting stable nodes merged: runtime=%q directory=%q nodes=%#v", runtimeID, directoryID, aggregator.state.Nodes)
	}
	if got := aggregator.state.Aliases["node-key:nodekey:shared"]; got != runtimeID {
		t.Fatalf("directory hijacked NodeKey alias: %q", got)
	}
	entry := aggregator.DeviceDirectory().Devices[0]
	if entry.ID != directoryID || entry.IdentityStatus != domain.IdentityConflict {
		t.Fatalf("directory identity conflict = %#v", entry)
	}
	runtimeNode := findTopologyNode(t, aggregator.Snapshot(), "stable-runtime")
	if runtimeNode.IdentityStatus != domain.IdentityConflict {
		t.Fatalf("runtime side did not expose identity conflict: %#v", runtimeNode)
	}
}

func TestDirectoryDuplicateNodeKeysRemainDistinct(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_alpha", "n_beta")
	_, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices: []domain.DirectoryDevice{
			{StableNodeID: "stable-alpha", NodeKey: "nodekey:shared", Hostname: "alpha"},
			{StableNodeID: "stable-beta", NodeKey: "nodekey:shared", Hostname: "beta"},
		},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	directory := aggregator.DeviceDirectory()
	if len(directory.Devices) != 2 || directory.Devices[0].ID == directory.Devices[1].ID {
		t.Fatalf("duplicate NodeKeys merged: %#v", directory.Devices)
	}
	for _, device := range directory.Devices {
		if device.IdentityStatus != domain.IdentityConflict {
			t.Fatalf("missing conflict on %#v", device)
		}
	}
	if _, exists := aggregator.state.Aliases["node-key:nodekey:shared"]; exists {
		t.Fatal("ambiguous directory NodeKey entered alias index")
	}
	if len(aggregator.Snapshot().Nodes) != 0 {
		t.Fatal("directory-only conflict entered Live")
	}
}

func TestRuntimeNodeKeyConflictAlsoKeepsStableNodesDistinct(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_alpha", "n_beta")
	applyHelloWithIdentity(t, aggregator, "reporter-a", domain.NodeIdentity{
		StableNodeID: "stable-alpha", NodeKey: "nodekey:shared", Hostname: "alpha",
	})
	applyHelloWithIdentity(t, aggregator, "reporter-b", domain.NodeIdentity{
		StableNodeID: "stable-beta", NodeKey: "nodekey:shared", Hostname: "beta",
	})
	if aggregator.state.Aliases["stable:stable-alpha"] == aggregator.state.Aliases["stable:stable-beta"] || len(aggregator.state.Nodes) != 2 {
		t.Fatalf("runtime NodeKey merged different StableNodeIDs: %#v", aggregator.state.Nodes)
	}
	if got := aggregator.state.Aliases["node-key:nodekey:shared"]; got != "n_alpha" {
		t.Fatalf("conflicting report replaced existing NodeKey alias: %q", got)
	}
}

func TestDirectoryStateClonesRestoresStalesAndClears(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_directory")
	_, err := aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices:     []domain.DirectoryDevice{{StableNodeID: "stable-directory", Hostname: "directory", Tags: []string{"tag:a"}}},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	attempt := now.Add(5 * time.Minute)
	retry := attempt.Add(30 * time.Second)
	if err := aggregator.UpdateDirectorySyncState(domain.DirectorySyncState{
		Status: domain.DirectorySyncStale, LastAttemptAt: &attempt, LastSuccessAt: &now,
		NextRetryAt: &retry, ErrorCode: domain.DirectoryErrorUnavailable,
	}); err != nil {
		t.Fatal(err)
	}
	clone, err := aggregator.Clone()
	if err != nil {
		t.Fatal(err)
	}
	clone.state.Directory.Snapshot.Devices[0].Tags[0] = "tag:changed"
	*clone.state.Directory.Sync.LastAttemptAt = now.Add(time.Hour)
	if got := aggregator.DeviceDirectory().Devices[0].Device.Tags[0]; got != "tag:a" {
		t.Fatalf("clone mutated source tag: %q", got)
	}
	if got := *aggregator.DeviceDirectory().Sync.LastAttemptAt; !got.Equal(attempt) {
		t.Fatalf("clone mutated source sync time: %s", got)
	}
	returned := aggregator.DeviceDirectory()
	returned.Devices[0].Device.Tags[0] = "tag:return-mutated"
	*returned.Sync.LastSuccessAt = now.Add(2 * time.Hour)
	unchanged := aggregator.DeviceDirectory()
	if got := unchanged.Devices[0].Device.Tags[0]; got != "tag:a" {
		t.Fatalf("returned directory mutated source tag: %q", got)
	}
	if got := *unchanged.Sync.LastSuccessAt; !got.Equal(now) {
		t.Fatalf("returned directory mutated source success time: %s", got)
	}
	payload, err := aggregator.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	restored := directoryTestAggregator(now)
	if err := restored.RestoreState(payload); err != nil {
		t.Fatal(err)
	}
	directory := restored.DeviceDirectory()
	if directory.Sync.Status != domain.DirectorySyncStale || len(directory.Devices) != 1 {
		t.Fatalf("restored directory = %#v", directory)
	}
	if !restored.ClearDirectory() || restored.DeviceDirectory().Sync.Status != domain.DirectorySyncDisabled {
		t.Fatalf("cleared directory = %#v", restored.DeviceDirectory())
	}
	if restored.state.Aliases["stable:stable-directory"] != "n_directory" || restored.state.Nodes["n_directory"] == nil {
		t.Fatal("clearing directory deleted canonical identity")
	}
	if cleared, err := json.Marshal(restored.state); err != nil || string(cleared) == "" {
		t.Fatalf("cleared state marshal: %s %v", cleared, err)
	}
}

func TestDirectoryReferenceFollowsLaterCanonicalMerge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	aggregator := directoryTestAggregator(now, "n_observer", "n_node_key", "n_disco")
	_, err := aggregator.ApplyAt(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer:            domain.NodeIdentity{StableNodeID: "observer", Hostname: "observer"},
			InventoryGeneration: "inventory",
			Peers: []domain.PeerObservation{
				{Peer: domain.NodeIdentity{NodeKey: "nodekey:peer", Hostname: "node-key-placeholder"}},
				{Peer: domain.NodeIdentity{DiscoKey: "discokey:peer", Hostname: "disco-placeholder"}},
			},
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = aggregator.ApplyDirectorySnapshot(domain.DirectorySnapshot{
		CollectedAt: now,
		Devices: []domain.DirectoryDevice{{
			StableNodeID: "stable-peer", NodeKey: "nodekey:peer", Hostname: "directory-peer",
		}},
	}, healthyDirectorySync(now))
	if err != nil {
		t.Fatal(err)
	}
	directoryBefore := aggregator.DeviceDirectory().Devices[0].ID
	if directoryBefore != "n_node_key" {
		t.Fatalf("directory initially bound to %q", directoryBefore)
	}

	_, err = aggregator.ApplyAt(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "traffic", ReporterInstanceID: "reporter", Sequence: 2,
		CollectedAt: now, Kind: domain.ReportInventoryUpdate,
		Observers: []domain.ObserverReport{{
			Observer:            domain.NodeIdentity{StableNodeID: "observer", Hostname: "observer"},
			InventoryGeneration: "inventory-2",
			Peers: []domain.PeerObservation{{Peer: domain.NodeIdentity{
				StableNodeID: "stable-peer", DiscoKey: "discokey:peer", Hostname: "runtime-peer",
			}}},
		}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	directoryAfter := aggregator.DeviceDirectory().Devices[0].ID
	stableID := aggregator.state.Aliases["stable:stable-peer"]
	if directoryAfter != stableID || directoryAfter != directoryBefore {
		t.Fatalf("directory reference did not follow merge: before=%q after=%q stable=%q", directoryBefore, directoryAfter, stableID)
	}
	if got := aggregator.state.Redirects["n_disco"]; got != directoryAfter {
		t.Fatalf("merged placeholder redirect = %q, want %q", got, directoryAfter)
	}
}

func healthyDirectorySync(at time.Time) domain.DirectorySyncState {
	return domain.DirectorySyncState{Status: domain.DirectorySyncHealthy, LastAttemptAt: &at, LastSuccessAt: &at}
}

func directoryTestAggregator(now time.Time, ids ...string) *Aggregator {
	return New(Options{
		HeartbeatInterval: time.Minute,
		Now:               func() time.Time { return now },
		NewNodeID: func() string {
			if len(ids) == 0 {
				panic("unexpected canonical node allocation")
			}
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
}

func applyHelloWithIdentity(t *testing.T, aggregator *Aggregator, reporterID string, identity domain.NodeIdentity) {
	t.Helper()
	now := aggregator.now().UTC()
	_, err := aggregator.ApplyAt(domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "hello-" + reporterID, ReporterInstanceID: reporterID, Sequence: 1,
		CollectedAt: now, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{Observer: identity, InventoryGeneration: "inventory"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
}

func findTopologyNode(t *testing.T, topology domain.Topology, stableID string) domain.TopologyNode {
	t.Helper()
	for _, node := range topology.Nodes {
		if node.StableNodeID == stableID {
			return node
		}
	}
	t.Fatalf("topology has no node with StableNodeID %q: %#v", stableID, topology.Nodes)
	return domain.TopologyNode{}
}
