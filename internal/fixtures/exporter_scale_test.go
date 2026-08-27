package fixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

const scaleExporterReporterID = "00000000-0000-4000-8000-000000700001"

type mutableScaleSource struct {
	mu       sync.RWMutex
	snapshot exporter.Snapshot
}

func (s *mutableScaleSource) Snapshot(context.Context) (exporter.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneExporterSnapshot(s.snapshot), nil
}

func (s *mutableScaleSource) set(snapshot exporter.Snapshot) {
	s.mu.Lock()
	s.snapshot = cloneExporterSnapshot(snapshot)
	s.mu.Unlock()
}

type appScaleReporter struct {
	mu          sync.Mutex
	application *app.App
	reports     []exporter.ReportEnvelope
}

func (r *appScaleReporter) Capabilities(context.Context) (exporter.Capabilities, error) {
	return exporter.Capabilities{
		ObserverProtocolVersions: []int{exporter.ProtocolVersion},
		Features: []string{
			exporter.FeatureMultiObserver,
			exporter.FeatureObserverWithdrawal,
		},
	}, nil
}

func (r *appScaleReporter) Send(ctx context.Context, report exporter.ReportEnvelope) (exporter.ReportReceipt, error) {
	payload, err := json.Marshal(report)
	if err != nil {
		return exporter.ReportReceipt{}, err
	}
	var internal domain.ReportEnvelope
	if err := json.Unmarshal(payload, &internal); err != nil {
		return exporter.ReportReceipt{}, err
	}
	r.mu.Lock()
	r.reports = append(r.reports, report)
	application := r.application
	r.mu.Unlock()
	receipt, err := application.Submit(ctx, internal)
	if err != nil {
		return exporter.ReportReceipt{}, err
	}
	return exporter.ReportReceipt{
		Accepted:             receipt.Accepted,
		ResyncRequired:       receipt.ResyncRequired,
		ControlStableNodeIDs: append([]string(nil), receipt.ControlStableNodeIDs...),
		HeartbeatIntervalMS:  receipt.HeartbeatIntervalMS,
	}, nil
}

func (r *appScaleReporter) snapshotReports() []exporter.ReportEnvelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]exporter.ReportEnvelope(nil), r.reports...)
}

func (r *appScaleReporter) trafficObservers() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, report := range r.reports {
		if report.Kind == exporter.ReportTrafficSample {
			count += len(report.Observers)
		}
	}
	return count
}

