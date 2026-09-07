// Command smokehitl is a headless HITL helper for scripts/smoke.sh G/H (#32).
// It is not a product entrypoint (deadbugz-guard / hitl stay the user commands).
//
//	smokehitl -pin FILE -audit FILE -decision approve|deny|timeout [-who NAME] [-live poison]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/audit"
	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/hitl"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
)

func main() {
	pinPath := flag.String("pin", "", "existing pin file (FileStore)")
	auditPath := flag.String("audit", "", "audit JSONL path")
	decision := flag.String("decision", "approve", "approve|deny|timeout")
	who := flag.String("who", "smoke", "actor recorded on approve/deny")
	liveMode := flag.String("live", mockmcp.ModePoison, "mockmcp live catalog (must differ from pin)")
	timeout := flag.Duration("timeout", 50*time.Millisecond, "HITL wait when -decision=timeout")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: smokehitl -pin FILE -audit FILE -decision approve|deny|timeout [-who NAME] [-live poison]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *pinPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	g := core.NewFileGate(*pinPath)
	old := g.CurrentPin()
	if old == nil || old.Aggregate == "" {
		fatal("pin missing or unreadable")
	}

	var approver core.Approver
	switch *decision {
	case "approve":
		approver = &hitl.CallbackApprover{Fn: func(_ context.Context, diff core.ToolDiffSummary, cand core.Pin) (string, string, error) {
			if diff.Empty() && diff.ReasonCode == "" {
				return "", *who, hitl.ErrDenied
			}
			return cand.Version, *who, nil
		}}
	case "deny":
		approver = &hitl.CallbackApprover{Fn: func(_ context.Context, _ core.ToolDiffSummary, _ core.Pin) (string, string, error) {
			return "", *who, hitl.ErrDenied
		}}
	case "timeout":
		approver = &hitl.CallbackApprover{Fn: func(ctx context.Context, _ core.ToolDiffSummary, _ core.Pin) (string, string, error) {
			<-ctx.Done()
			return "", *who, hitl.ErrTimeout
		}}
	default:
		fatal("decision must be approve|deny|timeout")
	}

	var aud core.Auditor
	if *auditPath != "" {
		aud = audit.NewJSONLAuditor(*auditPath)
	}
	h := &hitl.Hook{Gate: g, Approver: approver, Auditor: aud, Who: *who}

	ctx := context.Background()
	if *decision == "timeout" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	d := h.Evaluate(ctx, mockmcp.Tools(*liveMode))
	got := g.CurrentPin()
	newHash := ""
	if got != nil {
		newHash = got.Aggregate
	}

	out := map[string]any{
		"allowed":     d.Allowed,
		"reason_code": string(d.ReasonCode),
		"oldHash":     old.Aggregate,
		"newHash":     newHash,
		"decision":    *decision,
		"who":         *who,
	}

	ok := false
	switch *decision {
	case "approve":
		ok = d.Allowed && newHash != "" && newHash != old.Aggregate
	case "deny", "timeout":
		ok = !d.Allowed && newHash == old.Aggregate
	}
	out["ok"] = ok
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
	if !ok {
		os.Exit(1)
	}
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "smokehitl: %s\n", msg)
	os.Exit(1)
}
