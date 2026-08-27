package collector

import "github.com/GhostFlying/tailpath/exporter"

type RelayCapability = exporter.RelayCapability
type RelayIdentityEvidence = exporter.RelayIdentityEvidence

const (
	RelayOff              = exporter.RelayOff
	RelayUnsupported      = exporter.RelayUnsupported
	RelayDisabled         = exporter.RelayDisabled
	RelayEnabled          = exporter.RelayEnabled
	RelayTransientFailure = exporter.RelayTransientFailure

	RelayIdentityAvailable = exporter.RelayIdentityAvailable
	RelayIdentityDegraded  = exporter.RelayIdentityDegraded
)

type RelaySource = exporter.RelaySource
type RelaySnapshot = exporter.RelaySnapshot
type RelaySessionSnapshot = exporter.RelaySessionSnapshot
type RelayClientSnapshot = exporter.RelayClientSnapshot
