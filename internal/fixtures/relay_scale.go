package fixtures

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
)

const (
	DefaultRelayScaleNodeCount    = 250
	DefaultRelayScaleSessionCount = 1000
	DefaultRelayScaleRelayCount   = 8
	RelayScaleEndpointCanary      = "192.0.2."
	RelayScaleDiscoCanary         = "d:feed"
)

type RelayScaleConfig struct {
	Seed         int64
	NodeCount    int
	SessionCount int
	RelayCount   int
}

type RelayScaleScenario struct {
	seed       int64
	nodes      []domain.NodeIdentity
	relayStart int
	sessions   []relayScaleSession
	byRelay    [][]int
}

type relayScaleSession struct {
	index          int
	source         int
	target         int
	relay          int
	vni            int64
	sourceToTarget int64
	targetToSource int64
}

func DefaultRelayScaleConfig() RelayScaleConfig {
	return RelayScaleConfig{
		Seed:         DefaultScaleSeed,
		NodeCount:    DefaultRelayScaleNodeCount,
		SessionCount: DefaultRelayScaleSessionCount,
		RelayCount:   DefaultRelayScaleRelayCount,
	}
}

func NewRelayScaleScenario(config RelayScaleConfig) (*RelayScaleScenario, error) {
	if config.NodeCount < 4 || config.RelayCount < 1 || config.RelayCount > config.NodeCount-2 {
		return nil, errors.New("relay scale fixture requires clients and at least one relay")
	}
	clientCount := config.NodeCount - config.RelayCount
	maximumEdges := clientCount * (clientCount - 1) / 2
	if config.SessionCount < 1 || config.SessionCount > maximumEdges {
		return nil, fmt.Errorf("relay scale session count must be between 1 and %d", maximumEdges)
	}

	scenario := &RelayScaleScenario{
		seed:       config.Seed,
		nodes:      make([]domain.NodeIdentity, config.NodeCount),
		relayStart: clientCount,
		sessions:   make([]relayScaleSession, 0, config.SessionCount),
		byRelay:    make([][]int, config.RelayCount),
	}
	for index := range scenario.nodes {
		scenario.nodes[index] = scaleNode(index)
	}

	random := rand.New(rand.NewSource(config.Seed ^ 0x72656c6179))
	seen := make(map[string]struct{}, config.SessionCount)
	for offset := 1; len(scenario.sessions) < config.SessionCount; offset++ {
		for source := 0; source < clientCount && len(scenario.sessions) < config.SessionCount; source++ {
			target := (source + offset) % clientCount
			edgeID, _, _ := domain.EdgeID(scenario.nodes[source].StableNodeID, scenario.nodes[target].StableNodeID)
			if _, duplicate := seen[edgeID]; duplicate {
				continue
			}
			seen[edgeID] = struct{}{}
			index := len(scenario.sessions)
			relaySlot := index % config.RelayCount
			session := relayScaleSession{
				index:          index,
				source:         source,
				target:         target,
				relay:          clientCount + relaySlot,
				vni:            int64(1 + index%4096),
				sourceToTarget: int64(512 + random.Intn(32*1024*1024)),
				targetToSource: int64(256 + random.Intn(16*1024*1024)),
			}
			scenario.sessions = append(scenario.sessions, session)
			scenario.byRelay[relaySlot] = append(scenario.byRelay[relaySlot], index)
		}
	}
	return scenario, nil
}

func (s *RelayScaleScenario) NodeCount() int {
	return len(s.nodes)
}

func (s *RelayScaleScenario) SessionCount() int {
	return len(s.sessions)
}

func (s *RelayScaleScenario) RelayCount() int {
	return len(s.byRelay)
}

func (s *RelayScaleScenario) Reports(at time.Time, sequence int64) []TimedReport {
	at = at.UTC()
	reports := make([]TimedReport, 0, len(s.byRelay))
	for relaySlot, sessionIndexes := range s.byRelay {
		sessions := make([]domain.RelaySessionObservation, 0, len(sessionIndexes))
		for _, sessionIndex := range sessionIndexes {
			session := s.sessions[sessionIndex]
			sourceIdentity := s.nodes[session.source]
			targetIdentity := s.nodes[session.target]
			sessions = append(sessions, domain.RelaySessionObservation{
				Relay:     s.nodes[session.relay],
				SessionID: fmt.Sprintf("relay-scale-session-%04d", session.index+1),
				Source: domain.RelaySessionClient{
					SessionClientID: fmt.Sprintf("source-%04d", session.index+1),
					Identity:        &sourceIdentity,
					DiscoShort:      fmt.Sprintf("%s%012x", RelayScaleDiscoCanary, session.index+1),
					Endpoint:        relayScaleEndpoint(session.index, false),
				},
				Target: domain.RelaySessionClient{
					SessionClientID: fmt.Sprintf("target-%04d", session.index+1),
					Identity:        &targetIdentity,
					DiscoShort:      fmt.Sprintf("%s%012x", RelayScaleDiscoCanary, session.index+1+len(s.sessions)),
					Endpoint:        relayScaleEndpoint(session.index, true),
				},
				VNI:                 session.vni,
				SourceToTargetBytes: session.sourceToTarget * sequence,
				TargetToSourceBytes: session.targetToSource * sequence,
				SourceToTargetDelta: session.sourceToTarget,
				TargetToSourceDelta: session.targetToSource,
				SampleDurationMS:    2000,
				LastActive:          at,
			})
		}
		report := domain.ReportEnvelope{
			Version:            domain.ProtocolVersion,
			ReportID:           scaleUUID(3_000_000 + int(sequence)*10_000 + relaySlot),
			ReporterInstanceID: scaleUUID(2_000_000 + relaySlot),
			Sequence:           sequence,
			CollectedAt:        at,
			Kind:               domain.ReportRelaySessionUpdate,
			RelaySessions:      sessions,
		}
		reports = append(reports, TimedReport{Report: report, ReceivedAt: at})
	}
	return reports
}

func (s *RelayScaleScenario) Load(ctx context.Context, application *app.App, at time.Time) error {
	for _, timed := range s.Reports(at, 1) {
		receipt, err := application.SubmitAt(ctx, timed.Report, timed.ReceivedAt)
		if err != nil {
			return fmt.Errorf("submit relay scale report %s: %w", timed.Report.ReportID, err)
		}
		if !receipt.Accepted || receipt.ResyncRequired {
			return fmt.Errorf("relay scale report %s was not accepted cleanly", timed.Report.ReportID)
		}
	}
	return nil
}

func relayScaleEndpoint(index int, ipv6 bool) string {
	port := 20_000 + index%20_000
	if ipv6 {
		return fmt.Sprintf("[2001:db8::%x]:%d", index+1, port)
	}
	return fmt.Sprintf("%s%d:%d", RelayScaleEndpointCanary, index%250+1, port)
}
