package collector

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
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
	RelayTelemetry    bool
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
	relaySource         RelaySource
	relayTelemetry      bool
	relayBaseline       *RelaySnapshot
	relayCapability     RelayCapability
	relayIdentity       RelayIdentityEvidence
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
	result := &Collector{
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
		relayTelemetry:    options.RelayTelemetry,
		controlIDs:        make(map[string]struct{}),
	}
	if options.RelayTelemetry {
		result.relaySource, _ = source.(RelaySource)
	}
	return result
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
	wasConnected := c.connected
	result, err := c.stepOrdinary(ctx)
	if err != nil {
		c.relayBaseline = nil
		return result, err
	}
	if !c.relayTelemetry || c.relaySource == nil {
		return result, nil
	}
	if result.resyncRequired || !c.connected {
		c.relayBaseline = nil
		return result, nil
	}
	relayResync, err := c.stepRelay(ctx, result.helloAccepted || !wasConnected)
	result.resyncRequired = result.resyncRequired || relayResync
	return result, err
}

func (c *Collector) stepOrdinary(ctx context.Context) (stepResult, error) {
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

func (c *Collector) stepRelay(ctx context.Context, establishBaseline bool) (bool, error) {
	snapshot, err := c.relaySource.PeerRelaySnapshot(ctx)
	if err != nil {
		c.setRelayCapability(RelayTransientFailure)
		c.relayBaseline = nil
		return false, nil
	}
	c.setRelayCapability(snapshot.Capability)
	if snapshot.Capability != RelayEnabled {
		c.relayBaseline = nil
		return false, nil
	}
	c.setRelayIdentityEvidence(snapshot.IdentityEvidence)
	if establishBaseline || c.relayBaseline == nil {
		c.relayBaseline = &snapshot
		return false, nil
	}
	sessions, err := c.changedRelaySessions(snapshot)
	c.relayBaseline = &snapshot
	if err != nil {
		c.setRelayCapability(RelayTransientFailure)
		return false, nil
	}
	if len(sessions) == 0 {
		return false, nil
	}
	receipt, err := c.sendRelay(ctx, snapshot.CollectedAt, c.baseline.Observer, sessions)
	if err != nil {
		c.connected = false
		c.relayBaseline = nil
		return false, err
	}
	c.acceptReceipt(receipt)
	if !receipt.Accepted || receipt.ResyncRequired {
		c.connected = false
		c.relayBaseline = nil
		return true, nil
	}
	return false, nil
}

func (c *Collector) changedRelaySessions(snapshot RelaySnapshot) ([]domain.RelaySessionObservation, error) {
	previous := make(map[string]RelaySessionSnapshot, len(c.relayBaseline.Sessions))
	for _, session := range c.relayBaseline.Sessions {
		previous[session.SessionID] = session
	}
	duration := snapshot.CollectedAt.Sub(c.relayBaseline.CollectedAt)
	if duration <= 0 {
		duration = c.sampleInterval
	}
	result := make([]domain.RelaySessionObservation, 0, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		old, ok := previous[session.SessionID]
		if !ok || old.Source.SessionClientID != session.Source.SessionClientID ||
			old.Target.SessionClientID != session.Target.SessionClientID {
			continue
		}
		if session.Source.BytesSent < old.Source.BytesSent || session.Target.BytesSent < old.Target.BytesSent {
			continue
		}
		sourceDelta := session.Source.BytesSent - old.Source.BytesSent
		targetDelta := session.Target.BytesSent - old.Target.BytesSent
		if sourceDelta == 0 && targetDelta == 0 {
			continue
		}
		sourceBytes, err := relayCounter(session.Source.BytesSent)
		if err != nil {
			return nil, err
		}
		targetBytes, err := relayCounter(session.Target.BytesSent)
		if err != nil {
			return nil, err
		}
		sourceDeltaValue, err := relayCounter(sourceDelta)
		if err != nil {
			return nil, err
		}
		targetDeltaValue, err := relayCounter(targetDelta)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.RelaySessionObservation{
			Relay:     c.baseline.Observer,
			Source:    relayReportClient(session.Source),
			Target:    relayReportClient(session.Target),
			SessionID: session.SessionID, VNI: session.VNI,
			SourceToTargetBytes: sourceBytes, TargetToSourceBytes: targetBytes,
			SourceToTargetDelta: sourceDeltaValue, TargetToSourceDelta: targetDeltaValue,
			SampleDurationMS: max(duration.Milliseconds(), 1), LastActive: snapshot.CollectedAt,
		})
	}
	return result, nil
}

func relayReportClient(snapshot RelayClientSnapshot) domain.RelaySessionClient {
	return domain.RelaySessionClient{
		SessionClientID: snapshot.SessionClientID,
		Identity:        snapshot.Identity,
		DiscoShort:      snapshot.DiscoShort,
		Endpoint:        snapshot.Endpoint,
	}
}

func relayCounter(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, errors.New("relay traffic counter exceeds protocol range")
	}
	return int64(value), nil
}

func (c *Collector) sendRelay(
	ctx context.Context,
	collectedAt time.Time,
	relay domain.NodeIdentity,
	sessions []domain.RelaySessionObservation,
) (domain.ReportReceipt, error) {
	for index := range sessions {
		sessions[index].Relay = relay
	}
	c.sequence++
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: newUUID(), ReporterInstanceID: c.reporterInstance,
		Sequence: c.sequence, CollectedAt: collectedAt, Kind: domain.ReportRelaySessionUpdate,
		RelaySessions: sessions,
	}
	receipt, err := c.reporter.Send(ctx, report)
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("send %s: %w", domain.ReportRelaySessionUpdate, err)
	}
	return receipt, nil
}

func (c *Collector) setRelayCapability(capability RelayCapability) {
	if capability == c.relayCapability {
		return
	}
	previous := c.relayCapability
	c.relayCapability = capability
	if capability == RelayTransientFailure {
		c.logger.Warn("relay telemetry degraded", "capability", capability)
		return
	}
	if previous == RelayTransientFailure {
		c.logger.Info("relay telemetry recovered", "capability", capability)
		return
	}
	c.logger.Info("relay telemetry capability", "capability", capability)
}

func (c *Collector) setRelayIdentityEvidence(status RelayIdentityEvidence) {
	if status == "" || status == c.relayIdentity {
		return
	}
	previous := c.relayIdentity
	c.relayIdentity = status
	if status == RelayIdentityDegraded {
		c.logger.Warn("relay identity enrichment degraded")
		return
	}
	if previous == RelayIdentityDegraded {
		c.logger.Info("relay identity enrichment recovered")
	}
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
