package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func (s *SnapshotSink) sampleRelaySource(ctx context.Context, registration *Registration, source RelaySource) {
	failures := 0
	for {
		snapshotContext, cancel := context.WithTimeout(ctx, s.config.SnapshotTimeout)
		snapshot, err := source.PeerRelaySnapshot(snapshotContext)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			snapshot, err = validateAndCloneRelaySnapshot(snapshot, s.now())
		}
		select {
		case s.events <- relayResultEvent{registration: registration, snapshot: snapshot, err: err}:
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

func (s *SnapshotSink) relayOperations(states map[string]*sourceRuntimeState) []relayOperation {
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	operations := make([]relayOperation, 0, len(keys))
	for _, key := range keys {
		state := states[key]
		if state.withdrawing || !state.healthy || state.needsHello || state.serverRef == nil ||
			!state.relayHealthy || !state.relayHasLatest || !state.relayHasBaseline || !state.relayDirty {
			continue
		}
		sessions, err := changedRelaySessions(state.relayBaseline, state.relayLatest, s.config.SampleInterval)
		if err != nil {
			s.config.Logger.Warn("relay snapshot rejected", "source", state.registration.key, "error", err)
			state.relayBaseline = state.relayLatest
			state.relayDirty = false
			continue
		}
		if len(sessions) == 0 {
			state.relayBaseline = state.relayLatest
			state.relayDirty = false
			continue
		}
		for index := range sessions {
			sessions[index].Relay = cloneIdentity(state.latest.Observer)
		}
		operations = append(operations, relayOperation{
			state: state, collectedAt: state.relayLatest.CollectedAt,
			snapshot: cloneRelaySnapshot(state.relayLatest), sessions: sessions,
		})
	}
	return operations
}

func (s *SnapshotSink) sendRelayOperations(
	ctx context.Context,
	operations []relayOperation,
	transport *transportRuntimeState,
	controlIDs map[string]struct{},
	heartbeatInterval *time.Duration,
	sequence *int64,
) (bool, error) {
	for _, operation := range operations {
		*sequence++
		report := ReportEnvelope{
			Version: ProtocolVersion, ReportID: newExporterUUID(), ReporterInstanceID: s.config.ReporterInstanceID,
			Sequence: *sequence, CollectedAt: operation.collectedAt, Kind: ReportRelaySessionUpdate,
			RelaySessions: operation.sessions,
		}
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil || len(payload) > s.config.MaxRequestBytes {
			s.config.Logger.Warn("relay snapshot exceeds request bound", "source", operation.state.registration.key)
			operation.state.relayBaseline = operation.snapshot
			operation.state.relayDirty = false
			continue
		}
		receipt, err := s.reporter.Send(ctx, report)
		if err != nil {
			var status *HTTPStatusError
			if errors.As(err, &status) && (status.StatusCode == 400 || status.StatusCode == 413) {
				s.config.Logger.Warn("relay snapshot rejected", "source", operation.state.registration.key,
					"status", status.StatusCode)
				operation.state.relayBaseline = operation.snapshot
				operation.state.relayDirty = false
				continue
			}
			return false, err
		}
		if !receipt.Accepted {
			return false, errors.New("Tailpath server rejected relay report")
		}
		s.acceptReceipt(receipt, controlIDs, heartbeatInterval)
		operation.state.relayBaseline = operation.snapshot
		operation.state.relayHasBaseline = true
		operation.state.relayDirty = false
		if receipt.ResyncRequired {
			return true, nil
		}
	}
	return false, nil
}

func (s *SnapshotSink) resetRelayBaseline(state *sourceRuntimeState) {
	state.relayDirty = false
	if state.relayHasLatest && state.relayHealthy {
		state.relayBaseline = state.relayLatest
		state.relayHasBaseline = true
		return
	}
	state.relayHasBaseline = false
}

func (s *SnapshotSink) logRelayStateTransition(key string, state *sourceRuntimeState, snapshot RelaySnapshot) {
	if snapshot.Capability != state.relayCapability {
		s.config.Logger.Info("relay telemetry capability", "source", key, "capability", snapshot.Capability)
	}
	if snapshot.IdentityEvidence == "" || snapshot.IdentityEvidence == state.relayIdentityEvidence {
		return
	}
	if snapshot.IdentityEvidence == RelayIdentityDegraded {
		s.config.Logger.Warn("relay identity enrichment degraded", "source", key)
		return
	}
	if state.relayIdentityEvidence == RelayIdentityDegraded {
		s.config.Logger.Info("relay identity enrichment recovered", "source", key)
	}
}

func validateAndCloneRelaySnapshot(snapshot RelaySnapshot, fallback time.Time) (RelaySnapshot, error) {
	result := cloneRelaySnapshot(snapshot)
	if result.CollectedAt.IsZero() {
		result.CollectedAt = fallback
	}
	switch result.Capability {
	case RelayOff, RelayUnsupported, RelayDisabled, RelayEnabled, RelayTransientFailure:
	default:
		return RelaySnapshot{}, fmt.Errorf("relay capability %q is invalid", result.Capability)
	}
	if result.IdentityEvidence != "" && result.IdentityEvidence != RelayIdentityAvailable &&
		result.IdentityEvidence != RelayIdentityDegraded {
		return RelaySnapshot{}, fmt.Errorf("relay identity evidence %q is invalid", result.IdentityEvidence)
	}
	if result.Capability != RelayEnabled && len(result.Sessions) != 0 {
		return RelaySnapshot{}, errors.New("relay sessions require enabled capability")
	}
	seen := make(map[string]struct{}, len(result.Sessions))
	for index, session := range result.Sessions {
		if strings.TrimSpace(session.SessionID) == "" {
			return RelaySnapshot{}, fmt.Errorf("relay session %d ID is required", index)
		}
		if session.VNI < 0 || session.VNI > 1<<24-1 {
			return RelaySnapshot{}, fmt.Errorf("relay session %d VNI is invalid", index)
		}
		key := relaySessionKey(session)
		if _, duplicate := seen[key]; duplicate {
			return RelaySnapshot{}, fmt.Errorf("relay session %d is duplicated", index)
		}
		seen[key] = struct{}{}
		for clientIndex, client := range []RelayClientSnapshot{session.Source, session.Target} {
			if strings.TrimSpace(client.SessionClientID) == "" {
				return RelaySnapshot{}, fmt.Errorf("relay session %d client %d ID is required", index, clientIndex)
			}
			if client.Identity != nil && !client.Identity.HasIdentity() {
				return RelaySnapshot{}, fmt.Errorf("relay session %d client %d identity is invalid", index, clientIndex)
			}
		}
	}
	return result, nil
}

func cloneRelaySnapshot(snapshot RelaySnapshot) RelaySnapshot {
	result := RelaySnapshot{
		CollectedAt: snapshot.CollectedAt, Capability: snapshot.Capability,
		IdentityEvidence: snapshot.IdentityEvidence,
		Sessions:         make([]RelaySessionSnapshot, len(snapshot.Sessions)),
	}
	for index, session := range snapshot.Sessions {
		result.Sessions[index] = RelaySessionSnapshot{
			SessionID: session.SessionID, VNI: session.VNI,
			Source: cloneRelayClientSnapshot(session.Source), Target: cloneRelayClientSnapshot(session.Target),
		}
	}
	return result
}

func cloneRelayClientSnapshot(client RelayClientSnapshot) RelayClientSnapshot {
	if client.Identity != nil {
		identity := cloneIdentity(*client.Identity)
		client.Identity = &identity
	}
	return client
}

func changedRelaySessions(previous, current RelaySnapshot, fallback time.Duration) ([]RelaySessionObservation, error) {
	oldSessions := make(map[string]RelaySessionSnapshot, len(previous.Sessions))
	for _, session := range previous.Sessions {
		oldSessions[relaySessionKey(session)] = session
	}
	duration := current.CollectedAt.Sub(previous.CollectedAt)
	if duration <= 0 {
		duration = fallback
	}
	result := make([]RelaySessionObservation, 0, len(current.Sessions))
	for _, session := range current.Sessions {
		old, ok := oldSessions[relaySessionKey(session)]
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
		sourceBytes, err := relayCounterValue(session.Source.BytesSent)
		if err != nil {
			return nil, err
		}
		targetBytes, err := relayCounterValue(session.Target.BytesSent)
		if err != nil {
			return nil, err
		}
		sourceDeltaValue, err := relayCounterValue(sourceDelta)
		if err != nil {
			return nil, err
		}
		targetDeltaValue, err := relayCounterValue(targetDelta)
		if err != nil {
			return nil, err
		}
		result = append(result, RelaySessionObservation{
			Source: relayObservationClient(session.Source), Target: relayObservationClient(session.Target),
			SessionID: session.SessionID, VNI: session.VNI,
			SourceToTargetBytes: sourceBytes, TargetToSourceBytes: targetBytes,
			SourceToTargetDelta: sourceDeltaValue, TargetToSourceDelta: targetDeltaValue,
			SampleDurationMS: max(duration.Milliseconds(), 1), LastActive: current.CollectedAt,
		})
	}
	return result, nil
}

func relaySessionKey(session RelaySessionSnapshot) string {
	return fmt.Sprintf("%d:%s", session.VNI, session.SessionID)
}

func relayCounterValue(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, errors.New("relay traffic counter exceeds protocol range")
	}
	return int64(value), nil
}

func relayObservationClient(client RelayClientSnapshot) RelaySessionClient {
	result := RelaySessionClient{
		SessionClientID: client.SessionClientID, DiscoShort: client.DiscoShort, Endpoint: client.Endpoint,
	}
	if client.Identity != nil {
		identity := cloneIdentity(*client.Identity)
		result.Identity = &identity
	}
	return result
}
