package collector

import (
	"context"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type RelayCapability string

type RelayIdentityEvidence string

const (
	RelayOff              RelayCapability = "off"
	RelayUnsupported      RelayCapability = "unsupported"
	RelayDisabled         RelayCapability = "disabled"
	RelayEnabled          RelayCapability = "enabled"
	RelayTransientFailure RelayCapability = "transient_failure"

	RelayIdentityAvailable RelayIdentityEvidence = "available"
	RelayIdentityDegraded  RelayIdentityEvidence = "degraded"
)

type RelaySource interface {
	PeerRelaySnapshot(context.Context) (RelaySnapshot, error)
}

type RelaySnapshot struct {
	CollectedAt      time.Time
	Capability       RelayCapability
	IdentityEvidence RelayIdentityEvidence
	Sessions         []RelaySessionSnapshot
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
