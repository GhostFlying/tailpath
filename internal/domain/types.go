package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const ProtocolVersion = 1

type ReportKind string

const (
	ReportObserverHello      ReportKind = "observer_hello"
	ReportInventoryUpdate    ReportKind = "inventory_update"
	ReportTrafficSample      ReportKind = "traffic_sample"
	ReportObserverHeartbeat  ReportKind = "observer_heartbeat"
	ReportRelaySessionUpdate ReportKind = "relay_session_update"
)

type PathKind string

const (
	PathDirect    PathKind = "direct"
	PathDERP      PathKind = "derp"
	PathPeerRelay PathKind = "peer_relay"
	PathUnknown   PathKind = "unknown"
)

type EdgeState string

const (
	EdgeActive EdgeState = "active"
	EdgeRecent EdgeState = "recent"
)

type NodeIdentity struct {
	StableNodeID string   `json:"stableNodeId"`
	NodeID       string   `json:"nodeId,omitempty"`
	NodeKey      string   `json:"nodeKey,omitempty"`
	DiscoKey     string   `json:"discoKey,omitempty"`
	Hostname     string   `json:"hostname"`
	DNSName      string   `json:"dnsName,omitempty"`
	OS           string   `json:"os,omitempty"`
	TailscaleIPs []string `json:"tailscaleIps,omitempty"`
}

func (n NodeIdentity) CanonicalID() string {
	return n.IdentityKey()
}

func (n NodeIdentity) IdentityKey() string {
	for _, candidate := range []struct {
		prefix string
		value  string
	}{
		{"node", n.StableNodeID},
		{"node-id", n.NodeID},
		{"node-key", n.NodeKey},
		{"disco", n.DiscoKey},
	} {
		if candidate.value != "" {
			return candidate.prefix + ":" + candidate.value
		}
	}
	if len(n.TailscaleIPs) > 0 {
		ips := append([]string(nil), n.TailscaleIPs...)
		sort.Strings(ips)
		return "ip:" + ips[0]
	}
	return "unknown"
}

func (n NodeIdentity) HasIdentity() bool {
	return n.IdentityKey() != "unknown"
}

func (n NodeIdentity) DisplayName() string {
	if n.DNSName != "" {
		name := strings.TrimSuffix(n.DNSName, ".")
		if short, _, ok := strings.Cut(name, "."); ok {
			return short
		}
		return name
	}
	if n.Hostname != "" {
		return n.Hostname
	}
	return n.IdentityKey()
}

type PathObservation struct {
	Kind                  PathKind `json:"kind"`
	DirectEndpoint        string   `json:"directEndpoint,omitempty"`
	DERPRegion            string   `json:"derpRegion,omitempty"`
	PeerRelayStableNodeID string   `json:"peerRelayStableNodeId,omitempty"`
}

func (p PathObservation) Equal(other PathObservation) bool {
	return p.Kind == other.Kind && p.DirectEndpoint == other.DirectEndpoint &&
		p.DERPRegion == other.DERPRegion && p.PeerRelayStableNodeID == other.PeerRelayStableNodeID
}

type PeerObservation struct {
	Peer             NodeIdentity    `json:"peer"`
	RxBytes          int64           `json:"rxBytes"`
	TxBytes          int64           `json:"txBytes"`
	RxDelta          int64           `json:"rxDelta"`
	TxDelta          int64           `json:"txDelta"`
	SampleDurationMS int64           `json:"sampleDurationMs"`
	Path             PathObservation `json:"path"`
	LastActive       time.Time       `json:"lastActive"`
}

type ObserverReport struct {
	Observer            NodeIdentity      `json:"observer"`
	InventoryGeneration string            `json:"inventoryGeneration"`
	Peers               []PeerObservation `json:"peers,omitempty"`
}

type RelaySessionObservation struct {
	Relay               NodeIdentity `json:"relay"`
	Source              NodeIdentity `json:"source"`
	Target              NodeIdentity `json:"target"`
	SessionID           string       `json:"sessionId"`
	VNI                 int64        `json:"vni"`
	SourceEndpoint      string       `json:"sourceEndpoint,omitempty"`
	TargetEndpoint      string       `json:"targetEndpoint,omitempty"`
	SourceToTargetBytes int64        `json:"sourceToTargetBytes"`
	TargetToSourceBytes int64        `json:"targetToSourceBytes"`
	SourceToTargetDelta int64        `json:"sourceToTargetDelta"`
	TargetToSourceDelta int64        `json:"targetToSourceDelta"`
	SampleDurationMS    int64        `json:"sampleDurationMs"`
	LastActive          time.Time    `json:"lastActive"`
}

