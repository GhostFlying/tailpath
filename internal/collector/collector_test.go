package collector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/api/generated"
	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestControlTrafficDoesNotTriggerTrafficReport(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 100, 100, 20, 20),
		snapshot(start.Add(2*time.Second), 150, 140, 20, 20),
		snapshot(start.Add(4*time.Second), 180, 160, 25, 22),
	}}
	reporter := &recordingReporter{controlIDs: []string{"server"}}
	collector := New(source, reporter, Options{ReporterInstance: "collector", SampleInterval: 2 * time.Second})

	for range 3 {
		if err := collector.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reporter.reports) != 2 {
		t.Fatalf("got %d reports, want hello plus one traffic sample", len(reporter.reports))
	}
	if err := reporter.reports[0].Validate(); err != nil {
		t.Fatalf("collector hello violates the domain contract: %v", err)
	}
	for _, peer := range reporter.reports[0].Observers[0].Peers {
		if peer.SampleDurationMS != 0 {
			t.Fatalf("hello baseline sample duration = %d, want 0", peer.SampleDurationMS)
		}
	}
	traffic := reporter.reports[1]
	if traffic.Kind != domain.ReportTrafficSample || len(traffic.Observers[0].Peers) != 1 {
		t.Fatalf("unexpected traffic report: %#v", traffic)
	}
	if got := traffic.Observers[0].Peers[0].Peer.StableNodeID; got != "peer" {
		t.Fatalf("reported peer %q, want non-control peer", got)
	}
}

func TestCounterResetDoesNotCreateTraffic(t *testing.T) {
	if got := counterDelta(100, 20); got != 0 {
		t.Fatalf("counterDelta = %d, want 0", got)
	}
}

func TestCollectorAdoptsServerHeartbeatAndTrafficResetsIdleTimer(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 0, 0, 0, 0),
		snapshot(start.Add(20*time.Second), 0, 0, 10, 0),
		snapshot(start.Add(40*time.Second), 0, 0, 10, 0),
		snapshot(start.Add(51*time.Second), 0, 0, 10, 0),
	}}
	reporter := &recordingReporter{heartbeatIntervalMS: (30 * time.Second).Milliseconds()}
	collector := New(source, reporter, Options{ReporterInstance: "collector"})

	for range 4 {
		if err := collector.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reporter.reports) != 3 {
		t.Fatalf("got %d reports, want hello, traffic, and idle heartbeat", len(reporter.reports))
	}
	if reporter.reports[2].Kind != domain.ReportObserverHeartbeat {
		t.Fatalf("last report = %q, want heartbeat", reporter.reports[2].Kind)
	}
}

func TestCollectorHelloMatchesGeneratedAPIShape(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{snapshot(start, 100, 100, 20, 20)}}
	reporter := &recordingReporter{}
	collector := New(source, reporter, Options{ReporterInstance: "00000000-0000-4000-8000-000000000001"})
	if err := collector.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(reporter.reports[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire generated.ReportEnvelope
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("collector hello does not decode as generated API type: %v", err)
	}
	if wire.Observers == nil || len(*wire.Observers) != 1 || (*wire.Observers)[0].Peers == nil {
		t.Fatalf("generated hello observers = %#v", wire.Observers)
	}
	for _, peer := range *(*wire.Observers)[0].Peers {
		if peer.SampleDurationMs != 0 {
			t.Fatalf("generated hello baseline duration = %d, want 0", peer.SampleDurationMs)
		}
	}
}

type snapshotSource struct {
	snapshots []Snapshot
}

func (s *snapshotSource) Snapshot(context.Context) (Snapshot, error) {
	result := s.snapshots[0]
	s.snapshots = s.snapshots[1:]
	return result, nil
}

type recordingReporter struct {
	controlIDs          []string
	heartbeatIntervalMS int64
	reports             []domain.ReportEnvelope
}

func (r *recordingReporter) Send(_ context.Context, report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	r.reports = append(r.reports, report)
	return domain.ReportReceipt{
		Accepted: true, ControlStableNodeIDs: r.controlIDs, HeartbeatIntervalMS: r.heartbeatIntervalMS,
	}, nil
}

func snapshot(at time.Time, controlRx, controlTx, peerRx, peerTx int64) Snapshot {
	return Snapshot{
		CollectedAt: at,
		Observer:    domain.NodeIdentity{StableNodeID: "observer", Hostname: "observer"},
		Peers: []PeerSnapshot{
			{Identity: domain.NodeIdentity{StableNodeID: "server", Hostname: "tailpath"}, RxBytes: controlRx, TxBytes: controlTx, Path: domain.PathObservation{Kind: domain.PathDirect}},
			{Identity: domain.NodeIdentity{StableNodeID: "peer", Hostname: "peer"}, RxBytes: peerRx, TxBytes: peerTx, Path: domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"}},
		},
	}
}
