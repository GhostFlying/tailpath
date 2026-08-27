package exporter_test

import (
	"context"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/exporter"
)

type sourceFunc func(context.Context) (exporter.Snapshot, error)

func (function sourceFunc) Snapshot(ctx context.Context) (exporter.Snapshot, error) {
	return function(ctx)
}

type reporterStub struct{}

func (reporterStub) Capabilities(context.Context) (exporter.Capabilities, error) {
	return exporter.Capabilities{
		ObserverProtocolVersions: []int{exporter.ProtocolVersion},
		Features:                 []string{exporter.FeatureMultiObserver, exporter.FeatureObserverWithdrawal},
	}, nil
}

func (reporterStub) Send(context.Context, exporter.ReportEnvelope) (exporter.ReportReceipt, error) {
	return exporter.ReportReceipt{Accepted: true}, nil
}

func TestPublicContractsRequireNoInternalImports(t *testing.T) {
	var source exporter.Source = sourceFunc(func(context.Context) (exporter.Snapshot, error) {
		return exporter.Snapshot{
			CollectedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
			Observer:    exporter.NodeIdentity{StableNodeID: "runtime-a", Hostname: "Runtime A"},
			Peers: []exporter.PeerSnapshot{{
				Identity: exporter.NodeIdentity{StableNodeID: "peer-b"},
				TxBytes:  42, Path: exporter.Path{Kind: exporter.PathDirect},
			}},
		}, nil
	})
	var reporter exporter.Reporter = reporterStub{}

	snapshot, err := source.Snapshot(context.Background())
	if err != nil || snapshot.Observer.DisplayName() != "Runtime A" {
		t.Fatalf("snapshot = %#v, err=%v", snapshot, err)
	}
	capabilities, err := reporter.Capabilities(context.Background())
	if err != nil || !capabilities.SupportsFeature(exporter.FeatureObserverWithdrawal) {
		t.Fatalf("capabilities = %#v, err=%v", capabilities, err)
	}
}
