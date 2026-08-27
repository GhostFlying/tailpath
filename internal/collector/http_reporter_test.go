package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestHTTPReporterReturnsTypedStatusErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(status)
				_, _ = response.Write([]byte("diagnostic body"))
			}))
			defer server.Close()
			reporter, err := NewHTTPReporter(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = reporter.Send(context.Background(), domain.ReportEnvelope{})
			var statusError *HTTPStatusError
			if !errors.As(err, &statusError) {
				t.Fatalf("Send error = %T %v, want HTTPStatusError", err, err)
			}
			if statusError.StatusCode != status || statusError.Message != "diagnostic body" {
				t.Fatalf("status error = %#v", statusError)
			}
		})
	}
}

func TestHTTPReporterCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/capabilities" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"observerProtocolVersions":[1],"features":["multi-observer"]}`))
	}))
	defer server.Close()
	reporter, err := NewHTTPReporter(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.RequireCapabilities(context.Background(), domain.FeatureMultiObserver); err != nil {
		t.Fatal(err)
	}
	err = reporter.RequireCapabilities(context.Background(), domain.FeatureObserverWithdrawal)
	var incompatible *IncompatibleServerError
	if !errors.As(err, &incompatible) {
		t.Fatalf("missing feature error = %T %v", err, err)
	}
}

func TestHTTPReporterCapabilitiesClassifyFailures(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		body         string
		incompatible bool
	}{
		{name: "old server", status: http.StatusNotFound, incompatible: true},
		{name: "malformed", status: http.StatusOK, body: `{`, incompatible: true},
		{name: "unauthorized", status: http.StatusUnauthorized, body: "denied"},
		{name: "server failure", status: http.StatusInternalServerError, body: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			reporter, err := NewHTTPReporter(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = reporter.Capabilities(context.Background())
			var incompatible *IncompatibleServerError
			var statusError *HTTPStatusError
			if test.incompatible && !errors.As(err, &incompatible) {
				t.Fatalf("error = %T %v, want IncompatibleServerError", err, err)
			}
			if !test.incompatible && !errors.As(err, &statusError) {
				t.Fatalf("error = %T %v, want HTTPStatusError", err, err)
			}
		})
	}
}

func TestHTTPReporterHonorsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		response.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	reporter, err := NewHTTPReporter(server.URL, &http.Client{Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reporter.Send(context.Background(), domain.ReportEnvelope{})
	if err == nil {
		t.Fatal("Send unexpectedly completed before timeout")
	}
}

func TestExportReportPreservesObserverAndRelayFields(t *testing.T) {
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	vni := int64(7)
	identity := domain.NodeIdentity{StableNodeID: "peer", TailscaleIPs: []string{"100.64.0.2"}}
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "report", ReporterInstanceID: "reporter", Sequence: 3,
		CollectedAt: at, Kind: domain.ReportObserverHello,
		Observers: []domain.ObserverReport{{
			Observer: domain.NodeIdentity{StableNodeID: "observer", OS: "linux"}, InventoryGeneration: "generation",
			Peers: []domain.PeerObservation{{
				Peer: identity, RxBytes: 10, TxBytes: 20, RxDelta: 1, TxDelta: 2, SampleDurationMS: 2000,
				Path:       domain.PathObservation{Kind: domain.PathPeerRelay, PeerRelayStableNodeID: "relay", PeerRelayVNI: &vni},
				LastActive: at,
			}},
		}},
		RelaySessions: []domain.RelaySessionObservation{{
			Relay:  domain.NodeIdentity{StableNodeID: "relay"},
			Source: domain.RelaySessionClient{SessionClientID: "left", Identity: &identity, DiscoShort: "short", Endpoint: "192.0.2.1:1"},
			Target: domain.RelaySessionClient{SessionClientID: "right"}, SessionID: "session", VNI: 7,
			SourceToTargetBytes: 100, TargetToSourceBytes: 50, SourceToTargetDelta: 10, TargetToSourceDelta: 5,
			SampleDurationMS: 2000, LastActive: at,
		}},
	}

	exported := exportReport(report)
	peer := exported.Observers[0].Peers[0]
	if exported.ReportID != "report" || exported.Observers[0].Observer.OS != "linux" ||
		peer.Path.PeerRelayVNI == nil || *peer.Path.PeerRelayVNI != 7 || peer.TxDelta != 2 {
		t.Fatalf("observer conversion = %#v", exported.Observers)
	}
	session := exported.RelaySessions[0]
	if session.Source.Identity == nil || session.Source.Identity.StableNodeID != "peer" ||
		session.Source.Endpoint != "192.0.2.1:1" || session.TargetToSourceDelta != 5 {
		t.Fatalf("relay conversion = %#v", session)
	}
	report.Observers[0].Peers[0].Peer.TailscaleIPs[0] = "changed"
	if peer.Peer.TailscaleIPs[0] != "100.64.0.2" {
		t.Fatal("exported identity aliases mutable domain storage")
	}
}

func TestHTTPReporterPreservesProtocolWireShape(t *testing.T) {
	var received domain.ReportEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte(`{"accepted":true,"resyncRequired":false,"controlStableNodeIds":["server"],"heartbeatIntervalMs":60000}`))
	}))
	defer server.Close()
	reporter, err := NewHTTPReporter(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	report := domain.ReportEnvelope{
		Version: domain.ProtocolVersion, ReportID: "withdraw", ReporterInstanceID: "reporter", Sequence: 4,
		CollectedAt: at, Kind: domain.ReportObserverWithdrawal,
		Observers: []domain.ObserverReport{{
			Observer:            domain.NodeIdentity{StableNodeID: "runtime", DNSName: "runtime.example.ts.net."},
			InventoryGeneration: "generation",
		}},
	}
	receipt, err := reporter.Send(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if received.Kind != domain.ReportObserverWithdrawal || received.Observers[0].Observer.StableNodeID != "runtime" ||
		receipt.HeartbeatIntervalMS != 60000 || len(receipt.ControlStableNodeIDs) != 1 {
		t.Fatalf("received=%#v receipt=%#v", received, receipt)
	}
}
