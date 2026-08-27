package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedLogBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type sourceResult struct {
	snapshot Snapshot
	err      error
}

type channelSource struct {
	results chan sourceResult
}

func newChannelSource() *channelSource {
	return &channelSource{results: make(chan sourceResult, 16)}
}

func (s *channelSource) Snapshot(ctx context.Context) (Snapshot, error) {
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case result := <-s.results:
		return result.snapshot, result.err
	}
}

func (s *channelSource) push(snapshot Snapshot) {
	s.results <- sourceResult{snapshot: snapshot}
}

func (s *channelSource) fail(err error) {
	s.results <- sourceResult{err: err}
}

type recordingSinkReporter struct {
	mu sync.Mutex

	capabilities    Capabilities
	capabilityErr   error
	capabilityCalls int
	reports         []ReportEnvelope
	reportEvents    chan ReportEnvelope
	receipts        []ReportReceipt
	errors          []error
	defaultReceipt  ReportReceipt
	sendCalls       int
	maxObservers    int
	maxPayloadBytes int
}

func newRecordingSinkReporter() *recordingSinkReporter {
	return &recordingSinkReporter{
		capabilities: Capabilities{
			ObserverProtocolVersions: []int{ProtocolVersion},
			Features:                 []string{FeatureMultiObserver, FeatureObserverWithdrawal},
		},
		reportEvents:   make(chan ReportEnvelope, 256),
		defaultReceipt: ReportReceipt{Accepted: true, HeartbeatIntervalMS: 60000},
	}
}

func (r *recordingSinkReporter) Capabilities(context.Context) (Capabilities, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilityCalls++
	return r.capabilities, r.capabilityErr
}

func (r *recordingSinkReporter) Send(_ context.Context, report ReportEnvelope) (ReportReceipt, error) {
	payload, _ := json.Marshal(report)
	r.mu.Lock()
	index := r.sendCalls
	r.sendCalls++
	r.reports = append(r.reports, report)
	if len(report.Observers) > r.maxObservers {
		r.maxObservers = len(report.Observers)
	}
	if len(payload) > r.maxPayloadBytes {
		r.maxPayloadBytes = len(payload)
	}
	receipt := r.defaultReceipt
	if index < len(r.receipts) {
		receipt = r.receipts[index]
	}
	var err error
	if index < len(r.errors) {
		err = r.errors[index]
	}
	r.mu.Unlock()
	select {
	case r.reportEvents <- report:
	default:
	}
	return receipt, err
}

func (r *recordingSinkReporter) snapshotReports() []ReportEnvelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ReportEnvelope(nil), r.reports...)
}

func sinkOptions() snapshotSinkConfig {
	return snapshotSinkConfig{
		SampleInterval:     5 * time.Millisecond,
		BatchWindow:        10 * time.Millisecond,
		RetryMin:           5 * time.Millisecond,
		RetryMax:           20 * time.Millisecond,
		ReporterInstanceID: "00000000-0000-4000-8000-000000000001",
		Jitter:             func() float64 { return 0.5 },
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func runtimeSnapshot(at time.Time, observer string, peerRx, peerTx int64) Snapshot {
	return Snapshot{
		CollectedAt: at,
		Observer:    NodeIdentity{StableNodeID: observer, Hostname: observer},
		Peers: []PeerSnapshot{{
			Identity: NodeIdentity{StableNodeID: "peer-" + observer, Hostname: "peer-" + observer},
			RxBytes:  peerRx, TxBytes: peerTx, Path: Path{Kind: PathDirect},
		}},
	}
}

func waitReport(t *testing.T, reporter *recordingSinkReporter) ReportEnvelope {
	t.Helper()
	select {
	case report := <-reporter.reportEvents:
		return report
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for exporter report")
		return ReportEnvelope{}
	}
}

func startSink(t *testing.T, sink *SnapshotSink) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sink.Run(ctx) }()
	return cancel, done
}

func stopSink(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot sink did not stop")
	}
}

