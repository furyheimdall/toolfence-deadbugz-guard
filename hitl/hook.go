package hitl

import (
	"context"
	"errors"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

// Hook wires Approver outcomes into Gate.ApplyApproval and emits #8 events.
type Hook struct {
	Gate     core.Gate
	Approver core.Approver
	Auditor  core.Auditor
	// Who is the default actor recorded on re-approve / deny when the
	// Approver does not implement ActorAware.
	Who string
	Now func() time.Time
}

func (h *Hook) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().UTC()
}

func (h *Hook) actor() string {
	if a, ok := h.Approver.(ActorAware); ok {
		if w := a.Actor(); w != "" {
			return w
		}
	}
	if h.Who != "" {
		return h.Who
	}
	return "local"
}

func (h *Hook) record(event string, fields map[string]any) {
	if h.Auditor == nil {
		return
	}
	h.Auditor.Record(event, fields)
}

// Evaluate runs the fail-closed gate. On DIFF_NONEMPTY,
// config_or_inventory_changed, or tools_list_drift it blocks, requests
// HITL, and ApplyApproval(approve|deny) — timeout is deny (Chief H).
// tools_list_drift never auto-promotes (Evaluate still does not write).
func (h *Hook) Evaluate(ctx context.Context, live []core.ToolDef) core.GateDecision {
	if h.Gate == nil {
		return core.Deny(core.ReasonInternalError, nil)
	}
	d := h.Gate.Evaluate(ctx, live)
	if d.Allowed {
		return d
	}
	if d.ReasonCode == core.ReasonApprovalPending {
		h.recordBlocked(d)
		return d
	}
	if d.ReasonCode.RequiresApproval() {
		cand := h.Gate.PendingCandidate()
		if cand == nil {
			return core.Deny(core.ReasonInternalError, d.Diff)
		}
		diff := d.Diff
		if diff == nil {
			tmp := core.ToolDiffSummary{
				PinRevision: cand.Version,
				LiveHash:    cand.Aggregate,
				ReasonCode:  d.ReasonCode,
			}
			if p := h.Gate.CurrentPin(); p != nil {
				tmp.PinHash = p.Aggregate
				tmp.PinRevision = p.Version
			}
			diff = &tmp
		}
		return h.RequestAndApply(ctx, *diff, *cand)
	}
	h.recordBlocked(d)
	return d
}

// RequestAndApply is the #6 path: request_approval(diff) → apply_approval.
//
// G: approve → pin advances to candidate.Aggregate (newHash); audit
// who / when / oldHash / newHash (event approved).
// H: deny or timeout → old pin kept; hash does not advance (event denied).
func (h *Hook) RequestAndApply(ctx context.Context, diff core.ToolDiffSummary, candidate core.Pin) core.GateDecision {
	oldHash := ""
	oldRev := diff.PinRevision
	if p := h.Gate.CurrentPin(); p != nil {
		oldHash = p.Aggregate
		oldRev = p.Version
	}

	h.record("diff_detected", diffFields(h.now(), diff, oldHash, candidate.Aggregate, oldRev, "", approvalReason(diff)))
	h.recordBlockedDecision(diff, oldHash, oldRev)

	if h.Approver == nil {
		d := h.Gate.ApplyApproval(candidate.Version, core.OutcomeDeny)
		h.recordDenied(diff, oldHash, oldRev, "deny")
		return d
	}

	ver, err := h.Approver.RequestApproval(ctx, diff, candidate)
	decision := "deny"
	if err != nil && errors.Is(err, ErrTimeout) || (err != nil && ctx.Err() != nil) {
		decision = "timeout"
	}
	if err != nil || ver == "" || ver != candidate.Version {
		d := h.Gate.ApplyApproval(candidate.Version, core.OutcomeDeny)
		h.recordDenied(diff, oldHash, oldRev, decision)
		return d
	}

	d := h.Gate.ApplyApproval(candidate.Version, core.OutcomeApprove)
	newHash := oldHash
	if p := h.Gate.CurrentPin(); p != nil {
		newHash = p.Aggregate
	}
	h.record("approved", map[string]any{
		"when":             h.now().Format(time.RFC3339Nano),
		"who":              h.actor(),
		"oldHash":          oldHash,
		"newHash":          newHash,
		"pin_revision":     d.PinRevision,
		"pin_hash":         oldHash,
		"live_hash":        candidate.Aggregate,
		"added":            append([]string(nil), diff.Added...),
		"removed":          append([]string(nil), diff.Removed...),
		"changed":          append([]string(nil), diff.Changed...),
		"reason_code":      string(core.ReasonOK),
		"decision":         "approve",
		"approved_version": ver,
	})
	return d
}

