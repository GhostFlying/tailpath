package tailscaleadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/net/udprelay/status"
	"tailscale.com/types/key"

	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
)

const maxLocalAPIResponseBytes = 4 << 20

func (s *LocalSource) PeerRelaySnapshot(ctx context.Context) (collector.RelaySnapshot, error) {
	collectedAt := time.Now().UTC()
	var relayStatus status.ServerStatus
	statusCode, err := readLocalAPIJSON(ctx, s.client, http.MethodGet,
		"/localapi/v0/debug-peer-relay-sessions", &relayStatus)
	if err != nil {
		if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed {
			return collector.RelaySnapshot{CollectedAt: collectedAt, Capability: collector.RelayUnsupported}, nil
		}
		return collector.RelaySnapshot{CollectedAt: collectedAt, Capability: collector.RelayTransientFailure}, err
	}
	if relayStatus.UDPPort == nil {
		return collector.RelaySnapshot{CollectedAt: collectedAt, Capability: collector.RelayDisabled}, nil
	}
	if len(relayStatus.Sessions) == 0 {
		return collector.RelaySnapshot{
			CollectedAt: collectedAt, Capability: collector.RelayEnabled,
			Sessions: []collector.RelaySessionSnapshot{},
		}, nil
	}

	discoKeys, identityEvidence := readPeerDiscoKeys(ctx, s.client)
	sessions, err := adaptRelaySessions(relayStatus.Sessions, discoKeys)
	if err != nil {
		return collector.RelaySnapshot{CollectedAt: collectedAt, Capability: collector.RelayTransientFailure}, err
	}
	return collector.RelaySnapshot{
		CollectedAt: collectedAt, Capability: collector.RelayEnabled,
		IdentityEvidence: identityEvidence, Sessions: sessions,
	}, nil
}

func readPeerDiscoKeys(
	ctx context.Context,
	client *local.Client,
) (map[key.NodePublic]key.DiscoPublic, collector.RelayIdentityEvidence) {
	var result map[key.NodePublic]key.DiscoPublic
	_, err := readLocalAPIJSON(ctx, client, http.MethodPost,
		"/localapi/v0/debug?action=peer-disco-keys", &result)
	if err != nil {
		return nil, collector.RelayIdentityDegraded
	}
	return result, collector.RelayIdentityAvailable
}

func readLocalAPIJSON(ctx context.Context, client *local.Client, method, path string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://"+apitype.LocalAPIHost+path, nil)
	if err != nil {
		return 0, err
	}
	response, err := client.DoLocalRequest(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, fmt.Errorf("LocalAPI %s returned HTTP %d", path, response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxLocalAPIResponseBytes))
	if err := decoder.Decode(target); err != nil {
		return response.StatusCode, fmt.Errorf("decode LocalAPI %s: %w", path, err)
	}
	return response.StatusCode, nil
}

type discoIdentity struct {
	nodeKey  string
	discoKey string
}

func adaptRelaySessions(
	upstream []status.ServerSession,
	discoKeys map[key.NodePublic]key.DiscoPublic,
) ([]collector.RelaySessionSnapshot, error) {
	identities := uniqueDiscoIdentities(discoKeys)
	result := make([]collector.RelaySessionSnapshot, 0, len(upstream))
	for _, session := range upstream {
		clients := []collector.RelayClientSnapshot{
			adaptRelayClient(session.VNI, session.Client1, identities),
			adaptRelayClient(session.VNI, session.Client2, identities),
		}
		sort.Slice(clients, func(left, right int) bool {
			return relayClientSortKey(clients[left]) < relayClientSortKey(clients[right])
		})
		if relayClientSortKey(clients[0]) == relayClientSortKey(clients[1]) {
			clients[0].Identity = nil
			clients[1].Identity = nil
			if clients[0].Endpoint == clients[1].Endpoint {
				continue
			}
			for index := range clients {
				material := relayClientStableKey(clients[index]) + "\x00" + clients[index].Endpoint
				clients[index].SessionClientID = scopedRelayID("client", session.VNI, material)
			}
			sort.Slice(clients, func(left, right int) bool {
				return clients[left].Endpoint < clients[right].Endpoint
			})
		}
		result = append(result, collector.RelaySessionSnapshot{
			SessionID: scopedRelayID("session", session.VNI, ""),
			VNI:       int64(session.VNI),
			Source:    clients[0],
			Target:    clients[1],
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].VNI != result[right].VNI {
			return result[left].VNI < result[right].VNI
		}
		return result[left].SessionID < result[right].SessionID
	})
	return result, nil
}

func adaptRelayClient(
	vni uint32,
	client status.ClientInfo,
	identities map[string]discoIdentity,
) collector.RelayClientSnapshot {
	short := strings.TrimSpace(client.ShortDisco)
	endpoint := ""
	if client.Endpoint.IsValid() {
		endpoint = client.Endpoint.String()
	}
	result := collector.RelayClientSnapshot{
		DiscoShort: short, Endpoint: endpoint,
		PacketsSent: client.PacketsTx, BytesSent: client.BytesTx,
	}
	if identity, ok := identities[short]; ok {
		result.Identity = &domain.NodeIdentity{NodeKey: identity.nodeKey, DiscoKey: identity.discoKey}
	}
	result.SessionClientID = scopedRelayID("client", vni, relayClientStableKey(result))
	return result
}

func uniqueDiscoIdentities(keys map[key.NodePublic]key.DiscoPublic) map[string]discoIdentity {
	candidates := make(map[string][]discoIdentity)
	for nodeKey, discoKey := range keys {
		short := discoKey.ShortString()
		if short == "" || nodeKey.IsZero() {
			continue
		}
		candidates[short] = append(candidates[short], discoIdentity{
			nodeKey: nodeKey.String(), discoKey: discoKey.String(),
		})
	}
	result := make(map[string]discoIdentity)
	for short, matches := range candidates {
		if len(matches) == 1 {
			result[short] = matches[0]
		}
	}
	return result
}

func relayClientSortKey(client collector.RelayClientSnapshot) string {
	return relayClientStableKey(client)
}

func relayClientStableKey(client collector.RelayClientSnapshot) string {
	if short := strings.TrimSpace(client.DiscoShort); short != "" {
		return "0\x00" + short
	}
	return "1\x00" + client.Endpoint
}

func scopedRelayID(kind string, vni uint32, material string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", kind, vni, material)))
	return kind + "_" + hex.EncodeToString(digest[:8])
}
