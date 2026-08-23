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

	Aggregator *aggregate.Aggregator
	Store      *store.SQLite
	logger     *slog.Logger
}

func New(database *store.SQLite, options aggregate.Options, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	application := &App{
		Aggregator: aggregate.New(options),
		Store:      database,
		logger:     logger,
	}
	state, err := database.RestoreState(context.Background())
	if err != nil {
		return nil, fmt.Errorf("restore runtime state: %w", err)
	}
	if len(state) > 0 {
		if err := application.Aggregator.RestoreState(state); err != nil {
			return nil, fmt.Errorf("decode runtime state: %w", err)
		}
		return application, nil
	}

	reports, err := database.RestoreReports(context.Background())
	if err != nil {
		return nil, fmt.Errorf("restore legacy reports: %w", err)
	}
	var latest time.Time
	for _, stored := range reports {
		if _, err := application.Aggregator.ApplyAt(stored.Report, stored.ReceivedAt); err != nil {
			logger.Warn("skipping invalid stored report", "report_id", stored.Report.ReportID, "error", err)
			continue
		}
		if stored.ReceivedAt.After(latest) {
			latest = stored.ReceivedAt
		}
	}
	if len(reports) > 0 {
		payload, err := application.Aggregator.MarshalState()
		if err != nil {
			return nil, fmt.Errorf("encode restored runtime state: %w", err)
		}
		if err := database.SaveState(context.Background(), payload, latest); err != nil {
			return nil, fmt.Errorf("persist restored runtime state: %w", err)
		}
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
	payload, err := candidate.MarshalState()
	if err != nil {
		return result.Receipt, fmt.Errorf("encode runtime state: %w", err)
	}
	inserted, err := a.Store.Record(ctx, report, receivedAt, payload, result.Traffic, result.PathTransitions)
	if err != nil {
		return result.Receipt, fmt.Errorf("persist report: %w", err)
	}
	if !inserted {
		return result.Receipt, nil
	}
	if err := a.Aggregator.ReplaceWith(candidate); err != nil {
		return result.Receipt, fmt.Errorf("commit runtime state: %w", err)
	}
	return result.Receipt, nil
}

func (a *App) EdgeHistory(ctx context.Context, edgeID string) (domain.EdgeHistory, error) {
	return a.Store.EdgeHistory(ctx, edgeID, time.Now().Add(-a.StoreRetention()))
}

func (a *App) StoreRetention() time.Duration {
	return a.Store.Retention()
}
