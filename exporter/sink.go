package exporter

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSampleInterval    = 2 * time.Second
	defaultBatchWindow       = 100 * time.Millisecond
	defaultSnapshotTimeout   = 15 * time.Second
	defaultHeartbeatInterval = time.Minute
	defaultRetryMin          = 2 * time.Second
	defaultRetryMax          = 60 * time.Second
	defaultMaxBatchObservers = 64
	defaultMaxRequestBytes   = 1 << 20
)

var (
	ErrDuplicateRegistration = errors.New("exporter registration key already exists")
	ErrSnapshotSinkRunning   = errors.New("snapshot sink is already running")
	ErrSnapshotSinkStopped   = errors.New("snapshot sink is not running")
)

type SnapshotSinkOptions struct {
	ReporterInstanceID string
	Logger             *slog.Logger
}

type snapshotSinkConfig struct {
	SampleInterval     time.Duration
	SnapshotTimeout    time.Duration
	BatchWindow        time.Duration
	HeartbeatInterval  time.Duration
	RetryMin           time.Duration
	RetryMax           time.Duration
	MaxBatchObservers  int
	MaxRequestBytes    int
	ReporterInstanceID string
	Now                func() time.Time
	Jitter             func() float64
	Logger             *slog.Logger
}

type SnapshotSink struct {
	reporter Reporter
	config   snapshotSinkConfig
	events   chan sinkEvent

	mu            sync.Mutex
	registrations map[string]*Registration
	started       bool
	running       bool
	runContext    context.Context

	jitterMu sync.Mutex
	clockMu  sync.Mutex
	samplers sync.WaitGroup
}

type Registration struct {
	sink   *SnapshotSink
	key    string
	source Source

	mu           sync.Mutex
	cancel       context.CancelFunc
	withdrawOnce sync.Once
	done         chan struct{}
	result       error
}

type sinkEvent interface{ sinkEvent() }

type sourceResultEvent struct {
	registration *Registration
	snapshot     Snapshot
	err          error
}

func (sourceResultEvent) sinkEvent() {}

type withdrawEvent struct {
	registration *Registration
}

func (withdrawEvent) sinkEvent() {}

type observerReference struct {
	identity   NodeIdentity
	generation string
}

type sourceRuntimeState struct {
	registration *Registration
	latest       Snapshot
	baseline     Snapshot
	hasLatest    bool
	hasBaseline  bool
	healthy      bool
	dirty        bool
	needsHello   bool
	serverRef    *observerReference
	withdrawals  []observerReference
	withdrawing  bool
	lastReportAt time.Time
	sourceFailed bool
	oversized    bool
	withdrawErr  error
}

type observerOperation struct {
	state       *sourceRuntimeState
	kind        ReportKind
	collectedAt time.Time
	reference   observerReference
	observer    ObserverReport
	snapshot    Snapshot
}

type transportRuntimeState struct {
	capabilitiesReady bool
	failures          int
	nextAttempt       time.Time
	degraded          bool
}

func NewSnapshotSink(reporter Reporter, options SnapshotSinkOptions) *SnapshotSink {
	return newSnapshotSink(reporter, snapshotSinkConfig{
		SampleInterval: defaultSampleInterval, SnapshotTimeout: defaultSnapshotTimeout,
		BatchWindow: defaultBatchWindow, HeartbeatInterval: defaultHeartbeatInterval,
		RetryMin: defaultRetryMin, RetryMax: defaultRetryMax,
		MaxBatchObservers: defaultMaxBatchObservers, MaxRequestBytes: defaultMaxRequestBytes,
		ReporterInstanceID: options.ReporterInstanceID, Logger: options.Logger,
		Now: time.Now, Jitter: mathrand.Float64,
	})
}

