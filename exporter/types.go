// Package exporter provides the alpha public contracts for reporting passive
// Tailscale runtime observations to Tailpath.
package exporter

import (
	"context"
	"sort"
	"strings"
	"time"
)

const ProtocolVersion = 1

const (
	FeatureMultiObserver      = "multi-observer"
	FeatureObserverWithdrawal = "observer-withdrawal"
)

type Capabilities struct {
	ObserverProtocolVersions []int    `json:"observerProtocolVersions"`
	Features                 []string `json:"features"`
}

func (c Capabilities) SupportsProtocol(version int) bool {
	for _, candidate := range c.ObserverProtocolVersions {
		if candidate == version {
			return true
		}
	}
	return false
}

func (c Capabilities) SupportsFeature(feature string) bool {
	for _, candidate := range c.Features {
		if candidate == feature {
			return true
		}
	}
	return false
}

type ReportKind string

const (
	ReportObserverHello      ReportKind = "observer_hello"
	ReportInventoryUpdate    ReportKind = "inventory_update"
	ReportTrafficSample      ReportKind = "traffic_sample"
	ReportObserverHeartbeat  ReportKind = "observer_heartbeat"
	ReportObserverWithdrawal ReportKind = "observer_withdrawal"
	ReportRelaySessionUpdate ReportKind = "relay_session_update"
)

type PathKind string

const (
	PathDirect    PathKind = "direct"
	PathDERP      PathKind = "derp"
	PathPeerRelay PathKind = "peer_relay"
	PathUnknown   PathKind = "unknown"
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

type Path struct {
	Kind                  PathKind `json:"kind"`
	DirectEndpoint        string   `json:"directEndpoint,omitempty"`
	DERPRegion            string   `json:"derpRegion,omitempty"`
	PeerRelayStableNodeID string   `json:"peerRelayStableNodeId,omitempty"`
	PeerRelayVNI          *int64   `json:"peerRelayVni,omitempty"`
}

func (p Path) Equal(other Path) bool {
	return p.Kind == other.Kind && p.DirectEndpoint == other.DirectEndpoint &&
		p.DERPRegion == other.DERPRegion && p.PeerRelayStableNodeID == other.PeerRelayStableNodeID &&
		(p.PeerRelayVNI == nil && other.PeerRelayVNI == nil ||
			p.PeerRelayVNI != nil && other.PeerRelayVNI != nil && *p.PeerRelayVNI == *other.PeerRelayVNI)
}

type Snapshot struct {
	CollectedAt time.Time
	Observer    NodeIdentity
	Peers       []PeerSnapshot
}

type PeerSnapshot struct {
	Identity NodeIdentity
	RxBytes  int64
	TxBytes  int64
	Path     Path
}

// Source provides passive snapshots for one Tailscale runtime. Implementations
// must return promptly when the context is canceled or its deadline expires.
// SnapshotSink does not overlap Snapshot calls for one source, and Run waits
// for an in-flight call to return during shutdown.
type Source interface {
	Snapshot(context.Context) (Snapshot, error)
}

// RelaySource is an optional capability implemented by sources that can
// passively observe Peer Relay sessions in addition to ordinary peer state.
// SnapshotSink samples this capability independently so relay failures cannot
// delay ordinary observations. Implementations have the same cancellation and
// shutdown obligations as Source.
type RelaySource interface {
	PeerRelaySnapshot(context.Context) (RelaySnapshot, error)
}

type RelayCapability string

type RelayIdentityEvidence string

const (
	RelayOff              RelayCapability = "off"
	RelayUnsupported      RelayCapability = "unsupported"
	RelayDisabled         RelayCapability = "disabled"
	RelayEnabled          RelayCapability = "enabled"
	RelayTransientFailure RelayCapability = "transient_failure"

	RelayIdentityAvailable RelayIdentityEvidence = "available"
	RelayIdentityDegraded  RelayIdentityEvidence = "degraded"
)

type RelaySnapshot struct {
	CollectedAt      time.Time
	Capability       RelayCapability
	IdentityEvidence RelayIdentityEvidence
	Sessions         []RelaySessionSnapshot
}

type RelaySessionSnapshot struct {
	SessionID string
	VNI       int64
	Source    RelayClientSnapshot
	Target    RelayClientSnapshot
}

type RelayClientSnapshot struct {
	SessionClientID string
	Identity        *NodeIdentity
	DiscoShort      string
	Endpoint        string
	PacketsSent     uint64
	BytesSent       uint64
}

type Reporter interface {
	Capabilities(context.Context) (Capabilities, error)
	Send(context.Context, ReportEnvelope) (ReportReceipt, error)
}

type PeerObservation struct {
	Peer             NodeIdentity `json:"peer"`
	RxBytes          int64        `json:"rxBytes"`
	TxBytes          int64        `json:"txBytes"`
	RxDelta          int64        `json:"rxDelta"`
	TxDelta          int64        `json:"txDelta"`
	SampleDurationMS int64        `json:"sampleDurationMs"`
	Path             Path         `json:"path"`
	LastActive       time.Time    `json:"lastActive"`
}

type ObserverReport struct {
	Observer            NodeIdentity      `json:"observer"`
	InventoryGeneration string            `json:"inventoryGeneration"`
	CollectedAt         *time.Time        `json:"collectedAt,omitempty"`
	Peers               []PeerObservation `json:"peers,omitempty"`
}

type RelaySessionClient struct {
	SessionClientID string        `json:"sessionClientId"`
	Identity        *NodeIdentity `json:"identity,omitempty"`
	DiscoShort      string        `json:"discoShort,omitempty"`
	Endpoint        string        `json:"endpoint,omitempty"`
}

type RelaySessionObservation struct {
	Relay               NodeIdentity       `json:"relay"`
	Source              RelaySessionClient `json:"source"`
	Target              RelaySessionClient `json:"target"`
	SessionID           string             `json:"sessionId"`
	VNI                 int64              `json:"vni"`
	SourceToTargetBytes int64              `json:"sourceToTargetBytes"`
	TargetToSourceBytes int64              `json:"targetToSourceBytes"`
	SourceToTargetDelta int64              `json:"sourceToTargetDelta"`
	TargetToSourceDelta int64              `json:"targetToSourceDelta"`
	SampleDurationMS    int64              `json:"sampleDurationMs"`
	LastActive          time.Time          `json:"lastActive"`
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

type ReportReceipt struct {
	Accepted             bool     `json:"accepted"`
	ResyncRequired       bool     `json:"resyncRequired"`
	ControlStableNodeIDs []string `json:"controlStableNodeIds"`
	HeartbeatIntervalMS  int64    `json:"heartbeatIntervalMs"`
}
