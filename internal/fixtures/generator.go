package fixtures

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
)

type Generator struct {
	app      *app.App
	logger   *slog.Logger
	sequence int64
	tick     int64
}

func New(application *app.App, logger *slog.Logger) *Generator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Generator{app: application, logger: logger}
}

func (g *Generator) Start(ctx context.Context) error {
	if err := g.hello(ctx); err != nil {
		return fmt.Errorf("fixture hello: %w", err)
	}
	if err := g.seedHistory(ctx); err != nil {
		return fmt.Errorf("fixture history seed: %w", err)
	}
	g.sample(ctx)
	go g.run(ctx)
	return nil
}

func (g *Generator) run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.sample(ctx)
		}
	}
}

func (g *Generator) seedHistory(ctx context.Context) error {
	now := time.Now().UTC()
	for minutesAgo := 58; minutesAgo >= 2; minutesAgo -= 2 {
		at := now.Add(-time.Duration(minutesAgo) * time.Minute)
		if _, err := g.app.SubmitAt(ctx, g.sampleReport(at), at); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) hello(ctx context.Context) error {
	now := time.Now().UTC()
	report := g.envelope(domain.ReportObserverHello, now, []domain.ObserverReport{
		{Observer: fixtureNode("macbook", "MacBook"), InventoryGeneration: "fixture-v1", Peers: baselinePeers(now,
			fixtureNode("devbox", "DevBox"), fixtureNode("iphone", "iPhone"), fixtureNode("windows", "Windows"), fixtureNode("relay-hz", "Relay-HZ"))},
		{Observer: fixtureNode("devbox", "DevBox"), InventoryGeneration: "fixture-v1", Peers: baselinePeers(now, fixtureNode("macbook", "MacBook"))},
		{Observer: fixtureNode("linux", "Linux"), InventoryGeneration: "fixture-v1", Peers: baselinePeers(now, fixtureNode("ipad", "iPad"))},
	})
	_, err := g.app.Submit(ctx, report)
	return err
}

func (g *Generator) sample(ctx context.Context) {
	report := g.sampleReport(time.Now().UTC())
	if _, err := g.app.Submit(ctx, report); err != nil {
		g.logger.Warn("fixture sample failed", "error", err)
	}
}

func (g *Generator) sampleReport(now time.Time) domain.ReportEnvelope {
	g.tick++
	iphonePath := domain.PathObservation{Kind: domain.PathDERP, DERPRegion: "hkg"}
	if g.tick%15 >= 11 {
		iphonePath = domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "203.0.113.42:41641"}
	}
	return g.envelope(domain.ReportTrafficSample, now, []domain.ObserverReport{
		{
			Observer: fixtureNode("macbook", "MacBook"), InventoryGeneration: "fixture-v1",
			Peers: []domain.PeerObservation{
				fixturePeer("devbox", "DevBox", 8200+g.tick*1_300_000, 16400+g.tick*2_700_000, 1_300_000, 2_700_000, domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "192.168.10.24:41641"}, now),
				fixturePeer("iphone", "iPhone", 2800+g.tick*320, 5100+g.tick*740, 320, 740, iphonePath, now),
				fixturePeer("windows", "Windows", 1900+g.tick*180, 3800+g.tick*460, 180, 460, domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay-hz"}, now),
			},
		},
		{
			Observer: fixtureNode("devbox", "DevBox"), InventoryGeneration: "fixture-v1",
			Peers: []domain.PeerObservation{
				fixturePeer("macbook", "MacBook", 16400+g.tick*2_700_000, 8200+g.tick*1_300_000, 2_700_000, 1_300_000, domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: "192.168.10.5:41641"}, now),
			},
		},
		{
			Observer: fixtureNode("linux", "Linux"), InventoryGeneration: "fixture-v1",
			Peers: []domain.PeerObservation{
				fixturePeer("ipad", "iPad", 500+g.tick*50, 700+g.tick*90, 50, 90, domain.PathObservation{Kind: domain.PathUnknown}, now),
			},
		},
	})
}

func (g *Generator) envelope(kind domain.ReportKind, at time.Time, observers []domain.ObserverReport) domain.ReportEnvelope {
	g.sequence++
	return domain.ReportEnvelope{
		Version:            domain.ProtocolVersion,
		ReportID:           fmt.Sprintf("00000000-0000-4000-8000-%012d", g.sequence),
		ReporterInstanceID: "00000000-0000-4000-8000-000000000001",
		Sequence:           g.sequence,
		CollectedAt:        at,
		Kind:               kind,
		Observers:          observers,
	}
}

func baselinePeers(at time.Time, nodes ...domain.NodeIdentity) []domain.PeerObservation {
	peers := make([]domain.PeerObservation, 0, len(nodes))
	for _, node := range nodes {
		peers = append(peers, domain.PeerObservation{Peer: node, Path: domain.PathObservation{Kind: domain.PathUnknown}, LastActive: at})
	}
	return peers
}

func fixturePeer(id, hostname string, rx, tx, rxDelta, txDelta int64, path domain.PathObservation, at time.Time) domain.PeerObservation {
	return domain.PeerObservation{
		Peer:             fixtureNode(id, hostname),
		RxBytes:          rx,
		TxBytes:          tx,
		RxDelta:          rxDelta,
		TxDelta:          txDelta,
		SampleDurationMS: 2000,
		Path:             path,
		LastActive:       at,
	}
}

func fixtureNode(id, hostname string) domain.NodeIdentity {
	octets := map[string]int{
		"macbook":  11,
		"devbox":   12,
		"iphone":   13,
		"windows":  14,
		"relay-hz": 15,
		"linux":    16,
		"ipad":     17,
	}
	platforms := map[string]string{
		"macbook": "macos", "devbox": "linux", "iphone": "ios",
		"windows": "windows", "relay-hz": "linux", "linux": "linux", "ipad": "ios",
	}
	return domain.NodeIdentity{
		StableNodeID: id,
		Hostname:     hostname,
		DNSName:      hostname + ".example.ts.net.",
		OS:           platforms[id],
		TailscaleIPs: []string{"100.64.0." + fmt.Sprint(octets[id])},
	}
}
