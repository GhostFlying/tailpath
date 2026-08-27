package collector

import (
	"context"
	"net/http"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/domain"
)

type IncompatibleServerError = exporter.IncompatibleServerError
type HTTPStatusError = exporter.HTTPStatusError

type HTTPReporter struct {
	reporter *exporter.HTTPReporter
}

func NewHTTPReporter(serverURL string, client *http.Client) (*HTTPReporter, error) {
	reporter, err := exporter.NewHTTPReporter(serverURL, client)
	if err != nil {
		return nil, err
	}
	return &HTTPReporter{reporter: reporter}, nil
}

func (r *HTTPReporter) Capabilities(ctx context.Context) (domain.ServerCapabilities, error) {
	capabilities, err := r.reporter.Capabilities(ctx)
	return domain.ServerCapabilities{
		ObserverProtocolVersions: capabilities.ObserverProtocolVersions,
		Features:                 capabilities.Features,
	}, err
}

func (r *HTTPReporter) RequireCapabilities(ctx context.Context, features ...string) error {
	return r.reporter.RequireCapabilities(ctx, features...)
}

func (r *HTTPReporter) Send(ctx context.Context, report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	receipt, err := r.reporter.Send(ctx, exportReport(report))
	return domain.ReportReceipt{
		Accepted:             receipt.Accepted,
		ResyncRequired:       receipt.ResyncRequired,
		ControlStableNodeIDs: receipt.ControlStableNodeIDs,
		HeartbeatIntervalMS:  receipt.HeartbeatIntervalMS,
	}, err
}

func exportReport(report domain.ReportEnvelope) exporter.ReportEnvelope {
	result := exporter.ReportEnvelope{
		Version: report.Version, ReportID: report.ReportID, ReporterInstanceID: report.ReporterInstanceID,
		Sequence: report.Sequence, CollectedAt: report.CollectedAt, Kind: exporter.ReportKind(report.Kind),
		Observers:     make([]exporter.ObserverReport, len(report.Observers)),
		RelaySessions: make([]exporter.RelaySessionObservation, len(report.RelaySessions)),
	}
	for index, observer := range report.Observers {
		result.Observers[index] = exporter.ObserverReport{
			Observer: exportIdentity(observer.Observer), InventoryGeneration: observer.InventoryGeneration,
			Peers: make([]exporter.PeerObservation, len(observer.Peers)),
		}
		for peerIndex, peer := range observer.Peers {
			result.Observers[index].Peers[peerIndex] = exporter.PeerObservation{
				Peer: exportIdentity(peer.Peer), RxBytes: peer.RxBytes, TxBytes: peer.TxBytes,
				RxDelta: peer.RxDelta, TxDelta: peer.TxDelta, SampleDurationMS: peer.SampleDurationMS,
				Path: exportPath(peer.Path), LastActive: peer.LastActive,
			}
		}
	}
	for index, session := range report.RelaySessions {
		result.RelaySessions[index] = exporter.RelaySessionObservation{
			Relay:  exportIdentity(session.Relay),
			Source: exportRelayClient(session.Source), Target: exportRelayClient(session.Target),
			SessionID: session.SessionID, VNI: session.VNI,
			SourceToTargetBytes: session.SourceToTargetBytes, TargetToSourceBytes: session.TargetToSourceBytes,
			SourceToTargetDelta: session.SourceToTargetDelta, TargetToSourceDelta: session.TargetToSourceDelta,
			SampleDurationMS: session.SampleDurationMS, LastActive: session.LastActive,
		}
	}
	return result
}

func exportIdentity(identity domain.NodeIdentity) exporter.NodeIdentity {
	return exporter.NodeIdentity{
		StableNodeID: identity.StableNodeID, NodeID: identity.NodeID, NodeKey: identity.NodeKey,
		DiscoKey: identity.DiscoKey, Hostname: identity.Hostname, DNSName: identity.DNSName, OS: identity.OS,
		TailscaleIPs: append([]string(nil), identity.TailscaleIPs...),
	}
}

func exportPath(path domain.PathObservation) exporter.Path {
	result := exporter.Path{
		Kind: exporter.PathKind(path.Kind), DirectEndpoint: path.DirectEndpoint, DERPRegion: path.DERPRegion,
		PeerRelayStableNodeID: path.PeerRelayStableNodeID,
	}
	if path.PeerRelayVNI != nil {
		value := *path.PeerRelayVNI
		result.PeerRelayVNI = &value
	}
	return result
}

func exportRelayClient(client domain.RelaySessionClient) exporter.RelaySessionClient {
	result := exporter.RelaySessionClient{
		SessionClientID: client.SessionClientID, DiscoShort: client.DiscoShort, Endpoint: client.Endpoint,
	}
	if client.Identity != nil {
		identity := exportIdentity(*client.Identity)
		result.Identity = &identity
	}
	return result
}
