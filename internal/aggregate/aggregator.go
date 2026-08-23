package aggregate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

const activeWindow = 10 * time.Second

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
	Reporters     map[string]*reporterState `json:"reporters"`
	Nodes         map[string]*nodeState     `json:"nodes"`
	Aliases       map[string]string         `json:"aliases"`
	AliasLastSeen map[string]time.Time      `json:"aliasLastSeen,omitempty"`
	Edges         map[string]*edgeState     `json:"edges"`
}

type reporterState struct {
	LastSequence int64                          `json:"lastSequence"`
	ReportIDs    map[string]struct{}            `json:"reportIds"`
	ObserverIDs  map[string]struct{}            `json:"observerIds"`
	Inventories  map[string]string              `json:"inventories"`
	Memberships  map[string]map[string]struct{} `json:"memberships"`
}

type nodeState struct {
	Identity      domain.NodeIdentity `json:"identity"`
	Observable    bool                `json:"observable"`
	LastEvidence  time.Time           `json:"lastEvidence"`
	LastReport    time.Time           `json:"lastReport"`
	LastCollected time.Time           `json:"lastCollected"`
	ClockSkewMS   int64               `json:"clockSkewMs"`
	ClockSkewed   bool                `json:"clockSkewed"`
}

type edgeState struct {
	ID            string                     `json:"id"`
	Source        string                     `json:"source"`
	Target        string                     `json:"target"`
	LastActive    time.Time                  `json:"lastActive"`
	LastKnownPath domain.PathObservation     `json:"lastKnownPath"`
	Observations  map[string]edgeObservation `json:"observations"`
}

type edgeObservation struct {
	ObserverID  string                 `json:"observerId"`
	Path        domain.PathObservation `json:"path"`
	CollectedAt time.Time              `json:"collectedAt"`
	ReceivedAt  time.Time              `json:"receivedAt"`
	ClockSkewed bool                   `json:"clockSkewed"`
	TxRate      float64                `json:"txRate"`
	RxRate      float64                `json:"rxRate"`
	AToBRate    float64                `json:"aToBRate,omitempty"`
	BToARate    float64                `json:"bToARate,omitempty"`
}

