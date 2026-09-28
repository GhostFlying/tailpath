package domain

import "time"

type PathState string

const (
	PathStable    PathState = "stable"
	PathSwitching PathState = "switching"
)

type PathCandidate struct {
	Path           PathObservation `json:"path"`
	LastObservedAt time.Time       `json:"lastObservedAt"`
	ObserverCount  int             `json:"observerCount"`
}

func PathCandidates(path PathObservation, conflicts []PathObservation, observations []ObservationProvenance) (PathState, []PathCandidate) {
	paths := append([]PathObservation{path}, conflicts...)
	candidates := make([]PathCandidate, 0, len(paths))
	for _, candidatePath := range paths {
		if candidatePath.Kind != PathPeerRelay {
			continue
		}
		candidate := PathCandidate{Path: candidatePath}
		observers := make(map[string]struct{})
		for _, observation := range observations {
			if !sameRelayCandidate(candidatePath, observation.Path) {
				continue
			}
			observers[observation.ObserverID] = struct{}{}
			if observation.ReceivedAt.After(candidate.LastObservedAt) {
				candidate.LastObservedAt = observation.ReceivedAt
			}
		}
		candidate.ObserverCount = len(observers)
		candidates = append(candidates, candidate)
	}
	state := PathStable
	if len(candidates) > 1 {
		state = PathSwitching
	}
	return state, candidates
}

func sameRelayCandidate(left, right PathObservation) bool {
	if left.Kind != PathPeerRelay || right.Kind != PathPeerRelay {
		return false
	}
	if left.PeerRelayStableNodeID != "" && right.PeerRelayStableNodeID != "" {
		return left.PeerRelayStableNodeID == right.PeerRelayStableNodeID
	}
	if left.PeerRelayEndpoint != "" && right.PeerRelayEndpoint != "" {
		return left.PeerRelayEndpoint == right.PeerRelayEndpoint
	}
	return PathEvidenceKey(left) == PathEvidenceKey(right)
}
