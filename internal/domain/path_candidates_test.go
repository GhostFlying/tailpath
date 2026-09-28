package domain

import (
	"testing"
	"time"
)

func TestReconcilePathEvidenceEnrichesMatchingRelayEndpoint(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	endpoint := "203.0.113.8:40000"
	observations := []ObservationProvenance{
		{ObserverID: "source", ReceivedAt: now, Path: PathObservation{Kind: PathPeerRelay, PeerRelayEndpoint: endpoint}},
		{ObserverID: "target", ReceivedAt: now.Add(time.Second), Path: PathObservation{
			Kind: PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayEndpoint: endpoint,
			PeerRelayResolution: "endpoint_match",
		}},
	}
	reconciled := ReconcilePathEvidence("source", "target", PathObservation{}, observations)
	if reconciled.Path.PeerRelayStableNodeID != "relay" || reconciled.Path.PeerRelayEndpoint != endpoint || len(reconciled.Conflicts) != 0 {
		t.Fatalf("reconciled = %#v", reconciled)
	}
}

func TestPathCandidatesPreserveSwitchingEvidence(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	primary := PathObservation{Kind: PathPeerRelay, PeerRelayStableNodeID: "relay-hz", PeerRelayEndpoint: "203.0.113.8:40000"}
	pending := PathObservation{Kind: PathPeerRelay, PeerRelayEndpoint: "198.51.100.9:40000"}
	observations := []ObservationProvenance{
		{ObserverID: "source", ReceivedAt: now, Path: primary},
		{ObserverID: "target", ReceivedAt: now.Add(time.Second), Path: pending},
	}
	state, candidates := PathCandidates(primary, []PathObservation{pending}, observations)
	if state != PathSwitching || len(candidates) != 2 {
		t.Fatalf("state=%q candidates=%#v", state, candidates)
	}
	if candidates[0].ObserverCount != 1 || !candidates[0].LastObservedAt.Equal(now) ||
		candidates[1].ObserverCount != 1 || !candidates[1].LastObservedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("candidate provenance = %#v", candidates)
	}
}
