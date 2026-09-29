package tailscalestatus

import (
	"net/netip"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/GhostFlying/tailpath/exporter"
)

func TestSnapshotNormalizesIdentityCountersAndPaths(t *testing.T) {
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	relayKey := key.NewNode().Public()
	peerKey := key.NewNode().Public()
	selfKey := key.NewNode().Public()
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			ID: "self-stable", NodeID: 101, PublicKey: selfKey, HostName: "runtime",
			DNSName: "runtime.example.ts.net.", OS: "Darwin",
			TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")},
		},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			relayKey: {
				ID: "relay-stable", TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.8")},
				Addrs: []string{"192.168.1.8:41641", "203.0.113.8:41641"},
			},
			peerKey: {
				ID: "peer-stable", NodeID: tailcfg.NodeID(202), PublicKey: peerKey,
				HostName: "peer", OS: "linux", RxBytes: 123, TxBytes: 456,
				PeerRelay: "203.0.113.8:40000:vni:7", CurAddr: "192.0.2.5:41641", Relay: "hkg",
			},
		},
	}
	snapshot, err := Snapshot(status, at)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.CollectedAt.Equal(at) || snapshot.Observer.StableNodeID != "self-stable" ||
		snapshot.Observer.NodeID != "nodeid:101" || snapshot.Observer.NodeKey != selfKey.String() ||
		snapshot.Observer.DisplayName() != "runtime" || snapshot.Observer.OS != "macos" {
		t.Fatalf("observer = %#v", snapshot.Observer)
	}
	var peer exporter.PeerSnapshot
	var relay exporter.PeerSnapshot
	for _, candidate := range snapshot.Peers {
		if candidate.Identity.StableNodeID == "peer-stable" {
			peer = candidate
		}
		if candidate.Identity.StableNodeID == "relay-stable" {
			relay = candidate
		}
	}
	if peer.Identity.StableNodeID == "" || peer.RxBytes != 123 || peer.TxBytes != 456 ||
		peer.Path.Kind != exporter.PathPeerRelay || peer.Path.PeerRelayStableNodeID != "relay-stable" ||
		peer.Path.PeerRelayEndpoint != "203.0.113.8:40000" || peer.Path.PeerRelayResolution != "endpoint_match" ||
		peer.Path.PeerRelayVNI == nil || *peer.Path.PeerRelayVNI != 7 {
		t.Fatalf("peer = %#v", peer)
	}
	if got := relay.Identity.PublicEndpoints; len(got) != 1 || got[0] != "203.0.113.8:41641" {
		t.Fatalf("relay public endpoints = %#v", got)
	}
}

func TestRelayIdentitiesRequireUniquePublicIPOwnership(t *testing.T) {
	firstKey := key.NewNode().Public()
	secondKey := key.NewNode().Public()
	status := &ipnstate.Status{Peer: map[key.NodePublic]*ipnstate.PeerStatus{
		firstKey:  {ID: "first", Addrs: []string{"198.51.100.9:41641"}},
		secondKey: {ID: "second", Addrs: []string{"198.51.100.9:51234"}},
	}}
	if identity := RelayIdentities(status)["198.51.100.9"]; identity.StableNodeID != "" {
		t.Fatalf("shared public IP resolved to %#v", identity)
	}
	status.Peer[secondKey].Addrs = []string{"198.51.100.10:41641"}
	if identity := RelayIdentities(status)["198.51.100.9"]; identity.StableNodeID != "first" || identity.Resolution != "endpoint_match" {
		t.Fatalf("unique public IP resolved to %#v", identity)
	}
}

