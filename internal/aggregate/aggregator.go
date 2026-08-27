package aggregate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

const (
	activeWindow       = 10 * time.Second
	reportIDWindowSize = 32
)

type Options struct {
	HeartbeatInterval time.Duration
	ControlNodeIDs    []string
	Now               func() time.Time
	NewNodeID         func() string
}

type Aggregator struct {
	mu sync.RWMutex

	heartbeatInterval time.Duration
	evidenceWindow    time.Duration
	nodeWindow        time.Duration
	now               func() time.Time
	newNodeID         func() string
	controlIDs        []string
	state             runtimeState
	subscribers       map[chan struct{}]struct{}
}

type runtimeState struct {
	Reporters     map[string]*reporterState        `json:"reporters"`
	Observers     map[string]*observerRuntimeState `json:"observers,omitempty"`
	Nodes         map[string]*nodeState            `json:"nodes"`
	Aliases       map[string]string                `json:"aliases"`
	AliasLastSeen map[string]time.Time             `json:"aliasLastSeen,omitempty"`
	Redirects     map[string]string                `json:"redirects,omitempty"`
	Edges         map[string]*edgeState            `json:"edges"`
	RelayScopes   map[string]*relayScopeState      `json:"relayScopes,omitempty"`
}

type reporterState struct {
	LastSequence int64               `json:"lastSequence"`
	ReportIDs    map[string]struct{} `json:"reportIds"`
	ObserverIDs  map[string]struct{} `json:"observerIds"`

	// These fields are decoded only to migrate runtime state written before
	// inventory ownership moved to observerRuntimeState.
	LegacyInventories map[string]string              `json:"inventories,omitempty"`
	LegacyMemberships map[string]map[string]struct{} `json:"memberships,omitempty"`
}

type observerRuntimeState struct {
	OwnerReporterInstanceID string              `json:"ownerReporterInstanceId,omitempty"`
	InventoryGeneration     string              `json:"inventoryGeneration,omitempty"`
	Membership              map[string]struct{} `json:"membership,omitempty"`
}

type nodeState struct {
	Identity       domain.NodeIdentity   `json:"identity"`
	IdentityStatus domain.IdentityStatus `json:"identityStatus,omitempty"`
	Observable     bool                  `json:"observable"`
	LastEvidence   time.Time             `json:"lastEvidence"`
	LastReport     time.Time             `json:"lastReport"`
	LastCollected  time.Time             `json:"lastCollected"`
	ClockSkewMS    int64                 `json:"clockSkewMs"`
	ClockSkewed    bool                  `json:"clockSkewed"`
}

type relayScopeState struct {
	RelayID            string                        `json:"relayId"`
	VNI                int64                         `json:"vni"`
	PairSourceID       string                        `json:"pairSourceId,omitempty"`
	PairTargetID       string                        `json:"pairTargetId,omitempty"`
	PairObservedAt     time.Time                     `json:"pairObservedAt,omitempty"`
	ConflictObservedAt time.Time                     `json:"conflictObservedAt,omitempty"`
	LastSeen           time.Time                     `json:"lastSeen"`
	Sessions           map[string]*relaySessionState `json:"sessions,omitempty"`
}

type relaySessionState struct {
	Clients        map[string]string     `json:"clients,omitempty"`
	SourceClientID string                `json:"sourceClientId,omitempty"`
	TargetClientID string                `json:"targetClientId,omitempty"`
	SourceNodeID   string                `json:"sourceNodeId,omitempty"`
	TargetNodeID   string                `json:"targetNodeId,omitempty"`
	SourceStatus   domain.IdentityStatus `json:"sourceStatus,omitempty"`
	TargetStatus   domain.IdentityStatus `json:"targetStatus,omitempty"`
	LastSeen       time.Time             `json:"lastSeen"`
}

type edgeState struct {
	ID              string                     `json:"id"`
	Source          string                     `json:"source"`
	Target          string                     `json:"target"`
	SystemTelemetry bool                       `json:"systemTelemetry,omitempty"`
	LastActive      time.Time                  `json:"lastActive"`
	LastKnownPath   domain.PathObservation     `json:"lastKnownPath"`
	Observations    map[string]edgeObservation `json:"observations"`
}

type edgeObservation struct {
	ObserverID     string                         `json:"observerId"`
	Path           domain.PathObservation         `json:"path"`
	CollectedAt    time.Time                      `json:"collectedAt"`
	ReceivedAt     time.Time                      `json:"receivedAt"`
	ClockSkewed    bool                           `json:"clockSkewed"`
	RelaySession   *domain.RelaySessionProvenance `json:"relaySession,omitempty"`
	SourceEndpoint string                         `json:"-"`
	TargetEndpoint string                         `json:"-"`
	TxRate         float64                        `json:"txRate"`
	RxRate         float64                        `json:"rxRate"`
	AToBRate       float64                        `json:"aToBRate,omitempty"`
	BToARate       float64                        `json:"bToARate,omitempty"`
}

type ApplyResult struct {
	Receipt               domain.ReportReceipt
	Traffic               []domain.AcceptedTraffic
	PathTransitions       []domain.PathTransition
	Changed               bool
	CanonicalStateChanged bool
}

func New(options Options) *Aggregator {
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewNodeID == nil {
		options.NewNodeID = randomNodeID
	}
	return &Aggregator{
		heartbeatInterval: options.HeartbeatInterval,
		evidenceWindow:    2 * options.HeartbeatInterval,
		nodeWindow:        4 * options.HeartbeatInterval,
		now:               options.Now,
		newNodeID:         options.NewNodeID,
		controlIDs:        append([]string(nil), options.ControlNodeIDs...),
		state:             newRuntimeState(),
		subscribers:       make(map[chan struct{}]struct{}),
	}
}

func newRuntimeState() runtimeState {
	return runtimeState{
		Reporters:     make(map[string]*reporterState),
		Observers:     make(map[string]*observerRuntimeState),
		Nodes:         make(map[string]*nodeState),
		Aliases:       make(map[string]string),
		AliasLastSeen: make(map[string]time.Time),
		Redirects:     make(map[string]string),
		Edges:         make(map[string]*edgeState),
		RelayScopes:   make(map[string]*relayScopeState),
	}
}

func (a *Aggregator) Apply(report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	result, err := a.ApplyAt(report, a.now())
	return result.Receipt, err
}

func (a *Aggregator) ApplyAt(report domain.ReportEnvelope, receivedAt time.Time) (ApplyResult, error) {
	if err := report.Validate(); err != nil {
		return ApplyResult{}, err
	}
	if receivedAt.IsZero() {
		receivedAt = a.now()
	}
	receivedAt = receivedAt.UTC()

	a.mu.Lock()
	defer a.mu.Unlock()
	result, err := a.applyLocked(report, receivedAt)
	if err == nil && result.Changed {
		a.notifyLocked()
	}
	return result, err
}

