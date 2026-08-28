package exporter_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/httpapi"
	"github.com/GhostFlying/tailpath/internal/store"
)

type committedHelloSource struct {
	snapshot exporter.Snapshot
}

func (s committedHelloSource) Snapshot(context.Context) (exporter.Snapshot, error) {
	return s.snapshot, nil
}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(context.Context, string) (string, error) {
	return "test-reporter", nil
}

type dropFirstReportResponse struct {
	base    http.RoundTripper
	once    sync.Once
	dropped chan struct{}
}

func (t *dropFirstReportResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/api/v1/reports") {
		return response, nil
	}
	drop := false
	t.once.Do(func() {
		drop = true
		close(t.dropped)
	})
	if !drop {
		return response, nil
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return nil, errors.New("response lost after server commit")
}

func TestSnapshotSinkWithdrawsHelloCommittedBeforeResponseLoss(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "tailpath.db"), 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	application, err := app.New(database, aggregate.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(application, httpapi.Options{
		Authorizer: allowAllAuthorizer{},
	}).Handler())
	defer server.Close()

	transport := &dropFirstReportResponse{
		base: server.Client().Transport, dropped: make(chan struct{}),
	}
	reporter, err := exporter.NewHTTPReporter(server.URL, &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := exporter.NewSnapshotSink(reporter, exporter.SnapshotSinkOptions{
		ReporterInstanceID: "00000000-0000-4000-8000-000000000001",
	})
	registration, err := sink.Register("runtime", committedHelloSource{snapshot: exporter.Snapshot{
		CollectedAt: time.Now().UTC(),
		Observer: exporter.NodeIdentity{
			StableNodeID: "runtime", Hostname: "runtime",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	runContext, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sink.Run(runContext) }()
	select {
	case <-transport.dropped:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("hello response was not dropped")
	}
	topology := application.Aggregator.Snapshot()
	if len(topology.Observers) != 1 || !topology.Observers[0].Online {
		cancel()
		t.Fatalf("committed hello topology = %#v", topology)
	}

	withdrawContext, withdrawCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer withdrawCancel()
	if err := registration.Withdraw(withdrawContext); err != nil {
		cancel()
		t.Fatal(err)
	}
	topology = application.Aggregator.Snapshot()
	if len(topology.Observers) != 1 || topology.Observers[0].Online {
		cancel()
		t.Fatalf("topology after uncertain hello withdrawal = %#v", topology)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("snapshot sink did not stop")
	}
}
