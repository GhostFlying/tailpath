// Package tailscalestatus converts Tailscale runtime status into Tailpath's
// transport-independent exporter snapshot.
package tailscalestatus

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"tailscale.com/ipn/ipnstate"

	"github.com/GhostFlying/tailpath/exporter"
)

func Snapshot(status *ipnstate.Status, collectedAt time.Time) (exporter.Snapshot, error) {
	if status == nil {
		return exporter.Snapshot{}, errors.New("tailscale status is unavailable")
	}
	if status.Self == nil {
		return exporter.Snapshot{}, errors.New("tailscale status does not include self")
	}
	relays := RelayIdentities(status)
	snapshot := exporter.Snapshot{
		CollectedAt: collectedAt,
		Observer:    PeerIdentity(status.Self),
		Peers:       make([]exporter.PeerSnapshot, 0, len(status.Peer)),
	}
	for _, peer := range status.Peer {
		if peer == nil {
			continue
		}
		snapshot.Peers = append(snapshot.Peers, exporter.PeerSnapshot{
			Identity: PeerIdentity(peer),
			RxBytes:  peer.RxBytes,
			TxBytes:  peer.TxBytes,
			Path:     Path(peer, relays),
		})
	}
	return snapshot, nil
}

func NormalizeOS(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "linux":
		return "linux"
	case "darwin", "macos":
		return "macos"
	case "windows":
		return "windows"
	case "ios":
		return "ios"
	case "android":
		return "android"
	default:
		return value
	}
}

func PeerIdentity(peer *ipnstate.PeerStatus) exporter.NodeIdentity {
	ips := make([]string, 0, len(peer.TailscaleIPs))
	for _, ip := range peer.TailscaleIPs {
		ips = append(ips, ip.String())
	}
	identity := exporter.NodeIdentity{
		StableNodeID:    string(peer.ID),
		Hostname:        peer.HostName,
		DNSName:         peer.DNSName,
		OS:              NormalizeOS(peer.OS),
		TailscaleIPs:    ips,
		PublicEndpoints: PublicEndpoints(peer),
	}
	if peer.NodeID != 0 {
		identity.NodeID = fmt.Sprint(peer.NodeID)
	}
	if !peer.PublicKey.IsZero() {
		identity.NodeKey = peer.PublicKey.String()
	}
	return identity
}

type RelayIdentity struct {
	StableNodeID string
	Resolution   string
}

func Path(peer *ipnstate.PeerStatus, relayByIP map[string]RelayIdentity) exporter.Path {
	if peer.PeerRelay != "" {
		endpoint, relayIP, vni := ParsePeerRelay(peer.PeerRelay)
		relay := relayByIP[relayIP]
		return exporter.Path{
			Kind:                  exporter.PathPeerRelay,
			PeerRelayStableNodeID: relay.StableNodeID,
			PeerRelayEndpoint:     endpoint,
			PeerRelayResolution:   relay.Resolution,
			PeerRelayVNI:          vni,
		}
	}
	if peer.CurAddr != "" {
		return exporter.Path{Kind: exporter.PathDirect, DirectEndpoint: peer.CurAddr}
	}
	if peer.Relay != "" {
		return exporter.Path{Kind: exporter.PathDERP, DERPRegion: peer.Relay}
	}
	return exporter.Path{Kind: exporter.PathUnknown}
}

func RelayIdentities(status *ipnstate.Status) map[string]RelayIdentity {
	owners := make(map[string]map[string]string)
	for _, peer := range status.Peer {
		if peer == nil || peer.ID == "" {
			continue
		}
		addOwner := func(ip, resolution string) {
			if ip == "" {
				return
			}
			if owners[ip] == nil {
				owners[ip] = make(map[string]string)
			}
			stableID := string(peer.ID)
			if owners[ip][stableID] != "tailscale_ip" {
				owners[ip][stableID] = resolution
			}
		}
		for _, ip := range peer.TailscaleIPs {
			addOwner(ip.Unmap().String(), "tailscale_ip")
		}
		for _, endpoint := range PublicEndpoints(peer) {
			address, err := netip.ParseAddrPort(endpoint)
			if err == nil {
				addOwner(address.Addr().Unmap().String(), "endpoint_match")
			}
		}
	}
	result := make(map[string]RelayIdentity)
	for ip, candidates := range owners {
		if len(candidates) != 1 {
			continue
		}
		for stableID, resolution := range candidates {
			result[ip] = RelayIdentity{StableNodeID: stableID, Resolution: resolution}
		}
	}
	return result
}

func PublicEndpoints(peer *ipnstate.PeerStatus) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(peer.Addrs)+1)
	add := func(value string) {
		address, err := netip.ParseAddrPort(value)
		if err != nil || !isPublicEndpointAddress(address.Addr()) {
			return
		}
		normalized := netip.AddrPortFrom(address.Addr().Unmap(), address.Port()).String()
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	for _, value := range peer.Addrs {
		add(value)
	}
	add(peer.CurAddr)
	return result
}

func isPublicEndpointAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsValid() && address.IsGlobalUnicast() && !address.IsPrivate() &&
		!address.IsLoopback() && !address.IsLinkLocalUnicast() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(address)
}

func PeerRelayIP(value string) string {
	_, address, _ := ParsePeerRelay(value)
	return address
}

func PeerRelayEndpoint(value string) (string, *int64) {
	_, address, vni := ParsePeerRelay(value)
	return address, vni
}

func ParsePeerRelay(value string) (string, string, *int64) {
	endpoint := value
	var vni *int64
	if marker := strings.LastIndex(value, ":vni:"); marker >= 0 {
		endpoint = value[:marker]
		parsed, err := strconv.ParseUint(value[marker+len(":vni:"):], 10, 24)
		if err == nil {
			converted := int64(parsed)
			vni = &converted
		}
	}
	address, err := netip.ParseAddrPort(endpoint)
	if err != nil {
		return "", "", nil
	}
	normalized := netip.AddrPortFrom(address.Addr().Unmap(), address.Port())
	return normalized.String(), normalized.Addr().String(), vni
}