func newSnapshotSink(reporter Reporter, config snapshotSinkConfig) *SnapshotSink {
	if config.SampleInterval <= 0 {
		config.SampleInterval = defaultSampleInterval
	}
	if config.BatchWindow <= 0 {
		config.BatchWindow = defaultBatchWindow
	}
	if config.SnapshotTimeout <= 0 {
		config.SnapshotTimeout = defaultSnapshotTimeout
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeatInterval
	}
	if config.RetryMin <= 0 {
		config.RetryMin = defaultRetryMin
	}
	if config.RetryMax <= 0 {
		config.RetryMax = defaultRetryMax
	}
	if config.RetryMax < config.RetryMin {
		config.RetryMax = config.RetryMin
	}
	if config.MaxBatchObservers <= 0 || config.MaxBatchObservers > defaultMaxBatchObservers {
		config.MaxBatchObservers = defaultMaxBatchObservers
	}
	if config.MaxRequestBytes <= 0 || config.MaxRequestBytes > defaultMaxRequestBytes {
		config.MaxRequestBytes = defaultMaxRequestBytes
	}
	if config.ReporterInstanceID == "" {
		config.ReporterInstanceID = newExporterUUID()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Jitter == nil {
		config.Jitter = mathrand.Float64
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &SnapshotSink{
		reporter: reporter, config: config, events: make(chan sinkEvent, 1024),
		registrations: make(map[string]*Registration),
	}
}

func (s *SnapshotSink) Register(key string, source Source) (*Registration, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("exporter registration key is required")
	}
	if source == nil {
		return nil, errors.New("exporter source is required")
	}
	registration := &Registration{sink: s, key: key, source: source, done: make(chan struct{})}
	s.mu.Lock()
	if _, exists := s.registrations[key]; exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrDuplicateRegistration, key)
	}
	if s.started && !s.running {
		s.mu.Unlock()
		return nil, ErrSnapshotSinkStopped
	}
	s.registrations[key] = registration
	if s.running {
		s.startSamplerLocked(registration)
	}
	s.mu.Unlock()
	return registration, nil
}

func (r *Registration) Key() string {
	return r.key
}

func (r *Registration) Withdraw(ctx context.Context) error {
	r.withdrawOnce.Do(func() {
		r.sink.requestWithdrawal(r)
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.result
	}
}

func (s *SnapshotSink) requestWithdrawal(registration *Registration) {
	s.mu.Lock()
	if !s.started {
		delete(s.registrations, registration.key)
		s.mu.Unlock()
		registration.finish(nil)
		return
	}
	if !s.running {
		s.mu.Unlock()
		registration.finish(ErrSnapshotSinkStopped)
		return
	}
	events := s.events
	runContext := s.runContext
	s.mu.Unlock()
	select {
	case events <- withdrawEvent{registration: registration}:
	case <-runContext.Done():
		registration.finish(ErrSnapshotSinkStopped)
	}
}

func (r *Registration) finish(err error) {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return
	default:
	}
	r.result = err
	close(r.done)
	r.mu.Unlock()
}

func (s *SnapshotSink) Run(ctx context.Context) error {
	if s.reporter == nil {
		return errors.New("exporter reporter is required")
	}
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return ErrSnapshotSinkRunning
	}
	s.started = true
	s.running = true
	s.runContext = runContext
	for _, registration := range s.registrations {
		s.startSamplerLocked(registration)
	}
	s.mu.Unlock()

	states := make(map[string]*sourceRuntimeState)
	transport := transportRuntimeState{}
	controlIDs := make(map[string]struct{})
	heartbeatInterval := s.config.HeartbeatInterval
	sequence := int64(0)
	ticker := time.NewTicker(s.config.BatchWindow)
	defer ticker.Stop()
	defer s.stop(states)

	for {
		select {
		case <-runContext.Done():
			return nil
		case event := <-s.events:
			s.handleEvent(states, event)
		case <-ticker.C:
			err := s.flush(runContext, states, &transport, controlIDs, &heartbeatInterval, &sequence)
			var incompatible *IncompatibleServerError
			if errors.As(err, &incompatible) {
				return err
			}
		}
	}
}

func (s *SnapshotSink) startSamplerLocked(registration *Registration) {
	registration.mu.Lock()
	if registration.cancel != nil {
		registration.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.runContext)
	registration.cancel = cancel
	registration.mu.Unlock()
	s.samplers.Add(1)
	go func() {
		defer s.samplers.Done()
		s.sampleSource(ctx, registration)
	}()
}

