package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
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

func TestClockRollbackUsesHealthySampleDuration(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 0, 0, 0, 0),
		snapshot(start.Add(-time.Minute), 0, 0, 5, 0),
	}}
	reporter := &recordingReporter{}
	c := New(source, reporter, Options{ReporterInstance: "collector", SampleInterval: 2 * time.Second})
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reporter.reports[1].Observers[0].Peers[0].SampleDurationMS; got != 2000 {
		t.Fatalf("clock rollback sample duration = %dms, want healthy 2000ms fallback", got)
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

func TestAcceptedHelloSatisfiesResyncRequest(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 0, 0, 0, 0),
		snapshot(start.Add(2*time.Second), 0, 0, 5, 0),
	}}
	reporter := &scriptedReporter{receipts: []domain.ReportReceipt{
		{Accepted: true, ResyncRequired: true},
		{Accepted: true},
	}}
	c := New(source, reporter, Options{ReporterInstance: "collector"})
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reporter.reports[1].Kind; got != domain.ReportTrafficSample {
		t.Fatalf("report after accepted complete hello = %q, want traffic sample", got)
	}
}

func TestSampleResyncForcesFreshHello(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 0, 0, 0, 0),
		snapshot(start.Add(2*time.Second), 0, 0, 5, 0),
		snapshot(start.Add(4*time.Second), 0, 0, 9, 0),
	}}
	reporter := &scriptedReporter{receipts: []domain.ReportReceipt{
		{Accepted: true},
		{Accepted: true, ResyncRequired: true},
		{Accepted: true},
	}}
	c := New(source, reporter, Options{ReporterInstance: "collector"})
	for range 3 {
		if err := c.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := reporter.reports[2].Kind; got != domain.ReportObserverHello {
		t.Fatalf("report after resync request = %q, want fresh hello", got)
	}
	if got := reporter.reports[2].Observers[0].Peers[1].RxBytes; got != 9 {
		t.Fatalf("fresh hello counter = %d, want latest snapshot counter", got)
	}
}

func TestReconnectDoesNotReconstructOfflineTraffic(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &snapshotSource{snapshots: []Snapshot{
		snapshot(start, 0, 0, 0, 0),
		snapshot(start.Add(2*time.Second), 0, 0, 10, 0),
		snapshot(start.Add(20*time.Second), 0, 0, 100, 0),
		snapshot(start.Add(22*time.Second), 0, 0, 105, 0),
	}}
	reporter := &scriptedReporter{
		receipts: []domain.ReportReceipt{{Accepted: true}, {}, {Accepted: true}, {Accepted: true}},
		errors:   []error{nil, errors.New("server unavailable"), nil, nil},
	}
	c := New(source, reporter, Options{ReporterInstance: "collector"})
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(context.Background()); err == nil {
		t.Fatal("traffic report unexpectedly succeeded")
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := reporter.reports[len(reporter.reports)-1]
	if last.Kind != domain.ReportTrafficSample || last.Observers[0].Peers[0].RxDelta != 5 {
		t.Fatalf("post-recovery traffic = %#v, want only five bytes after hello baseline", last)
	}
}

func TestRetryDelayUsesExponentialCapAndJitter(t *testing.T) {
	c := New(nil, nil, Options{RetryMin: 2 * time.Second, RetryMax: 60 * time.Second, Jitter: func() float64 { return 0.5 }})
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 60 * time.Second, 60 * time.Second}
	for index, expected := range want {
		if got := c.retryDelay(index + 1); got != expected {
			t.Fatalf("retryDelay(%d) = %s, want %s", index+1, got, expected)
		}
	}
	c.jitter = func() float64 { return 0 }
	if got := c.retryDelay(1); got != 1600*time.Millisecond {
		t.Fatalf("minimum jitter delay = %s", got)
	}
	c.jitter = func() float64 { return 1 }
	if got := c.retryDelay(1); got != 2400*time.Millisecond {
		t.Fatalf("maximum jitter delay = %s", got)
	}
}

func TestRunBacksOffAndLogsOneRecovery(t *testing.T) {
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	source := &scriptedSource{
		results: []Snapshot{
			{},
			snapshot(start, 0, 0, 0, 0),
			snapshot(start.Add(2*time.Second), 0, 0, 0, 0),
		},
		errors: []error{errors.New("tailscaled unavailable"), nil, nil},
	}
	reporter := &scriptedReporter{
		receipts: []domain.ReportReceipt{{}, {Accepted: true}},
		errors:   []error{errors.New("request timeout"), nil},
	}
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	var logs bytes.Buffer
	c := New(source, reporter, Options{
		ReporterInstance: "collector",
		Now:              func() time.Time { return start.Add(time.Duration(len(waits)) * time.Second) },
		Jitter:           func() float64 { return 0.5 },
		Wait: func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			if len(waits) == 3 {
				cancel()
				return context.Canceled
			}
			return nil
		},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	wantWaits := []time.Duration{2 * time.Second, 4 * time.Second, 2 * time.Second}
	if len(waits) != len(wantWaits) {
		t.Fatalf("waits = %v, want %v", waits, wantWaits)
	}
	for index := range waits {
		if waits[index] != wantWaits[index] {
			t.Fatalf("waits = %v, want %v", waits, wantWaits)
		}
	}
	output := logs.String()
	if strings.Count(output, "collector degraded") != 1 || strings.Count(output, "collector recovered") != 1 {
		t.Fatalf("transition logs = %q", output)
	}
}

func TestWaitContextCancelsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitContext error = %v, want context cancellation", err)
	}
}

type snapshotSource struct {
	snapshots []Snapshot
}

type scriptedSource struct {
	results []Snapshot
	errors  []error
	calls   int
}

func (s *scriptedSource) Snapshot(context.Context) (Snapshot, error) {
	index := s.calls
	s.calls++
	return s.results[index], s.errors[index]
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

type scriptedReporter struct {
	receipts []domain.ReportReceipt
	errors   []error
	reports  []domain.ReportEnvelope
	calls    int
}

func (r *scriptedReporter) Send(_ context.Context, report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	r.reports = append(r.reports, report)
	index := r.calls
	r.calls++
	var err error
	if index < len(r.errors) {
		err = r.errors[index]
	}
	return r.receipts[index], err
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
