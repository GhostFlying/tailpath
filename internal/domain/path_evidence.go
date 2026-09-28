package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type PathEvidenceState struct {
	Path      PathObservation
	Conflicts []PathObservation
}

// ReconcilePathEvidence selects a stable primary without using cross-observer
// receipt order as a path signal.
func ReconcilePathEvidence(sourceID, targetID string, previous PathObservation, observations []ObservationProvenance) PathEvidenceState {
	if len(observations) == 0 {
		return PathEvidenceState{Path: PathObservation{Kind: PathUnknown}, Conflicts: []PathObservation{}}
	}
	type evidence struct {
		path       PathObservation
		role       int
		observerID string
		receivedAt time.Time
	}
	evidenceByKey := make(map[string]evidence)
	stableIDByEndpoint := uniqueRelayStableIDsByEndpoint(observations)
	for _, observation := range observations {
		path := observation.Path
		if path.Kind == PathPeerRelay && path.PeerRelayStableNodeID == "" {
			if stableID := stableIDByEndpoint[strings.TrimSpace(path.PeerRelayEndpoint)]; stableID != "" {
				path.PeerRelayStableNodeID = stableID
				path.PeerRelayResolution = "endpoint_match"
			}
		}
		key := PathEvidenceKey(path)
		candidate := evidence{
			path: path, role: observerEvidenceRole(sourceID, targetID, observation.ObserverID),
			observerID: observation.ObserverID, receivedAt: observation.ReceivedAt,
		}
		current, exists := evidenceByKey[key]
		if !exists || candidate.role < current.role ||
			(candidate.role == current.role && candidate.observerID < current.observerID) ||
			(candidate.role == current.role && candidate.observerID == current.observerID && candidate.receivedAt.After(current.receivedAt)) {
			evidenceByKey[key] = candidate
		} else if candidate.receivedAt.After(current.receivedAt) {
			current.path = enrichEvidencePath(current.path, candidate.path)
			current.receivedAt = candidate.receivedAt
			evidenceByKey[key] = current
		}
	}
	keys := make([]string, 0, len(evidenceByKey))
	for key := range evidenceByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := evidenceByKey[keys[i]], evidenceByKey[keys[j]]
		if left.role != right.role {
			return left.role < right.role
		}
		if left.observerID != right.observerID {
			return left.observerID < right.observerID
		}
		return keys[i] < keys[j]
	})
	primaryKey := PathEvidenceKey(previous)
	if _, supported := evidenceByKey[primaryKey]; !supported || previous.Kind == "" {
		primaryKey = keys[0]
	}
	conflictKeys := make([]string, 0, len(keys)-1)
	for key := range evidenceByKey {
		if key != primaryKey {
			conflictKeys = append(conflictKeys, key)
		}
	}
	sort.Strings(conflictKeys)
	result := PathEvidenceState{
		Path:      evidenceByKey[primaryKey].path,
		Conflicts: make([]PathObservation, 0, len(conflictKeys)),
	}
	for _, key := range conflictKeys {
		result.Conflicts = append(result.Conflicts, evidenceByKey[key].path)
	}
	return result
}

func PathEvidenceKey(path PathObservation) string {
	switch path.Kind {
	case PathDirect:
		return string(PathDirect)
	case PathDERP:
		region := strings.ToLower(strings.TrimSpace(path.DERPRegion))
		if region == "" {
			region = "unknown"
		}
		return string(PathDERP) + ":" + region
	case PathPeerRelay:
		stableID := strings.TrimSpace(path.PeerRelayStableNodeID)
		if stableID != "" {
			return string(PathPeerRelay) + ":" + stableID
		}
		if endpoint := strings.TrimSpace(path.PeerRelayEndpoint); endpoint != "" {
			return string(PathPeerRelay) + ":endpoint:" + endpoint
		}
		if path.PeerRelayVNI != nil {
			return string(PathPeerRelay) + ":vni:" + fmt.Sprint(*path.PeerRelayVNI)
		}
		return string(PathPeerRelay) + ":unknown"
	default:
		return string(PathUnknown)
	}
}

func SamePathEvidence(left, right PathEvidenceState) bool {
	if PathEvidenceKey(left.Path) != PathEvidenceKey(right.Path) || len(left.Conflicts) != len(right.Conflicts) {
		return false
	}
	for index := range left.Conflicts {
		if PathEvidenceKey(left.Conflicts[index]) != PathEvidenceKey(right.Conflicts[index]) {
			return false
		}
	}
	return true
}

func observerEvidenceRole(sourceID, targetID, observerID string) int {
	if observerID == sourceID {
		return 0
	}
	if observerID == targetID {
		return 1
	}
	return 2
}

func enrichEvidencePath(path, detail PathObservation) PathObservation {
	result := path
	switch result.Kind {
	case PathDERP:
		if result.DERPRegion == "" {
			result.DERPRegion = detail.DERPRegion
		}
	case PathPeerRelay:
		if result.PeerRelayStableNodeID == "" {
			result.PeerRelayStableNodeID = detail.PeerRelayStableNodeID
		}
		if result.PeerRelayVNI == nil {
			result.PeerRelayVNI = detail.PeerRelayVNI
		}
		if result.PeerRelayEndpoint == "" {
			result.PeerRelayEndpoint = detail.PeerRelayEndpoint
		}
		if result.PeerRelayResolution == "" {
			result.PeerRelayResolution = detail.PeerRelayResolution
		}
	}
	return result
}

func uniqueRelayStableIDsByEndpoint(observations []ObservationProvenance) map[string]string {
	candidates := make(map[string]map[string]struct{})
	for _, observation := range observations {
		path := observation.Path
		endpoint := strings.TrimSpace(path.PeerRelayEndpoint)
		stableID := strings.TrimSpace(path.PeerRelayStableNodeID)
		if path.Kind != PathPeerRelay || endpoint == "" || stableID == "" {
			continue
		}
		if candidates[endpoint] == nil {
			candidates[endpoint] = make(map[string]struct{})
		}
		candidates[endpoint][stableID] = struct{}{}
	}
	result := make(map[string]string)
	for endpoint, stableIDs := range candidates {
		if len(stableIDs) != 1 {
			continue
		}
		for stableID := range stableIDs {
			result[endpoint] = stableID
		}
	}
	return result
}
