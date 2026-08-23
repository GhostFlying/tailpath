package fixtures

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
)

const (
	DefaultScaleSeed      int64 = 0x7461696c70617468
	DefaultScaleNodeCount       = 250
	DefaultScaleEdgeCount       = 1000
)

type ScaleConfig struct {
	Seed      int64
	NodeCount int
	EdgeCount int
}

type TimedReport struct {
	Report     domain.ReportEnvelope
	ReceivedAt time.Time
}

type ScaleScenario struct {
	seed      int64
	nodes     []domain.NodeIdentity
	edges     []scaleEdge
	neighbors [][]int
}

type scaleEdge struct {
	index     int
	source    int
	target    int
	path      domain.PathObservation
	aToBBytes int64
	bToABytes int64
	recent    bool
}

func DefaultScaleConfig() ScaleConfig {
	return ScaleConfig{
		Seed:      DefaultScaleSeed,
		NodeCount: DefaultScaleNodeCount,
		EdgeCount: DefaultScaleEdgeCount,
	}
}

func NewScaleScenario(config ScaleConfig) (*ScaleScenario, error) {
	if config.NodeCount < 3 {
		return nil, errors.New("scale fixture requires at least three nodes")
	}
	if config.EdgeCount < 1 || config.EdgeCount%config.NodeCount != 0 {
		return nil, errors.New("scale fixture edge count must be a positive multiple of node count")
	}
	offsets := config.EdgeCount / config.NodeCount
	if offsets >= config.NodeCount/2 {
		return nil, errors.New("scale fixture edge count is too dense for the ring generator")
	}

	scenario := &ScaleScenario{
		seed:      config.Seed,
		nodes:     make([]domain.NodeIdentity, config.NodeCount),
		edges:     make([]scaleEdge, 0, config.EdgeCount),
		neighbors: make([][]int, config.NodeCount),
	}
	for index := range scenario.nodes {
		scenario.nodes[index] = scaleNode(index)
	}

	random := rand.New(rand.NewSource(config.Seed))
	for offset := 1; offset <= offsets; offset++ {
		for source := range scenario.nodes {
			target := (source + offset) % len(scenario.nodes)
			edgeIndex := len(scenario.edges)
			aToBBytes := int64(512 + random.Intn(32*1024*1024))
			bToABytes := int64(256 + random.Intn(16*1024*1024))
			edge := scaleEdge{
				index:     edgeIndex,
				source:    source,
				target:    target,
				path:      scenario.scalePath(edgeIndex, source, target),
				aToBBytes: aToBBytes,
				bToABytes: bToABytes,
				recent:    edgeIndex%3 == 0,
			}
			scenario.edges = append(scenario.edges, edge)
			scenario.neighbors[source] = append(scenario.neighbors[source], edgeIndex)
			scenario.neighbors[target] = append(scenario.neighbors[target], edgeIndex)
		}
	}
	return scenario, nil
}

func (s *ScaleScenario) NodeCount() int {
	return len(s.nodes)
}

func (s *ScaleScenario) EdgeCount() int {
	return len(s.edges)
}

func (s *ScaleScenario) Reports(at time.Time) []TimedReport {
	at = at.UTC()
	recentAt := at.Add(-20 * time.Second)
	helloAt := recentAt.Add(-time.Second)
	reports := make([]TimedReport, 0, len(s.nodes)*3)
	for node := range s.nodes {
		reports = append(reports, TimedReport{
			Report:     s.helloReport(node, helloAt),
			ReceivedAt: helloAt,
		})
	}
	for node := range s.nodes {
		reports = append(reports, TimedReport{
			Report:     s.trafficReport(node, recentAt, true, 2),
			ReceivedAt: recentAt,
		})
	}
	for node := range s.nodes {
		reports = append(reports, TimedReport{
			Report:     s.trafficReport(node, at, false, 3),
			ReceivedAt: at,
		})
	}
	return reports
}

func (s *ScaleScenario) Load(ctx context.Context, application *app.App, at time.Time) error {
	for _, timed := range s.Reports(at) {
		receipt, err := application.SubmitAt(ctx, timed.Report, timed.ReceivedAt)
		if err != nil {
			return fmt.Errorf("submit scale report %s: %w", timed.Report.ReportID, err)
		}
		if !receipt.Accepted || receipt.ResyncRequired {
			return fmt.Errorf("scale report %s was not accepted cleanly", timed.Report.ReportID)
		}
	}
	return nil
}

// RefreshRuntime keeps the test-only browser fixture inside the ten-second
// active window after the intentionally unoptimized persistent load finishes.
func (s *ScaleScenario) RefreshRuntime(aggregator *aggregate.Aggregator, at time.Time, sequence int64) error {
	for node := range s.nodes {
		report := s.trafficReport(node, at.UTC(), false, sequence)
		result, err := aggregator.ApplyAt(report, at.UTC())
		if err != nil {
			return fmt.Errorf("refresh scale report %s: %w", report.ReportID, err)
		}
		if !result.Receipt.Accepted || result.Receipt.ResyncRequired {
			return fmt.Errorf("scale refresh report %s was not accepted cleanly", report.ReportID)
		}
	}
	return nil
}