func TestExporterScalePersistsWithdrawalAndReconnectAcrossRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-runtime exporter integration is a scale gate")
	}
	scenario, err := NewScaleScenario(DefaultScaleConfig())
	if err != nil {
		t.Fatal(err)
	}
	databasePath := fmt.Sprintf("%s/exporter-scale.db", t.TempDir())
	database, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	application, err := app.New(database, aggregate.Options{HeartbeatInterval: 10 * time.Minute}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reporter := &appScaleReporter{application: application}
	sink := exporter.NewSnapshotSink(reporter, exporter.SnapshotSinkOptions{ReporterInstanceID: scaleExporterReporterID})

	baselineAt := time.Now().UTC()
	baseline := scenario.ExporterSnapshots(baselineAt, 1)
	sources := make([]*mutableScaleSource, len(baseline))
	registrations := make([]*exporter.Registration, len(baseline))
	for index, snapshot := range baseline {
		sources[index] = &mutableScaleSource{snapshot: snapshot}
		registrations[index], err = sink.Register(fmt.Sprintf("runtime-%03d", index+1), sources[index])
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sink.Run(ctx) }()
	sinkStopped := false
	defer func() {
		if sinkStopped {
			return
		}
		cancel()
		select {
		case runErr := <-done:
			if runErr != nil {
				t.Errorf("snapshot sink: %v", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("snapshot sink did not stop")
		}
	}()

	waitForScaleTopology(t, application, func(topology domain.Topology) bool {
		return len(topology.Observers) == DefaultScaleNodeCount && countOnline(topology) == DefaultScaleNodeCount
	})
	trafficAt := time.Now().UTC()
	for index, snapshot := range scenario.ExporterSnapshots(trafficAt, 2) {
		sources[index].set(snapshot)
	}
	waitForScaleCondition(t, application, func(topology domain.Topology) bool {
		return len(topology.Edges) == DefaultScaleEdgeCount && reporter.trafficObservers() >= DefaultScaleNodeCount
	})
	before := application.Aggregator.Snapshot()
	edgeID := edgeForStableNode(t, before, "scale-001")
	historyBefore, err := database.EdgeHistory(context.Background(), edgeID, baselineAt.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	withdrawContext, withdrawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := registrations[0].Withdraw(withdrawContext); err != nil {
		withdrawCancel()
		t.Fatal(err)
	}
	withdrawCancel()
	waitForScaleTopology(t, application, func(topology domain.Topology) bool {
		return len(topology.Observers) == DefaultScaleNodeCount && countOnline(topology) == DefaultScaleNodeCount-1
	})

	replacement := &mutableScaleSource{snapshot: scenario.ExporterSnapshots(time.Now().UTC(), 2)[0]}
	if _, err := sink.Register("runtime-001", replacement); err != nil {
		t.Fatal(err)
	}
	waitForScaleTopology(t, application, func(topology domain.Topology) bool {
		return countOnline(topology) == DefaultScaleNodeCount && len(topology.Edges) == DefaultScaleEdgeCount
	})
	historyAfter, err := database.EdgeHistory(context.Background(), edgeID, baselineAt.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if trafficTotal(historyAfter) != trafficTotal(historyBefore) {
		t.Fatalf("withdraw/reconnect fabricated traffic: before=%d after=%d", trafficTotal(historyBefore), trafficTotal(historyAfter))
	}

	cancel()
	if runErr := <-done; runErr != nil {
		t.Fatal(runErr)
	}
	sinkStopped = true

	reports := reporter.snapshotReports()
	if len(reports) < 10 {
		t.Fatalf("reports = %d, want batched hello, traffic, withdrawal, and reconnect", len(reports))
	}
	for index, report := range reports {
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if report.ReporterInstanceID != scaleExporterReporterID || report.Sequence != int64(index+1) {
			t.Fatalf("report %d identity/sequence = %s/%d", index, report.ReporterInstanceID, report.Sequence)
		}
		if len(report.Observers) > 64 || len(payload) > 1<<20 {
			t.Fatalf("report %d bounds = %d observers/%d bytes", index, len(report.Observers), len(payload))
		}
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	restartedDatabase, err := store.Open(databasePath, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedDatabase.Close()
	restarted, err := app.New(restartedDatabase, aggregate.Options{HeartbeatInterval: 10 * time.Minute}, nil)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart := restarted.Aggregator.Snapshot()
	if len(afterRestart.Nodes) != DefaultScaleNodeCount || len(afterRestart.Edges) != DefaultScaleEdgeCount ||
		len(afterRestart.Observers) != DefaultScaleNodeCount || countOnline(afterRestart) != DefaultScaleNodeCount {
		t.Fatalf("restart topology = %d nodes/%d edges/%d observers/%d online",
			len(afterRestart.Nodes), len(afterRestart.Edges), len(afterRestart.Observers), countOnline(afterRestart))
	}
	restartedHistory, err := restartedDatabase.EdgeHistory(context.Background(), edgeID, baselineAt.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if trafficTotal(restartedHistory) != trafficTotal(historyBefore) {
		t.Fatalf("restart traffic total = %d, want %d", trafficTotal(restartedHistory), trafficTotal(historyBefore))
	}
}

func cloneExporterSnapshot(snapshot exporter.Snapshot) exporter.Snapshot {
	copy := snapshot
	copy.Observer.TailscaleIPs = append([]string(nil), snapshot.Observer.TailscaleIPs...)
	copy.Peers = append([]exporter.PeerSnapshot(nil), snapshot.Peers...)
	for index := range copy.Peers {
		copy.Peers[index].Identity.TailscaleIPs = append([]string(nil), snapshot.Peers[index].Identity.TailscaleIPs...)
		if snapshot.Peers[index].Path.PeerRelayVNI != nil {
			vni := *snapshot.Peers[index].Path.PeerRelayVNI
			copy.Peers[index].Path.PeerRelayVNI = &vni
		}
	}
	return copy
}

func waitForScaleTopology(t *testing.T, application *app.App, predicate func(domain.Topology) bool) {
	t.Helper()
	waitForScaleCondition(t, application, predicate)
}

func waitForScaleCondition(t *testing.T, application *app.App, predicate func(domain.Topology) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if predicate(application.Aggregator.Snapshot()) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	topology := application.Aggregator.Snapshot()
	t.Fatalf("timed out with %d nodes/%d edges/%d observers/%d online",
		len(topology.Nodes), len(topology.Edges), len(topology.Observers), countOnline(topology))
}

func countOnline(topology domain.Topology) int {
	count := 0
	for _, observer := range topology.Observers {
		if observer.Online {
			count++
		}
	}
	return count
}

func edgeForStableNode(t *testing.T, topology domain.Topology, stableNodeID string) string {
	t.Helper()
	canonical := ""
	for _, node := range topology.Nodes {
		if node.StableNodeID == stableNodeID {
			canonical = node.ID
			break
		}
	}
	for _, edge := range topology.Edges {
		if edge.Source == canonical || edge.Target == canonical {
			return edge.ID
		}
	}
	t.Fatalf("no edge for %s", stableNodeID)
	return ""
}

func trafficTotal(history domain.EdgeHistory) int64 {
	var total int64
	for _, bucket := range history.Traffic {
		total += bucket.AToBBytes + bucket.BToABytes
	}
	return total
}