func (h *Hook) recordBlocked(d core.GateDecision) {
	oldHash, rev := "", ""
	if p := h.Gate.CurrentPin(); p != nil {
		oldHash = p.Aggregate
		rev = p.Version
	}
	diff := core.ToolDiffSummary{PinRevision: rev, PinHash: oldHash}
	if d.Diff != nil {
		diff = *d.Diff
	}
	h.recordBlockedDecision(diff, oldHash, rev)
}

func (h *Hook) recordBlockedDecision(diff core.ToolDiffSummary, oldHash, rev string) {
	h.record("blocked", map[string]any{
		"when":         h.now().Format(time.RFC3339Nano),
		"who":          h.actor(),
		"oldHash":      oldHash,
		"newHash":      oldHash, // no transition while blocked
		"pin_revision": rev,
		"pin_hash":     diff.PinHash,
		"live_hash":    diff.LiveHash,
		"added":        append([]string(nil), diff.Added...),
		"removed":      append([]string(nil), diff.Removed...),
		"changed":      append([]string(nil), diff.Changed...),
		"reason_code":  string(core.ReasonApprovalPending),
		"decision":     "deny",
	})
}

func (h *Hook) recordDenied(diff core.ToolDiffSummary, oldHash, rev, decision string) {
	h.record("denied", map[string]any{
		"when":         h.now().Format(time.RFC3339Nano),
		"who":          h.actor(),
		"oldHash":      oldHash,
		"newHash":      oldHash, // Chief H: hash does not advance
		"pin_revision": rev,
		"pin_hash":     oldHash,
		"live_hash":    diff.LiveHash,
		"added":        append([]string(nil), diff.Added...),
		"removed":      append([]string(nil), diff.Removed...),
		"changed":      append([]string(nil), diff.Changed...),
		"reason_code":  string(core.ReasonApprovalDenied),
		"decision":     decision,
	})
}

func approvalReason(diff core.ToolDiffSummary) core.ReasonCode {
	if diff.ReasonCode != "" {
		return diff.ReasonCode
	}
	return core.ReasonDiffNonempty
}

func diffFields(when time.Time, diff core.ToolDiffSummary, oldHash, liveHash, rev, who string, reason core.ReasonCode) map[string]any {
	if reason == "" {
		reason = approvalReason(diff)
	}
	return map[string]any{
		"when":         when.Format(time.RFC3339Nano),
		"who":          who,
		"oldHash":      oldHash,
		"newHash":      oldHash,
		"pin_revision": rev,
		"pin_hash":     diff.PinHash,
		"live_hash":    liveHash,
		"added":        append([]string(nil), diff.Added...),
		"removed":      append([]string(nil), diff.Removed...),
		"changed":      append([]string(nil), diff.Changed...),
		"reason_code":  string(reason),
		"decision":     "deny",
	}
}

// RecordPinCreated emits the #8 pin / pin_created event (initial pin only).
func RecordPinCreated(a core.Auditor, who string, pin core.Pin) {
	if a == nil {
		return
	}
	if who == "" {
		who = "local"
	}
	a.Record("pin_created", map[string]any{
		"when":         pin.CreatedAt.UTC().Format(time.RFC3339Nano),
		"who":          who,
		"oldHash":      "",
		"newHash":      pin.Aggregate,
		"pin_revision": pin.Version,
		"pin_hash":     pin.Aggregate,
		"live_hash":    pin.Aggregate,
		"added":        []string{},
		"removed":      []string{},
		"changed":      []string{},
		"reason_code":  string(core.ReasonOK),
		"decision":     "approve",
	})
}
