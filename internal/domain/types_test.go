package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOSIsDisplayMetadataOnly(t *testing.T) {
	linux := NodeIdentity{StableNodeID: "node-a", OS: "linux"}
	macos := NodeIdentity{StableNodeID: "node-a", OS: "macos"}
	if linux.IdentityKey() != macos.IdentityKey() || linux.CanonicalID() != macos.CanonicalID() {
		t.Fatal("OS changed canonical identity")
	}
	payload, err := json.Marshal(NodeIdentity{StableNodeID: "node-a"})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"stableNodeId":"node-a","hostname":""}` {
		t.Fatalf("legacy identity JSON = %s", payload)
	}
}

func TestHelloBaselineAllowsZeroSampleDuration(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: ReportObserverHello,
		Observers: []ObserverReport{{
			Observer: NodeIdentity{StableNodeID: "a", Hostname: "A"}, InventoryGeneration: "inventory",
			Peers: []PeerObservation{{
				Peer: NodeIdentity{StableNodeID: "b", Hostname: "B"},
				Path: PathObservation{Kind: PathUnknown}, LastActive: at,
			}},
		}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("zero-duration hello baseline is invalid: %v", err)
	}
	report.Kind = ReportTrafficSample
	if err := report.Validate(); err == nil {
		t.Fatal("zero-duration traffic sample was accepted")
	}
}

func TestIdentityDoesNotRequireHostname(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: "hello", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: ReportObserverHello,
		Observers: []ObserverReport{{
			Observer: NodeIdentity{StableNodeID: "a"}, InventoryGeneration: "inventory",
			Peers: []PeerObservation{{
				Peer: NodeIdentity{DiscoKey: "disco-b"},
				Path: PathObservation{Kind: PathUnknown}, LastActive: at,
			}},
		}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("identity without hostname is invalid: %v", err)
	}
}

func TestRelaySessionUpdateRequiresThirdPartyTrafficObservation(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: "relay", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: ReportRelaySessionUpdate,
		RelaySessions: []RelaySessionObservation{{
			Relay:     NodeIdentity{StableNodeID: "relay"},
			Source:    NodeIdentity{DiscoKey: "disco-a"},
			Target:    NodeIdentity{TailscaleIPs: []string{"100.64.0.2"}},
			SessionID: "session", VNI: 7,
			SourceToTargetBytes: 1200, TargetToSourceBytes: 400,
			SourceToTargetDelta: 120, TargetToSourceDelta: 40,
			SampleDurationMS: 2000, LastActive: at,
		}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("valid relay session update rejected: %v", err)
	}

	report.Observers = []ObserverReport{{
		Observer: NodeIdentity{StableNodeID: "relay"}, InventoryGeneration: "unused",
	}}
	if err := report.Validate(); err == nil {
		t.Fatal("mixed relay session and observer peer report was accepted")
	}
}
