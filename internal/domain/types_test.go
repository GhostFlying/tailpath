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
			Source:    RelaySessionClient{SessionClientID: "client-a", Identity: &NodeIdentity{DiscoKey: "disco-a"}},
			Target:    RelaySessionClient{SessionClientID: "client-b", Identity: &NodeIdentity{TailscaleIPs: []string{"100.64.0.2"}}},
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

func TestRelaySessionValidationAllowsScopedAnonymousClients(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: "relay", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: ReportRelaySessionUpdate,
		RelaySessions: []RelaySessionObservation{{
			Relay:     NodeIdentity{StableNodeID: "relay"},
			Source:    RelaySessionClient{SessionClientID: "left", DiscoShort: "d:0011223344556677"},
			Target:    RelaySessionClient{SessionClientID: "right"},
			SessionID: "session", VNI: 1<<24 - 1,
			SourceToTargetDelta: 1, SampleDurationMS: 2000, LastActive: at,
		}},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("scoped anonymous relay clients were rejected: %v", err)
	}
	if got := report.RelaySessions[0].Source.IdentityStatus(); got != IdentityPartial {
		t.Fatalf("source identity status = %q, want partial", got)
	}
	if got := report.RelaySessions[0].Target.IdentityStatus(); got != IdentityAnonymous {
		t.Fatalf("target identity status = %q, want anonymous", got)
	}
}

func TestRelaySessionValidationRejectsInvalidScopeAndVNI(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	valid := RelaySessionObservation{
		Relay:     NodeIdentity{StableNodeID: "relay"},
		Source:    RelaySessionClient{SessionClientID: "left"},
		Target:    RelaySessionClient{SessionClientID: "right"},
		SessionID: "session", VNI: 7, SourceToTargetDelta: 1,
		SampleDurationMS: 2000, LastActive: at,
	}
	tests := map[string]RelaySessionObservation{
		"same client":      func() RelaySessionObservation { value := valid; value.Target.SessionClientID = "left"; return value }(),
		"missing client":   func() RelaySessionObservation { value := valid; value.Source.SessionClientID = ""; return value }(),
		"missing relay":    func() RelaySessionObservation { value := valid; value.Relay = NodeIdentity{}; return value }(),
		"negative VNI":     func() RelaySessionObservation { value := valid; value.VNI = -1; return value }(),
		"oversized VNI":    func() RelaySessionObservation { value := valid; value.VNI = 1 << 24; return value }(),
		"empty identity":   func() RelaySessionObservation { value := valid; value.Source.Identity = &NodeIdentity{}; return value }(),
		"zero delta":       func() RelaySessionObservation { value := valid; value.SourceToTargetDelta = 0; return value }(),
		"zero duration":    func() RelaySessionObservation { value := valid; value.SampleDurationMS = 0; return value }(),
		"missing session":  func() RelaySessionObservation { value := valid; value.SessionID = ""; return value }(),
		"negative counter": func() RelaySessionObservation { value := valid; value.SourceToTargetBytes = -1; return value }(),
	}
	for name, session := range tests {
		t.Run(name, func(t *testing.T) {
			if err := session.Validate(); err == nil {
				t.Fatal("invalid relay session was accepted")
			}
		})
	}
}
