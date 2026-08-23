package fixtures

import (
	"context"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

func TestStartSeedsHistoryBeforeReturning(t *testing.T) {
	database, err := store.Open(":memory:", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	application, err := app.New(database, aggregate.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := New(application, nil).Start(ctx); err != nil {
		t.Fatal(err)
	}

	page, err := database.HistoryEdges(ctx, domain.HistoryEdgeQuery{
		Window: domain.History1Hour,
		Limit:  50,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Edges) == 0 {
		t.Fatal("fixture history is empty after Start returned")
	}
}
