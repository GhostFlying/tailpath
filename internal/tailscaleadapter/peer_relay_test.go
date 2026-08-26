package tailscaleadapter

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"tailscale.com/client/local"

	"github.com/GhostFlying/tailpath/internal/collector"
)

func TestPeerRelaySnapshotClassifiesCapability(t *testing.T) {
	tests := []struct {
		name       string
		transport  *relayFixtureTransport
		capability collector.RelayCapability
		wantError  bool
		requests   []string
	}{
		{
			name: "unsupported", transport: &relayFixtureTransport{relayStatusCode: http.StatusNotFound},
			capability: collector.RelayUnsupported,
			requests:   []string{"GET /localapi/v0/debug-peer-relay-sessions"},
		},
		{
			name: "disabled", transport: &relayFixtureTransport{relayFixture: "disabled.json"},
			capability: collector.RelayDisabled,
			requests:   []string{"GET /localapi/v0/debug-peer-relay-sessions"},
		},
		{
			name: "enabled empty", transport: &relayFixtureTransport{relayFixture: "empty.json"},
			capability: collector.RelayEnabled,
			requests:   []string{"GET /localapi/v0/debug-peer-relay-sessions"},
		},
		{
			name: "server failure", transport: &relayFixtureTransport{relayStatusCode: http.StatusInternalServerError},
			capability: collector.RelayTransientFailure, wantError: true,
			requests: []string{"GET /localapi/v0/debug-peer-relay-sessions"},
		},
		{
			name: "malformed payload", transport: &relayFixtureTransport{relayFixture: "malformed-endpoint.json"},
			capability: collector.RelayTransientFailure, wantError: true,
			requests: []string{"GET /localapi/v0/debug-peer-relay-sessions"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.transport.t = t
			source := NewLocalSourceWithClient(&local.Client{Transport: test.transport, OmitAuth: true})
			snapshot, err := source.PeerRelaySnapshot(context.Background())
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %v", err, test.wantError)
			}
			if snapshot.Capability != test.capability {
				t.Fatalf("capability = %q, want %q", snapshot.Capability, test.capability)
			}
			if !reflect.DeepEqual(test.transport.requests, test.requests) {
				t.Fatalf("requests = %v, want %v", test.transport.requests, test.requests)
			}
		})
	}
}

func TestPeerRelaySnapshotStabilizesOrderingAndEnrichesUniqueDisco(t *testing.T) {
	active := peerRelaySnapshotFromFixture(t, "active.json", http.StatusOK)
	reordered := peerRelaySnapshotFromFixture(t, "reordered.json", http.StatusOK)
	if active.Capability != collector.RelayEnabled || len(active.Sessions) != 1 {
		t.Fatalf("active snapshot = %#v", active)
	}
	left := active.Sessions[0]
	right := reordered.Sessions[0]
	if left.SessionID != right.SessionID || left.Source.SessionClientID != right.Source.SessionClientID ||
		left.Target.SessionClientID != right.Target.SessionClientID {
		t.Fatalf("reordered session IDs changed: active=%#v reordered=%#v", left, right)
	}
	if left.Source.Identity == nil || left.Source.Identity.NodeKey == "" || left.Source.Identity.DiscoKey == "" {
		t.Fatalf("unique disco hint was not enriched: %#v", left.Source)
	}
	if left.Target.Identity != nil {
		t.Fatalf("ambiguous disco hint resolved: %#v", left.Target.Identity)
	}
	if left.Source.BytesSent != 4096 || left.Target.BytesSent != 8192 ||
		right.Source.BytesSent != 4608 || right.Target.BytesSent != 9216 {
		t.Fatalf("directional counters changed with upstream ordering: active=%#v reordered=%#v", left, right)
	}
	if left.VNI != 7 || left.Source.Endpoint == "" || left.Target.Endpoint == "" {
		t.Fatalf("relay runtime attributes missing: %#v", left)
	}
}

func TestPeerRelaySnapshotKeepsSessionsWhenDiscoEvidenceIsUnsupported(t *testing.T) {
	snapshot := peerRelaySnapshotFromFixture(t, "active.json", http.StatusNotFound)
	if snapshot.Capability != collector.RelayEnabled || len(snapshot.Sessions) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Sessions[0].Source.Identity != nil || snapshot.Sessions[0].Target.Identity != nil {
		t.Fatalf("unsupported disco API invented identity: %#v", snapshot.Sessions[0])
	}
}

func TestPeerRelaySnapshotTreatsDiscoFailureAsTransient(t *testing.T) {
	transport := &relayFixtureTransport{t: t, discoStatusCode: http.StatusInternalServerError}
	source := NewLocalSourceWithClient(&local.Client{Transport: transport, OmitAuth: true})
	snapshot, err := source.PeerRelaySnapshot(context.Background())
	if err == nil || snapshot.Capability != collector.RelayTransientFailure {
		t.Fatalf("snapshot=%#v error=%v", snapshot, err)
	}
}

func peerRelaySnapshotFromFixture(t *testing.T, fixture string, discoStatus int) collector.RelaySnapshot {
	t.Helper()
	transport := &relayFixtureTransport{t: t, relayFixture: fixture, discoStatusCode: discoStatus}
	source := NewLocalSourceWithClient(&local.Client{Transport: transport, OmitAuth: true})
	snapshot, err := source.PeerRelaySnapshot(context.Background())
	if err != nil {
		t.Fatalf("read fixture %q: %v", fixture, err)
	}
	transport.assertPassive(t)
	return snapshot
}
