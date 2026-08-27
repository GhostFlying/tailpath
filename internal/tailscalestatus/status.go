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
		StableNodeID: string(peer.ID),
		Hostname:     peer.HostName,
		DNSName:      peer.DNSName,
		OS:           NormalizeOS(peer.OS),
		TailscaleIPs: ips,
	}
	if peer.NodeID != 0 {
		identity.NodeID = fmt.Sprint(peer.NodeID)
	}
	if !peer.PublicKey.IsZero() {
		identity.NodeKey = peer.PublicKey.String()
	}
	return identity
}

func Path(peer *ipnstate.PeerStatus, relayByIP map[string]string) exporter.Path {
	if peer.PeerRelay != "" {
		relayIP, vni := PeerRelayEndpoint(peer.PeerRelay)
		return exporter.Path{
			Kind:                  exporter.PathPeerRelay,
			PeerRelayStableNodeID: relayByIP[relayIP],
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

func RelayIdentities(status *ipnstate.Status) map[string]string {
	result := make(map[string]string)
	for _, peer := range status.Peer {
		if peer == nil || peer.ID == "" {
			continue
		}
		for _, ip := range peer.TailscaleIPs {
			result[ip.String()] = string(peer.ID)
		}
	}
	return result
}

func PeerRelayIP(value string) string {
	address, _ := PeerRelayEndpoint(value)
	return address
}

func PeerRelayEndpoint(value string) (string, *int64) {
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
		return "", nil
	}
	return address.Addr().Unmap().String(), vni
}
