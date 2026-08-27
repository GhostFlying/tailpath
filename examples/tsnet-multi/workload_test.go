package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/exporter"
)

type fakeWorkloadRuntime struct {
	client *http.Client
}

func (r *fakeWorkloadRuntime) Snapshot(context.Context) (exporter.Snapshot, error) {
	return exporter.Snapshot{Observer: exporter.NodeIdentity{StableNodeID: "fake"}}, nil
}

func (r *fakeWorkloadRuntime) Close() error { return nil }

func (r *fakeWorkloadRuntime) Listen(_, _ string) (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

func (r *fakeWorkloadRuntime) HTTPClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return &http.Client{}
}

func (r *fakeWorkloadRuntime) TailscaleIPv4() string { return "127.0.0.1" }

func TestDogfoodWorkloadTransfersBoundedOrdinaryHTTP(t *testing.T) {
	manager := &runtimeManager{runtimes: map[string]*managedRuntime{
		"runtime-a": {instance: &fakeWorkloadRuntime{}},
		"runtime-b": {instance: &fakeWorkloadRuntime{}},
	}}
	workload, err := startDogfoodWorkload(context.Background(), manager, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && workload.completed.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if workload.completed.Load() == 0 {
		t.Fatal("workload did not complete a transfer")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := workload.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDogfoodWorkloadDoesNotLogTransportDetails(t *testing.T) {
	const canary = "sensitive-endpoint.example:1234"
	var logs synchronizedBuffer
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New(canary)
	})}
	manager := &runtimeManager{runtimes: map[string]*managedRuntime{
		"runtime-a": {instance: &fakeWorkloadRuntime{client: client}},
		"runtime-b": {instance: &fakeWorkloadRuntime{}},
	}}
	workload, err := startDogfoodWorkload(context.Background(), manager, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(logs.String(), "degraded") {
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := workload.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("workload logs leaked transport detail: %s", logs.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