func (a *Aggregator) applyLocked(report domain.ReportEnvelope, receivedAt time.Time) (ApplyResult, error) {
	a.pruneIndexesLocked(receivedAt)
	reporter := a.state.Reporters[report.ReporterInstanceID]
	newReporter := reporter == nil
	if reporter == nil {
		reporter = &reporterState{
			ReportIDs:   make(map[string]struct{}),
			ObserverIDs: make(map[string]struct{}),
		}
		a.state.Reporters[report.ReporterInstanceID] = reporter
	}
	result := ApplyResult{Receipt: domain.ReportReceipt{
		Accepted:             true,
		ControlStableNodeIDs: append([]string(nil), a.controlIDs...),
		HeartbeatIntervalMS:  a.heartbeatInterval.Milliseconds(),
	}}
	resolveIdentity := func(identity domain.NodeIdentity) (string, bool) {
		nodeID, created, canonicalChanged := a.resolveIdentityLocked(identity, receivedAt)
		result.CanonicalStateChanged = result.CanonicalStateChanged || canonicalChanged
		return nodeID, created
	}
	if _, duplicate := reporter.ReportIDs[report.ReportID]; duplicate {
		return result, nil
	}
	if report.Sequence <= reporter.LastSequence {
		result.Receipt.Accepted = false
		return result, nil
	}
	if reporter.LastSequence > 0 && report.Sequence != reporter.LastSequence+1 {
		result.Receipt.ResyncRequired = true
	}
	if report.Kind != domain.ReportObserverHello && report.Kind != domain.ReportRelaySessionUpdate &&
		!a.reporterOwnsObserversLocked(report.ReporterInstanceID, report.Observers, receivedAt) {
		if newReporter {
			delete(a.state.Reporters, report.ReporterInstanceID)
		}
		result.Receipt.Accepted = false
		result.Receipt.ResyncRequired = true
		return result, nil
	}

	touchedEdges := make(map[string]domain.PathObservation)
	for _, observation := range report.Observers {
		observerID, _ := resolveIdentity(observation.Observer)
		if report.Kind == domain.ReportObserverHello {
			a.claimReporterObserverLocked(report.ReporterInstanceID, reporter, observerID)
		}
		a.touchObserverLocked(observerID, report.CollectedAt, receivedAt)
		observerState := a.state.Observers[observerID]

		switch report.Kind {
		case domain.ReportObserverHello, domain.ReportInventoryUpdate:
			members := make(map[string]struct{}, len(observation.Peers))
			for _, peer := range observation.Peers {
				peerID, created := resolveIdentity(peer.Peer)
				if created {
					a.state.Nodes[peerID].LastEvidence = receivedAt
				}
				members[peerID] = struct{}{}
			}
			a.replaceInventoryLocked(observerState, observerID, observation.InventoryGeneration, members)
		case domain.ReportTrafficSample:
			if observerState.InventoryGeneration != observation.InventoryGeneration {
				result.Receipt.ResyncRequired = true
			}
			for _, peer := range observation.Peers {
				if peer.RxDelta == 0 && peer.TxDelta == 0 {
					continue
				}
				peerID, _ := resolveIdentity(peer.Peer)
				a.touchPeerLocked(peerID, receivedAt)
				edgeID, source, target := domain.EdgeID(observerID, peerID)
				if _, seen := touchedEdges[edgeID]; !seen {
					if edge := a.state.Edges[edgeID]; edge != nil {
						touchedEdges[edgeID] = edge.LastKnownPath
					} else {
						touchedEdges[edgeID] = domain.PathObservation{}
					}
				}
				if peer.Path.Kind == domain.PathPeerRelay && peer.Path.PeerRelayVNI != nil &&
					strings.TrimSpace(peer.Path.PeerRelayStableNodeID) != "" {
					relayID, _, canonicalChanged := a.resolveIdentityLocked(domain.NodeIdentity{
						StableNodeID: peer.Path.PeerRelayStableNodeID,
					}, receivedAt)
					result.CanonicalStateChanged = result.CanonicalStateChanged || canonicalChanged
					if a.recordRelayPairLocked(relayID, *peer.Path.PeerRelayVNI, observerID, peerID, receivedAt) {
						result.CanonicalStateChanged = true
					}
				}
				a.applyPeerLocked(report.CollectedAt, receivedAt, observerID, peerID, peer)
				aToBBytes, bToABytes := peer.TxDelta, peer.RxDelta
				if observerID != source {
					aToBBytes, bToABytes = peer.RxDelta, peer.TxDelta
				}
				result.Traffic = append(result.Traffic, domain.AcceptedTraffic{
					EdgeID: edgeID, SourceID: source, TargetID: target, ObserverID: observerID,
					AToBBytes: aToBBytes, BToABytes: bToABytes, ReceivedAt: receivedAt,
				})
			}
		case domain.ReportObserverHeartbeat:
			if observerState.InventoryGeneration != observation.InventoryGeneration {
				result.Receipt.ResyncRequired = true
			}
		}
	}
	if report.Kind == domain.ReportRelaySessionUpdate {
		for _, session := range report.RelaySessions {
			relayID, _ := resolveIdentity(session.Relay)
			a.touchObserverLocked(relayID, report.CollectedAt, receivedAt)
			a.claimReporterObserverLocked(report.ReporterInstanceID, reporter, relayID)
			sourceID, targetID, sourceStatus, targetStatus, canonicalChanged :=
				a.resolveRelaySessionLocked(relayID, session, receivedAt)
			result.CanonicalStateChanged = result.CanonicalStateChanged || canonicalChanged
			if sourceID == targetID {
				continue
			}
			systemTelemetry := a.isControlNodeLocked(sourceID) || a.isControlNodeLocked(targetID)
			a.touchPeerLocked(sourceID, receivedAt)
			a.touchPeerLocked(targetID, receivedAt)
			edgeID, source, target := domain.EdgeID(sourceID, targetID)
			if _, seen := touchedEdges[edgeID]; !seen {
				if edge := a.state.Edges[edgeID]; edge != nil {
					touchedEdges[edgeID] = edge.LastKnownPath
				} else {
					touchedEdges[edgeID] = domain.PathObservation{}
				}
			}
			aToBBytes, bToABytes := session.SourceToTargetDelta, session.TargetToSourceDelta
			if sourceID != source {
				aToBBytes, bToABytes = session.TargetToSourceDelta, session.SourceToTargetDelta
				sourceStatus, targetStatus = targetStatus, sourceStatus
			}
			a.applyRelaySessionLocked(
				report.CollectedAt, receivedAt, relayID, source, target, session,
				sourceStatus, targetStatus, aToBBytes, bToABytes,
			)
			if systemTelemetry {
				a.state.Edges[edgeID].SystemTelemetry = true
			}
			result.Traffic = append(result.Traffic, domain.AcceptedTraffic{
				EdgeID: edgeID, SourceID: source, TargetID: target, ObserverID: relayID,
				AToBBytes: aToBBytes, BToABytes: bToABytes, ReceivedAt: receivedAt,
			})
		}
	}

	for edgeID, previous := range touchedEdges {
		edge := a.state.Edges[edgeID]
		current := a.snapshotEdgeLocked(edge, receivedAt)
		if current.Path.Kind == "" {
			current.Path.Kind = domain.PathUnknown
		}
		if previous.Kind == "" || !equivalentPath(previous, current.Path) {
			result.PathTransitions = append(result.PathTransitions, domain.PathTransition{
				EdgeID: edgeID, ObservedAt: receivedAt, Path: current.Path,
				Observations: append([]domain.ObservationProvenance(nil), current.Observations...),
			})
		}
		if previous.Kind == "" || !equivalentPath(previous, current.Path) ||
			pathSpecificity(current.Path) >= pathSpecificity(previous) {
			edge.LastKnownPath = current.Path
		}
	}

	reporter.LastSequence = report.Sequence
	reporter.ReportIDs[report.ReportID] = struct{}{}
	if len(reporter.ReportIDs) > reportIDWindowSize {
		reporter.ReportIDs = map[string]struct{}{report.ReportID: {}}
	}
	result.Changed = true
	return result, nil
}

func (a *Aggregator) isControlNodeLocked(nodeID string) bool {
	node := a.state.Nodes[nodeID]
	if node == nil || node.Identity.StableNodeID == "" {
		return false
	}
	for _, controlID := range a.controlIDs {
		if node.Identity.StableNodeID == controlID {
			return true
		}
	}
	return false
}

