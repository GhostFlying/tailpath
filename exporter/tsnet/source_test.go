package tsnet_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	tailscaletsnet "tailscale.com/tsnet"
	"tailscale.com/types/key"

	"github.com/GhostFlying/tailpath/exporter"
	tailpathtsnet "github.com/GhostFlying/tailpath/exporter/tsnet"
)

type statusTransport struct {
	status   *ipnstate.Status
	err      error
	requests []*http.Request
}

func (t *statusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, request.Clone(request.Context()))
	if t.err != nil {
		return nil, t.err
	}
	payload, err := json.Marshal(t.status)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(string(payload))),
	}, nil
}

func TestLocalClientSourceReadsOnlyPassiveStatus(t *testing.T) {
	peerKey := key.NewNode().Public()
	transport := &statusTransport{status: &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			ID: "runtime", HostName: "runtime", OS: "linux",
			TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")},
		},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			peerKey: {ID: "peer", HostName: "peer", RxBytes: 12, TxBytes: 34, CurAddr: "192.0.2.10:41641"},
		},
	}}
	client := &local.Client{Transport: transport, OmitAuth: true}
	var source exporter.Source
	source, err := tailpathtsnet.NewLocalClient(client)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Observer.StableNodeID != "runtime" || len(snapshot.Peers) != 1 ||
		snapshot.Peers[0].Path.Kind != exporter.PathDirect {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(transport.requests) != 1 || transport.requests[0].Method != http.MethodGet ||
		transport.requests[0].URL.Path != "/localapi/v0/status" || transport.requests[0].URL.RawQuery != "" {
		t.Fatalf("LocalAPI requests = %#v", transport.requests)
	}
}

func TestSourceReturnsBoundedStatusErrors(t *testing.T) {
	private := "private-status-payload"
	transport := &statusTransport{err: errors.New(private)}
	source, err := tailpathtsnet.NewLocalClient(&local.Client{Transport: transport, OmitAuth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Snapshot(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime status") ||
		strings.Contains(err.Error(), private) {
		t.Fatalf("status error = %v", err)
	}
	transport = &statusTransport{status: &ipnstate.Status{}}
	source, err = tailpathtsnet.NewLocalClient(&local.Client{Transport: transport, OmitAuth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Snapshot(context.Background()); err == nil || strings.Contains(err.Error(), "{") {
		t.Fatalf("missing-self error = %v", err)
	}
}

func TestSourcePreservesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := &statusTransport{err: context.Canceled}
	source, err := tailpathtsnet.NewLocalClient(&local.Client{Transport: transport, OmitAuth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestConstructorsRejectNilInputs(t *testing.T) {
	if _, err := tailpathtsnet.New(nil); err == nil {
		t.Fatal("nil tsnet server was accepted")
	}
	if _, err := tailpathtsnet.NewLocalClient(nil); err == nil {
		t.Fatal("nil LocalAPI client was accepted")
	}
}

func TestServerConstructorHasPublicSignature(t *testing.T) {
	var constructor func(*tailscaletsnet.Server) (*tailpathtsnet.Source, error) = tailpathtsnet.New
	if constructor == nil {
		t.Fatal("server constructor is nil")
	}
}
