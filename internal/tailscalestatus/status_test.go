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
			},
			peerKey: {
				ID: "peer-stable", NodeID: tailcfg.NodeID(202), PublicKey: peerKey,
				HostName: "peer", OS: "linux", RxBytes: 123, TxBytes: 456,
				PeerRelay: "100.64.0.8:41641:vni:7", CurAddr: "192.0.2.5:41641", Relay: "hkg",
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
	for _, candidate := range snapshot.Peers {
		if candidate.Identity.StableNodeID == "peer-stable" {
			peer = candidate
		}
	}
	if peer.Identity.StableNodeID == "" || peer.RxBytes != 123 || peer.TxBytes != 456 ||
		peer.Path.Kind != exporter.PathPeerRelay || peer.Path.PeerRelayStableNodeID != "relay-stable" ||
		peer.Path.PeerRelayVNI == nil || *peer.Path.PeerRelayVNI != 7 {
		t.Fatalf("peer = %#v", peer)
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
			if got := Path(&test.peer, map[string]string{"100.64.0.8": "relay"}).Kind; got != test.want {
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