func (a *Aggregator) replaceInventoryLocked(observer *observerRuntimeState, observerID, generation string, members map[string]struct{}) {
	for peerID := range observer.Membership {
		if _, stillVisible := members[peerID]; stillVisible {
			continue
		}
		edgeID, _, _ := domain.EdgeID(observerID, peerID)
		if edge := a.state.Edges[edgeID]; edge != nil {
			delete(edge.Observations, observerID)
		}
	}
	observer.InventoryGeneration = generation
	observer.Membership = members
}

func (a *Aggregator) applyPeerLocked(collectedAt, receivedAt time.Time, observerID, peerID string, peer domain.PeerObservation) {
	edgeID, source, target := domain.EdgeID(observerID, peerID)
	edge := a.state.Edges[edgeID]
	if edge == nil {
		edge = &edgeState{
			ID: edgeID, Source: source, Target: target,
			Observations: make(map[string]edgeObservation),
		}
		a.state.Edges[edgeID] = edge
	}
	duration := float64(peer.SampleDurationMS) / 1000
	edge.Observations[observerID] = edgeObservation{
		ObserverID: observerID, Path: peer.Path, CollectedAt: collectedAt, ReceivedAt: receivedAt,
		ClockSkewed: a.isClockSkewed(collectedAt, receivedAt),
		TxRate:      float64(peer.TxDelta) / duration,
		RxRate:      float64(peer.RxDelta) / duration,
	}
	if receivedAt.After(edge.LastActive) {
		edge.LastActive = receivedAt
	}
}

func (a *Aggregator) applyRelaySessionLocked(
	collectedAt, receivedAt time.Time,
	relayID, sourceID, targetID string,
	session domain.RelaySessionObservation,
	sourceStatus, targetStatus domain.IdentityStatus,
	aToBBytes, bToABytes int64,
) {
	edgeID, source, target := domain.EdgeID(sourceID, targetID)
	edge := a.state.Edges[edgeID]
	if edge == nil {
		edge = &edgeState{
			ID: edgeID, Source: source, Target: target,
			Observations: make(map[string]edgeObservation),
		}
		a.state.Edges[edgeID] = edge
	}
	duration := float64(session.SampleDurationMS) / 1000
	vni := session.VNI
	path := domain.PathObservation{
		Kind: domain.PathPeerRelay, PeerRelayStableNodeID: session.Relay.StableNodeID,
		PeerRelayVNI: &vni,
	}
	observation := edgeObservation{
		ObserverID: relayID, Path: path, CollectedAt: collectedAt, ReceivedAt: receivedAt,
		ClockSkewed: a.isClockSkewed(collectedAt, receivedAt),
		RelaySession: &domain.RelaySessionProvenance{
			SessionID: session.SessionID, VNI: session.VNI,
			SourceIdentityStatus: sourceStatus,
			TargetIdentityStatus: targetStatus,
		},
		SourceEndpoint: session.Source.Endpoint, TargetEndpoint: session.Target.Endpoint,
		AToBRate: float64(aToBBytes) / duration,
		BToARate: float64(bToABytes) / duration,
	}
	if current, ok := edge.Observations[relayID]; ok && current.ReceivedAt.Equal(receivedAt) && equivalentPath(current.Path, path) {
		observation.AToBRate += current.AToBRate
		observation.BToARate += current.BToARate
	}
	edge.Observations[relayID] = observation
	if receivedAt.After(edge.LastActive) {
		edge.LastActive = receivedAt
	}
}

func relayScopeKey(relayID string, vni int64) string {
	return relayID + ":" + strconv.FormatInt(vni, 10)
}

func (a *Aggregator) relayScopeLocked(relayID string, vni int64, seenAt time.Time) (*relayScopeState, bool) {
	key := relayScopeKey(relayID, vni)
	scope := a.state.RelayScopes[key]
	created := scope == nil
	if scope == nil {
		scope = &relayScopeState{
			RelayID: relayID, VNI: vni, Sessions: make(map[string]*relaySessionState),
		}
		a.state.RelayScopes[key] = scope
	}
	scope.LastSeen = seenAt
	return scope, created
}

func (a *Aggregator) resolveRelaySessionLocked(
	relayID string,
	observation domain.RelaySessionObservation,
	seenAt time.Time,
) (string, string, domain.IdentityStatus, domain.IdentityStatus, bool) {
	scope, changed := a.relayScopeLocked(relayID, observation.VNI, seenAt)
	session := scope.Sessions[observation.SessionID]
	if session == nil {
		session = &relaySessionState{Clients: make(map[string]string)}
		scope.Sessions[observation.SessionID] = session
		changed = true
	}
	session.LastSeen = seenAt
	sourceID, sourceStatus, sourceChanged := a.resolveRelayClientLocked(session, observation.Source, seenAt)
	targetID, targetStatus, targetChanged := a.resolveRelayClientLocked(session, observation.Target, seenAt)
	changed = changed || sourceChanged || targetChanged
	session.SourceClientID = observation.Source.SessionClientID
	session.TargetClientID = observation.Target.SessionClientID
	session.SourceNodeID = sourceID
	session.TargetNodeID = targetID
	session.SourceStatus = sourceStatus
	session.TargetStatus = targetStatus

	if sourceID == targetID {
		// Ambiguous or conflicting identity evidence must never establish a pair.
	} else if sourceStatus == domain.IdentityResolved && targetStatus == domain.IdentityResolved {
		changed = a.recordRelayPairLocked(relayID, observation.VNI, sourceID, targetID, seenAt) || changed
	} else {
		changed = a.reconcileRelayScopeLocked(scope, seenAt) || changed
	}

	sourceID = session.SourceNodeID
	targetID = session.TargetNodeID
	sourceStatus = a.relayClientStatusLocked(scope, sourceID, session.SourceStatus, seenAt)
	targetStatus = a.relayClientStatusLocked(scope, targetID, session.TargetStatus, seenAt)
	return sourceID, targetID, sourceStatus, targetStatus, changed
}

func (a *Aggregator) resolveRelayClientLocked(
	session *relaySessionState,
	client domain.RelaySessionClient,
	seenAt time.Time,
) (string, domain.IdentityStatus, bool) {
	existingID := session.Clients[client.SessionClientID]
	if client.Identity != nil && identityStatus(*client.Identity) == domain.IdentityResolved {
		nodeID, _, canonicalChanged := a.resolveIdentityLocked(*client.Identity, seenAt)
		if existingID != "" && existingID != nodeID {
			a.mergeNodesLocked(nodeID, existingID)
			canonicalChanged = true
		}
		session.Clients[client.SessionClientID] = nodeID
		return nodeID, domain.IdentityResolved, canonicalChanged || existingID == ""
	}
	status := client.IdentityStatus()
	if existingID != "" {
		if node := a.state.Nodes[existingID]; node != nil && node.IdentityStatus != domain.IdentityResolved {
			node.IdentityStatus = status
		}
		return existingID, status, false
	}
	nodeID := a.newNodeID()
	a.state.Nodes[nodeID] = &nodeState{
		IdentityStatus: status, LastEvidence: seenAt,
	}
	session.Clients[client.SessionClientID] = nodeID
	return nodeID, status, true
}

