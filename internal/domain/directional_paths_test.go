package domain

import (
	"testing"
	"time"
)

func TestDirectionalPathsUsesEndpointVotesAndThirdPartyIdentityOnly(t *testing.T) {
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	vni8 := int64(8)
	vni4293 := int64(4293)
	observations := []ObservationProvenance{
		{ObserverID: "a", Path: PathObservation{Kind: PathPeerRelay, PeerRelayEndpoint: "203.0.113.8:40000", PeerRelayVNI: &vni8}, CollectedAt: at, ReceivedAt: at},
		{ObserverID: "b", Path: PathObservation{Kind: PathPeerRelay, PeerRelayStableNodeID: "relay-hz", PeerRelayEndpoint: "203.0.113.9:40000", PeerRelayVNI: &vni4293}, CollectedAt: at, ReceivedAt: at},
		{ObserverID: "relay-hz", Path: PathObservation{Kind: PathPeerRelay, PeerRelayStableNodeID: "relay-hz", PeerRelayEndpoint: "203.0.113.9:40000", PeerRelayVNI: &vni4293}, CollectedAt: at, ReceivedAt: at},
	}
	directions := DirectionalPaths("a", "b", observations)
	if len(directions) != 2 || directions[0].FromNodeID != "a" || directions[1].FromNodeID != "b" {
		t.Fatalf("directions = %#v", directions)
	}
	if directions[0].PrimaryPath.PeerRelayStableNodeID != "" {
		t.Fatalf("unrelated relay vote changed a direction: %#v", directions[0])
	}
	if directions[1].PrimaryPath.PeerRelayStableNodeID != "relay-hz" {
		t.Fatalf("known endpoint direction = %#v", directions[1])
	}
}

func TestSameDirectionalPathsIgnoresObservationTimeButTracksFallback(t *testing.T) {
	vni := int64(8)
	left := []DirectionalPathState{{
		FromNodeID: "a", ToNodeID: "b", ObserverID: "a", Evidence: PathEvidenceInferred,
		PrimaryPath:  PathObservation{Kind: PathPeerRelay, PeerRelayVNI: &vni},
		FallbackPath: &PathObservation{Kind: PathDERP, DERPRegion: "hgh-custom"},
		ReceivedAt:   time.Unix(1, 0),
	}}
	right := CloneDirectionalPaths(left)
	right[0].ReceivedAt = time.Unix(2, 0)
	if !SameDirectionalPaths(left, right) {
		t.Fatal("timestamp-only update changed logical direction")
	}
	right[0].FallbackPath.DERPRegion = "fra"
	if SameDirectionalPaths(left, right) {
		t.Fatal("fallback region change was ignored")
	}
}
