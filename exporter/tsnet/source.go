// Package tsnet adapts embedded Tailscale runtimes to Tailpath exporter
// snapshots.
package tsnet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	tailscaletsnet "tailscale.com/tsnet"

	"github.com/GhostFlying/tailpath/exporter"
	"github.com/GhostFlying/tailpath/internal/tailscalestatus"
)

type statusClient interface {
	Status(context.Context) (*ipnstate.Status, error)
}

// Source reads the passive runtime status of one embedded Tailscale identity.
type Source struct {
	client statusClient
	now    func() time.Time
}

// New obtains a LocalClient from server and returns a Source for that embedded
// identity. Tailscale may start an unstarted server while obtaining its
// LocalClient; the application remains responsible for all server lifecycle.
func New(server *tailscaletsnet.Server) (*Source, error) {
	if server == nil {
		return nil, errors.New("tsnet server is required")
	}
	client, err := server.LocalClient()
	if err != nil {
		return nil, fmt.Errorf("obtain tsnet LocalAPI client: %w", err)
	}
	return NewLocalClient(client)
}

// NewLocalClient returns a Source backed by an existing embedded LocalAPI
// client without changing the associated server lifecycle.
func NewLocalClient(client *local.Client) (*Source, error) {
	if client == nil {
		return nil, errors.New("tsnet LocalAPI client is required")
	}
	return newSource(client, time.Now), nil
}

func newSource(client statusClient, now func() time.Time) *Source {
	return &Source{client: client, now: now}
}

// Snapshot reads one passive LocalAPI status snapshot. It does not dial or
// probe peers and does not alter Tailscale preferences.
func (s *Source) Snapshot(ctx context.Context) (exporter.Snapshot, error) {
	status, err := s.client.Status(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return exporter.Snapshot{}, ctx.Err()
		}
		return exporter.Snapshot{}, errors.New("read tsnet runtime status")
	}
	snapshot, err := tailscalestatus.Snapshot(status, s.now())
	if err != nil {
		return exporter.Snapshot{}, err
	}
	return snapshot, nil
}

var _ exporter.Source = (*Source)(nil)
