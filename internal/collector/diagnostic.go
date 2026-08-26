package collector

import (
	"context"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type Diagnostic struct {
	Self              domain.NodeIdentity `json:"self"`
	OS                string              `json:"os"`
	PeerCount         int                 `json:"peerCount"`
	RelayCapability   RelayCapability     `json:"relayCapability"`
	RelayEnabled      bool                `json:"relayEnabled"`
	RelaySessionCount int                 `json:"relaySessionCount"`
}

type DiagnosticSource interface {
	Diagnostic(context.Context) (Diagnostic, error)
}
