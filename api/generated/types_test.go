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
