package tailscaleadapter

import (
	"context"
	"fmt"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/tailscalestatus"
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

func (s *LocalSource) Snapshot(ctx context.Context) (exporter.Snapshot, error) {
	status, err := s.client.Status(ctx)
	if err != nil {
		return exporter.Snapshot{}, err
	}
	return tailscalestatus.Snapshot(status, time.Now())
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
		Self:      domainIdentity(peerIdentity(status.Self)),
		OS:        normalizeOS(status.Self.OS),
		PeerCount: peerCount,
	}, nil
}

func normalizeOS(value string) string {
	return tailscalestatus.NormalizeOS(value)
}

func peerIdentity(peer *ipnstate.PeerStatus) exporter.NodeIdentity {
	return tailscalestatus.PeerIdentity(peer)
}

func pathObservation(peer *ipnstate.PeerStatus, relayByIP map[string]string) exporter.Path {
	return tailscalestatus.Path(peer, relayByIP)
}

func domainIdentity(identity exporter.NodeIdentity) domain.NodeIdentity {
	return domain.NodeIdentity{
		StableNodeID: identity.StableNodeID, NodeID: identity.NodeID, NodeKey: identity.NodeKey,
		DiscoKey: identity.DiscoKey, Hostname: identity.Hostname, DNSName: identity.DNSName, OS: identity.OS,
		TailscaleIPs: append([]string(nil), identity.TailscaleIPs...),
	}
}

func relayIdentities(status *ipnstate.Status) map[string]string {
	return tailscalestatus.RelayIdentities(status)
}

func peerRelayIP(value string) string {
	return tailscalestatus.PeerRelayIP(value)
}

func peerRelayEndpoint(value string) (string, *int64) {
	return tailscalestatus.PeerRelayEndpoint(value)
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
