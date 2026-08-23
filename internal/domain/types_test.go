package domain

import (
	"testing"
	"time"
)

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

func TestRelaySessionUpdateRequiresThirdPartyTrafficObservation(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: "relay", ReporterInstanceID: "reporter", Sequence: 1,
		CollectedAt: at, Kind: ReportRelaySessionUpdate,
		RelaySessions: []RelaySessionObservation{{
			Relay:     NodeIdentity{StableNodeID: "relay", Hostname: "Relay"},
			Source:    NodeIdentity{StableNodeID: "a", Hostname: "A"},
			Target:    NodeIdentity{StableNodeID: "b", Hostname: "B"},
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
		Observer: NodeIdentity{StableNodeID: "relay", Hostname: "Relay"}, InventoryGeneration: "unused",
	}}
	if err := report.Validate(); err == nil {
		t.Fatal("mixed relay session and observer peer report was accepted")
	}
}
