package devicesync

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/devicesapi"
	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestSynchronizerStartsImmediatelyAndReplacesFullSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	lister := &scriptedLister{results: []listResult{
		{devices: []devicesapi.Device{
			{NodeID: "stable-a", Name: "alpha.example.ts.net", Addresses: []string{"invalid", "100.64.0.1"}},
			{NodeID: "stable-b", Name: "beta.example.ts.net"},
		}},
		{devices: []devicesapi.Device{{NodeID: "stable-b", Name: "renamed.example.ts.net"}}},
	}}
	store := &fakeDirectoryStore{directory: domain.DeviceDirectory{Sync: domain.DirectorySyncState{Status: domain.DirectorySyncDisabled}}}
	waits := []time.Duration{}
	synchronizer := New(lister, store, Options{
		Logger: discardLogger(), Now: func() time.Time { return now }, Jitter: func(value time.Duration) time.Duration { return value },
		Wait: func(_ context.Context, duration time.Duration) bool {
			waits = append(waits, duration)
			return len(waits) == 1
		},
	})
	synchronizer.Run(context.Background())

	if lister.calls != 2 || store.applyCalls != 2 {
		t.Fatalf("calls: list=%d apply=%d", lister.calls, store.applyCalls)
	}
	if !reflect.DeepEqual(waits, []time.Duration{RefreshInterval, RefreshInterval}) {
		t.Fatalf("waits = %v", waits)
	}
	if store.directory.Sync.Status != domain.DirectorySyncHealthy || store.directory.Sync.InvalidAddressCount != 0 {
		t.Fatalf("sync = %#v", store.directory.Sync)
	}
	if len(store.directory.Devices) != 1 || store.directory.Devices[0].Device.StableNodeID != "stable-b" ||
		store.directory.Devices[0].Device.DNSName != "renamed.example.ts.net" {
		t.Fatalf("replacement directory = %#v", store.directory)
	}
	if len(store.applied[0].Devices[0].TailscaleIPs) != 1 || store.applied[0].Devices[0].TailscaleIPs[0] != "100.64.0.1" {
		t.Fatalf("first normalized snapshot = %#v", store.applied[0])
	}
}

func TestSynchronizerPublishesSanitizedBackoffSequence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	errors := []error{
		&devicesapi.RequestError{Kind: devicesapi.ErrorUnauthorized},
		&devicesapi.RequestError{Kind: devicesapi.ErrorForbidden},
		&devicesapi.RequestError{Kind: devicesapi.ErrorRateLimited},
		&devicesapi.RequestError{Kind: devicesapi.ErrorUnavailable},
		&devicesapi.RequestError{Kind: devicesapi.ErrorTimeout},
		&devicesapi.RequestError{Kind: devicesapi.ErrorInvalidResponse},
		invalidResponseError{},
	}
	results := make([]listResult, len(errors))
	for index, err := range errors {
		results[index].err = err
	}
	lister := &scriptedLister{results: results}
	store := &fakeDirectoryStore{directory: domain.DeviceDirectory{Sync: domain.DirectorySyncState{Status: domain.DirectorySyncSyncing}}}
	waits := []time.Duration{}
	synchronizer := New(lister, store, Options{
		Logger: discardLogger(), Now: func() time.Time { return now }, Jitter: func(value time.Duration) time.Duration { return value },
		Wait: func(_ context.Context, duration time.Duration) bool {
			waits = append(waits, duration)
			return len(waits) < len(errors)
		},
	})
	synchronizer.Run(context.Background())

	wantWaits := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	if !reflect.DeepEqual(waits, wantWaits) {
		t.Fatalf("waits = %v, want %v", waits, wantWaits)
	}
	wantCodes := []domain.DirectoryErrorCode{
		domain.DirectoryErrorUnauthorized, domain.DirectoryErrorForbidden, domain.DirectoryErrorRateLimited,
		domain.DirectoryErrorUnavailable, domain.DirectoryErrorTimeout, domain.DirectoryErrorInvalidResponse,
		domain.DirectoryErrorInvalidResponse,
	}
	if !reflect.DeepEqual(store.errorCodes, wantCodes) {
		t.Fatalf("error codes = %v, want %v", store.errorCodes, wantCodes)
	}
	for index, state := range store.states {
		if state.NextRetryAt == nil || !state.NextRetryAt.Equal(now.Add(wantWaits[index])) {
			t.Fatalf("retry %d = %#v", index, state)
		}
	}
}