func (a *Aggregator) recordRelayPairLocked(
	relayID string,
	vni int64,
	leftID, rightID string,
	seenAt time.Time,
) bool {
	if leftID == "" || rightID == "" || leftID == rightID {
		return false
	}
	scope, changed := a.relayScopeLocked(relayID, vni, seenAt)
	_, sourceID, targetID := domain.EdgeID(leftID, rightID)
	pairExpired := scope.PairObservedAt.IsZero() || seenAt.Sub(scope.PairObservedAt) > a.evidenceWindow
	if pairExpired {
		scope.PairSourceID = sourceID
		scope.PairTargetID = targetID
		scope.PairObservedAt = seenAt
		scope.ConflictObservedAt = time.Time{}
	} else if scope.PairSourceID == sourceID && scope.PairTargetID == targetID {
		scope.PairObservedAt = seenAt
	} else {
		scope.ConflictObservedAt = seenAt
	}
	return a.reconcileRelayScopeLocked(scope, seenAt) || changed
}

func (a *Aggregator) reconcileRelayScopeLocked(scope *relayScopeState, seenAt time.Time) bool {
	if scope.PairSourceID == "" || scope.PairTargetID == "" ||
		seenAt.Sub(scope.PairObservedAt) > a.evidenceWindow ||
		(!scope.ConflictObservedAt.IsZero() && seenAt.Sub(scope.ConflictObservedAt) <= a.evidenceWindow) {
		return false
	}
	changed := false
	for _, session := range scope.Sessions {
		if seenAt.Sub(session.LastSeen) > a.evidenceWindow {
			continue
		}
		sourceInPair := session.SourceNodeID == scope.PairSourceID || session.SourceNodeID == scope.PairTargetID
		targetInPair := session.TargetNodeID == scope.PairSourceID || session.TargetNodeID == scope.PairTargetID
		switch {
		case sourceInPair && !targetInPair && a.nodeCanBeInferredLocked(session.TargetNodeID):
			complement := scope.PairSourceID
			if session.SourceNodeID == scope.PairSourceID {
				complement = scope.PairTargetID
			}
			a.mergeNodesLocked(complement, session.TargetNodeID)
			session.TargetNodeID = complement
			session.TargetStatus = domain.IdentityResolved
			changed = true
		case targetInPair && !sourceInPair && a.nodeCanBeInferredLocked(session.SourceNodeID):
			complement := scope.PairSourceID
			if session.TargetNodeID == scope.PairSourceID {
				complement = scope.PairTargetID
			}
			a.mergeNodesLocked(complement, session.SourceNodeID)
			session.SourceNodeID = complement
			session.SourceStatus = domain.IdentityResolved
			changed = true
		}
	}
	return changed
}

func (a *Aggregator) nodeCanBeInferredLocked(nodeID string) bool {
	node := a.state.Nodes[nodeID]
	return node != nil && node.IdentityStatus != domain.IdentityResolved
}

func (a *Aggregator) relayClientStatusLocked(
	scope *relayScopeState,
	nodeID string,
	fallback domain.IdentityStatus,
	seenAt time.Time,
) domain.IdentityStatus {
	if !scope.ConflictObservedAt.IsZero() && seenAt.Sub(scope.ConflictObservedAt) <= a.evidenceWindow {
		if node := a.state.Nodes[nodeID]; node == nil || node.IdentityStatus != domain.IdentityResolved {
			return domain.IdentityConflict
		}
	}
	if node := a.state.Nodes[nodeID]; node != nil && node.IdentityStatus == domain.IdentityResolved {
		return domain.IdentityResolved
	}
	return fallback
}

