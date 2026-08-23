package collector

import (
	"context"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type Diagnostic struct {
	Self      domain.NodeIdentity `json:"self"`
	OS        string              `json:"os"`
	PeerCount int                 `json:"peerCount"`
}

type DiagnosticSource interface {
	Diagnostic(context.Context) (Diagnostic, error)
}