func (s *SnapshotSink) sampleSource(ctx context.Context, registration *Registration) {
	failures := 0
	for {
		snapshotContext, cancel := context.WithTimeout(ctx, s.config.SnapshotTimeout)
		snapshot, err := registration.source.Snapshot(snapshotContext)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			snapshot, err = validateAndCloneSnapshot(snapshot, s.now())
		}
		select {
		case s.events <- sourceResultEvent{registration: registration, snapshot: snapshot, err: err}:
		case <-ctx.Done():
			return
		}
		waitFor := s.config.SampleInterval
		if err != nil {
			failures++
			waitFor = s.retryDelay(failures)
		} else {
			failures = 0
		}
		timer := time.NewTimer(waitFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *SnapshotSink) handleEvent(states map[string]*sourceRuntimeState, event sinkEvent) {
	switch event := event.(type) {
	case sourceResultEvent:
		s.mu.Lock()
		active := s.registrations[event.registration.key] == event.registration
		s.mu.Unlock()
		if !active {
			return
		}
		state := states[event.registration.key]
		if state == nil {
			state = &sourceRuntimeState{registration: event.registration, needsHello: true}
			states[event.registration.key] = state
		}
		if state.withdrawing {
			return
		}
		if event.err != nil {
			if !state.sourceFailed {
				s.config.Logger.Warn("exporter source degraded", "source", event.registration.key, "error", event.err)
			}
			state.sourceFailed = true
			state.healthy = false
			state.hasBaseline = false
			state.dirty = false
			state.needsHello = true
			return
		}
		if state.sourceFailed {
			s.config.Logger.Info("exporter source recovered", "source", event.registration.key)
		}
		state.sourceFailed = false
		state.healthy = true
		state.latest = event.snapshot
		state.hasLatest = true
		state.dirty = true
		if !state.hasBaseline {
			state.baseline = event.snapshot
			state.hasBaseline = true
			state.needsHello = true
		}
		if state.serverRef != nil && state.serverRef.identity.IdentityKey() != event.snapshot.Observer.IdentityKey() {
			state.withdrawals = appendObserverReference(state.withdrawals, *state.serverRef)
			state.serverRef = nil
			state.needsHello = true
		}
	case withdrawEvent:
		state := states[event.registration.key]
		if state == nil {
			state = &sourceRuntimeState{registration: event.registration}
			states[event.registration.key] = state
		}
		state.withdrawing = true
		state.healthy = false
		state.dirty = false
		event.registration.mu.Lock()
		if event.registration.cancel != nil {
			event.registration.cancel()
		}
		event.registration.mu.Unlock()
		if state.serverRef != nil {
			state.withdrawals = appendObserverReference(state.withdrawals, *state.serverRef)
			state.serverRef = nil
		}
	}
}

func appendObserverReference(values []observerReference, candidate observerReference) []observerReference {
	for _, value := range values {
		if value.identity.IdentityKey() == candidate.identity.IdentityKey() {
			return values
		}
	}
	return append(values, candidate)
}

func (s *SnapshotSink) flush(
	ctx context.Context,
	states map[string]*sourceRuntimeState,
	transport *transportRuntimeState,
	controlIDs map[string]struct{},
	heartbeatInterval *time.Duration,
	sequence *int64,
) error {
	now := s.now()
	s.finalizeUnreportedWithdrawals(states)
	if !transport.nextAttempt.IsZero() && now.Before(transport.nextAttempt) {
		return nil
	}
	if !transport.capabilitiesReady {
		capabilities, err := s.reporter.Capabilities(ctx)
		if err != nil {
			return s.transportFailed(states, transport, now, err)
		}
		if !capabilities.SupportsProtocol(ProtocolVersion) {
			return &IncompatibleServerError{Reason: fmt.Sprintf("observer protocol %d is not supported", ProtocolVersion)}
		}
		for _, feature := range []string{FeatureMultiObserver, FeatureObserverWithdrawal} {
			if !capabilities.SupportsFeature(feature) {
				return &IncompatibleServerError{Reason: fmt.Sprintf("required feature %q is unavailable", feature)}
			}
		}
		transport.capabilitiesReady = true
		transport.nextAttempt = time.Time{}
	}

	for _, kind := range []ReportKind{
		ReportObserverWithdrawal, ReportObserverHello, ReportInventoryUpdate,
		ReportTrafficSample, ReportObserverHeartbeat,
	} {
		operations := s.operationsForKind(states, kind, controlIDs, *heartbeatInterval, now)
		if len(operations) == 0 {
			continue
		}
		resync, err := s.sendOperations(ctx, operations, transport, controlIDs, heartbeatInterval, sequence, now)
		if err != nil {
			return s.transportFailed(states, transport, now, err)
		}
		if resync {
			s.markDisconnected(states)
			return nil
		}
		s.finalizeUnreportedWithdrawals(states)
	}
	return nil
}

func (s *SnapshotSink) operationsForKind(
	states map[string]*sourceRuntimeState,
	kind ReportKind,
	controlIDs map[string]struct{},
	heartbeatInterval time.Duration,
	now time.Time,
) []observerOperation {
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	operations := make([]observerOperation, 0, len(keys))
	for _, key := range keys {
		state := states[key]
		switch kind {
		case ReportObserverWithdrawal:
			if len(state.withdrawals) == 0 {
				continue
			}
			reference := state.withdrawals[0]
			operations = append(operations, observerOperation{
				state: state, kind: kind, collectedAt: now, reference: reference,
				observer: ObserverReport{Observer: cloneIdentity(reference.identity), InventoryGeneration: reference.generation},
			})
		case ReportObserverHello:
			if state.withdrawing || len(state.withdrawals) != 0 || !state.healthy || !state.hasLatest || !state.needsHello {
				continue
			}
			generation := snapshotInventoryHash(state.latest)
			operations = append(operations, snapshotOperation(state, kind, generation, baselineSnapshotPeers(state.latest)))
		case ReportInventoryUpdate:
			if state.withdrawing || len(state.withdrawals) != 0 || !state.healthy || !state.hasLatest ||
				state.needsHello || state.serverRef == nil || !state.dirty {
				continue
			}
			generation := snapshotInventoryHash(state.latest)
			if generation != state.serverRef.generation {
				operations = append(operations, snapshotOperation(state, kind, generation, baselineSnapshotPeers(state.latest)))
			}
		case ReportTrafficSample:
			if state.withdrawing || len(state.withdrawals) != 0 || !state.healthy || !state.hasLatest ||
				state.needsHello || state.serverRef == nil || !state.dirty {
				continue
			}
			generation := snapshotInventoryHash(state.latest)
			if generation != state.serverRef.generation {
				continue
			}
			peers := changedSnapshotPeers(state.baseline, state.latest, controlIDs, s.config.SampleInterval)
			if len(peers) == 0 {
				state.baseline = state.latest
				state.hasBaseline = true
				state.dirty = false
				continue
			}
			operations = append(operations, snapshotOperation(state, kind, generation, peers))
		case ReportObserverHeartbeat:
			if state.withdrawing || len(state.withdrawals) != 0 || !state.healthy || !state.hasLatest ||
				state.needsHello || state.serverRef == nil || state.dirty {
				continue
			}
			elapsed := now.Sub(state.lastReportAt)
			if elapsed >= 0 && elapsed < heartbeatInterval {
				continue
			}
			operations = append(operations, snapshotOperation(state, kind, state.serverRef.generation, nil))
		}
	}
	return operations
}

func snapshotOperation(state *sourceRuntimeState, kind ReportKind, generation string, peers []PeerObservation) observerOperation {
	return observerOperation{
		state: state, kind: kind, collectedAt: state.latest.CollectedAt,
		reference: observerReference{identity: cloneIdentity(state.latest.Observer), generation: generation},
		observer: ObserverReport{
			Observer: cloneIdentity(state.latest.Observer), InventoryGeneration: generation, Peers: peers,
		},
		snapshot: cloneSnapshot(state.latest),
	}
}

func (s *SnapshotSink) sendOperations(
	ctx context.Context,
	operations []observerOperation,
	transport *transportRuntimeState,
	controlIDs map[string]struct{},
	heartbeatInterval *time.Duration,
	sequence *int64,
	reportedAt time.Time,
) (bool, error) {
	for len(operations) > 0 {
		count := s.batchCount(operations, *sequence+1)
		if count == 0 {
			s.rejectOversized(operations[0])
			operations = operations[1:]
			continue
		}
		resync, err := s.sendBatch(ctx, operations[:count], transport, controlIDs, heartbeatInterval, sequence, reportedAt)
		if err != nil {
			return false, err
		}
		if resync {
			return true, nil
		}
		operations = operations[count:]
	}
	return false, nil
}

func (s *SnapshotSink) batchCount(operations []observerOperation, sequence int64) int {
	limit := min(len(operations), s.config.MaxBatchObservers)
	count := 0
	for index := 1; index <= limit; index++ {
		report := s.reportFor(operations[:index], sequence, "00000000-0000-4000-8000-000000000000")
		payload, err := json.Marshal(report)
		if err != nil || len(payload) > s.config.MaxRequestBytes {
			break
		}
		count = index
	}
	return count
}

func (s *SnapshotSink) sendBatch(
	ctx context.Context,
	operations []observerOperation,
	transport *transportRuntimeState,
	controlIDs map[string]struct{},
	heartbeatInterval *time.Duration,
	sequence *int64,
	reportedAt time.Time,
) (bool, error) {
	*sequence++
	report := s.reportFor(operations, *sequence, newExporterUUID())
	receipt, err := s.reporter.Send(ctx, report)
	if err != nil {
		var status *HTTPStatusError
		if errors.As(err, &status) && (status.StatusCode == 400 || status.StatusCode == 413) {
			if len(operations) > 1 {
				middle := len(operations) / 2
				resync, leftErr := s.sendBatch(ctx, operations[:middle], transport, controlIDs, heartbeatInterval, sequence, reportedAt)
				if leftErr != nil || resync {
					return resync, leftErr
				}
				return s.sendBatch(ctx, operations[middle:], transport, controlIDs, heartbeatInterval, sequence, reportedAt)
			}
			s.rejectOperation(operations[0], err)
			return false, nil
		}
		return false, err
	}
	if !receipt.Accepted {
		return false, errors.New("Tailpath server rejected exporter report")
	}
	s.acceptReceipt(receipt, controlIDs, heartbeatInterval)
	for _, operation := range operations {
		s.applyOperation(operation, reportedAt)
	}
	if report.Kind == ReportObserverHello {
		if transport.degraded {
			s.config.Logger.Info("exporter transport recovered", "failures", transport.failures)
		}
		transport.failures = 0
		transport.nextAttempt = time.Time{}
		transport.degraded = false
	}
	return receipt.ResyncRequired && report.Kind != ReportObserverHello, nil
}

func (s *SnapshotSink) reportFor(operations []observerOperation, sequence int64, reportID string) ReportEnvelope {
	report := ReportEnvelope{
		Version: ProtocolVersion, ReportID: reportID, ReporterInstanceID: s.config.ReporterInstanceID,
		Sequence: sequence, Kind: operations[0].kind,
		Observers: make([]ObserverReport, len(operations)),
	}
	for index, operation := range operations {
		report.Observers[index] = operation.observer
		if operation.collectedAt.After(report.CollectedAt) {
			report.CollectedAt = operation.collectedAt
		}
	}
	return report
}

func (s *SnapshotSink) applyOperation(operation observerOperation, now time.Time) {
	state := operation.state
	switch operation.kind {
	case ReportObserverWithdrawal:
		if len(state.withdrawals) > 0 {
			state.withdrawals = state.withdrawals[1:]
		}
	case ReportObserverHello:
		reference := operation.reference
		state.serverRef = &reference
		state.needsHello = false
		state.baseline = operation.snapshot
		state.hasBaseline = true
		state.dirty = false
		state.lastReportAt = now
		state.oversized = false
	case ReportInventoryUpdate:
		if state.serverRef != nil {
			state.serverRef.generation = operation.reference.generation
		}
		state.baseline = operation.snapshot
		state.hasBaseline = true
		state.dirty = false
		state.lastReportAt = now
		state.oversized = false
	case ReportTrafficSample:
		state.baseline = operation.snapshot
		state.hasBaseline = true
		state.dirty = false
		state.lastReportAt = now
		state.oversized = false
	case ReportObserverHeartbeat:
		state.lastReportAt = now
	}
}

func (s *SnapshotSink) acceptReceipt(receipt ReportReceipt, controlIDs map[string]struct{}, heartbeatInterval *time.Duration) {
	if duration := time.Duration(receipt.HeartbeatIntervalMS) * time.Millisecond; duration >= 10*time.Second && duration <= 10*time.Minute {
		*heartbeatInterval = duration
	}
	clear(controlIDs)
	for _, stableID := range receipt.ControlStableNodeIDs {
		if stableID != "" {
			controlIDs[stableID] = struct{}{}
		}
	}
}

func (s *SnapshotSink) rejectOversized(operation observerOperation) {
	if !operation.state.oversized {
		s.config.Logger.Warn("exporter snapshot exceeds request bound", "source", operation.state.registration.key)
	}
	operation.state.oversized = true
	if operation.kind == ReportObserverWithdrawal {
		if operation.state.withdrawing {
			operation.state.withdrawErr = errors.New("observer withdrawal exceeds request bound")
		}
		operation.state.withdrawals = operation.state.withdrawals[1:]
		return
	}
	operation.state.healthy = false
	operation.state.baseline = operation.snapshot
	operation.state.hasBaseline = true
	operation.state.dirty = false
}

func (s *SnapshotSink) rejectOperation(operation observerOperation, err error) {
	s.config.Logger.Warn("exporter source report rejected", "source", operation.state.registration.key, "error", err)
	if operation.kind == ReportObserverWithdrawal {
		if operation.state.withdrawing {
			operation.state.withdrawErr = fmt.Errorf("observer withdrawal rejected: %w", err)
		}
		operation.state.withdrawals = operation.state.withdrawals[1:]
		return
	}
	operation.state.healthy = false
	operation.state.baseline = operation.snapshot
	operation.state.hasBaseline = true
	operation.state.dirty = false
}

func (s *SnapshotSink) transportFailed(
	states map[string]*sourceRuntimeState,
	transport *transportRuntimeState,
	now time.Time,
	err error,
) error {
	var incompatible *IncompatibleServerError
	if errors.As(err, &incompatible) {
		return err
	}
	transport.capabilitiesReady = false
	transport.failures++
	delay := s.retryDelay(transport.failures)
	transport.nextAttempt = now.Add(delay)
	if !transport.degraded {
		s.config.Logger.Warn("exporter transport degraded", "error", err, "retry_in", delay)
		transport.degraded = true
	}
	s.markDisconnected(states)
	return nil
}

func (s *SnapshotSink) markDisconnected(states map[string]*sourceRuntimeState) {
	for _, state := range states {
		if state.withdrawing {
			continue
		}
		state.needsHello = true
		if state.hasLatest {
			state.baseline = state.latest
			state.hasBaseline = true
			state.dirty = true
		}
	}
}

func (s *SnapshotSink) finalizeUnreportedWithdrawals(states map[string]*sourceRuntimeState) {
	for key, state := range states {
		if !state.withdrawing || len(state.withdrawals) != 0 {
			continue
		}
		s.mu.Lock()
		delete(s.registrations, key)
		s.mu.Unlock()
		state.registration.finish(state.withdrawErr)
		delete(states, key)
	}
}

func (s *SnapshotSink) stop(states map[string]*sourceRuntimeState) {
	s.mu.Lock()
	s.running = false
	for _, registration := range s.registrations {
		registration.mu.Lock()
		if registration.cancel != nil {
			registration.cancel()
		}
		registration.mu.Unlock()
	}
	s.mu.Unlock()
	s.samplers.Wait()
	for _, state := range states {
		if state.withdrawing {
			state.registration.finish(ErrSnapshotSinkStopped)
		}
	}
}

func (s *SnapshotSink) retryDelay(failures int) time.Duration {
	base := s.config.RetryMin
	for attempt := 1; attempt < failures && base < s.config.RetryMax; attempt++ {
		if base > s.config.RetryMax/2 {
			base = s.config.RetryMax
			break
		}
		base *= 2
	}
	if base > s.config.RetryMax {
		base = s.config.RetryMax
	}
	s.jitterMu.Lock()
	jitter := s.config.Jitter()
	s.jitterMu.Unlock()
	return time.Duration(float64(base) * (0.8 + 0.4*jitter))
}

func (s *SnapshotSink) now() time.Time {
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	return s.config.Now()
}

func validateAndCloneSnapshot(snapshot Snapshot, fallback time.Time) (Snapshot, error) {
	result := cloneSnapshot(snapshot)
	if result.CollectedAt.IsZero() {
		result.CollectedAt = fallback
	}
	if !result.Observer.HasIdentity() {
		return Snapshot{}, errors.New("observer identity is required")
	}
	seen := make(map[string]struct{}, len(result.Peers))
	for index := range result.Peers {
		peer := &result.Peers[index]
		if !peer.Identity.HasIdentity() {
			return Snapshot{}, fmt.Errorf("peer %d identity is required", index)
		}
		if peer.RxBytes < 0 || peer.TxBytes < 0 {
			return Snapshot{}, fmt.Errorf("peer %d counters cannot be negative", index)
		}
		key := peer.Identity.IdentityKey()
		if _, duplicate := seen[key]; duplicate {
			return Snapshot{}, fmt.Errorf("peer identity %q is duplicated", key)
		}
		seen[key] = struct{}{}
		if peer.Path.Kind == "" {
			peer.Path.Kind = PathUnknown
		}
		switch peer.Path.Kind {
		case PathDirect, PathDERP, PathPeerRelay, PathUnknown:
		default:
			return Snapshot{}, fmt.Errorf("peer %d path %q is invalid", index, peer.Path.Kind)
		}
		if peer.Path.PeerRelayVNI != nil && (*peer.Path.PeerRelayVNI < 0 || *peer.Path.PeerRelayVNI > 1<<24-1) {
			return Snapshot{}, fmt.Errorf("peer %d relay VNI is invalid", index)
		}
	}
	return result, nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	result := Snapshot{CollectedAt: snapshot.CollectedAt, Observer: cloneIdentity(snapshot.Observer)}
	result.Peers = make([]PeerSnapshot, len(snapshot.Peers))
	for index, peer := range snapshot.Peers {
		result.Peers[index] = PeerSnapshot{
			Identity: cloneIdentity(peer.Identity), RxBytes: peer.RxBytes, TxBytes: peer.TxBytes,
			Path: clonePath(peer.Path),
		}
	}
	return result
}

func cloneIdentity(identity NodeIdentity) NodeIdentity {
	identity.TailscaleIPs = append([]string(nil), identity.TailscaleIPs...)
	return identity
}

func clonePath(path Path) Path {
	if path.PeerRelayVNI != nil {
		value := *path.PeerRelayVNI
		path.PeerRelayVNI = &value
	}
	return path
}

func snapshotInventoryHash(snapshot Snapshot) string {
	identities := make([]NodeIdentity, 0, len(snapshot.Peers)+1)
	identities = append(identities, cloneIdentity(snapshot.Observer))
	for _, peer := range snapshot.Peers {
		identities = append(identities, cloneIdentity(peer.Identity))
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

func baselineSnapshotPeers(snapshot Snapshot) []PeerObservation {
	peers := make([]PeerObservation, 0, len(snapshot.Peers))
	for _, peer := range snapshot.Peers {
		peers = append(peers, PeerObservation{
			Peer: cloneIdentity(peer.Identity), RxBytes: peer.RxBytes, TxBytes: peer.TxBytes,
			Path: clonePath(peer.Path), LastActive: snapshot.CollectedAt,
		})
	}
	return peers
}

func changedSnapshotPeers(previous, current Snapshot, controlIDs map[string]struct{}, fallback time.Duration) []PeerObservation {
	oldPeers := make(map[string]PeerSnapshot, len(previous.Peers))
	for _, peer := range previous.Peers {
		oldPeers[peer.Identity.IdentityKey()] = peer
	}
	duration := current.CollectedAt.Sub(previous.CollectedAt)
	if duration <= 0 {
		duration = fallback
	}
	var changed []PeerObservation
	for _, peer := range current.Peers {
		if _, control := controlIDs[peer.Identity.StableNodeID]; control {
			continue
		}
		old, ok := oldPeers[peer.Identity.IdentityKey()]
		if !ok {
			continue
		}
		rxDelta := snapshotCounterDelta(old.RxBytes, peer.RxBytes)
		txDelta := snapshotCounterDelta(old.TxBytes, peer.TxBytes)
		if rxDelta == 0 && txDelta == 0 {
			continue
		}
		changed = append(changed, PeerObservation{
			Peer: cloneIdentity(peer.Identity), RxBytes: peer.RxBytes, TxBytes: peer.TxBytes,
			RxDelta: rxDelta, TxDelta: txDelta, SampleDurationMS: max(duration.Milliseconds(), 1),
			Path: clonePath(peer.Path), LastActive: current.CollectedAt,
		})
	}
	return changed
}

func snapshotCounterDelta(previous, current int64) int64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func newExporterUUID() string {
	var value [16]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
