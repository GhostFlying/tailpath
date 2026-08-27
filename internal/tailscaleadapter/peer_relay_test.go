package tailscaleadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"reflect"
	"testing"

	"tailscale.com/client/local"
	"tailscale.com/net/udprelay/status"
	"tailscale.com/types/key"

	"github.com/GhostFlying/tailpath/exporter"
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
	if left.Target.Identity == nil || left.Target.Identity.NodeKey == "" || left.Target.Identity.DiscoKey == "" {
		t.Fatalf("unique disco hint was not enriched: %#v", left.Target)
	}
	if left.Source.Identity != nil {
		t.Fatalf("ambiguous disco hint resolved: %#v", left.Source.Identity)
	}
	if left.Source.BytesSent != 8192 || left.Target.BytesSent != 4096 ||
		right.Source.BytesSent != 9216 || right.Target.BytesSent != 4608 {
		t.Fatalf("directional counters changed with upstream ordering: active=%#v reordered=%#v", left, right)
	}
	if left.VNI != 7 || left.Source.Endpoint == "" || left.Target.Endpoint == "" {
		t.Fatalf("relay runtime attributes missing: %#v", left)
	}
}

func TestPeerRelaySnapshotKeepsClientIDsAcrossIdentityEnrichment(t *testing.T) {
	anonymous := peerRelaySnapshotFromFixture(t, "active.json", http.StatusNotFound)
	resolved := peerRelaySnapshotFromFixture(t, "active.json", http.StatusOK)
	if anonymous.IdentityEvidence != collector.RelayIdentityDegraded ||
		resolved.IdentityEvidence != collector.RelayIdentityAvailable {
		t.Fatalf("identity evidence = %q/%q", anonymous.IdentityEvidence, resolved.IdentityEvidence)
	}
	left, right := anonymous.Sessions[0], resolved.Sessions[0]
	if left.Source.SessionClientID != right.Source.SessionClientID ||
		left.Target.SessionClientID != right.Target.SessionClientID {
		t.Fatalf("identity enrichment changed client IDs: anonymous=%#v resolved=%#v", left, right)
	}
	if left.Source.DiscoShort != right.Source.DiscoShort || left.Target.DiscoShort != right.Target.DiscoShort {
		t.Fatalf("identity enrichment changed canonical direction: anonymous=%#v resolved=%#v", left, right)
	}
}

func TestRelayClientIDIgnoresEndpointDriftWhenShortDiscoIsStable(t *testing.T) {
	initial := status.ClientInfo{
		Endpoint: netip.MustParseAddrPort("192.0.2.10:51001"), ShortDisco: "d:0011223344556677",
	}
	moved := initial
	moved.Endpoint = netip.MustParseAddrPort("192.0.2.20:52002")
	left := adaptRelayClient(7, initial, nil)
	right := adaptRelayClient(7, moved, nil)
	if left.SessionClientID != right.SessionClientID {
		t.Fatalf("endpoint drift changed client ID: %q != %q", left.SessionClientID, right.SessionClientID)
	}
}

func TestRelayClientIDUsesEndpointOnlyWithoutShortDisco(t *testing.T) {
	initial := status.ClientInfo{Endpoint: netip.MustParseAddrPort("192.0.2.10:51001")}
	moved := initial
	moved.Endpoint = netip.MustParseAddrPort("192.0.2.20:52002")
	left := adaptRelayClient(7, initial, nil)
	right := adaptRelayClient(7, moved, nil)
	if left.SessionClientID == right.SessionClientID {
		t.Fatalf("endpoint-only clients retained one ID across endpoint drift: %q", left.SessionClientID)
	}
}

func TestRelaySessionDisambiguatesCollidingShortDiscoByEndpoint(t *testing.T) {
	var discoKeys map[key.NodePublic]key.DiscoPublic
	if err := json.Unmarshal(readRelayFixture(t, "peer-disco-keys.json"), &discoKeys); err != nil {
		t.Fatal(err)
	}
	sessions, err := adaptRelaySessions([]status.ServerSession{{
		VNI: 7,
		Client1: status.ClientInfo{
			Endpoint: netip.MustParseAddrPort("192.0.2.20:52002"), ShortDisco: "d:50d20b455ecf12bc",
		},
		Client2: status.ClientInfo{
			Endpoint: netip.MustParseAddrPort("192.0.2.10:51001"), ShortDisco: "d:50d20b455ecf12bc",
		},
	}}, discoKeys)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions=%#v error=%v", sessions, err)
	}
	session := sessions[0]
	if session.Source.SessionClientID == session.Target.SessionClientID {
		t.Fatalf("colliding short disco produced one client ID: %#v", session)
	}
	if session.Source.Endpoint != "192.0.2.10:51001" || session.Target.Endpoint != "192.0.2.20:52002" {
		t.Fatalf("endpoint fallback did not stabilize direction: %#v", session)
	}
	if session.Source.Identity != nil || session.Target.Identity != nil {
		t.Fatalf("colliding short disco retained ambiguous identity: %#v", session)
	}
}

func TestRelaySessionSkipsOnlyIndistinguishableCollision(t *testing.T) {
	sessions, err := adaptRelaySessions([]status.ServerSession{
		{
			VNI:     7,
			Client1: status.ClientInfo{ShortDisco: "d:collision"},
			Client2: status.ClientInfo{ShortDisco: "d:collision"},
		},
		{
			VNI: 8,
			Client1: status.ClientInfo{
				Endpoint: netip.MustParseAddrPort("192.0.2.10:51001"), ShortDisco: "d:left",
			},
			Client2: status.ClientInfo{
				Endpoint: netip.MustParseAddrPort("192.0.2.20:52002"), ShortDisco: "d:right",
			},
		},
	}, nil)
	if err != nil || len(sessions) != 1 || sessions[0].VNI != 8 {
		t.Fatalf("sessions=%#v error=%v", sessions, err)
	}
}

func TestRelayClientOrderingIgnoresCanonicalIdentityOrder(t *testing.T) {
	left := collector.RelayClientSnapshot{
		DiscoShort: "d:bbbbbbbbbbbbbbbb",
		Identity:   &exporter.NodeIdentity{NodeKey: "node-a", DiscoKey: "disco-a"},
	}
	right := collector.RelayClientSnapshot{
		DiscoShort: "d:aaaaaaaaaaaaaaaa",
		Identity:   &exporter.NodeIdentity{NodeKey: "node-z", DiscoKey: "disco-z"},
	}
	if relayClientSortKey(left) < relayClientSortKey(right) {
		t.Fatalf("canonical identity order replaced short-disco order: %q < %q",
			relayClientSortKey(left), relayClientSortKey(right))
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

func TestPeerRelaySnapshotDegradesDiscoFailureWithoutDroppingSessions(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		err        error
	}{
		{name: "forbidden", statusCode: http.StatusForbidden},
		{name: "server error", statusCode: http.StatusInternalServerError},
		{name: "transport error", err: context.DeadlineExceeded},
		{name: "malformed response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &relayFixtureTransport{
				t: t, discoStatusCode: test.statusCode, discoError: test.err,
			}
			if test.name == "malformed response" {
				transport.discoPayload = []byte("{")
			}
			source := NewLocalSourceWithClient(&local.Client{Transport: transport, OmitAuth: true})
			snapshot, err := source.PeerRelaySnapshot(context.Background())
			if err != nil || snapshot.Capability != collector.RelayEnabled ||
				snapshot.IdentityEvidence != collector.RelayIdentityDegraded || len(snapshot.Sessions) != 1 {
				t.Fatalf("snapshot=%#v error=%v", snapshot, err)
			}
		})
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
