package generated

import (
	"encoding/json"
	"testing"
)

func TestNodeIdentityDoesNotRequireHostname(t *testing.T) {
	stableNodeID := "node-a"
	payload, err := json.Marshal(NodeIdentity{StableNodeId: &stableNodeID})
	if err != nil {
		t.Fatal(err)
	}
	var identity map[string]any
	if err := json.Unmarshal(payload, &identity); err != nil {
		t.Fatal(err)
	}
	if identity["stableNodeId"] != stableNodeID {
		t.Fatalf("stableNodeId = %#v, want %q", identity["stableNodeId"], stableNodeID)
	}
	if _, exists := identity["hostname"]; exists {
		t.Fatalf("optional hostname was serialized: %s", payload)
	}
}

func TestNodeIdentityOSIsOptionalAndRoundTrips(t *testing.T) {
	var legacy NodeIdentity
	if err := json.Unmarshal([]byte(`{"stableNodeId":"node-a"}`), &legacy); err != nil {
		t.Fatalf("legacy identity without OS failed to decode: %v", err)
	}
	if legacy.Os != nil {
		t.Fatalf("legacy identity OS = %q, want nil", *legacy.Os)
	}
	os := "linux"
	payload, err := json.Marshal(NodeIdentity{Os: &os})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["os"] != "linux" {
		t.Fatalf("OS payload = %s", payload)
	}
}
