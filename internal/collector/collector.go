package collector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type Snapshot struct {
	CollectedAt time.Time
	Observer    domain.NodeIdentity
	Peers       []PeerSnapshot
}

type PeerSnapshot struct {
	Identity domain.NodeIdentity
	RxBytes  int64
	TxBytes  int64
	Path     domain.PathObservation
}

type Source interface {
	Snapshot(context.Context) (Snapshot, error)
}

type Reporter interface {
	Send(context.Context, domain.ReportEnvelope) (domain.ReportReceipt, error)
}

type Options struct {
	SampleInterval    time.Duration
	HeartbeatInterval time.Duration
	ReporterInstance  string
	Now               func() time.Time
	Logger            *slog.Logger
}

type Collector struct {
	source              Source
	reporter            Reporter
	sampleInterval      time.Duration
	heartbeatInterval   time.Duration
	reporterInstance    string
	now                 func() time.Time
	logger              *slog.Logger
	sequence            int64
	connected           bool
	baseline            Snapshot
	inventoryGeneration string
	controlIDs          map[string]struct{}
	lastReport          time.Time
}

func New(source Source, reporter Reporter, options Options) *Collector {
	if options.SampleInterval == 0 {
		options.SampleInterval = 2 * time.Second
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = time.Minute
	}
	if options.ReporterInstance == "" {
		options.ReporterInstance = newUUID()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Collector{
		source:            source,
		reporter:          reporter,
		sampleInterval:    options.SampleInterval,
		heartbeatInterval: options.HeartbeatInterval,
		reporterInstance:  options.ReporterInstance,
		now:               options.Now,
		logger:            options.Logger,
		controlIDs:        make(map[string]struct{}),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	if err := c.Step(ctx); err != nil {
		c.logger.Warn("initial collection failed", "error", err)
	}
	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.Step(ctx); err != nil {
				c.logger.Warn("collection failed", "error", err)
			}
		}
	}
}

func (c *Collector) Step(ctx context.Context) error {
	snapshot, err := c.source.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("read local status: %w", err)
	}
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = c.now()
	}
	generation := inventoryHash(snapshot)

	if !c.connected {
		receipt, sendErr := c.send(ctx, domain.ReportObserverHello, snapshot, generation, baselinePeers(snapshot))
		if sendErr != nil {
			c.baseline = snapshot
			return sendErr
		}
		c.acceptReceipt(receipt)
		c.connected = receipt.Accepted && !receipt.ResyncRequired
		c.baseline = snapshot
		c.inventoryGeneration = generation
		c.lastReport = snapshot.CollectedAt
		return nil
	}

	if generation != c.inventoryGeneration {
		receipt, sendErr := c.send(ctx, domain.ReportInventoryUpdate, snapshot, generation, baselinePeers(snapshot))
		if sendErr != nil {
			c.connected = false
			c.baseline = snapshot
			return sendErr
		}
		c.acceptReceipt(receipt)
		if !receipt.Accepted || receipt.ResyncRequired {
			c.connected = false
			c.baseline = snapshot
			return nil
		}
		c.inventoryGeneration = generation
		c.lastReport = snapshot.CollectedAt
	}

	peers := c.changedPeers(snapshot)
	if len(peers) > 0 {
		receipt, sendErr := c.send(ctx, domain.ReportTrafficSample, snapshot, c.inventoryGeneration, peers)
		c.baseline = snapshot
		if sendErr != nil {
			c.connected = false
			return sendErr
		}
		c.acceptReceipt(receipt)
		if receipt.Accepted {
			c.lastReport = snapshot.CollectedAt
		}
		if !receipt.Accepted || receipt.ResyncRequired {
			c.connected = false
		}
		return nil
	}

	c.baseline = snapshot
	if snapshot.CollectedAt.Sub(c.lastReport) < c.heartbeatInterval {
		return nil
	}
	receipt, sendErr := c.send(ctx, domain.ReportObserverHeartbeat, snapshot, c.inventoryGeneration, nil)
	if sendErr != nil {
		c.connected = false
		return sendErr
	}
	c.acceptReceipt(receipt)
	if receipt.Accepted {
		c.lastReport = snapshot.CollectedAt
	}
	if !receipt.Accepted || receipt.ResyncRequired {
		c.connected = false
	}
	return nil
}