func (a *Aggregator) resolveIdentityLocked(identity domain.NodeIdentity, seenAt time.Time) (string, bool, bool) {
	strong, addresses := identityAliases(identity)
	matches := make(map[string]struct{})
	for _, alias := range strong {
		if id := a.state.Aliases[alias]; id != "" {
			matches[id] = struct{}{}
		}
	}
	for _, alias := range addresses {
		if id := a.state.Aliases[alias]; id != "" && a.canUseAddressMatch(identity, alias, id, seenAt) {
			matches[id] = struct{}{}
		}
	}

	created := false
	merged := false
	var nodeID string
	if len(matches) == 0 {
		nodeID = a.newNodeID()
		a.state.Nodes[nodeID] = &nodeState{
			Identity: identity, IdentityStatus: identityStatus(identity), LastEvidence: seenAt,
		}
		created = true
	} else {
		ids := make([]string, 0, len(matches))
		for id := range matches {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		nodeID = ids[0]
		if identity.StableNodeID != "" {
			if stableID := a.state.Aliases["stable:"+identity.StableNodeID]; stableID != "" {
				nodeID = stableID
			}
		}
		for _, id := range ids {
			if id != nodeID {
				a.mergeNodesLocked(nodeID, id)
				merged = true
			}
		}
		a.state.Nodes[nodeID].Identity = mergeIdentity(a.state.Nodes[nodeID].Identity, identity)
		if identityStatus(identity) == domain.IdentityResolved {
			a.state.Nodes[nodeID].IdentityStatus = domain.IdentityResolved
		}
	}
	for _, alias := range strong {
		a.state.Aliases[alias] = nodeID
	}
	for _, alias := range addresses {
		a.state.Aliases[alias] = nodeID
		a.state.AliasLastSeen[alias] = seenAt
	}
	return nodeID, created, created || merged
}

func (a *Aggregator) canUseAddressMatch(identity domain.NodeIdentity, alias, nodeID string, seenAt time.Time) bool {
	lastSeen := a.state.AliasLastSeen[alias]
	if lastSeen.IsZero() {
		if node := a.state.Nodes[nodeID]; node != nil {
			lastSeen = node.LastEvidence
		}
	}
	if lastSeen.IsZero() || seenAt.Sub(lastSeen) > a.nodeWindow {
		return false
	}
	existing := a.state.Nodes[nodeID]
	return existing == nil || identity.StableNodeID == "" || existing.Identity.StableNodeID == "" ||
		identity.StableNodeID == existing.Identity.StableNodeID
}

func (a *Aggregator) pruneIndexesLocked(now time.Time) {
	for alias, nodeID := range a.state.Aliases {
		if !strings.HasPrefix(alias, "ip:") {
			continue
		}
		lastSeen := a.state.AliasLastSeen[alias]
		if lastSeen.IsZero() {
			if node := a.state.Nodes[nodeID]; node != nil {
				lastSeen = node.LastEvidence
			}
		}
		if lastSeen.IsZero() || now.Sub(lastSeen) > a.nodeWindow {
			delete(a.state.Aliases, alias)
			delete(a.state.AliasLastSeen, alias)
			a.removeNodeAddressLocked(nodeID, strings.TrimPrefix(alias, "ip:"))
		}
	}
	for key, scope := range a.state.RelayScopes {
		if scope.LastSeen.IsZero() || now.Sub(scope.LastSeen) > a.nodeWindow {
			delete(a.state.RelayScopes, key)
			continue
		}
		if !scope.ConflictObservedAt.IsZero() && now.Sub(scope.ConflictObservedAt) > a.evidenceWindow {
			scope.ConflictObservedAt = time.Time{}
		}
		for sessionID, session := range scope.Sessions {
			if session.LastSeen.IsZero() || now.Sub(session.LastSeen) > a.nodeWindow {
				delete(scope.Sessions, sessionID)
			}
		}
	}
}

func identityStatus(identity domain.NodeIdentity) domain.IdentityStatus {
	strong, _ := identityAliases(identity)
	if len(strong) > 0 {
		return domain.IdentityResolved
	}
	if identity.HasIdentity() {
		return domain.IdentityPartial
	}
	return domain.IdentityAnonymous
}

func (a *Aggregator) reporterOwnsObserversLocked(reporterID string, observations []domain.ObserverReport, seenAt time.Time) bool {
	for _, observation := range observations {
		observerID, ok := a.lookupIdentityLocked(observation.Observer, seenAt)
		if !ok {
			return false
		}
		observer := a.state.Observers[observerID]
		if observer == nil || observer.OwnerReporterInstanceID != reporterID {
			return false
		}
	}
	return true
}

func (a *Aggregator) lookupIdentityLocked(identity domain.NodeIdentity, seenAt time.Time) (string, bool) {
	strong, addresses := identityAliases(identity)
	for _, alias := range strong {
		if nodeID := a.state.Aliases[alias]; nodeID != "" {
			return nodeID, true
		}
	}
	for _, alias := range addresses {
		if nodeID := a.state.Aliases[alias]; nodeID != "" && a.canUseAddressMatch(identity, alias, nodeID, seenAt) {
			return nodeID, true
		}
	}
	return "", false
}

func (a *Aggregator) claimReporterObserverLocked(reporterID string, reporter *reporterState, observerID string) {
	observer := a.state.Observers[observerID]
	if observer == nil {
		observer = &observerRuntimeState{Membership: make(map[string]struct{})}
		a.state.Observers[observerID] = observer
	}
	previousReporterID := observer.OwnerReporterInstanceID
	if previousReporterID != "" && previousReporterID != reporterID {
		if previous := a.state.Reporters[previousReporterID]; previous != nil {
			delete(previous.ObserverIDs, observerID)
			if len(previous.ObserverIDs) == 0 {
				delete(a.state.Reporters, previousReporterID)
			}
		}
	}
	observer.OwnerReporterInstanceID = reporterID
	reporter.ObserverIDs[observerID] = struct{}{}
}

func (a *Aggregator) removeNodeAddressLocked(nodeID, expiredAddress string) {
	node := a.state.Nodes[nodeID]
	if node == nil {
		return
	}
	addresses := node.Identity.TailscaleIPs[:0]
	for _, address := range node.Identity.TailscaleIPs {
		if strings.TrimSpace(address) != expiredAddress {
			addresses = append(addresses, address)
		}
	}
	node.Identity.TailscaleIPs = addresses
}

func identityAliases(identity domain.NodeIdentity) (strong, addresses []string) {
	for _, candidate := range []struct {
		prefix string
		value  string
	}{
		{"stable:", identity.StableNodeID},
		{"node-id:", identity.NodeID},
		{"node-key:", identity.NodeKey},
		{"disco:", identity.DiscoKey},
	} {
		if value := strings.TrimSpace(candidate.value); value != "" && value != "0" {
			strong = append(strong, candidate.prefix+value)
		}
	}
	for _, address := range identity.TailscaleIPs {
		if address = strings.TrimSpace(address); address != "" {
			addresses = append(addresses, "ip:"+address)
		}
	}
	return strong, addresses
}

func (a *Aggregator) mergeNodesLocked(keepID, removeID string) {
	keep, remove := a.state.Nodes[keepID], a.state.Nodes[removeID]
	if keep == nil || remove == nil || keepID == removeID {
		return
	}
	removeObserverIsNewer := remove.LastReport.After(keep.LastReport)
	keep.Identity = mergeIdentity(remove.Identity, keep.Identity)
	keep.IdentityStatus = mergeIdentityStatus(keep.IdentityStatus, remove.IdentityStatus)
	keep.Observable = keep.Observable || remove.Observable
	if remove.LastEvidence.After(keep.LastEvidence) {
		keep.LastEvidence = remove.LastEvidence
	}
	if remove.LastReport.After(keep.LastReport) {
		keep.LastReport = remove.LastReport
		keep.LastCollected = remove.LastCollected
		keep.ClockSkewMS = remove.ClockSkewMS
		keep.ClockSkewed = remove.ClockSkewed
	}
	delete(a.state.Nodes, removeID)
	for fromID, toID := range a.state.Redirects {
		if toID == removeID {
			a.state.Redirects[fromID] = keepID
		}
	}
	a.state.Redirects[removeID] = keepID
	for alias, id := range a.state.Aliases {
		if id == removeID {
			a.state.Aliases[alias] = keepID
		}
	}
	a.mergeObserverRuntimeStatesLocked(keepID, removeID, removeObserverIsNewer)
	a.rebuildEdgesLocked(keepID, removeID)
	a.replaceRelayNodeReferencesLocked(keepID, removeID)
}

func mergeIdentityStatus(left, right domain.IdentityStatus) domain.IdentityStatus {
	priority := map[domain.IdentityStatus]int{
		domain.IdentityAnonymous: 1,
		domain.IdentityPartial:   2,
		domain.IdentityConflict:  3,
		domain.IdentityResolved:  4,
	}
	if priority[right] > priority[left] {
		return right
	}
	return left
}

func (a *Aggregator) mergeObserverRuntimeStatesLocked(keepID, removeID string, removeIsNewer bool) {
	keep := a.state.Observers[keepID]
	remove := a.state.Observers[removeID]
	if keep == nil && remove != nil {
		keep = remove
		a.state.Observers[keepID] = keep
	} else if keep != nil && remove != nil {
		if keep.Membership == nil {
			keep.Membership = make(map[string]struct{})
		}
		for memberID := range remove.Membership {
			keep.Membership[memberID] = struct{}{}
		}
		if removeIsNewer || keep.OwnerReporterInstanceID == "" {
			keep.OwnerReporterInstanceID = remove.OwnerReporterInstanceID
			keep.InventoryGeneration = remove.InventoryGeneration
		}
	}
	delete(a.state.Observers, removeID)

	for _, observer := range a.state.Observers {
		if _, exists := observer.Membership[removeID]; exists {
			delete(observer.Membership, removeID)
			observer.Membership[keepID] = struct{}{}
		}
	}
	for _, reporter := range a.state.Reporters {
		delete(reporter.ObserverIDs, keepID)
		delete(reporter.ObserverIDs, removeID)
	}
	if keep != nil && keep.OwnerReporterInstanceID != "" {
		if reporter := a.state.Reporters[keep.OwnerReporterInstanceID]; reporter != nil {
			reporter.ObserverIDs[keepID] = struct{}{}
		}
	}
}

func (a *Aggregator) rebuildEdgesLocked(keepID, removeID string) {
	rebuilt := make(map[string]*edgeState, len(a.state.Edges))
	for _, edge := range a.state.Edges {
		source, target := edge.Source, edge.Target
		if source == removeID {
			source = keepID
		}
		if target == removeID {
			target = keepID
		}
		if source == target {
			continue
		}
		id, source, target := domain.EdgeID(source, target)
		current := rebuilt[id]
		if current == nil {
			current = &edgeState{ID: id, Source: source, Target: target, Observations: make(map[string]edgeObservation)}
			rebuilt[id] = current
		}
		if edge.LastActive.After(current.LastActive) {
			current.LastActive = edge.LastActive
			current.LastKnownPath = edge.LastKnownPath
		}
		for observerID, observation := range edge.Observations {
			if observerID == removeID {
				observerID = keepID
				observation.ObserverID = keepID
			}
			previous, exists := current.Observations[observerID]
			if !exists || observation.ReceivedAt.After(previous.ReceivedAt) {
				current.Observations[observerID] = observation
			}
		}
	}
	a.state.Edges = rebuilt
}

func (a *Aggregator) replaceRelayNodeReferencesLocked(keepID, removeID string) {
	rebuilt := make(map[string]*relayScopeState, len(a.state.RelayScopes))
	for _, scope := range a.state.RelayScopes {
		if scope.RelayID == removeID {
			scope.RelayID = keepID
		}
		if scope.PairSourceID == removeID {
			scope.PairSourceID = keepID
		}
		if scope.PairTargetID == removeID {
			scope.PairTargetID = keepID
		}
		if scope.PairSourceID != "" && scope.PairTargetID != "" {
			_, scope.PairSourceID, scope.PairTargetID = domain.EdgeID(scope.PairSourceID, scope.PairTargetID)
		}
		for _, session := range scope.Sessions {
			if session.SourceNodeID == removeID {
				session.SourceNodeID = keepID
			}
			if session.TargetNodeID == removeID {
				session.TargetNodeID = keepID
			}
			for clientID, nodeID := range session.Clients {
				if nodeID == removeID {
					session.Clients[clientID] = keepID
				}
			}
		}
		key := relayScopeKey(scope.RelayID, scope.VNI)
		if current := rebuilt[key]; current != nil {
			mergeRelayScopes(current, scope)
		} else {
			rebuilt[key] = scope
		}
	}
	a.state.RelayScopes = rebuilt
}

func mergeRelayScopes(keep, update *relayScopeState) {
	if update.LastSeen.After(keep.LastSeen) {
		keep.LastSeen = update.LastSeen
	}
	if update.ConflictObservedAt.After(keep.ConflictObservedAt) {
		keep.ConflictObservedAt = update.ConflictObservedAt
	}
	if update.PairObservedAt.After(keep.PairObservedAt) {
		keep.PairSourceID = update.PairSourceID
		keep.PairTargetID = update.PairTargetID
		keep.PairObservedAt = update.PairObservedAt
	}
	if keep.Sessions == nil {
		keep.Sessions = make(map[string]*relaySessionState)
	}
	for sessionID, session := range update.Sessions {
		if current := keep.Sessions[sessionID]; current == nil || session.LastSeen.After(current.LastSeen) {
			keep.Sessions[sessionID] = session
		}
	}
}

func mergeIdentity(current, update domain.NodeIdentity) domain.NodeIdentity {
	if update.StableNodeID != "" {
		current.StableNodeID = update.StableNodeID
	}
	if update.NodeID != "" && update.NodeID != "0" {
		current.NodeID = update.NodeID
	}
	if update.NodeKey != "" {
		current.NodeKey = update.NodeKey
	}
	if update.DiscoKey != "" {
		current.DiscoKey = update.DiscoKey
	}
	if update.Hostname != "" {
		current.Hostname = update.Hostname
	}
	if update.DNSName != "" {
		current.DNSName = update.DNSName
	}
	if update.OS != "" {
		current.OS = update.OS
	}
	if len(update.TailscaleIPs) > 0 {
		current.TailscaleIPs = append([]string(nil), update.TailscaleIPs...)
		sort.Strings(current.TailscaleIPs)
	}
	return current
}

func (a *Aggregator) touchObserverLocked(nodeID string, collectedAt, receivedAt time.Time) {
	node := a.state.Nodes[nodeID]
	node.Observable = true
	node.LastEvidence = receivedAt
	node.LastReport = receivedAt
	node.LastCollected = collectedAt
	node.ClockSkewMS = collectedAt.Sub(receivedAt).Milliseconds()
	node.ClockSkewed = a.isClockSkewed(collectedAt, receivedAt)
}

func (a *Aggregator) touchPeerLocked(nodeID string, receivedAt time.Time) {
	if node := a.state.Nodes[nodeID]; node != nil {
		node.LastEvidence = receivedAt
	}
}

func (a *Aggregator) isClockSkewed(collectedAt, receivedAt time.Time) bool {
	skew := collectedAt.Sub(receivedAt)
	if skew < 0 {
		skew = -skew
	}
	return skew > a.heartbeatInterval/2
}

func (a *Aggregator) Snapshot() domain.Topology {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.snapshotLocked(a.now().UTC())
}

func (a *Aggregator) snapshotLocked(now time.Time) domain.Topology {
	topology := domain.Topology{
		GeneratedAt: now,
		Nodes:       []domain.TopologyNode{},
		Edges:       []domain.TopologyEdge{},
		Observers:   []domain.ObserverState{},
	}
	visibleNodes := make(map[string]struct{})
	var next time.Time
	consider := func(deadline time.Time) {
		if deadline.After(now) && (next.IsZero() || deadline.Before(next)) {
			next = deadline
		}
	}

	for _, edge := range a.state.Edges {
		age := now.Sub(edge.LastActive)
		if age > a.evidenceWindow {
			continue
		}
		state := domain.EdgeRecent
		if age <= activeWindow {
			state = domain.EdgeActive
			consider(edge.LastActive.Add(activeWindow))
		}
		consider(edge.LastActive.Add(a.evidenceWindow))
		result := a.snapshotEdgeLocked(edge, now)
		result.State = state
		topology.Edges = append(topology.Edges, result)
		visibleNodes[edge.Source] = struct{}{}
		visibleNodes[edge.Target] = struct{}{}
		a.markPeerRelayNodesVisibleLocked(visibleNodes, result.Path)
		for _, observation := range result.Observations {
			a.markPeerRelayNodesVisibleLocked(visibleNodes, observation.Path)
		}
		for _, observation := range edge.Observations {
			consider(observation.ReceivedAt.Add(activeWindow))
			consider(observation.ReceivedAt.Add(a.evidenceWindow))
		}
	}

	for id, node := range a.state.Nodes {
		_, onEdge := visibleNodes[id]
		if !onEdge && (node.LastEvidence.IsZero() || now.Sub(node.LastEvidence) > a.nodeWindow) {
			continue
		}
		online := node.Observable && now.Sub(node.LastReport) <= a.evidenceWindow
		topology.Nodes = append(topology.Nodes, domain.TopologyNode{
			NodeIdentity: node.Identity, ID: id, Observable: node.Observable, Online: online,
			LastEvidenceAt: node.LastEvidence, ClockSkewed: node.ClockSkewed,
			IdentityStatus: a.nodeIdentityStatusLocked(id, node, now),
		})
		consider(node.LastEvidence.Add(a.nodeWindow))
	}
	for id, node := range a.state.Nodes {
		if !node.Observable {
			continue
		}
		online := now.Sub(node.LastReport) <= a.evidenceWindow
		topology.Observers = append(topology.Observers, domain.ObserverState{
			ID: id, Hostname: node.Identity.DisplayName(), Online: online, LastSeen: node.LastReport,
			LastCollectedAt: node.LastCollected, ClockSkewMS: node.ClockSkewMS, ClockSkewed: node.ClockSkewed,
		})
		if online {
			consider(node.LastReport.Add(a.evidenceWindow))
		}
	}

	if !next.IsZero() {
		topology.NextChangeAt = &next
	}
	sort.Slice(topology.Nodes, func(i, j int) bool { return topology.Nodes[i].ID < topology.Nodes[j].ID })
	sort.Slice(topology.Edges, func(i, j int) bool { return topology.Edges[i].ID < topology.Edges[j].ID })
	sort.Slice(topology.Observers, func(i, j int) bool { return topology.Observers[i].ID < topology.Observers[j].ID })
	return topology
}

func (a *Aggregator) nodeIdentityStatusLocked(id string, node *nodeState, now time.Time) domain.IdentityStatus {
	if node.IdentityStatus == domain.IdentityResolved {
		return domain.IdentityResolved
	}
	for _, scope := range a.state.RelayScopes {
		if scope.ConflictObservedAt.IsZero() || now.Sub(scope.ConflictObservedAt) > a.evidenceWindow {
			continue
		}
		for _, session := range scope.Sessions {
			if session.SourceNodeID == id || session.TargetNodeID == id {
				return domain.IdentityConflict
			}
		}
	}
	return node.IdentityStatus
}

func (a *Aggregator) markPeerRelayNodesVisibleLocked(visibleNodes map[string]struct{}, path domain.PathObservation) {
	if path.Kind != domain.PathPeerRelay {
		return
	}
	stableID := strings.TrimSpace(path.PeerRelayStableNodeID)
	if stableID == "" {
		return
	}
	if nodeID := a.state.Aliases["stable:"+stableID]; nodeID != "" {
		visibleNodes[nodeID] = struct{}{}
	}
}

func (a *Aggregator) snapshotEdgeLocked(edge *edgeState, now time.Time) domain.TopologyEdge {
	result := domain.TopologyEdge{
		ID: edge.ID, Source: edge.Source, Target: edge.Target, LastActive: edge.LastActive,
		SystemTelemetry: edge.SystemTelemetry || a.isControlNodeLocked(edge.Source) || a.isControlNodeLocked(edge.Target),
		Observations:    []domain.ObservationProvenance{},
	}
	var sourceObservation, targetObservation, relayObservation *edgeObservation
	for _, observation := range edge.Observations {
		if now.Sub(observation.ReceivedAt) > a.evidenceWindow {
			continue
		}
		copy := observation
		if observation.ObserverID == edge.Source {
			sourceObservation = &copy
		}
		if observation.ObserverID == edge.Target {
			targetObservation = &copy
		}
		if observation.ObserverID != edge.Source && observation.ObserverID != edge.Target &&
			observation.Path.Kind == domain.PathPeerRelay &&
			(relayObservation == nil || observation.ReceivedAt.After(relayObservation.ReceivedAt)) {
			relayObservation = &copy
		}
		result.Observations = append(result.Observations, domain.ObservationProvenance{
			ObserverID: observation.ObserverID, Path: observation.Path, CollectedAt: observation.CollectedAt,
			ReceivedAt: observation.ReceivedAt, ClockSkewed: observation.ClockSkewed,
			RelaySession: observation.RelaySession,
		})
	}
	aToBCurrent := sourceObservation != nil && now.Sub(sourceObservation.ReceivedAt) <= activeWindow
	bToACurrent := targetObservation != nil && now.Sub(targetObservation.ReceivedAt) <= activeWindow
	if aToBCurrent {
		result.AToBBytesPerSecond = sourceObservation.TxRate
		result.BToABytesPerSecond = sourceObservation.RxRate
	}
	if bToACurrent {
		if !aToBCurrent {
			result.AToBBytesPerSecond = targetObservation.RxRate
		}
		result.BToABytesPerSecond = targetObservation.TxRate
	}
	if relayObservation != nil && now.Sub(relayObservation.ReceivedAt) <= activeWindow {
		if !aToBCurrent && !bToACurrent {
			result.AToBBytesPerSecond = relayObservation.AToBRate
			result.BToABytesPerSecond = relayObservation.BToARate
		}
	}
	result.Path, result.Conflicts = reconcilePaths(result.Observations)
	if len(result.Observations) == 0 {
		result.Path = edge.LastKnownPath
		if result.Path.Kind == "" {
			result.Path.Kind = domain.PathUnknown
		}
	}
	sort.Slice(result.Observations, func(i, j int) bool {
		return result.Observations[i].ObserverID < result.Observations[j].ObserverID
	})
	return result
}

func reconcilePaths(observations []domain.ObservationProvenance) (domain.PathObservation, []domain.PathObservation) {
	if len(observations) == 0 {
		return domain.PathObservation{Kind: domain.PathUnknown}, nil
	}
	chosen := observations[0]
	for _, observation := range observations[1:] {
		if observation.ReceivedAt.After(chosen.ReceivedAt) ||
			(observation.ReceivedAt.Equal(chosen.ReceivedAt) && pathSpecificity(observation.Path) > pathSpecificity(chosen.Path)) {
			chosen = observation
		}
	}
	path := chosen.Path
	var detail *domain.ObservationProvenance
	for index := range observations {
		observation := &observations[index]
		if !equivalentPath(observation.Path, path) || pathSpecificity(observation.Path) <= pathSpecificity(path) {
			continue
		}
		if detail == nil || observation.ReceivedAt.After(detail.ReceivedAt) {
			detail = observation
		}
	}
	if detail != nil {
		path = enrichEquivalentPath(path, detail.Path)
	}
	var conflicts []domain.PathObservation
	for _, observation := range observations {
		if !equivalentPath(observation.Path, path) && !containsPath(conflicts, observation.Path) {
			conflicts = append(conflicts, observation.Path)
		}
	}
	return path, conflicts
}

func enrichEquivalentPath(path, detail domain.PathObservation) domain.PathObservation {
	result := path
	switch result.Kind {
	case domain.PathDERP:
		if result.DERPRegion == "" {
			result.DERPRegion = detail.DERPRegion
		}
	case domain.PathPeerRelay:
		if result.PeerRelayStableNodeID == "" {
			result.PeerRelayStableNodeID = detail.PeerRelayStableNodeID
		}
		if result.PeerRelayVNI == nil {
			result.PeerRelayVNI = detail.PeerRelayVNI
		}
	}
	return result
}

func equivalentPath(left, right domain.PathObservation) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case domain.PathDirect, domain.PathUnknown:
		return true
	case domain.PathDERP:
		return left.DERPRegion == "" || right.DERPRegion == "" || left.DERPRegion == right.DERPRegion
	case domain.PathPeerRelay:
		return left.PeerRelayStableNodeID == "" || right.PeerRelayStableNodeID == "" ||
			left.PeerRelayStableNodeID == right.PeerRelayStableNodeID
	default:
		return false
	}
}

