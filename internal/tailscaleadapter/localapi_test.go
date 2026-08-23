package tailscaleadapter

import "testing"

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
