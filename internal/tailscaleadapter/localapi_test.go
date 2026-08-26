package tailscaleadapter

import (
	"testing"

	"tailscale.com/ipn/ipnstate"
)

func TestPeerRelayIP(t *testing.T) {
	tests := map[string]string{
		"100.64.0.8:41641:vni:7":          "100.64.0.8",
		"[fd7a::8]:41641:vni:12":          "fd7a::8",
		"[::ffff:100.64.0.8]:41641:vni:7": "100.64.0.8",
		"100.64.0.8:41641":                "100.64.0.8",
		"not-an-endpoint:vni:7":           "",
	}
	for input, expected := range tests {
		if got := peerRelayIP(input); got != expected {
			t.Errorf("peerRelayIP(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestPeerRelayEndpointParsesBoundedVNI(t *testing.T) {
	tests := []struct {
		value   string
		wantIP  string
		wantVNI *int64
	}{
		{value: "100.64.0.8:41641:vni:7", wantIP: "100.64.0.8", wantVNI: int64Pointer(7)},
		{value: "[fd7a::8]:41641:vni:16777215", wantIP: "fd7a::8", wantVNI: int64Pointer(16777215)},
		{value: "100.64.0.8:41641", wantIP: "100.64.0.8"},
		{value: "100.64.0.8:41641:vni:16777216", wantIP: "100.64.0.8"},
		{value: "100.64.0.8:41641:vni:not-a-number", wantIP: "100.64.0.8"},
		{value: "not-an-endpoint:vni:7"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			ip, vni := peerRelayEndpoint(test.value)
			if ip != test.wantIP || !equalInt64Pointers(vni, test.wantVNI) {
				t.Fatalf("peerRelayEndpoint(%q) = %q/%v, want %q/%v", test.value, ip, vni, test.wantIP, test.wantVNI)
			}
		})
	}
}

func TestPathObservationCarriesPeerRelayVNI(t *testing.T) {
	path := pathObservation(&ipnstate.PeerStatus{PeerRelay: "100.64.0.8:41641:vni:7"}, map[string]string{
		"100.64.0.8": "relay-stable-id",
	})
	if path.PeerRelayStableNodeID != "relay-stable-id" || path.PeerRelayVNI == nil || *path.PeerRelayVNI != 7 {
		t.Fatalf("peer relay path = %#v", path)
	}
}

func int64Pointer(value int64) *int64 { return &value }

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestNormalizeOS(t *testing.T) {
	tests := map[string]string{
		"linux":      "linux",
		"Darwin":     "macos",
		"macOS":      "macos",
		"WINDOWS":    "windows",
		"iOS":        "ios",
		"android":    "android",
		"freebsd-14": "freebsd-14",
	}
	for input, expected := range tests {
		if got := normalizeOS(input); got != expected {
			t.Errorf("normalizeOS(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestPeerIdentityCarriesNormalizedOS(t *testing.T) {
	identity := peerIdentity(&ipnstate.PeerStatus{OS: "Darwin"})
	if identity.OS != "macos" {
		t.Fatalf("peer identity OS = %q, want macos", identity.OS)
	}
}
