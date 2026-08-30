package devicesync

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/devicesapi"
	"github.com/GhostFlying/tailpath/internal/domain"
)

const RefreshInterval = 5 * time.Minute

var retryBackoff = [...]time.Duration{
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	15 * time.Minute,
}

type DirectoryStore interface {
	ApplyDirectorySnapshotAt(
		context.Context,
		domain.DirectorySnapshot,
		domain.DirectorySyncState,
		time.Time,
	) (aggregate.DirectoryApplyResult, error)
	UpdateDirectorySyncStateAt(context.Context, domain.DirectorySyncState, time.Time) error
	DeviceDirectory() domain.DeviceDirectory
}

type Options struct {
	Logger *slog.Logger
	Now    func() time.Time
	Jitter func(time.Duration) time.Duration
	Wait   func(context.Context, time.Duration) bool
}

type Synchronizer struct {
	lister devicesapi.Lister
	store  DirectoryStore
	logger *slog.Logger
	now    func() time.Time
	jitter func(time.Duration) time.Duration
	wait   func(context.Context, time.Duration) bool
}

func New(lister devicesapi.Lister, store DirectoryStore, options Options) *Synchronizer {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.Jitter == nil {
		options.Jitter = func(value time.Duration) time.Duration {
			return time.Duration(float64(value) * (0.8 + rand.Float64()*0.4))
		}
	}
	if options.Wait == nil {
		options.Wait = wait
	}
	return &Synchronizer{
		lister: lister,
		store:  store,
		logger: options.Logger,
		now:    options.Now,
		jitter: options.Jitter,
		wait:   options.Wait,
	}
}

func (s *Synchronizer) Run(ctx context.Context) {
	current := s.store.DeviceDirectory()
	if current.Sync.Status == domain.DirectorySyncDisabled {
		if err := s.store.UpdateDirectorySyncStateAt(ctx, domain.DirectorySyncState{
			Status: domain.DirectorySyncSyncing,
		}, s.now().UTC()); err != nil && ctx.Err() == nil {
			s.logger.Warn("device directory state update failed", "error_code", domain.DirectoryErrorUnavailable)
		}
	}

	failures := 0
	for ctx.Err() == nil {
		devices, err := s.lister.List(ctx)
		if ctx.Err() != nil || isCanceled(err) {
			return
		}
		attemptAt := s.now().UTC()
		snapshot := directorySnapshot(devices, attemptAt)
		if err == nil {
			if _, _, normalizeErr := domain.NormalizeDirectorySnapshot(snapshot); normalizeErr != nil {
				err = invalidResponseError{}
			}
		}
		if err == nil {
			healthy := domain.DirectorySyncState{
				Status: domain.DirectorySyncHealthy, LastAttemptAt: timePointer(attemptAt), LastSuccessAt: timePointer(attemptAt),
			}
			if _, applyErr := s.store.ApplyDirectorySnapshotAt(ctx, snapshot, healthy, attemptAt); applyErr == nil {
				failures = 0
				if !s.wait(ctx, RefreshInterval) {
					return
				}
				continue
			} else {
				err = applyErr
			}
		}

		delay := s.jitter(retryBackoff[min(failures, len(retryBackoff)-1)])
		if delay <= 0 {
			delay = retryBackoff[min(failures, len(retryBackoff)-1)]
		}
		nextRetryAt := attemptAt.Add(delay)
		current = s.store.DeviceDirectory()
		stale := domain.DirectorySyncState{
			Status: domain.DirectorySyncStale, LastAttemptAt: timePointer(attemptAt),
			LastSuccessAt: current.Sync.LastSuccessAt, NextRetryAt: timePointer(nextRetryAt),
			ErrorCode: classifyError(err), InvalidAddressCount: current.Sync.InvalidAddressCount,
		}
		if stateErr := s.store.UpdateDirectorySyncStateAt(ctx, stale, attemptAt); stateErr != nil && ctx.Err() == nil {
			s.logger.Warn("device directory state update failed", "error_code", domain.DirectoryErrorUnavailable)
		}
		if ctx.Err() == nil {
			s.logger.Warn("device directory sync failed", "error_code", stale.ErrorCode, "retry_in", delay)
		}
		failures++
		if !s.wait(ctx, delay) {
			return
		}
	}
}

func directorySnapshot(devices []devicesapi.Device, collectedAt time.Time) domain.DirectorySnapshot {
	result := domain.DirectorySnapshot{
		CollectedAt: collectedAt,
		Devices:     make([]domain.DirectoryDevice, 0, len(devices)),
	}
	for _, device := range devices {
		result.Devices = append(result.Devices, domain.DirectoryDevice{
			StableNodeID: device.NodeID, NodeKey: device.NodeKey, DNSName: device.Name,
			Hostname: device.Hostname, OS: device.OS, TailscaleIPs: append([]string{}, device.Addresses...),
			Tags: append([]string{}, device.Tags...), ConnectedToControl: device.ConnectedToControl,
			LastSeen: device.LastSeen,
		})
	}
	return result
}

func classifyError(err error) domain.DirectoryErrorCode {
	var requestError *devicesapi.RequestError
	if errors.As(err, &requestError) {
		switch requestError.Kind {
		case devicesapi.ErrorUnauthorized:
			return domain.DirectoryErrorUnauthorized
		case devicesapi.ErrorForbidden:
			return domain.DirectoryErrorForbidden
		case devicesapi.ErrorRateLimited:
			return domain.DirectoryErrorRateLimited
		case devicesapi.ErrorTimeout:
			return domain.DirectoryErrorTimeout
		case devicesapi.ErrorInvalidResponse:
			return domain.DirectoryErrorInvalidResponse
		}
	}
	var invalid invalidResponseError
	if errors.As(err, &invalid) {
		return domain.DirectoryErrorInvalidResponse
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.DirectoryErrorTimeout
	}
	return domain.DirectoryErrorUnavailable
}

func isCanceled(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	var requestError *devicesapi.RequestError
	return errors.As(err, &requestError) && requestError.Kind == devicesapi.ErrorCanceled
}

type invalidResponseError struct{}

func (invalidResponseError) Error() string { return "invalid device directory response" }

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
