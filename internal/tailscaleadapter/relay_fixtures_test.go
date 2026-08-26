package tailscaleadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tailscale.com/net/udprelay/status"
	"tailscale.com/types/key"
)

func TestPeerRelayStatusFixturesMatchUpstreamShape(t *testing.T) {
	tests := []struct {
		name         string
		wantPort     *uint16
		wantSessions int
		wantError    bool
	}{
		{name: "disabled", wantPort: nil},
		{name: "empty", wantPort: uint16Pointer(0)},
		{name: "active", wantPort: uint16Pointer(41641), wantSessions: 1},
		{name: "reordered", wantPort: uint16Pointer(41641), wantSessions: 1},
		{name: "counter-reset", wantPort: uint16Pointer(41641), wantSessions: 1},
		{name: "malformed-endpoint", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got status.ServerStatus
			err := json.Unmarshal(readRelayFixture(t, test.name+".json"), &got)
			if test.wantError {
				if err == nil {
					t.Fatal("malformed upstream fixture decoded without an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("decode upstream fixture: %v", err)
			}
			if !equalOptionalPort(got.UDPPort, test.wantPort) {
				t.Fatalf("UDPPort = %v, want %v", got.UDPPort, test.wantPort)
			}
			if len(got.Sessions) != test.wantSessions {
				t.Fatalf("sessions = %d, want %d", len(got.Sessions), test.wantSessions)
			}
		})
	}
}

func TestActivePeerRelayFixturePreservesDirectionalCounters(t *testing.T) {
	var got status.ServerStatus
	if err := json.Unmarshal(readRelayFixture(t, "active.json"), &got); err != nil {
		t.Fatalf("decode active fixture: %v", err)
	}
	session := got.Sessions[0]
	if session.VNI != 7 {
		t.Fatalf("VNI = %d, want 7", session.VNI)
	}
	if session.Client1.ShortDisco != "d:003cd7453e04a653" || session.Client1.BytesTx != 8192 {
		t.Fatalf("client 1 = %#v", session.Client1)
	}
	if session.Client2.ShortDisco != "d:50d20b455ecf12bc" || session.Client2.BytesTx != 4096 {
		t.Fatalf("client 2 = %#v", session.Client2)
	}
	if !session.Client1.Endpoint.Addr().Is4() || !session.Client2.Endpoint.Addr().Is6() {
		t.Fatalf("fixture endpoints are not the expected IPv4/IPv6 pair: %#v", session)
	}
}

func TestPeerDiscoKeyFixtureIncludesAmbiguousShortHint(t *testing.T) {
	var got map[key.NodePublic]key.DiscoPublic
	if err := json.Unmarshal(readRelayFixture(t, "peer-disco-keys.json"), &got); err != nil {
		t.Fatalf("decode peer disco keys: %v", err)
	}
	counts := make(map[string]int)
	for _, disco := range got {
		counts[disco.ShortString()]++
	}
	if counts["d:003cd7453e04a653"] != 2 {
		t.Fatalf("ambiguous short hint count = %d, want 2", counts["d:003cd7453e04a653"])
	}
	if counts["d:50d20b455ecf12bc"] != 1 {
		t.Fatalf("unique short hint count = %d, want 1", counts["d:50d20b455ecf12bc"])
	}
}

func readRelayFixture(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "peer-relay", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return payload
}

func uint16Pointer(value uint16) *uint16 { return &value }

func equalOptionalPort(left, right *uint16) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
