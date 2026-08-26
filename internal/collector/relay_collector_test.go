package collector

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestRelayCollectorReportsOnlyPostBaselineTraffic(t *testing.T) {
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	source := &relaySnapshotSource{
		normal: []Snapshot{
			snapshot(start, 0, 0, 0, 0),
			snapshot(start.Add(2*time.Second), 0, 0, 0, 0),
		},
		relay: []RelaySnapshot{
			relaySnapshot(start, relaySession(100, 50)),
			relaySnapshot(start.Add(2*time.Second), relaySession(140, 65)),
		},
	}
	reporter := &recordingReporter{}
	c := New(source, reporter, Options{ReporterInstance: "collector", RelayTelemetry: true})
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reporter.reports) != 1 || reporter.reports[0].Kind != domain.ReportObserverHello {
		t.Fatalf("first relay sample was not baseline-only: %#v", reporter.reports)
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reporter.reports) != 2 || reporter.reports[1].Kind != domain.ReportRelaySessionUpdate {
		t.Fatalf("relay delta report = %#v", reporter.reports)
	}
	report := reporter.reports[1]
	if err := report.Validate(); err != nil {
		t.Fatalf("relay report violates protocol: %v", err)
	}
	session := report.RelaySessions[0]
	if session.SourceToTargetDelta != 40 || session.TargetToSourceDelta != 15 || session.SampleDurationMS != 2000 {
		t.Fatalf("relay delta = %#v", session)
	}
	if session.Relay.StableNodeID != "observer" || session.Source.Endpoint == "" || session.Target.Endpoint == "" {
		t.Fatalf("relay report attributes = %#v", session)
	}
}