func TestRelayIdentitiesMarkExactTailscaleIPResolution(t *testing.T) {
	peerKey := key.NewNode().Public()
	status := &ipnstate.Status{Peer: map[key.NodePublic]*ipnstate.PeerStatus{
		peerKey: {ID: "relay", TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.8")}},
	}}
	identity := RelayIdentities(status)["100.64.0.8"]
	if identity.StableNodeID != "relay" || identity.Resolution != "tailscale_ip" {
		t.Fatalf("Tailscale IP resolution = %#v", identity)
	}
}

func TestPublicEndpointsKeepOnlyNormalizedPublicAddresses(t *testing.T) {
	peer := &ipnstate.PeerStatus{Addrs: []string{
		"192.168.1.8:41641", "100.64.0.8:41641", "203.0.113.8:41641", "[2001:4860:4860::8888]:41641",
	}}
	got := PublicEndpoints(peer)
	if len(got) != 2 || got[0] != "203.0.113.8:41641" || got[1] != "[2001:4860:4860::8888]:41641" {
		t.Fatalf("public endpoints = %#v", got)
	}
}

func TestPathPrecedenceAndUnknown(t *testing.T) {
	tests := []struct {
		name string
		peer ipnstate.PeerStatus
		want exporter.PathKind
	}{
		{name: "peer relay", peer: ipnstate.PeerStatus{PeerRelay: "100.64.0.8:41641:vni:7", CurAddr: "192.0.2.1:1", Relay: "hkg"}, want: exporter.PathPeerRelay},
		{name: "direct", peer: ipnstate.PeerStatus{CurAddr: "192.0.2.1:1", Relay: "hkg"}, want: exporter.PathDirect},
		{name: "derp", peer: ipnstate.PeerStatus{Relay: "hkg"}, want: exporter.PathDERP},
		{name: "unknown", want: exporter.PathUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Path(&test.peer, map[string]RelayIdentity{"100.64.0.8": {StableNodeID: "relay", Resolution: "tailscale_ip"}}).Kind; got != test.want {
				t.Fatalf("path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSnapshotRejectsUnavailableSelf(t *testing.T) {
	for _, status := range []*ipnstate.Status{nil, {}} {
		if _, err := Snapshot(status, time.Now()); err == nil {
			t.Fatal("status without self was accepted")
		}
	}
}

func TestTrackerInfersShortDERPFallbackAndExpiresIt(t *testing.T) {
	tracker := NewTracker()
	peerKey := key.NewNode().Public()
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{ID: "self"},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerKey: {ID: "peer", PeerRelay: "203.0.113.8:40000:vni:4293"},
		},
	}
	first, err := tracker.Snapshot(status, at)
	if err != nil {
		t.Fatal(err)
	}
	if first.Peers[0].Path.Kind != exporter.PathPeerRelay || first.Peers[0].PathEvidence != exporter.PathEvidenceObserved {
		t.Fatalf("first path = %#v", first.Peers[0])
	}

	status.Peer[peerKey].PeerRelay = ""
	status.Peer[peerKey].Relay = "hgh-custom"
	short, err := tracker.Snapshot(status, at.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	peer := short.Peers[0]
	if peer.Path.Kind != exporter.PathPeerRelay || peer.FallbackPath == nil ||
		peer.FallbackPath.Kind != exporter.PathDERP || peer.FallbackPath.DERPRegion != "hgh-custom" ||
		peer.PathEvidence != exporter.PathEvidenceInferred || peer.PathInferenceRule != FallbackInferenceRule {
		t.Fatalf("short fallback = %#v", peer)
	}

	long, err := tracker.Snapshot(status, at.Add(FallbackInferenceWindow+time.Second))
	if err != nil {
		t.Fatal(err)
	}
	peer = long.Peers[0]
	if peer.Path.Kind != exporter.PathDERP || peer.FallbackPath != nil ||
		peer.PathEvidence != exporter.PathEvidenceInferred || peer.PathInferenceRule != FallbackInferenceRule {
		t.Fatalf("expired fallback = %#v", peer)
	}
}

func TestTrackerDoesNotGuessStartupDERPAndReplacesRelay(t *testing.T) {
	tracker := NewTracker()
	peerKey := key.NewNode().Public()
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{ID: "self"},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerKey: {ID: "peer", Relay: "hgh-custom"},
		},
	}
	startup, err := tracker.Snapshot(status, at)
	if err != nil {
		t.Fatal(err)
	}
	if peer := startup.Peers[0]; peer.Path.Kind != exporter.PathDERP ||
		peer.PathEvidence != exporter.PathEvidenceObserved || peer.FallbackPath != nil {
		t.Fatalf("startup DERP = %#v", peer)
	}

	status.Peer[peerKey].Relay = ""
	status.Peer[peerKey].PeerRelay = "203.0.113.9:40000:vni:8"
	second, err := tracker.Snapshot(status, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if peer := second.Peers[0]; peer.Path.PeerRelayVNI == nil || *peer.Path.PeerRelayVNI != 8 || peer.FallbackPath != nil {
		t.Fatalf("replacement relay = %#v", peer)
	}
}

func TestTrackerClearsRelayWhenPeerDisappears(t *testing.T) {
	tracker := NewTracker()
	peerKey := key.NewNode().Public()
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{ID: "self"},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerKey: {ID: "peer", PeerRelay: "203.0.113.8:40000:vni:4293"},
		},
	}
	if _, err := tracker.Snapshot(status, at); err != nil {
		t.Fatal(err)
	}

	delete(status.Peer, peerKey)
	if _, err := tracker.Snapshot(status, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	status.Peer[peerKey] = &ipnstate.PeerStatus{ID: "peer", Relay: "hgh-custom"}
	reappeared, err := tracker.Snapshot(status, at.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	peer := reappeared.Peers[0]
	if peer.Path.Kind != exporter.PathDERP || peer.FallbackPath != nil ||
		peer.PathEvidence != exporter.PathEvidenceObserved || peer.PathInferenceRule != "" {
		t.Fatalf("reappeared DERP = %#v", peer)
	}
}

func TestTrackerResetBreaksRelayInferenceContinuity(t *testing.T) {
	tracker := NewTracker()
	peerKey := key.NewNode().Public()
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{ID: "self"},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerKey: {ID: "peer", PeerRelay: "203.0.113.8:40000:vni:4293"},
		},
	}
	if _, err := tracker.Snapshot(status, at); err != nil {
		t.Fatal(err)
	}

	tracker.Reset()
	status.Peer[peerKey].PeerRelay = ""
	status.Peer[peerKey].Relay = "hgh-custom"
	snapshot, err := tracker.Snapshot(status, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	peer := snapshot.Peers[0]
	if peer.Path.Kind != exporter.PathDERP || peer.FallbackPath != nil ||
		peer.PathEvidence != exporter.PathEvidenceObserved || peer.PathInferenceRule != "" {
		t.Fatalf("DERP after reset = %#v", peer)
	}
}
