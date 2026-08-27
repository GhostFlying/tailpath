package exporter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type relaySourceResult struct {
	snapshot RelaySnapshot
	err      error
}

type channelRelaySource struct {
	*channelSource
	relayResults chan relaySourceResult
}

func newChannelRelaySource() *channelRelaySource {
	return &channelRelaySource{
		channelSource: newChannelSource(),
		relayResults:  make(chan relaySourceResult, 16),
	}
}

func (s *channelRelaySource) PeerRelaySnapshot(ctx context.Context) (RelaySnapshot, error) {
	select {
	case <-ctx.Done():
		return RelaySnapshot{}, ctx.Err()
	case result := <-s.relayResults:
		return result.snapshot, result.err
	}
}

func (s *channelRelaySource) pushRelay(snapshot RelaySnapshot) {
	s.relayResults <- relaySourceResult{snapshot: snapshot}
}

func (s *channelRelaySource) failRelay(err error) {
	s.relayResults <- relaySourceResult{err: err}
}

func relayRuntimeSnapshot(at time.Time, sourceBytes, targetBytes uint64) RelaySnapshot {
	return RelaySnapshot{
		CollectedAt: at, Capability: RelayEnabled, IdentityEvidence: RelayIdentityAvailable,
		Sessions: []RelaySessionSnapshot{{
			SessionID: "session", VNI: 7,
			Source: RelayClientSnapshot{
				SessionClientID: "left", DiscoShort: "d:0011223344556677",
				Endpoint: "192.0.2.10:41641", BytesSent: sourceBytes,
			},
			Target: RelayClientSnapshot{
				SessionClientID: "right", DiscoShort: "d:8899aabbccddeeff",
				Endpoint: "[2001:db8::10]:41641", BytesSent: targetBytes,
			},
		}},
	}
}

func TestSnapshotSinkReportsRelayDeltaOnOrdinarySequence(t *testing.T) {
	reporter := newRecordingSinkReporter()
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelRelaySource()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	source.push(runtimeSnapshot(at, "relay", 0, 0))
	source.pushRelay(relayRuntimeSnapshot(at, 100, 50))
	if _, err := sink.Register("relay", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	hello := waitReport(t, reporter)
	source.pushRelay(relayRuntimeSnapshot(at.Add(2*time.Second), 140, 65))
	relay := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if hello.Kind != ReportObserverHello || hello.Sequence != 1 {
		t.Fatalf("ordinary hello = %#v", hello)
	}
	if relay.Kind != ReportRelaySessionUpdate || relay.Sequence != 2 || len(relay.RelaySessions) != 1 {
		t.Fatalf("relay report = %#v", relay)
	}
	session := relay.RelaySessions[0]
	if session.Relay.StableNodeID != "relay" || session.SourceToTargetDelta != 40 ||
		session.TargetToSourceDelta != 15 || session.SampleDurationMS != 2000 {
		t.Fatalf("relay delta = %#v", session)
	}
}

func TestSnapshotSinkIsolatesRelaySourceFailureAndLogDetails(t *testing.T) {
	reporter := newRecordingSinkReporter()
	var logs bytes.Buffer
	options := sinkOptions()
	options.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	sink := newSnapshotSink(reporter, options)
	source := newChannelRelaySource()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	source.push(runtimeSnapshot(at, "relay", 0, 0))
	secret := "192.0.2.44:41641 session_private d:0011223344556677"
	source.failRelay(errors.New(secret))
	if _, err := sink.Register("relay", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.push(runtimeSnapshot(at.Add(2*time.Second), "relay", 5, 0))
	traffic := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if traffic.Kind != ReportTrafficSample || traffic.Observers[0].Peers[0].RxDelta != 5 {
		t.Fatalf("ordinary traffic after relay failure = %#v", traffic)
	}
	for _, private := range []string{secret, "192.0.2.44", "session_private", "d:001122"} {
		if strings.Contains(logs.String(), private) {
			t.Fatalf("relay details leaked into logs: %s", logs.String())
		}
	}
}

func TestSnapshotSinkRelayTransportFailureForcesHelloWithoutCatchup(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.errors = []error{nil, errors.New("server unavailable"), nil, nil}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelRelaySource()
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	source.push(runtimeSnapshot(at, "relay", 0, 0))
	source.pushRelay(relayRuntimeSnapshot(at, 100, 50))
	if _, err := sink.Register("relay", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.pushRelay(relayRuntimeSnapshot(at.Add(2*time.Second), 120, 60))
	failed := waitReport(t, reporter)
	if failed.Kind != ReportRelaySessionUpdate {
		t.Fatalf("failed report = %q", failed.Kind)
	}
	source.pushRelay(relayRuntimeSnapshot(at.Add(20*time.Second), 200, 100))
	source.push(runtimeSnapshot(at.Add(20*time.Second), "relay", 0, 0))
	rehello := waitReport(t, reporter)
	if rehello.Kind != ReportObserverHello {
		t.Fatalf("report after relay outage = %#v, want hello", rehello)
	}
	source.pushRelay(relayRuntimeSnapshot(at.Add(22*time.Second), 207, 103))
	relay := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if relay.Kind != ReportRelaySessionUpdate || relay.RelaySessions[0].SourceToTargetDelta != 7 ||
		relay.RelaySessions[0].TargetToSourceDelta != 3 {
		t.Fatalf("post-recovery relay report = %#v", relay)
	}
	if relay.Sequence != rehello.Sequence+1 {
		t.Fatalf("shared sequence skipped: hello=%d relay=%d", rehello.Sequence, relay.Sequence)
	}
}

func TestSnapshotSinkRelayResyncForcesFreshOrdinaryHello(t *testing.T) {
	reporter := newRecordingSinkReporter()
	reporter.receipts = []ReportReceipt{
		{Accepted: true, HeartbeatIntervalMS: 60000},
		{Accepted: true, ResyncRequired: true, HeartbeatIntervalMS: 60000},
		{Accepted: true, HeartbeatIntervalMS: 60000},
	}
	sink := newSnapshotSink(reporter, sinkOptions())
	source := newChannelRelaySource()
	at := time.Now().UTC()
	source.push(runtimeSnapshot(at, "relay", 0, 0))
	source.pushRelay(relayRuntimeSnapshot(at, 10, 5))
	if _, err := sink.Register("relay", source); err != nil {
		t.Fatal(err)
	}
	cancel, done := startSink(t, sink)
	_ = waitReport(t, reporter)
	source.pushRelay(relayRuntimeSnapshot(at.Add(2*time.Second), 15, 8))
	if report := waitReport(t, reporter); report.Kind != ReportRelaySessionUpdate {
		t.Fatalf("resync report = %q", report.Kind)
	}
	rehello := waitReport(t, reporter)
	stopSink(t, cancel, done)
	if rehello.Kind != ReportObserverHello {
		t.Fatalf("relay resync did not force hello: %#v", rehello)
	}
}