func pathSpecificity(path domain.PathObservation) int {
	switch path.Kind {
	case domain.PathDirect:
		if path.DirectEndpoint != "" {
			return 1
		}
	case domain.PathDERP:
		if path.DERPRegion != "" {
			return 1
		}
	case domain.PathPeerRelay:
		if path.PeerRelayStableNodeID != "" {
			return 1
		}
	}
	return 0
}

func containsPath(paths []domain.PathObservation, candidate domain.PathObservation) bool {
	for _, path := range paths {
		if path.Equal(candidate) {
			return true
		}
	}
	return false
}

func (a *Aggregator) MarshalState() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return json.Marshal(a.state)
}

func (a *Aggregator) HistoryMetadata() domain.HistoryMetadata {
	a.mu.RLock()
	defer a.mu.RUnlock()
	metadata := domain.HistoryMetadata{
		Nodes:     make([]domain.TopologyNode, 0, len(a.state.Nodes)),
		Redirects: make(map[string]string, len(a.state.Redirects)),
	}
	now := a.now().UTC()
	for id, node := range a.state.Nodes {
		metadata.Nodes = append(metadata.Nodes, domain.TopologyNode{
			NodeIdentity:   node.Identity,
			ID:             id,
			Observable:     node.Observable,
			LastEvidenceAt: node.LastEvidence,
			IdentityStatus: a.nodeIdentityStatusLocked(id, node, now),
		})
	}
	sort.Slice(metadata.Nodes, func(i, j int) bool { return metadata.Nodes[i].ID < metadata.Nodes[j].ID })
	for fromID, toID := range a.state.Redirects {
		metadata.Redirects[fromID] = toID
	}
	return metadata
}

