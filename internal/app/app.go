package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

type App struct {
	mu sync.Mutex

	Aggregator      *aggregate.Aggregator
	Store           *store.SQLite
	logger          *slog.Logger
	lastCheckpoint  time.Time
	checkpointEvery time.Duration
}

func New(database *store.SQLite, options aggregate.Options, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	application := &App{
		Aggregator:      aggregate.New(options),
		Store:           database,
		logger:          logger,
		checkpointEvery: time.Second,
	}
	checkpoint, err := database.RestoreCheckpoint(context.Background())
	if err != nil {
		return nil, fmt.Errorf("restore runtime state: %w", err)
	}
	if len(checkpoint.Payload) > 0 {
		if err := application.Aggregator.RestoreState(checkpoint.Payload); err != nil {
			return nil, fmt.Errorf("decode runtime state: %w", err)
		}
		application.lastCheckpoint = checkpoint.UpdatedAt
	}

	reports, err := database.RestoreReportsAfter(context.Background(), checkpoint.LastReportRowID)
	if err != nil {
		return nil, fmt.Errorf("restore reports after checkpoint: %w", err)
	}
	latest := checkpoint.UpdatedAt
	lastReportRowID := checkpoint.LastReportRowID
	for _, stored := range reports {
		if _, err := application.Aggregator.ApplyAt(stored.Report, stored.ReceivedAt); err != nil {
			logger.Warn("skipping invalid stored report", "report_id", stored.Report.ReportID, "error", err)
		}
		if stored.ReceivedAt.After(latest) {
			latest = stored.ReceivedAt
		}
		lastReportRowID = stored.RowID
	}
	if len(reports) > 0 {
		payload, err := application.Aggregator.MarshalState()
		if err != nil {
			return nil, fmt.Errorf("encode restored runtime state: %w", err)
		}
		if err := database.SaveCheckpoint(context.Background(), payload, lastReportRowID, latest); err != nil {
			return nil, fmt.Errorf("persist restored runtime state: %w", err)
		}
		application.lastCheckpoint = latest
	}
	if err := database.SaveHistoryMetadata(context.Background(), application.Aggregator.HistoryMetadata(), latest); err != nil {
		return nil, fmt.Errorf("persist restored history metadata: %w", err)
	}
	return application, nil
}

func (a *App) Submit(ctx context.Context, report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	return a.SubmitAt(ctx, report, time.Now().UTC())
}

func (a *App) SubmitAt(ctx context.Context, report domain.ReportEnvelope, receivedAt time.Time) (domain.ReportReceipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	candidate, err := a.Aggregator.Clone()
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("clone runtime state: %w", err)
	}
	result, err := candidate.ApplyAt(report, receivedAt)
	if err != nil {
		return result.Receipt, err
	}
	if !result.Changed {
		return result.Receipt, nil
	}
	var checkpoint []byte
	var historyMetadata *domain.HistoryMetadata
	if result.CanonicalStateChanged || a.shouldCheckpoint(receivedAt) {
		checkpoint, err = candidate.MarshalState()
		if err != nil {
			return result.Receipt, fmt.Errorf("encode runtime state: %w", err)
		}
		metadata := candidate.HistoryMetadata()
		historyMetadata = &metadata
	}
	inserted, err := a.Store.RecordWithMetadata(ctx, report, receivedAt, checkpoint, result.Traffic, result.PathTransitions, historyMetadata)
	if err != nil {
		return result.Receipt, fmt.Errorf("persist report: %w", err)
	}
	if !inserted {
		return result.Receipt, nil
	}
	if err := a.Aggregator.ReplaceWith(candidate); err != nil {
		return result.Receipt, fmt.Errorf("commit runtime state: %w", err)
	}
	if checkpoint != nil {
		a.lastCheckpoint = receivedAt.UTC()
	}
	return result.Receipt, nil
}

func (a *App) shouldCheckpoint(receivedAt time.Time) bool {
	receivedAt = receivedAt.UTC()
	if a.lastCheckpoint.IsZero() || receivedAt.Before(a.lastCheckpoint) {
		return true
	}
	return !receivedAt.Before(a.lastCheckpoint.Add(a.checkpointEvery))
}

func (a *App) Maintain(ctx context.Context, now time.Time) error {
	return a.Store.Maintain(ctx, now.UTC())
}

func (a *App) RunMaintenance(ctx context.Context) {
	if err := a.Maintain(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
		a.logger.Warn("storage maintenance failed", "error", err)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := a.Maintain(ctx, now); err != nil && ctx.Err() == nil {
				a.logger.Warn("storage maintenance failed", "error", err)
			}
		}
	}
}

func (a *App) EdgeHistory(ctx context.Context, edgeID string) (domain.EdgeHistory, error) {
	return a.Store.EdgeHistory(ctx, edgeID, time.Now().Add(-a.StoreRetention()))
}

func (a *App) HistoryNodes(ctx context.Context, window domain.HistoryWindow) (domain.HistoryNodes, error) {
	return a.Store.HistoryNodes(ctx, window, time.Now().UTC())
}

func (a *App) HistoryEdges(ctx context.Context, query domain.HistoryEdgeQuery) (domain.HistoryEdgePage, error) {
	return a.Store.HistoryEdges(ctx, query, time.Now().UTC())
}

func (a *App) EdgeHistoryWindow(ctx context.Context, edgeID string, window domain.HistoryWindow) (domain.EdgeHistory, bool, error) {
	return a.Store.EdgeHistoryWindow(ctx, edgeID, window, time.Now().UTC())
}

func (a *App) StoreRetention() time.Duration {
	return a.Store.Retention()
}