func (s *ScaleScenario) helloReport(node int, receivedAt time.Time) domain.ReportEnvelope {
	peers := make([]domain.PeerObservation, 0, len(s.neighbors[node]))
	for _, edgeIndex := range s.neighbors[node] {
		edge := s.edges[edgeIndex]
		peer := edge.source
		if peer == node {
			peer = edge.target
		}
		peers = append(peers, domain.PeerObservation{
			Peer:       s.nodes[peer],
			Path:       domain.PathObservation{Kind: domain.PathUnknown},
			LastActive: receivedAt,
		})
	}
	return s.envelope(node, 1, domain.ReportObserverHello, receivedAt, peers)
}

func (s *ScaleScenario) trafficReport(node int, receivedAt time.Time, recent bool, sequence int64) domain.ReportEnvelope {
	peers := make([]domain.PeerObservation, 0, len(s.neighbors[node])/2)
	for _, edgeIndex := range s.neighbors[node] {
		edge := s.edges[edgeIndex]
		if edge.recent != recent {
			continue
		}
		peer := edge.source
		txDelta, rxDelta := edge.bToABytes, edge.aToBBytes
		if peer == node {
			peer = edge.target
			txDelta, rxDelta = edge.aToBBytes, edge.bToABytes
		}
		peers = append(peers, domain.PeerObservation{
			Peer:             s.nodes[peer],
			RxBytes:          10_000_000 + rxDelta,
			TxBytes:          20_000_000 + txDelta,
			RxDelta:          rxDelta,
			TxDelta:          txDelta,
			SampleDurationMS: 2000,
			Path:             s.pathForObserver(edge, peer),
			LastActive:       receivedAt,
		})
	}
	return s.envelope(node, sequence, domain.ReportTrafficSample, receivedAt, peers)
}

func (s *ScaleScenario) envelope(
	node int,
	sequence int64,
	kind domain.ReportKind,
	receivedAt time.Time,
	peers []domain.PeerObservation,
) domain.ReportEnvelope {
	collectedAt := receivedAt
	if node%29 == 0 {
		collectedAt = collectedAt.Add(6 * time.Minute)
	}
	return domain.ReportEnvelope{
		Version:            domain.ProtocolVersion,
		ReportID:           scaleUUID(int(sequence)*100_000 + node + 1),
		ReporterInstanceID: scaleUUID(900_000 + node + 1),
		Sequence:           sequence,
		CollectedAt:        collectedAt,
		Kind:               kind,
		Observers: []domain.ObserverReport{{
			Observer:            s.nodes[node],
			InventoryGeneration: fmt.Sprintf("scale-%016x", uint64(s.seed)),
			Peers:               peers,
		}},
	}
}

func (s *ScaleScenario) scalePath(index, source, target int) domain.PathObservation {
	switch index % 4 {
	case 0:
		return domain.PathObservation{Kind: domain.PathDirect}
	case 1:
		regions := []string{"hkg", "lhr", "sfo", "syd", "fra"}
		return domain.PathObservation{Kind: domain.PathDERP, DERPRegion: regions[(index/4)%len(regions)]}
	case 2:
		relay := len(s.nodes) - 1 - (index/4)%8
		for relay == source || relay == target {
			relay = (relay - 1 + len(s.nodes)) % len(s.nodes)
		}
		return domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: s.nodes[relay].StableNodeID}
	default:
		return domain.PathObservation{Kind: domain.PathUnknown}
	}
}

func (s *ScaleScenario) pathForObserver(edge scaleEdge, peer int) domain.PathObservation {
	path := edge.path
	if path.Kind == domain.PathDirect {
		path.DirectEndpoint = scaleEndpoint(peer)
	}
	return path
}

func scaleNode(index int) domain.NodeIdentity {
	hostname := fmt.Sprintf("scale-node-%03d", index+1)
	platforms := [...]string{"linux", "macos", "windows", "ios", "android"}
	return domain.NodeIdentity{
		StableNodeID: fmt.Sprintf("scale-%03d", index+1),
		NodeID:       fmt.Sprintf("nodeid-%03d", index+1),
		NodeKey:      fmt.Sprintf("nodekey:%064x", index+1),
		DiscoKey:     fmt.Sprintf("discokey:%064x", index+1),
		Hostname:     hostname,
		DNSName:      hostname + ".scale.example.ts.net.",
		OS:           platforms[index%len(platforms)],
		TailscaleIPs: []string{fmt.Sprintf("100.100.%d.%d", index/250, index%250+1)},
	}
}

func scaleEndpoint(index int) string {
	return fmt.Sprintf("203.0.%d.%d:41641", index/250, index%250+1)
}

func scaleUUID(value int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", value)
}