type ApplyResult struct {
	Receipt         domain.ReportReceipt
	Traffic         []domain.AcceptedTraffic
	PathTransitions []domain.PathTransition
	Changed         bool
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
		Nodes:         make(map[string]*nodeState),
		Aliases:       make(map[string]string),
		AliasLastSeen: make(map[string]time.Time),
		Edges:         make(map[string]*edgeState),
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
	if reporter == nil {
		reporter = &reporterState{
			ReportIDs:   make(map[string]struct{}),
			ObserverIDs: make(map[string]struct{}),
			Inventories: make(map[string]string),
			Memberships: make(map[string]map[string]struct{}),
		}
		a.state.Reporters[report.ReporterInstanceID] = reporter
	}
	result := ApplyResult{Receipt: domain.ReportReceipt{
		Accepted:             true,
		ControlStableNodeIDs: append([]string(nil), a.controlIDs...),
		HeartbeatIntervalMS:  a.heartbeatInterval.Milliseconds(),
	}}
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

	touchedEdges := make(map[string]domain.PathObservation)
	for _, observation := range report.Observers {
		observerID, _ := a.resolveIdentityLocked(observation.Observer, receivedAt)
		a.touchObserverLocked(observerID, report.CollectedAt, receivedAt)
		a.claimReporterObserverLocked(report.ReporterInstanceID, reporter, observerID)

		switch report.Kind {
		case domain.ReportObserverHello, domain.ReportInventoryUpdate:
			members := make(map[string]struct{}, len(observation.Peers))
			for _, peer := range observation.Peers {
				peerID, created := a.resolveIdentityLocked(peer.Peer, receivedAt)
				if created {
					a.state.Nodes[peerID].LastEvidence = receivedAt
				}
				members[peerID] = struct{}{}
			}
			a.replaceInventoryLocked(reporter, observerID, observation.InventoryGeneration, members)
		case domain.ReportTrafficSample:
			if reporter.Inventories[observerID] != observation.InventoryGeneration {
				result.Receipt.ResyncRequired = true
			}
			for _, peer := range observation.Peers {
				if peer.RxDelta == 0 && peer.TxDelta == 0 {
					continue
				}
				peerID, _ := a.resolveIdentityLocked(peer.Peer, receivedAt)
				a.touchPeerLocked(peerID, receivedAt)
				edgeID, source, target := domain.EdgeID(observerID, peerID)
				if _, seen := touchedEdges[edgeID]; !seen {
					if edge := a.state.Edges[edgeID]; edge != nil {
						touchedEdges[edgeID] = edge.LastKnownPath
					} else {
						touchedEdges[edgeID] = domain.PathObservation{}
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
			if reporter.Inventories[observerID] != observation.InventoryGeneration {
				result.Receipt.ResyncRequired = true
			}
		}
	}
	if report.Kind == domain.ReportRelaySessionUpdate {
		for _, session := range report.RelaySessions {
			relayID, _ := a.resolveIdentityLocked(session.Relay, receivedAt)
			a.touchObserverLocked(relayID, report.CollectedAt, receivedAt)
			a.claimReporterObserverLocked(report.ReporterInstanceID, reporter, relayID)
			sourceID, _ := a.resolveIdentityLocked(session.Source, receivedAt)
			targetID, _ := a.resolveIdentityLocked(session.Target, receivedAt)
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
			}
			a.applyRelaySessionLocked(report.CollectedAt, receivedAt, relayID, source, target, session, aToBBytes, bToABytes)
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
	if len(reporter.ReportIDs) > 4096 {
		reporter.ReportIDs = map[string]struct{}{report.ReportID: {}}
	}
	result.Changed = true
	return result, nil
}

func (a *Aggregator) replaceInventoryLocked(reporter *reporterState, observerID, generation string, members map[string]struct{}) {
	for peerID := range reporter.Memberships[observerID] {
		if _, stillVisible := members[peerID]; stillVisible {
			continue
		}
		edgeID, _, _ := domain.EdgeID(observerID, peerID)
		if edge := a.state.Edges[edgeID]; edge != nil {
			delete(edge.Observations, observerID)
		}
	}
	reporter.Inventories[observerID] = generation
	reporter.Memberships[observerID] = members
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
	path := domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: session.Relay.StableNodeID}
	observation := edgeObservation{
		ObserverID: relayID, Path: path, CollectedAt: collectedAt, ReceivedAt: receivedAt,
		ClockSkewed: a.isClockSkewed(collectedAt, receivedAt),
		AToBRate:    float64(aToBBytes) / duration,
		BToARate:    float64(bToABytes) / duration,
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

func (a *Aggregator) resolveIdentityLocked(identity domain.NodeIdentity, seenAt time.Time) (string, bool) {
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
	var nodeID string
	if len(matches) == 0 {
		nodeID = a.newNodeID()
		a.state.Nodes[nodeID] = &nodeState{Identity: identity, LastEvidence: seenAt}
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
			}
		}
		a.state.Nodes[nodeID].Identity = mergeIdentity(a.state.Nodes[nodeID].Identity, identity)
	}
	for _, alias := range strong {
		a.state.Aliases[alias] = nodeID
	}
	for _, alias := range addresses {
		a.state.Aliases[alias] = nodeID
		a.state.AliasLastSeen[alias] = seenAt
	}
	return nodeID, created
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
}

func (a *Aggregator) claimReporterObserverLocked(reporterID string, reporter *reporterState, observerID string) {
	reporter.ObserverIDs[observerID] = struct{}{}
	for previousReporterID, previous := range a.state.Reporters {
		if previousReporterID == reporterID {
			continue
		}
		if _, claimed := previous.ObserverIDs[observerID]; !claimed {
			continue
		}
		delete(previous.ObserverIDs, observerID)
		delete(previous.Inventories, observerID)
		delete(previous.Memberships, observerID)
		if len(previous.ObserverIDs) == 0 {
			delete(a.state.Reporters, previousReporterID)
		}
	}
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
	keep.Identity = mergeIdentity(remove.Identity, keep.Identity)
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
	for alias, id := range a.state.Aliases {
		if id == removeID {
			a.state.Aliases[alias] = keepID
		}
	}
	for _, reporter := range a.state.Reporters {
		if generation, ok := reporter.Inventories[removeID]; ok {
			if _, exists := reporter.Inventories[keepID]; !exists {
				reporter.Inventories[keepID] = generation
			}
			delete(reporter.Inventories, removeID)
		}
		if members, ok := reporter.Memberships[removeID]; ok {
			if reporter.Memberships[keepID] == nil {
				reporter.Memberships[keepID] = make(map[string]struct{})
			}
			for member := range members {
				reporter.Memberships[keepID][member] = struct{}{}
			}
			delete(reporter.Memberships, removeID)
		}
		for _, members := range reporter.Memberships {
			if _, ok := members[removeID]; ok {
				delete(members, removeID)
				members[keepID] = struct{}{}
			}
		}
	}
	a.rebuildEdgesLocked(keepID, removeID)
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
		Observations: []domain.ObservationProvenance{},
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
	var conflicts []domain.PathObservation
	for _, observation := range observations {
		if !equivalentPath(observation.Path, chosen.Path) && !containsPath(conflicts, observation.Path) {
			conflicts = append(conflicts, observation.Path)
		}
	}
	return chosen.Path, conflicts
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
	payload, err := a.MarshalState()
	if err != nil {
		return nil, err
	}
	clone := New(Options{
		HeartbeatInterval: a.heartbeatInterval,
		ControlNodeIDs:    a.controlIDs,
		Now:               a.now,
		NewNodeID:         a.newNodeID,
	})
	if err := clone.RestoreState(payload); err != nil {
		return nil, err
	}
	return clone, nil
}

func (a *Aggregator) ReplaceWith(candidate *Aggregator) error {
	payload, err := candidate.MarshalState()
	if err != nil {
		return err
	}
	var state runtimeState
	if err := json.Unmarshal(payload, &state); err != nil {
		return err
	}
	normalizeState(&state)
	a.mu.Lock()
	a.state = state
	a.notifyLocked()
	a.mu.Unlock()
	return nil
}

func normalizeState(state *runtimeState) {
	if state.Reporters == nil {
		state.Reporters = make(map[string]*reporterState)
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
	if state.Edges == nil {
		state.Edges = make(map[string]*edgeState)
	}
	for _, reporter := range state.Reporters {
		if reporter.ReportIDs == nil {
			reporter.ReportIDs = make(map[string]struct{})
		}
		if reporter.ObserverIDs == nil {
			reporter.ObserverIDs = make(map[string]struct{})
		}
		if reporter.Inventories == nil {
			reporter.Inventories = make(map[string]string)
		}
		if reporter.Memberships == nil {
			reporter.Memberships = make(map[string]map[string]struct{})
		}
		for observerID := range reporter.Inventories {
			reporter.ObserverIDs[observerID] = struct{}{}
		}
	}
	for _, edge := range state.Edges {
		if edge.Observations == nil {
			edge.Observations = make(map[string]edgeObservation)
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