func (c *Collector) changedPeers(snapshot Snapshot) []domain.PeerObservation {
	previous := make(map[string]PeerSnapshot, len(c.baseline.Peers))
	for _, peer := range c.baseline.Peers {
		previous[peer.Identity.IdentityKey()] = peer
	}
	duration := snapshot.CollectedAt.Sub(c.baseline.CollectedAt)
	if duration <= 0 {
		duration = c.sampleInterval
	}
	var changed []domain.PeerObservation
	for _, peer := range snapshot.Peers {
		if _, control := c.controlIDs[peer.Identity.StableNodeID]; control {
			continue
		}
		old, ok := previous[peer.Identity.IdentityKey()]
		if !ok {
			continue
		}
		rxDelta := counterDelta(old.RxBytes, peer.RxBytes)
		txDelta := counterDelta(old.TxBytes, peer.TxBytes)
		if rxDelta == 0 && txDelta == 0 {
			continue
		}
		changed = append(changed, domain.PeerObservation{
			Peer:             peer.Identity,
			RxBytes:          peer.RxBytes,
			TxBytes:          peer.TxBytes,
			RxDelta:          rxDelta,
			TxDelta:          txDelta,
			SampleDurationMS: max(duration.Milliseconds(), 1),
			Path:             peer.Path,
			LastActive:       snapshot.CollectedAt,
		})
	}
	return changed
}

func counterDelta(previous, current int64) int64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func baselinePeers(snapshot Snapshot) []domain.PeerObservation {
	peers := make([]domain.PeerObservation, 0, len(snapshot.Peers))
	for _, peer := range snapshot.Peers {
		peers = append(peers, domain.PeerObservation{
			Peer:       peer.Identity,
			RxBytes:    peer.RxBytes,
			TxBytes:    peer.TxBytes,
			Path:       peer.Path,
			LastActive: snapshot.CollectedAt,
		})
	}
	return peers
}

func (c *Collector) send(ctx context.Context, kind domain.ReportKind, snapshot Snapshot, generation string, peers []domain.PeerObservation) (domain.ReportReceipt, error) {
	c.sequence++
	report := domain.ReportEnvelope{
		Version:            domain.ProtocolVersion,
		ReportID:           newUUID(),
		ReporterInstanceID: c.reporterInstance,
		Sequence:           c.sequence,
		CollectedAt:        snapshot.CollectedAt,
		Kind:               kind,
		Observers: []domain.ObserverReport{{
			Observer:            snapshot.Observer,
			InventoryGeneration: generation,
			Peers:               peers,
		}},
	}
	receipt, err := c.reporter.Send(ctx, report)
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("send %s: %w", kind, err)
	}
	return receipt, nil
}

func (c *Collector) acceptReceipt(receipt domain.ReportReceipt) {
	if receipt.HeartbeatIntervalMS > 0 {
		c.heartbeatInterval = time.Duration(receipt.HeartbeatIntervalMS) * time.Millisecond
	}
	for _, stableID := range receipt.ControlStableNodeIDs {
		if stableID != "" {
			c.controlIDs[stableID] = struct{}{}
		}
	}
}

func inventoryHash(snapshot Snapshot) string {
	identities := make([]domain.NodeIdentity, 0, len(snapshot.Peers)+1)
	identities = append(identities, snapshot.Observer)
	for _, peer := range snapshot.Peers {
		identities = append(identities, peer.Identity)
	}
	for index := range identities {
		sort.Strings(identities[index].TailscaleIPs)
	}
	sort.Slice(identities, func(i, j int) bool {
		return identities[i].IdentityKey() < identities[j].IdentityKey()
	})
	encoded, _ := json.Marshal(identities)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func newUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
