// Command deadbugz-guard is the stdio wrap adapter:
//
//	deadbugz-guard [flags] -- <server> [args...]
//	deadbugz-guard approve [flags] -- <server> [args...]
//
// approve / --write-pin -- <server> is the non-TTY bootstrap path (issue #20).
// The wrap itself never prompts; IDE spawn has no TTY.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "approve" {
		os.Exit(runApprove(os.Args[2:]))
	}
	os.Exit(runWrap(os.Args[1:]))
}

func runWrap(args []string) int {
	cfg := sidecar.ConfigFromEnv(sidecar.Config{})
	fs := flag.NewFlagSet("deadbugz-guard", flag.ExitOnError)
	pinPath := fs.String("pin", cfg.PinPath, "pin file path (PIN_PATH); default ~/.deadbugz/pins/<name>.json")
	auditPath := fs.String("audit", cfg.AuditPath, "audit JSONL path (AUDIT_PATH)")
	hitl := fs.String("hitl", cfg.HITLEndpoint, "HITL endpoint (HITL_ENDPOINT)")
	callGate := fs.Int("call-gate", cfg.CallGate, "Deadbugz call gate depth (CALL_GATE, default 3)")
	name := fs.String("name", cfg.ServerName, "server name for per-server pin path (DEADBUGZ_SERVER_NAME)")
	approve := fs.Bool("approve", cfg.Approve, "non-TTY bootstrap: write first pin when file is missing (DEADBUGZ_APPROVE=1)")
	approveFile := fs.String("approve-file", cfg.ApproveFile, "one-shot approve token file (DEADBUGZ_APPROVE_FILE)")
	writePin := fs.Bool("write-pin", false, "hash a fixture or live tools/list into --pin and exit")
	fromMode := fs.String("from-mode", mockmcp.ModeBenign, "with --write-pin and no -- <server>, hash this mock mode")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:\n  deadbugz-guard [flags] -- <server> [args...]\n  deadbugz-guard approve [flags] -- <server> [args...]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg.PinPath = *pinPath
	cfg.AuditPath = *auditPath
	cfg.HITLEndpoint = *hitl
	cfg.CallGate = *callGate
	cfg.ServerName = *name
	cfg.Approve = *approve || cfg.Approve
	cfg.ApproveFile = *approveFile
	cfg = sidecar.Normalize(cfg)

	if *writePin {
		return writePinMode(cfg, *fromMode, fs.Args())
	}

	argv := fs.Args()
	if len(argv) == 0 {
		fs.Usage()
		return 2
	}
	cfg.ServerArgv = argv

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := sidecar.Run(ctx, cfg, os.Stdin, os.Stdout, sidecar.ExecStart(argv), nil); err != nil && err != context.Canceled {
		fatal(err.Error())
		return 1
	}
	return 0
}

func runApprove(args []string) int {
	cfg := sidecar.ConfigFromEnv(sidecar.Config{})
	fs := flag.NewFlagSet("approve", flag.ExitOnError)
	pinPath := fs.String("pin", cfg.PinPath, "pin file to write (PIN_PATH or ~/.deadbugz/pins/<name>.json)")
	name := fs.String("name", cfg.ServerName, "server name (filesystem, fetch, ...)")
	fromPending := fs.String("from-pending", "", "pending snapshot (default: <pin>.pending)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:\n  deadbugz-guard approve [--name filesystem] [--pin PATH] -- <server> [args...]\n  deadbugz-guard approve --pin PATH [--from-pending PATH]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg.PinPath = *pinPath
	cfg.ServerName = *name
	cfg = sidecar.Normalize(cfg)
	if cfg.PinPath == "" {
		fatal("approve requires --pin or --name")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	argv := fs.Args()
	if len(argv) > 0 {
		cfg.ServerArgv = argv
		written, werr := sidecar.ApproveFromServerConfig(ctx, cfg, sidecar.ExecStart(argv))
		if werr != nil {
			fatal(werr.Error())
			return 1
		}
		fmt.Fprintf(os.Stderr, "wrote pin %s mode=0600 hash=%s fingerprint=%s\n", cfg.PinPath, written.Aggregate, written.ConfigFingerprint)
		return 0
	}
	written, err := sidecar.ApproveFromPending(cfg.PinPath, *fromPending)
	if err != nil {
		fatal(err.Error())
		return 1
	}
	fmt.Fprintf(os.Stderr, "wrote pin %s mode=0600 hash=%s (from pending)\n", cfg.PinPath, written.Aggregate)
	return 0
}

func writePinMode(cfg sidecar.Config, fromMode string, argv []string) int {
	if cfg.PinPath == "" {
		fatal(" --write-pin requires --pin / PIN_PATH / --name")
		return 1
	}
	if len(argv) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cfg.ServerArgv = argv
		p, err := sidecar.ApproveFromServerConfig(ctx, cfg, sidecar.ExecStart(argv))
		if err != nil {
			fatal(err.Error())
			return 1
		}
		fmt.Fprintf(os.Stderr, "wrote pin %s mode=0600 hash=%s fingerprint=%s\n", cfg.PinPath, p.Aggregate, p.ConfigFingerprint)
		return 0
	}
	ident := core.ConfigIdentity{}
	if _, err := sidecar.WritePinFileWithConfig(cfg.PinPath, "", mockmcp.Tools(fromMode), ident); err != nil {
		fatal(err.Error())
		return 1
	}
	fmt.Fprintf(os.Stderr, "wrote pin %s\n", cfg.PinPath)
	return 0
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "deadbugz-guard: %s\n", msg)
}