type ReportEnvelope struct {
	Version            int                       `json:"version"`
	ReportID           string                    `json:"reportId"`
	ReporterInstanceID string                    `json:"reporterInstanceId"`
	Sequence           int64                     `json:"sequence"`
	CollectedAt        time.Time                 `json:"collectedAt"`
	Kind               ReportKind                `json:"kind"`
	Observers          []ObserverReport          `json:"observers,omitempty"`
	RelaySessions      []RelaySessionObservation `json:"relaySessions,omitempty"`
}

func (r ReportEnvelope) Validate() error {
	if r.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version %d", r.Version)
	}
	if r.ReportID == "" || r.ReporterInstanceID == "" {
		return errors.New("reportId and reporterInstanceId are required")
	}
	if r.Sequence < 1 {
		return errors.New("sequence must be positive")
	}
	if r.CollectedAt.IsZero() {
		return errors.New("collectedAt is required")
	}
	switch r.Kind {
	case ReportObserverHello, ReportInventoryUpdate, ReportTrafficSample,
		ReportObserverHeartbeat, ReportRelaySessionUpdate:
	default:
		return fmt.Errorf("unknown report kind %q", r.Kind)
	}
	if r.Kind == ReportRelaySessionUpdate {
		if len(r.Observers) != 0 {
			return errors.New("relay session updates cannot contain observer peer reports")
		}
		if len(r.RelaySessions) == 0 {
			return errors.New("relay session updates require at least one session")
		}
		for _, session := range r.RelaySessions {
			if err := session.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if len(r.RelaySessions) != 0 {
		return errors.New("relay sessions require relay_session_update kind")
	}
	if len(r.Observers) == 0 {
		return errors.New("at least one observer is required")
	}
	for _, observer := range r.Observers {
		if !observer.Observer.HasIdentity() {
			return errors.New("observer identity is required")
		}
		if observer.InventoryGeneration == "" {
			return errors.New("inventoryGeneration is required")
		}
		for _, peer := range observer.Peers {
			if !peer.Peer.HasIdentity() {
				return errors.New("peer identity is required")
			}
			if peer.RxBytes < 0 || peer.TxBytes < 0 || peer.RxDelta < 0 || peer.TxDelta < 0 {
				return errors.New("traffic counters cannot be negative")
			}
			if peer.SampleDurationMS < 0 {
				return errors.New("sampleDurationMs cannot be negative")
			}
			if r.Kind == ReportTrafficSample && peer.SampleDurationMS < 1 {
				return errors.New("traffic samples require a positive sampleDurationMs")
			}
		}
	}
	return nil
}

func (s RelaySessionObservation) Validate() error {
	if !s.Relay.HasIdentity() || !s.Source.HasIdentity() || !s.Target.HasIdentity() {
		return errors.New("relay, source, and target identities are required")
	}
	if strings.TrimSpace(s.Relay.StableNodeID) == "" {
		return errors.New("relay StableNodeID is required")
	}
	if s.Source.IdentityKey() == s.Target.IdentityKey() {
		return errors.New("relay session source and target must differ")
	}
	if strings.TrimSpace(s.SessionID) == "" {
		return errors.New("relay sessionId is required")
	}
	if s.VNI < 0 {
		return errors.New("relay VNI cannot be negative")
	}
	if s.SourceToTargetBytes < 0 || s.TargetToSourceBytes < 0 ||
		s.SourceToTargetDelta < 0 || s.TargetToSourceDelta < 0 {
		return errors.New("relay traffic counters cannot be negative")
	}
	if s.SourceToTargetDelta == 0 && s.TargetToSourceDelta == 0 {
		return errors.New("relay session updates require a traffic delta")
	}
	if s.SampleDurationMS < 1 {
		return errors.New("relay sessions require a positive sampleDurationMs")
	}
	if s.LastActive.IsZero() {
		return errors.New("relay session lastActive is required")
	}
	return nil
}

type ReportReceipt struct {
	Accepted             bool     `json:"accepted"`
	ResyncRequired       bool     `json:"resyncRequired"`
	ControlStableNodeIDs []string `json:"controlStableNodeIds"`
	HeartbeatIntervalMS  int64    `json:"heartbeatIntervalMs"`
}

type TopologyNode struct {
	NodeIdentity
	ID             string    `json:"id"`
	Observable     bool      `json:"observable"`
	Online         bool      `json:"online"`
	LastEvidenceAt time.Time `json:"lastEvidenceAt"`
	ClockSkewed    bool      `json:"clockSkewed"`
}

type ObservationProvenance struct {
	ObserverID  string          `json:"observerId"`
	Path        PathObservation `json:"path"`
	CollectedAt time.Time       `json:"collectedAt"`
	ReceivedAt  time.Time       `json:"receivedAt"`
	ClockSkewed bool            `json:"clockSkewed"`
}

type TopologyEdge struct {
	ID                 string                  `json:"id"`
	Source             string                  `json:"source"`
	Target             string                  `json:"target"`
	Path               PathObservation         `json:"path"`
	State              EdgeState               `json:"state"`
	AToBBytesPerSecond float64                 `json:"aToBBytesPerSecond"`
	BToABytesPerSecond float64                 `json:"bToABytesPerSecond"`
	LastActive         time.Time               `json:"lastActive"`
	Observations       []ObservationProvenance `json:"observations"`
	Conflicts          []PathObservation       `json:"conflicts,omitempty"`
}

type ObserverState struct {
	ID              string    `json:"id"`
	Hostname        string    `json:"hostname"`
	Online          bool      `json:"online"`
	LastSeen        time.Time `json:"lastSeen"`
	LastCollectedAt time.Time `json:"lastCollectedAt"`
	ClockSkewMS     int64     `json:"clockSkewMs"`
	ClockSkewed     bool      `json:"clockSkewed"`
}

type Topology struct {
	GeneratedAt  time.Time       `json:"generatedAt"`
	NextChangeAt *time.Time      `json:"nextChangeAt,omitempty"`
	Nodes        []TopologyNode  `json:"nodes"`
	Edges        []TopologyEdge  `json:"edges"`
	Observers    []ObserverState `json:"observers"`
}

type TrafficBucket struct {
	BucketStart time.Time `json:"bucketStart"`
	AToBBytes   int64     `json:"aToBBytes"`
	BToABytes   int64     `json:"bToABytes"`
}

type PathEvent struct {
	ObservedAt   time.Time               `json:"observedAt"`
	Path         PathObservation         `json:"path"`
	Observations []ObservationProvenance `json:"observations"`
}

type AcceptedTraffic struct {
	EdgeID     string
	SourceID   string
	TargetID   string
	ObserverID string
	AToBBytes  int64
	BToABytes  int64
	ReceivedAt time.Time
}

type PathTransition struct {
	EdgeID       string
	ObservedAt   time.Time
	Path         PathObservation
	Observations []ObservationProvenance
}

type EdgeHistory struct {
	EdgeID              string               `json:"edgeId"`
	Source              HistoryNodeReference `json:"source"`
	Target              HistoryNodeReference `json:"target"`
	From                time.Time            `json:"from"`
	To                  time.Time            `json:"to"`
	BucketDurationMS    int64                `json:"bucketDurationMs"`
	Traffic             []TrafficBucket      `json:"traffic"`
	PathAnchor          *PathEvent           `json:"pathAnchor,omitempty"`
	PathEvents          []PathEvent          `json:"pathEvents"`
	TrafficTruncated    bool                 `json:"trafficTruncated"`
	PathEventsTruncated bool                 `json:"pathEventsTruncated"`
}

type HistoryMetadata struct {
	Nodes     []TopologyNode
	Redirects map[string]string
}

type HistoryWindow string

const (
	History15Minutes HistoryWindow = "15m"
	History1Hour     HistoryWindow = "1h"
	History6Hours    HistoryWindow = "6h"
	History24Hours   HistoryWindow = "24h"
	History7Days     HistoryWindow = "7d"
)

func (window HistoryWindow) Duration() time.Duration {
	switch window {
	case History15Minutes:
		return 15 * time.Minute
	case History1Hour:
		return time.Hour
	case History6Hours:
		return 6 * time.Hour
	case History24Hours:
		return 24 * time.Hour
	case History7Days:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

func (window HistoryWindow) Resolution() time.Duration {
	switch window {
	case History15Minutes:
		return 10 * time.Second
	case History1Hour:
		return 30 * time.Second
	case History6Hours:
		return 3 * time.Minute
	case History24Hours:
		return 12 * time.Minute
	case History7Days:
		return time.Hour
	default:
		return 0
	}
}

func (window HistoryWindow) Valid() bool {
	return window.Duration() > 0
}

type HistoryNodeReference struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Hostname string `json:"hostname,omitempty"`
	DNSName  string `json:"dnsName,omitempty"`
	OS       string `json:"os,omitempty"`
}

type HistoryNodes struct {
	Nodes []HistoryNodeReference `json:"nodes"`
}

type HistoryEdgeSummary struct {
	EdgeID        string               `json:"edgeId"`
	Source        HistoryNodeReference `json:"source"`
	Target        HistoryNodeReference `json:"target"`
	LastTrafficAt time.Time            `json:"lastTrafficAt"`
	AToBBytes     int64                `json:"aToBBytes"`
	BToABytes     int64                `json:"bToABytes"`
	Paths         []PathKind           `json:"paths"`
}

type HistoryEdgePage struct {
	Edges      []HistoryEdgeSummary `json:"edges"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

type HistoryEdgeQuery struct {
	Window HistoryWindow
	NodeID string
	Path   PathKind
	Cursor string
	Limit  int
}

func EdgeID(a, b string) (id, source, target string) {
	if a > b {
		a, b = b, a
	}
	return a + "--" + b, a, b
}
