package collector

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	mathrand "math/rand"
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
	RetryMin          time.Duration
	RetryMax          time.Duration
	SummaryInterval   time.Duration
	ReporterInstance  string
	Now               func() time.Time
	Jitter            func() float64
	Wait              func(context.Context, time.Duration) error
	Logger            *slog.Logger
}

type Collector struct {
	source              Source
	reporter            Reporter
	sampleInterval      time.Duration
	heartbeatInterval   time.Duration
	retryMin            time.Duration
	retryMax            time.Duration
	summaryInterval     time.Duration
	reporterInstance    string
	now                 func() time.Time
	jitter              func() float64
	wait                func(context.Context, time.Duration) error
	logger              *slog.Logger
	sequence            int64
	connected           bool
	baseline            Snapshot
	inventoryGeneration string
	controlIDs          map[string]struct{}
	lastReportScheduled time.Time
}

func New(source Source, reporter Reporter, options Options) *Collector {
	if options.SampleInterval == 0 {
		options.SampleInterval = 2 * time.Second
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = time.Minute
	}
	if options.RetryMin == 0 {
		options.RetryMin = 2 * time.Second
	}
	if options.RetryMax == 0 {
		options.RetryMax = 60 * time.Second
	}
	if options.SummaryInterval == 0 {
		options.SummaryInterval = 5 * time.Minute
	}
	if options.ReporterInstance == "" {
		options.ReporterInstance = newUUID()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Jitter == nil {
		options.Jitter = mathrand.Float64
	}
	if options.Wait == nil {
		options.Wait = waitContext
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Collector{
		source:            source,
		reporter:          reporter,
		sampleInterval:    options.SampleInterval,
		heartbeatInterval: options.HeartbeatInterval,
		retryMin:          options.RetryMin,
		retryMax:          options.RetryMax,
		summaryInterval:   options.SummaryInterval,
		reporterInstance:  options.ReporterInstance,
		now:               options.Now,
		jitter:            options.Jitter,
		wait:              options.Wait,
		logger:            options.Logger,
		controlIDs:        make(map[string]struct{}),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	consecutiveFailures := 0
	degraded := false
	var degradedSince time.Time
	var lastSummary time.Time
	for {
		result, err := c.step(ctx)
		waitFor := c.sampleInterval
		if err != nil {
			c.connected = false
			consecutiveFailures++
			waitFor = c.retryDelay(consecutiveFailures)
			now := c.now()
			if !degraded {
				degraded = true
				degradedSince = now
				lastSummary = now
				c.logger.Warn("collector degraded", "error", err, "retry_in", waitFor)
			} else if now.Sub(lastSummary) >= c.summaryInterval || now.Before(lastSummary) {
				lastSummary = now
				c.logger.Warn("collector remains degraded", "error", err, "failures", consecutiveFailures, "degraded_for", now.Sub(degradedSince), "retry_in", waitFor)
			}
		} else if result.helloAccepted {
			if degraded {
				c.logger.Info("collector recovered", "failures", consecutiveFailures, "degraded_for", c.now().Sub(degradedSince))
			}
			degraded = false
			consecutiveFailures = 0
		} else if result.resyncRequired {
			c.logger.Warn("collector resync required")
		}
		if waitErr := c.wait(ctx, waitFor); waitErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("wait for next collection: %w", waitErr)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func (c *Collector) Step(ctx context.Context) error {
	_, err := c.step(ctx)
	return err
}

type stepResult struct {
	helloAccepted  bool
	resyncRequired bool
}

func (c *Collector) step(ctx context.Context) (stepResult, error) {
	snapshot, err := c.source.Snapshot(ctx)
	if err != nil {
		c.connected = false
		return stepResult{}, fmt.Errorf("read local status: %w", err)
	}
	scheduledAt := c.now()
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = scheduledAt
	}
	generation := inventoryHash(snapshot)

	if !c.connected {
		receipt, sendErr := c.send(ctx, domain.ReportObserverHello, snapshot, generation, baselinePeers(snapshot))
		if sendErr != nil {
			c.baseline = snapshot
			return stepResult{}, sendErr
		}
		c.acceptReceipt(receipt)
		c.baseline = snapshot
		if !receipt.Accepted {
			c.connected = false
			return stepResult{}, fmt.Errorf("server rejected observer hello")
		}
		c.connected = true
		c.inventoryGeneration = generation
		c.lastReportScheduled = scheduledAt
		return stepResult{helloAccepted: true}, nil
	}

	if generation != c.inventoryGeneration {
		receipt, sendErr := c.send(ctx, domain.ReportInventoryUpdate, snapshot, generation, baselinePeers(snapshot))
		if sendErr != nil {
			c.connected = false
			c.baseline = snapshot
			return stepResult{}, sendErr
		}
		c.acceptReceipt(receipt)
		if !receipt.Accepted || receipt.ResyncRequired {
			c.connected = false
			c.baseline = snapshot
			return stepResult{resyncRequired: true}, nil
		}
		c.inventoryGeneration = generation
		c.lastReportScheduled = scheduledAt
	}

	peers := c.changedPeers(snapshot)
	if len(peers) > 0 {
		receipt, sendErr := c.send(ctx, domain.ReportTrafficSample, snapshot, c.inventoryGeneration, peers)
		c.baseline = snapshot
		if sendErr != nil {
			c.connected = false
			return stepResult{}, sendErr
		}
		c.acceptReceipt(receipt)
		if receipt.Accepted {
			c.lastReportScheduled = scheduledAt
		}
		if !receipt.Accepted || receipt.ResyncRequired {
			c.connected = false
		}
		return stepResult{resyncRequired: !receipt.Accepted || receipt.ResyncRequired}, nil
	}

	c.baseline = snapshot
	elapsed := scheduledAt.Sub(c.lastReportScheduled)
	if elapsed >= 0 && elapsed < c.heartbeatInterval {
		return stepResult{}, nil
	}
	receipt, sendErr := c.send(ctx, domain.ReportObserverHeartbeat, snapshot, c.inventoryGeneration, nil)
	if sendErr != nil {
		c.connected = false
		return stepResult{}, sendErr
	}
	c.acceptReceipt(receipt)
	if receipt.Accepted {
		c.lastReportScheduled = scheduledAt
	}
	if !receipt.Accepted || receipt.ResyncRequired {
		c.connected = false
	}
	return stepResult{resyncRequired: !receipt.Accepted || receipt.ResyncRequired}, nil
}

func (c *Collector) retryDelay(failures int) time.Duration {
	base := c.retryMin
	for attempt := 1; attempt < failures && base < c.retryMax; attempt++ {
		if base > c.retryMax/2 {
			base = c.retryMax
			break
		}
		base *= 2
	}
	if base > c.retryMax {
		base = c.retryMax
	}
	multiplier := 0.8 + 0.4*c.jitter()
	return time.Duration(float64(base) * multiplier)
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	if _, err := cryptorand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
