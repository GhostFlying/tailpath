package collector

import (
	"context"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type RelayCapability string

const (
	RelayOff              RelayCapability = "off"
	RelayUnsupported      RelayCapability = "unsupported"
	RelayDisabled         RelayCapability = "disabled"
	RelayEnabled          RelayCapability = "enabled"
	RelayTransientFailure RelayCapability = "transient_failure"
)

type RelaySource interface {
	PeerRelaySnapshot(context.Context) (RelaySnapshot, error)
}

type RelaySnapshot struct {
	CollectedAt time.Time
	Capability  RelayCapability
	Sessions    []RelaySessionSnapshot
}

type RelaySessionSnapshot struct {
	SessionID string
	VNI       int64
	Source    RelayClientSnapshot
	Target    RelayClientSnapshot
}

type RelayClientSnapshot struct {
	SessionClientID string
	Identity        *domain.NodeIdentity
	DiscoShort      string
	Endpoint        string
	PacketsSent     uint64
	BytesSent       uint64
}