func TestSynchronizerKeepsLastGoodOnInvalidSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-5 * time.Minute)
	store := &fakeDirectoryStore{directory: domain.DeviceDirectory{
		Sync:    domain.DirectorySyncState{Status: domain.DirectorySyncHealthy, LastAttemptAt: &lastSuccess, LastSuccessAt: &lastSuccess},
		Devices: []domain.DirectoryNode{{Device: domain.DirectoryDevice{StableNodeID: "last-good"}}},
	}}
	lister := &scriptedLister{results: []listResult{{devices: []devicesapi.Device{
		{NodeID: "duplicate", Name: "alpha.example.ts.net"},
		{NodeID: "duplicate", Name: "beta.example.ts.net"},
	}}}}
	synchronizer := New(lister, store, Options{
		Logger: discardLogger(), Now: func() time.Time { return now }, Jitter: func(value time.Duration) time.Duration { return value },
		Wait: func(context.Context, time.Duration) bool { return false },
	})
	synchronizer.Run(context.Background())

	if store.applyCalls != 0 || len(store.directory.Devices) != 1 ||
		store.directory.Devices[0].Device.StableNodeID != "last-good" {
		t.Fatalf("last-good snapshot changed: %#v", store.directory)
	}
	if store.directory.Sync.Status != domain.DirectorySyncStale ||
		store.directory.Sync.ErrorCode != domain.DirectoryErrorInvalidResponse ||
		store.directory.Sync.LastSuccessAt == nil || !store.directory.Sync.LastSuccessAt.Equal(lastSuccess) {
		t.Fatalf("stale state = %#v", store.directory.Sync)
	}
}

func TestSynchronizerCancellationDoesNotPublishFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	lister := blockingLister{started: make(chan struct{})}
	store := &fakeDirectoryStore{directory: domain.DeviceDirectory{Sync: domain.DirectorySyncState{Status: domain.DirectorySyncSyncing}}}
	done := make(chan struct{})
	go func() {
		New(lister, store, Options{Logger: discardLogger()}).Run(ctx)
		close(done)
	}()
	<-lister.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("synchronizer did not stop after cancellation")
	}
	if len(store.states) != 0 {
		t.Fatalf("cancellation published stale state: %#v", store.states)
	}
}

type listResult struct {
	devices []devicesapi.Device
	err     error
}

type scriptedLister struct {
	results []listResult
	calls   int
}

func (lister *scriptedLister) List(context.Context) ([]devicesapi.Device, error) {
	if lister.calls >= len(lister.results) {
		return nil, context.Canceled
	}
	result := lister.results[lister.calls]
	lister.calls++
	return result.devices, result.err
}

type blockingLister struct {
	started chan struct{}
}

func (lister blockingLister) List(ctx context.Context) ([]devicesapi.Device, error) {
	close(lister.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

type fakeDirectoryStore struct {
	mu         sync.Mutex
	directory  domain.DeviceDirectory
	states     []domain.DirectorySyncState
	errorCodes []domain.DirectoryErrorCode
	applied    []domain.DirectorySnapshot
	applyCalls int
}

func (store *fakeDirectoryStore) ApplyDirectorySnapshotAt(
	_ context.Context,
	snapshot domain.DirectorySnapshot,
	syncState domain.DirectorySyncState,
	_ time.Time,
) (aggregate.DirectoryApplyResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	normalized, normalization, err := domain.NormalizeDirectorySnapshot(snapshot)
	if err != nil {
		return aggregate.DirectoryApplyResult{}, err
	}
	syncState.InvalidAddressCount = normalization.InvalidAddressCount
	store.directory = domain.DeviceDirectory{Sync: syncState, Devices: make([]domain.DirectoryNode, len(normalized.Devices))}
	for index, device := range normalized.Devices {
		store.directory.Devices[index] = domain.DirectoryNode{ID: device.StableNodeID, Device: device, CollectedAt: normalized.CollectedAt}
	}
	store.applied = append(store.applied, normalized)
	store.applyCalls++
	return aggregate.DirectoryApplyResult{Changed: true, Normalization: normalization}, nil
}

func (store *fakeDirectoryStore) UpdateDirectorySyncStateAt(
	_ context.Context,
	state domain.DirectorySyncState,
	_ time.Time,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.directory.Sync = state
	store.states = append(store.states, state)
	if state.ErrorCode != "" {
		store.errorCodes = append(store.errorCodes, state.ErrorCode)
	}
	return nil
}

func (store *fakeDirectoryStore) DeviceDirectory() domain.DeviceDirectory {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.directory
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