func (a *Aggregator) RestoreState(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	var state runtimeState
	if err := json.Unmarshal(payload, &state); err != nil {
		return err
	}
	normalizeState(&state)
	a.mu.Lock()
	a.state = state
	a.mu.Unlock()
	return nil
}

func (a *Aggregator) Clone() (*Aggregator, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	clone := New(Options{
		HeartbeatInterval: a.heartbeatInterval,
		ControlNodeIDs:    a.controlIDs,
		Now:               a.now,
		NewNodeID:         a.newNodeID,
	})
	clone.state = cloneRuntimeState(a.state)
	return clone, nil
}

func (a *Aggregator) ReplaceWith(candidate *Aggregator) error {
	candidate.mu.Lock()
	state := candidate.state
	candidate.state = newRuntimeState()
	candidate.mu.Unlock()
	a.mu.Lock()
	a.state = state
	a.notifyLocked()
	a.mu.Unlock()
	return nil
}

func cloneRuntimeState(source runtimeState) runtimeState {
	clone := runtimeState{
		Reporters:     make(map[string]*reporterState, len(source.Reporters)),
		Observers:     make(map[string]*observerRuntimeState, len(source.Observers)),
		Nodes:         make(map[string]*nodeState, len(source.Nodes)),
		Aliases:       make(map[string]string, len(source.Aliases)),
		AliasLastSeen: make(map[string]time.Time, len(source.AliasLastSeen)),
		Redirects:     make(map[string]string, len(source.Redirects)),
		Edges:         make(map[string]*edgeState, len(source.Edges)),
		RelayScopes:   make(map[string]*relayScopeState, len(source.RelayScopes)),
	}
	for id, reporter := range source.Reporters {
		copy := *reporter
		copy.ReportIDs = cloneSet(reporter.ReportIDs)
		copy.ObserverIDs = cloneSet(reporter.ObserverIDs)
		copy.LegacyInventories = cloneStringMap(reporter.LegacyInventories)
		if reporter.LegacyMemberships != nil {
			copy.LegacyMemberships = make(map[string]map[string]struct{}, len(reporter.LegacyMemberships))
			for observerID, membership := range reporter.LegacyMemberships {
				copy.LegacyMemberships[observerID] = cloneSet(membership)
			}
		}
		clone.Reporters[id] = &copy
	}
	for id, observer := range source.Observers {
		copy := *observer
		copy.Membership = cloneSet(observer.Membership)
		clone.Observers[id] = &copy
	}
	for id, node := range source.Nodes {
		copy := *node
		copy.Identity.TailscaleIPs = append([]string(nil), node.Identity.TailscaleIPs...)
		clone.Nodes[id] = &copy
	}
	for alias, id := range source.Aliases {
		clone.Aliases[alias] = id
	}
	for alias, seenAt := range source.AliasLastSeen {
		clone.AliasLastSeen[alias] = seenAt
	}
	for fromID, toID := range source.Redirects {
		clone.Redirects[fromID] = toID
	}
	for id, edge := range source.Edges {
		copy := *edge
		copy.Observations = make(map[string]edgeObservation, len(edge.Observations))
		for observerID, observation := range edge.Observations {
			copy.Observations[observerID] = observation
		}
		clone.Edges[id] = &copy
	}
	for key, scope := range source.RelayScopes {
		copy := *scope
		copy.Sessions = make(map[string]*relaySessionState, len(scope.Sessions))
		for sessionID, session := range scope.Sessions {
			sessionCopy := *session
			sessionCopy.Clients = cloneStringMap(session.Clients)
			copy.Sessions[sessionID] = &sessionCopy
		}
		clone.RelayScopes[key] = &copy
	}
	return clone
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	if source == nil {
		return nil
	}
	clone := make(map[string]struct{}, len(source))
	for value := range source {
		clone[value] = struct{}{}
	}
	return clone
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func normalizeState(state *runtimeState) {
	if state.Reporters == nil {
		state.Reporters = make(map[string]*reporterState)
	}
	if state.Observers == nil {
		state.Observers = make(map[string]*observerRuntimeState)
	}
	if state.Nodes == nil {
		state.Nodes = make(map[string]*nodeState)
	}
	if state.Aliases == nil {
		state.Aliases = make(map[string]string)
	}
	if state.AliasLastSeen == nil {
		state.AliasLastSeen = make(map[string]time.Time)
	}
	if state.Redirects == nil {
		state.Redirects = make(map[string]string)
	}
	if state.Edges == nil {
		state.Edges = make(map[string]*edgeState)
	}
	if state.RelayScopes == nil {
		state.RelayScopes = make(map[string]*relayScopeState)
	}
	reporterIDs := make([]string, 0, len(state.Reporters))
	for reporterID := range state.Reporters {
		reporterIDs = append(reporterIDs, reporterID)
	}
	sort.Strings(reporterIDs)
	for _, reporterID := range reporterIDs {
		reporter := state.Reporters[reporterID]
		if reporter.ReportIDs == nil {
			reporter.ReportIDs = make(map[string]struct{})
		}
		if reporter.ObserverIDs == nil {
			reporter.ObserverIDs = make(map[string]struct{})
		}

		legacyObserverIDs := make(map[string]struct{}, len(reporter.ObserverIDs)+len(reporter.LegacyInventories)+len(reporter.LegacyMemberships))
		for observerID := range reporter.ObserverIDs {
			legacyObserverIDs[observerID] = struct{}{}
		}
		for observerID := range reporter.LegacyInventories {
			legacyObserverIDs[observerID] = struct{}{}
		}
		for observerID := range reporter.LegacyMemberships {
			legacyObserverIDs[observerID] = struct{}{}
		}
		for observerID := range legacyObserverIDs {
			observer := state.Observers[observerID]
			if observer == nil {
				observer = &observerRuntimeState{Membership: make(map[string]struct{})}
				state.Observers[observerID] = observer
			}
			if observer.OwnerReporterInstanceID == "" {
				observer.OwnerReporterInstanceID = reporterID
			}
			if observer.OwnerReporterInstanceID != reporterID {
				continue
			}
			if observer.InventoryGeneration == "" {
				observer.InventoryGeneration = reporter.LegacyInventories[observerID]
			}
			if observer.Membership == nil {
				observer.Membership = make(map[string]struct{})
			}
			for memberID := range reporter.LegacyMemberships[observerID] {
				observer.Membership[memberID] = struct{}{}
			}
		}
		reporter.ObserverIDs = make(map[string]struct{})
		reporter.LegacyInventories = nil
		reporter.LegacyMemberships = nil
	}
	for observerID, observer := range state.Observers {
		if observer.Membership == nil {
			observer.Membership = make(map[string]struct{})
		}
		if observer.OwnerReporterInstanceID == "" {
			continue
		}
		reporter := state.Reporters[observer.OwnerReporterInstanceID]
		if reporter == nil {
			reporter = &reporterState{ReportIDs: make(map[string]struct{}), ObserverIDs: make(map[string]struct{})}
			state.Reporters[observer.OwnerReporterInstanceID] = reporter
		}
		reporter.ObserverIDs[observerID] = struct{}{}
	}
	for _, edge := range state.Edges {
		if edge.Observations == nil {
			edge.Observations = make(map[string]edgeObservation)
		}
	}
	for _, scope := range state.RelayScopes {
		if scope.Sessions == nil {
			scope.Sessions = make(map[string]*relaySessionState)
		}
		for _, session := range scope.Sessions {
			if session.Clients == nil {
				session.Clients = make(map[string]string)
			}
		}
	}
}

func randomNodeID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return "n_" + hex.EncodeToString(value[:])
}

func (a *Aggregator) Subscribe() (<-chan struct{}, func()) {
	channel := make(chan struct{}, 1)
	a.mu.Lock()
	a.subscribers[channel] = struct{}{}
	a.mu.Unlock()
	return channel, func() {
		a.mu.Lock()
		if _, ok := a.subscribers[channel]; ok {
			delete(a.subscribers, channel)
			close(channel)
		}
		a.mu.Unlock()
	}
}

func (a *Aggregator) notifyLocked() {
	for subscriber := range a.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
