// Command deadbugz-guard is the stdio wrap adapter:
//
//	deadbugz-guard -- <server> [args...]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/furyheimdall/toolfence-deadbugz-guard/internal/mockmcp"
	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar"
)

func main() {
	cfg := sidecar.ConfigFromEnv(sidecar.Config{})
	pinPath := flag.String("pin", cfg.PinPath, "pin file path (PIN_PATH)")
	auditPath := flag.String("audit", cfg.AuditPath, "audit JSONL path (AUDIT_PATH)")
	hitl := flag.String("hitl", cfg.HITLEndpoint, "HITL endpoint (HITL_ENDPOINT)")
	callGate := flag.Int("call-gate", cfg.CallGate, "Deadbugz call gate depth (CALL_GATE, default 3)")
	writePin := flag.Bool("write-pin", false, "hash a fixture/tools listing into --pin and exit")
	fromMode := flag.String("from-mode", mockmcp.ModeBenign, "with --write-pin, hash this mock mode (default benign)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: deadbugz-guard [flags] -- <server> [args...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg.PinPath = *pinPath
	cfg.AuditPath = *auditPath
	cfg.HITLEndpoint = *hitl
	cfg.CallGate = *callGate

	if *writePin {
		if cfg.PinPath == "" {
			fatal(" --write-pin requires --pin / PIN_PATH")
		}
		if _, err := sidecar.WritePinFile(cfg.PinPath, "", mockmcp.Tools(*fromMode)); err != nil {
			fatal(err.Error())
		}
		fmt.Fprintf(os.Stderr, "wrote pin %s\n", cfg.PinPath)
		os.Exit(0)
	}

	argv := flag.Args()
	if len(argv) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := sidecar.Run(ctx, cfg, os.Stdin, os.Stdout, sidecar.ExecStart(argv), nil); err != nil && err != context.Canceled {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "deadbugz-guard: %s\n", msg)
	os.Exit(1)
}