func TestRelayCollectorResetsBaselinesWithoutSyntheticTraffic(t *testing.T) {
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	source := &relaySnapshotSource{
		normal: repeatedSnapshots(start, 7),
		relay: []RelaySnapshot{
			relaySnapshot(start, relaySession(100, 50)),
			relaySnapshot(start.Add(2*time.Second), relaySession(10, 5)),
			relaySnapshot(start.Add(4*time.Second), relaySession(20, 8)),
			relaySnapshot(start.Add(6 * time.Second)),
			relaySnapshot(start.Add(8*time.Second), relaySession(30, 10)),
			relaySnapshot(start.Add(10*time.Second), relaySession(35, 12)),
			relaySnapshot(start.Add(12*time.Second), relaySession(35, 12)),
		},
	}
	reporter := &recordingReporter{}
	c := New(source, reporter, Options{ReporterInstance: "collector", RelayTelemetry: true})
	for range 7 {
		if err := c.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var relayReports []domain.ReportEnvelope
	for _, report := range reporter.reports {
		if report.Kind == domain.ReportRelaySessionUpdate {
			relayReports = append(relayReports, report)
		}
	}
	if len(relayReports) != 2 {
		t.Fatalf("relay reports = %#v, want post-reset and post-return deltas only", relayReports)
	}
	if got := relayReports[0].RelaySessions[0].SourceToTargetDelta; got != 10 {
		t.Fatalf("post-reset delta = %d, want 10", got)
	}
	if got := relayReports[1].RelaySessions[0].SourceToTargetDelta; got != 5 {
		t.Fatalf("post-return delta = %d, want 5", got)
	}
}

func TestRelayFailureDoesNotDegradeOrdinaryCollectionOrLeakDetails(t *testing.T) {
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	secretDetail := "192.0.2.44:41641 session_private d:0011223344556677"
	source := &relaySnapshotSource{
		normal: []Snapshot{
			snapshot(start, 0, 0, 0, 0),
			snapshot(start.Add(2*time.Second), 0, 0, 5, 0),
		},
		relay: []RelaySnapshot{
			relaySnapshot(start, relaySession(100, 50)),
			{CollectedAt: start.Add(2 * time.Second), Capability: RelayTransientFailure},
		},
		relayErrors: []error{nil, errors.New(secretDetail)},
	}
	var logs bytes.Buffer
	reporter := &recordingReporter{}
	c := New(source, reporter, Options{
		ReporterInstance: "collector", RelayTelemetry: true,
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	for range 2 {
		if err := c.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reporter.reports) != 2 || reporter.reports[1].Kind != domain.ReportTrafficSample {
		t.Fatalf("ordinary reports were degraded by relay failure: %#v", reporter.reports)
	}
	if strings.Contains(logs.String(), secretDetail) || strings.Contains(logs.String(), "192.0.2.44") ||
		strings.Contains(logs.String(), "session_private") || strings.Contains(logs.String(), "d:001122") {
		t.Fatalf("relay details leaked into logs: %s", logs.String())
	}
}

func TestRelayReportFailureForcesHelloAndDropsOfflineDelta(t *testing.T) {
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	source := &relaySnapshotSource{
		normal: repeatedSnapshots(start, 4),
		relay: []RelaySnapshot{
			relaySnapshot(start, relaySession(100, 50)),
			relaySnapshot(start.Add(2*time.Second), relaySession(120, 60)),
			relaySnapshot(start.Add(4*time.Second), relaySession(200, 100)),
			relaySnapshot(start.Add(6*time.Second), relaySession(207, 103)),
		},
	}
	reporter := &scriptedReporter{
		receipts: []domain.ReportReceipt{
			{Accepted: true}, {}, {Accepted: true}, {Accepted: true},
		},
		errors: []error{nil, errors.New("server unavailable"), nil, nil},
	}
	c := New(source, reporter, Options{ReporterInstance: "collector", RelayTelemetry: true})
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(context.Background()); err == nil {
		t.Fatal("relay report failure was not returned")
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reporter.reports[2].Kind; got != domain.ReportObserverHello {
		t.Fatalf("report after relay outage = %q, want hello", got)
	}
	if err := c.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := reporter.reports[len(reporter.reports)-1]
	if last.Kind != domain.ReportRelaySessionUpdate || last.RelaySessions[0].SourceToTargetDelta != 7 {
		t.Fatalf("post-reconnect relay report = %#v", last)
	}
}

type relaySnapshotSource struct {
	normal      []Snapshot
	relay       []RelaySnapshot
	relayErrors []error
	relayCalls  int
}

func (source *relaySnapshotSource) Snapshot(context.Context) (Snapshot, error) {
	result := source.normal[0]
	source.normal = source.normal[1:]
	return result, nil
}

func (source *relaySnapshotSource) PeerRelaySnapshot(context.Context) (RelaySnapshot, error) {
	index := source.relayCalls
	source.relayCalls++
	var err error
	if index < len(source.relayErrors) {
		err = source.relayErrors[index]
	}
	return source.relay[index], err
}

func relaySnapshot(at time.Time, sessions ...RelaySessionSnapshot) RelaySnapshot {
	return RelaySnapshot{CollectedAt: at, Capability: RelayEnabled, Sessions: sessions}
}

func relaySession(sourceBytes, targetBytes uint64) RelaySessionSnapshot {
	return RelaySessionSnapshot{
		SessionID: "session", VNI: 7,
		Source: RelayClientSnapshot{
			SessionClientID: "left", DiscoShort: "d:0011223344556677",
			Endpoint: "192.0.2.10:41641", BytesSent: sourceBytes,
		},
		Target: RelayClientSnapshot{
			SessionClientID: "right", DiscoShort: "d:8899aabbccddeeff",
			Endpoint: "[2001:db8::10]:41641", BytesSent: targetBytes,
		},
	}
}

func repeatedSnapshots(start time.Time, count int) []Snapshot {
	result := make([]Snapshot, count)
	for index := range count {
		result[index] = snapshot(start.Add(time.Duration(index)*2*time.Second), 0, 0, 0, 0)
	}
	return result
}