func TestSnapshotSinkRedactsTransportErrorDetails(t *testing.T) {
	const canary = "private-tailnet-host.example:8080"
	reporter := newRecordingSinkReporter()
	reporter.errors = []error{errors.New(canary)}
	var logs lockedLogBuffer
	options := sinkOptions()
	options.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	sink := newSnapshotSink(reporter, options)
	source := newChannelSource()
	source.push(runtimeSnapshot(time.Now(), "runtime-a", 0, 0))
	if _, err := sink.Register("source-a", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(logs.String(), "exporter transport degraded") {
		time.Sleep(10 * time.Millisecond)
	}
	stopSink(t, cancel, done)
	if !strings.Contains(logs.String(), "error_kind=transport") {
		t.Fatalf("transport log missing safe classification: %s", logs.String())
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("transport log leaked error detail: %s", logs.String())
	}
}

func TestBoundedReportErrorPreservesClassificationWithoutDetails(t *testing.T) {
	const canary = "private response or endpoint"
	bounded := boundedReportError(&HTTPStatusError{StatusCode: 403, Status: "403 Forbidden", Message: canary})
	var statusError *HTTPStatusError
	if !errors.As(bounded, &statusError) || statusError.StatusCode != 403 {
		t.Fatalf("bounded error = %#v", bounded)
	}
	if strings.Contains(bounded.Error(), canary) {
		t.Fatalf("bounded status error leaked detail: %v", bounded)
	}
	if transport := boundedReportError(errors.New(canary)); strings.Contains(transport.Error(), canary) {
		t.Fatalf("bounded transport error leaked detail: %v", transport)
	}
}

func TestSnapshotSinkBatchesIndependentObserversOnOneSequence(t *testing.T) {
	reporter := newRecordingSinkReporter()
	sink := newSnapshotSink(reporter, sinkOptions())
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for index := range 3 {
		source := newChannelSource()
		source.push(runtimeSnapshot(at, fmt.Sprintf("runtime-%d", index), 0, 0))
		if _, err := sink.Register(fmt.Sprintf("source-%d", index), source); err != nil {
			t.Fatal(err)
		}
	}
	cancel, done := startSink(t, sink)
	report := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if report.Kind != ReportObserverHello || report.Sequence != 1 || len(report.Observers) != 3 {
		t.Fatalf("batched hello = %#v", report)
	}
	if report.ReporterInstanceID != sinkOptions().ReporterInstanceID {
		t.Fatalf("reporter instance = %q", report.ReporterInstanceID)
	}
	reporter.mu.Lock()
	capabilityCalls := reporter.capabilityCalls
	reporter.mu.Unlock()
	if capabilityCalls != 1 {
		t.Fatalf("capability calls = %d, want 1", capabilityCalls)
	}
}

func TestSnapshotSinkReportsSparseBusinessTraffic(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.defaultReceipt.ControlStableNodeIDs = []string{"server"}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	first := runtimeSnapshot(at, "runtime", 0, 0)
	first.Peers = append(first.Peers, PeerSnapshot{
		Identity: NodeIdentity{StableNodeID: "server"}, RxBytes: 100, TxBytes: 100, Path: Path{Kind: PathDirect},
	})
	source.push(first)
	if _, err := sink.Register("runtime", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	if report := waitReport(t, reporter); report.Kind != ReportObserverHello {
		t.Fatalf("first report = %q", report.Kind)
	}
	second := cloneSnapshot(first)
	second.CollectedAt = at.Add(2 * time.Second)
	second.Peers[0].RxBytes = 20
	second.Peers[0].TxBytes = 10
	second.Peers[1].RxBytes = 1000
	second.Peers[1].TxBytes = 1000
	source.push(second)
	traffic := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if traffic.Kind != ReportTrafficSample || len(traffic.Observers) != 1 || len(traffic.Observers[0].Peers) != 1 {
		t.Fatalf("traffic report = %#v", traffic)
	}
	peer := traffic.Observers[0].Peers[0]
	if peer.Peer.StableNodeID != "peer-runtime" || peer.RxDelta != 20 || peer.TxDelta != 10 {
		t.Fatalf("business delta = %#v", peer)
	}
}

func TestSnapshotSinkReconnectUsesLatestHelloWithoutCatchup(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.receipts = []ReportReceipt{
		{Accepted: true, HeartbeatIntervalMS: 60000},
		{},
		{Accepted: true, HeartbeatIntervalMS: 60000},
		{Accepted: true, HeartbeatIntervalMS: 60000},
	}
	reporter.errors = []error{nil, errors.New("server unavailable"), nil, nil}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	at := time.Now().UTC()
	source.push(runtimeSnapshot(at, "runtime", 0, 0))
	if _, err := sink.Register("runtime", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.push(runtimeSnapshot(at.Add(2*time.Second), "runtime", 10, 0))
	failed := waitReport(t, reporter)
	if failed.Kind != ReportTrafficSample {
		t.Fatalf("failed report = %q", failed.Kind)
	}
	source.push(runtimeSnapshot(at.Add(20*time.Second), "runtime", 100, 0))
	rehello := waitReport(t, reporter)
	if rehello.Kind != ReportObserverHello || rehello.Observers[0].Peers[0].RxBytes != 100 {
		t.Fatalf("reconnect hello = %#v", rehello)
	}
	source.push(runtimeSnapshot(at.Add(22*time.Second), "runtime", 105, 0))
	traffic := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if traffic.Kind != ReportTrafficSample || traffic.Observers[0].Peers[0].RxDelta != 5 {
		t.Fatalf("post-reconnect delta = %#v", traffic)
	}
}

func TestSnapshotSinkIsolatesFailedAndOversizedSources(t *testing.T) {
	reporter := newRecordingSinkReporter()
	options := sinkOptions()
	options.MaxRequestBytes = 900
	sink := newSnapshotSink(reporter, options)
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	failed := newChannelSource()
	failed.fail(errors.New("runtime unavailable"))
	oversized := newChannelSource()
	huge := runtimeSnapshot(at, "oversized", 0, 0)
	huge.Observer.Hostname = strings.Repeat("x", 2000)
	oversized.push(huge)
	healthy := newChannelSource()
	healthy.push(runtimeSnapshot(at, "healthy", 0, 0))
	for key, source := range map[string]*channelSource{"failed": failed, "oversized": oversized, "healthy": healthy} {
		if _, err := sink.Register(key, source); err != nil {
			t.Fatal(err)
		}
	}
	cancel, done := startSink(t, sink)
	report := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if report.Kind != ReportObserverHello || len(report.Observers) != 1 || report.Observers[0].Observer.StableNodeID != "healthy" {
		t.Fatalf("healthy sibling report = %#v", report)
	}
}

func TestSnapshotSinkWithdrawalAndIdentityReplacementAreOrdered(t *testing.T) {
	reporter := newRecordingSinkReporter()
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	source.push(runtimeSnapshot(at, "runtime-a", 0, 0))
	registration, err := sink.Register("runtime", source)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.push(runtimeSnapshot(at.Add(2*time.Second), "runtime-b", 0, 0))
	withdrawOld := waitReport(t, reporter)
	helloNew := waitReport(t, reporter)
	if withdrawOld.Kind != ReportObserverWithdrawal || withdrawOld.Observers[0].Observer.StableNodeID != "runtime-a" ||
		helloNew.Kind != ReportObserverHello || helloNew.Observers[0].Observer.StableNodeID != "runtime-b" ||
		helloNew.Sequence != withdrawOld.Sequence+1 {
		t.Fatalf("identity replacement reports = %#v then %#v", withdrawOld, helloNew)
	}
	withdrawContext, withdrawCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer withdrawCancel()
	if err := registration.Withdraw(withdrawContext); err != nil {
		t.Fatal(err)
	}
	withdrawNew := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if withdrawNew.Kind != ReportObserverWithdrawal || withdrawNew.Observers[0].Observer.StableNodeID != "runtime-b" {
		t.Fatalf("explicit withdrawal = %#v", withdrawNew)
	}
}

func TestSnapshotSinkRetriesWithdrawalAfterTransportFailure(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.errors = []error{nil, errors.New("server unavailable"), nil}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	source.push(runtimeSnapshot(time.Now().UTC(), "runtime", 0, 0))
	registration, err := sink.Register("runtime", source)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	withdrawDone := make(chan error, 1)
	go func() { withdrawDone <- registration.Withdraw(context.Background()) }()
	first := waitReport(t, reporter)
	second := waitReport(t, reporter)
	select {
	case err := <-withdrawDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("withdrawal did not recover")
	}
	stopSink(t, cancel, done)
	if first.Kind != ReportObserverWithdrawal || second.Kind != ReportObserverWithdrawal || second.Sequence != first.Sequence+1 {
		t.Fatalf("withdrawal retries = %#v then %#v", first, second)
	}
}

func TestSnapshotSinkSplitsRejectedBatchToProtectSibling(t *testing.T) {
	reporter := newRecordingSinkReporter()
	badRequest := &HTTPStatusError{StatusCode: 400, Status: "400 Bad Request", Message: "invalid observer"}
	reporter.errors = []error{badRequest, badRequest, nil}
	sink := newSnapshotSink(reporter, sinkOptions())
	at := time.Now().UTC()
	for _, key := range []string{"bad", "good"} {
		source := newChannelSource()
		source.push(runtimeSnapshot(at, key, 0, 0))
		if _, err := sink.Register(key, source); err != nil {
			t.Fatal(err)
		}
	}
	cancel, done := startSink(t, sink)
	for range 3 {
		_ = waitReport(t, reporter)
	}
	stopSink(t, cancel, done)
	reports := reporter.snapshotReports()
	last := reports[len(reports)-1]
	if len(last.Observers) != 1 || last.Observers[0].Observer.StableNodeID != "good" {
		t.Fatalf("healthy sibling was not isolated: %#v", reports)
	}
}

func TestSnapshotSinkResyncSendsFreshHello(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.receipts = []ReportReceipt{
		{Accepted: true, HeartbeatIntervalMS: 60000},
		{Accepted: true, ResyncRequired: true, HeartbeatIntervalMS: 60000},
		{Accepted: true, HeartbeatIntervalMS: 60000},
	}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	at := time.Now().UTC()
	source.push(runtimeSnapshot(at, "runtime", 0, 0))
	if _, err := sink.Register("runtime", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.push(runtimeSnapshot(at.Add(2*time.Second), "runtime", 10, 0))
	if report := waitReport(t, reporter); report.Kind != ReportTrafficSample {
		t.Fatalf("resync report = %q", report.Kind)
	}
	rehello := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if rehello.Kind != ReportObserverHello || rehello.Observers[0].Peers[0].RxBytes != 10 {
		t.Fatalf("resync hello = %#v", rehello)
	}
}

func TestSnapshotSinkSourceRecoveryUsesFreshBaseline(t *testing.T) {
	reporter := newRecordingSinkReporter()
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelSource()
	at := time.Now().UTC()
	source.push(runtimeSnapshot(at, "runtime", 0, 0))
	if _, err := sink.Register("runtime", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.fail(errors.New("runtime unavailable"))
	source.push(runtimeSnapshot(at.Add(20*time.Second), "runtime", 100, 0))
	rehello := waitReport(t, reporter)
	if rehello.Kind != ReportObserverHello || rehello.Observers[0].Peers[0].RxBytes != 100 {
		t.Fatalf("source recovery hello = %#v", rehello)
	}
	source.push(runtimeSnapshot(at.Add(22*time.Second), "runtime", 105, 0))
	traffic := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if traffic.Kind != ReportTrafficSample || traffic.Observers[0].Peers[0].RxDelta != 5 {
		t.Fatalf("source recovery delta = %#v", traffic)
	}
}

func TestSnapshotSinkDoesNotHeartbeatHungSource(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.defaultReceipt.HeartbeatIntervalMS = 0
	options := sinkOptions()
	options.HeartbeatInterval = 20 * time.Millisecond
	options.SnapshotTimeout = 15 * time.Millisecond
	sink := newSnapshotSink(reporter, options)
	source := newChannelSource()
	source.push(runtimeSnapshot(time.Now().UTC(), "runtime", 0, 0))
	if _, err := sink.Register("runtime", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	select {
	case report := <-reporter.reportEvents:
		t.Fatalf("hung source emitted %q", report.Kind)
	case <-time.After(80 * time.Millisecond):
	}
	stopSink(t, cancel, done)
}

func TestSnapshotSinkSupportsDynamicRegistration(t *testing.T) {
	reporter := newRecordingSinkReporter()
	sink := newSnapshotSink(reporter, sinkOptions())
	first := newChannelSource()
	first.push(runtimeSnapshot(time.Now().UTC(), "runtime-a", 0, 0))
	if _, err := sink.Register("a", first); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	second := newChannelSource()
	second.push(runtimeSnapshot(time.Now().UTC(), "runtime-b", 0, 0))
	if _, err := sink.Register("b", second); err != nil {
		t.Fatal(err)
	}
	report := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if report.Kind != ReportObserverHello || len(report.Observers) != 1 || report.Observers[0].Observer.StableNodeID != "runtime-b" {
		t.Fatalf("dynamic registration report = %#v", report)
	}
}

func TestSnapshotSinkEnforcesBatchBounds(t *testing.T) {
	reporter := newRecordingSinkReporter()
	options := sinkOptions()
	options.MaxBatchObservers = 8
	options.MaxRequestBytes = 4096
	sink := newSnapshotSink(reporter, options)
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for index := range 20 {
		source := newChannelSource()
		source.push(runtimeSnapshot(at, fmt.Sprintf("runtime-%02d", index), 0, 0))
		if _, err := sink.Register(fmt.Sprintf("source-%02d", index), source); err != nil {
			t.Fatal(err)
		}
	}
	cancel, done := startSink(t, sink)
	seen := 0
	for seen < 20 {
		report := waitReport(t, reporter)
		if report.Kind != ReportObserverHello {
			t.Fatalf("bounded report kind = %q", report.Kind)
		}
		seen += len(report.Observers)
	}
	stopSink(t, cancel, done)
	reporter.mu.Lock()
	maxObservers, maxBytes := reporter.maxObservers, reporter.maxPayloadBytes
	reporter.mu.Unlock()
	if maxObservers > 8 || maxBytes > 4096 {
		t.Fatalf("batch bounds observers=%d bytes=%d", maxObservers, maxBytes)
	}
	reports := reporter.snapshotReports()
	for index, report := range reports {
		if report.Sequence != int64(index+1) {
			t.Fatalf("report %d sequence = %d", index, report.Sequence)
		}
	}
}

func TestSnapshotSinkRejectsIncompatibleServer(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.capabilities.Features = []string{FeatureMultiObserver}
	options := sinkOptions()
	options.BatchWindow = time.Millisecond
	sink := newSnapshotSink(reporter, options)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := sink.Run(ctx)
	var incompatible *IncompatibleServerError
	if !errors.As(err, &incompatible) {
		t.Fatalf("Run error = %T %v, want IncompatibleServerError", err, err)
	}
}

func TestRegistrationCanWithdrawBeforeRun(t *testing.T) {
	sink := NewSnapshotSink(newRecordingSinkReporter(), SnapshotSinkOptions{
		ReporterInstanceID: "00000000-0000-4000-8000-000000000001",
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	registration, err := sink.Register("runtime", newChannelSource())
	if err != nil {
		t.Fatal(err)
	}
	if err := registration.Withdraw(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Register("runtime", newChannelSource()); err != nil {
		t.Fatalf("registration key was not released: %v", err)
	}
}

func TestSnapshotSinkRejectsDuplicateRegistrationKey(t *testing.T) {
	sink := NewSnapshotSink(newRecordingSinkReporter(), SnapshotSinkOptions{})
	if _, err := sink.Register("runtime", newChannelSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Register("runtime", newChannelSource()); !errors.Is(err, ErrDuplicateRegistration) {
		t.Fatalf("duplicate registration error = %v", err)
	}
}
