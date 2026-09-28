package domain

import (
	"sort"
	"strings"
)

// DirectionalPaths projects only endpoint-owned observations into outbound
// path state. Third-party observations may enrich a selected relay identity,
// but never select an endpoint's active path.
func DirectionalPaths(sourceID, targetID string, observations []ObservationProvenance) []DirectionalPathState {
	stableIDByEndpoint := uniqueRelayStableIDsByEndpoint(observations)
	result := make([]DirectionalPathState, 0, 2)
	for _, observation := range observations {
		var toNodeID string
		switch observation.ObserverID {
		case sourceID:
			toNodeID = targetID
		case targetID:
			toNodeID = sourceID
		default:
			continue
		}
		primary := enrichDirectionalRelay(observation.Path, stableIDByEndpoint)
		fallback := cloneDirectionalPath(observation.FallbackPath)
		if fallback != nil {
			value := enrichDirectionalRelay(*fallback, stableIDByEndpoint)
			fallback = &value
		}
		evidence := observation.PathEvidence
		if evidence == "" {
			evidence = PathEvidenceObserved
		}
		result = append(result, DirectionalPathState{
			FromNodeID: observation.ObserverID, ToNodeID: toNodeID,
			PrimaryPath: primary, FallbackPath: fallback, Evidence: evidence,
			InferenceRule: observation.PathInferenceRule, ObserverID: observation.ObserverID,
			CollectedAt: observation.CollectedAt, ReceivedAt: observation.ReceivedAt,
			ClockSkewed: observation.ClockSkewed,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FromNodeID == sourceID {
			return result[j].FromNodeID != sourceID
		}
		if result[j].FromNodeID == sourceID {
			return false
		}
		return result[i].FromNodeID < result[j].FromNodeID
	})
	return result
}

func LegacyDirectionalPaths(sourceID, targetID string, observations []ObservationProvenance) []DirectionalPathState {
	result := DirectionalPaths(sourceID, targetID, observations)
	for index := range result {
		result[index].Evidence = PathEvidenceLegacy
		result[index].InferenceRule = ""
		result[index].FallbackPath = nil
	}
	return result
}

func SameDirectionalPaths(left, right []DirectionalPathState) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].FromNodeID != right[index].FromNodeID || left[index].ToNodeID != right[index].ToNodeID ||
			left[index].ObserverID != right[index].ObserverID || left[index].Evidence != right[index].Evidence ||
			left[index].InferenceRule != right[index].InferenceRule ||
			PathEvidenceKey(left[index].PrimaryPath) != PathEvidenceKey(right[index].PrimaryPath) ||
			optionalPathEvidenceKey(left[index].FallbackPath) != optionalPathEvidenceKey(right[index].FallbackPath) {
			return false
		}
	}
	return true
}

func CloneDirectionalPaths(values []DirectionalPathState) []DirectionalPathState {
	result := append([]DirectionalPathState(nil), values...)
	for index := range result {
		result[index].FallbackPath = cloneDirectionalPath(result[index].FallbackPath)
	}
	return result
}

func RemapDirectionalPaths(values []DirectionalPathState, redirects map[string]string) []DirectionalPathState {
	result := CloneDirectionalPaths(values)
	for index := range result {
		result[index].FromNodeID = resolveDirectionalNodeID(redirects, result[index].FromNodeID)
		result[index].ToNodeID = resolveDirectionalNodeID(redirects, result[index].ToNodeID)
		result[index].ObserverID = resolveDirectionalNodeID(redirects, result[index].ObserverID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FromNodeID < result[j].FromNodeID })
	return result
}

func enrichDirectionalRelay(path PathObservation, stableIDByEndpoint map[string]string) PathObservation {
	if path.Kind != PathPeerRelay || path.PeerRelayStableNodeID != "" {
		return path
	}
	if stableID := stableIDByEndpoint[strings.TrimSpace(path.PeerRelayEndpoint)]; stableID != "" {
		path.PeerRelayStableNodeID = stableID
		path.PeerRelayResolution = "endpoint_match"
	}
	return path
}

func cloneDirectionalPath(path *PathObservation) *PathObservation {
	if path == nil {
		return nil
	}
	copy := *path
	if path.PeerRelayVNI != nil {
		value := *path.PeerRelayVNI
		copy.PeerRelayVNI = &value
	}
	return &copy
}

func optionalPathEvidenceKey(path *PathObservation) string {
	if path == nil {
		return ""
	}
	return PathEvidenceKey(*path)
}

func resolveDirectionalNodeID(redirects map[string]string, nodeID string) string {
	seen := make(map[string]struct{})
	for redirects[nodeID] != "" {
		if _, cycle := seen[nodeID]; cycle {
			break
		}
		seen[nodeID] = struct{}{}
		nodeID = redirects[nodeID]
	}
	return nodeID
}
