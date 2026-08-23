package tailscaleadapter

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"

	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
)

type LocalSource struct {
	client *local.Client
}

func NewLocalSource(socket string) *LocalSource {
	return &LocalSource{client: &local.Client{Socket: socket, UseSocketOnly: socket != ""}}
}

func NewLocalSourceWithClient(client *local.Client) *LocalSource {
	return &LocalSource{client: client}
}

func (s *LocalSource) Snapshot(ctx context.Context) (collector.Snapshot, error) {
	status, err := s.client.Status(ctx)
	if err != nil {
		return collector.Snapshot{}, err
	}
	if status.Self == nil {
		return collector.Snapshot{}, fmt.Errorf("tailscale status does not include self")
	}
	relays := relayIdentities(status)
	snapshot := collector.Snapshot{
		CollectedAt: time.Now(),
		Observer:    peerIdentity(status.Self),
		Peers:       make([]collector.PeerSnapshot, 0, len(status.Peer)),
	}
	for _, peer := range status.Peer {
		if peer == nil {
			continue
		}
		snapshot.Peers = append(snapshot.Peers, collector.PeerSnapshot{
			Identity: peerIdentity(peer),
			RxBytes:  peer.RxBytes,
			TxBytes:  peer.TxBytes,
			Path:     pathObservation(peer, relays),
		})
	}
	return snapshot, nil
}

func (s *LocalSource) Diagnostic(ctx context.Context) (collector.Diagnostic, error) {
	status, err := s.client.Status(ctx)
	if err != nil {
		return collector.Diagnostic{}, err
	}
	if status.Self == nil {
		return collector.Diagnostic{}, fmt.Errorf("tailscale status does not include self")
	}
	peerCount := 0
	for _, peer := range status.Peer {
		if peer != nil {
			peerCount++
		}
	}
	return collector.Diagnostic{
		Self:      peerIdentity(status.Self),
		OS:        normalizeOS(status.Self.OS),
		PeerCount: peerCount,
	}, nil
}

func normalizeOS(value string) string {
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

func peerIdentity(peer *ipnstate.PeerStatus) domain.NodeIdentity {
	ips := make([]string, 0, len(peer.TailscaleIPs))
	for _, ip := range peer.TailscaleIPs {
		ips = append(ips, ip.String())
	}
	identity := domain.NodeIdentity{
		StableNodeID: string(peer.ID),
		Hostname:     peer.HostName,
		DNSName:      peer.DNSName,
		OS:           normalizeOS(peer.OS),
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

func pathObservation(peer *ipnstate.PeerStatus, relayByIP map[string]string) domain.PathObservation {
	if peer.PeerRelay != "" {
		return domain.PathObservation{
			Kind:                  domain.PathPeerRelay,
			PeerRelayStableNodeID: relayByIP[peerRelayIP(peer.PeerRelay)],
		}
	}
	if peer.CurAddr != "" {
		return domain.PathObservation{Kind: domain.PathDirect, DirectEndpoint: peer.CurAddr}
	}
	if peer.Relay != "" {
		return domain.PathObservation{Kind: domain.PathDERP, DERPRegion: peer.Relay}
	}
	return domain.PathObservation{Kind: domain.PathUnknown}
}

func relayIdentities(status *ipnstate.Status) map[string]string {
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

func peerRelayIP(value string) string {
	endpoint := value
	if marker := strings.LastIndex(value, ":vni:"); marker >= 0 {
		endpoint = value[:marker]
	}
	address, err := netip.ParseAddrPort(endpoint)
	if err != nil {
		return ""
	}
	return address.Addr().Unmap().String()
}

type Authorizer struct {
	client *local.Client
}

func NewAuthorizer(client *local.Client) *Authorizer {
	return &Authorizer{client: client}
}

func (a *Authorizer) Authorize(ctx context.Context, remoteAddr string) (string, error) {
	who, err := a.client.WhoIs(ctx, remoteAddr)
	if err != nil {
		return "", err
	}
	if who.Node == nil || who.Node.StableID == "" {
		return "", fmt.Errorf("WhoIs response does not include a stable node identity")
	}
	return string(who.Node.StableID), nil
}

func ControlStableNodeIDs(ctx context.Context, client *local.Client) ([]string, error) {
	status, err := client.StatusWithoutPeers(ctx)
	if err != nil {
		return nil, err
	}
	if status.Self == nil || status.Self.ID == "" {
		return nil, fmt.Errorf("tailscale status does not include a stable self identity")
	}
	return []string{string(status.Self.ID)}, nil
}
