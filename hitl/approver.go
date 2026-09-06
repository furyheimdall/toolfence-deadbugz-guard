package hitl

import (
	"context"
	"errors"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// ErrDenied is returned by Approver when the human denies re-pin.
var ErrDenied = errors.New("hitl: approval denied")

// ErrTimeout is returned when the approval wait exceeds the deadline.
// The hook treats timeout as deny (Chief H) and does not advance the pin.
var ErrTimeout = errors.New("hitl: approval timeout")

// ActorAware is an optional side channel so HTTP/CLI stubs can report who
// decided. Approver.RequestApproval keeps the (version, error) shape.
type ActorAware interface {
	Actor() string
}

// CallbackApprover is the in-process Approver (no SaaS).
type CallbackApprover struct {
	Fn  func(ctx context.Context, diff core.ToolDiffSummary, candidate core.Pin) (approvedVersion, who string, err error)
	who string
}

// RequestApproval implements core.Approver.
func (c *CallbackApprover) RequestApproval(ctx context.Context, diff core.ToolDiffSummary, candidate core.Pin) (string, error) {
	if c.Fn == nil {
		return "", ErrDenied
	}
	ver, who, err := c.Fn(ctx, diff, candidate)
	c.who = who
	return ver, err
}

// Actor implements ActorAware.
func (c *CallbackApprover) Actor() string { return c.who }
