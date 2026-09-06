package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/hitl"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		listen := fs.String("listen", "127.0.0.1:8765", "local bind address")
		_ = fs.Parse(os.Args[2:])
		srv := hitl.NewServer()
		fmt.Fprintf(os.Stderr, "HITL stub listening on http://%s (GET /v1/approvals/pending)\n", *listen)
		if err := http.ListenAndServe(*listen, srv.Handler()); err != nil {
			fmt.Fprintf(os.Stderr, "hitl serve: %v\n", err)
			os.Exit(1)
		}
	case "request":
		fs := flag.NewFlagSet("request", flag.ExitOnError)
		base := fs.String("base", "http://127.0.0.1:8765", "stub base URL")
		file := fs.String("file", "", "JSON file with {diff, candidate}")
		timeout := fs.Int("timeout", 60, "seconds to wait (timeout = deny)")
		_ = fs.Parse(os.Args[2:])
		payload, err := loadRequest(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hitl request: %v\n", err)
			os.Exit(1)
		}
		payload["timeout_sec"] = *timeout
		raw, _ := json.Marshal(payload)
		cli := &http.Client{Timeout: time.Duration(*timeout+5) * time.Second}
		resp, err := cli.Post(*base+"/v1/approvals", "application/json", bytes.NewReader(raw))
		if err != nil {
			fmt.Fprintf(os.Stderr, "hitl request: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		_, _ = os.Stdout.ReadFrom(resp.Body)
		_, _ = os.Stdout.Write([]byte("\n"))
		if resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
	case "decide":
		fs := flag.NewFlagSet("decide", flag.ExitOnError)
		base := fs.String("base", "http://127.0.0.1:8765", "stub base URL")
		decision := fs.String("decision", "approve", "approve|deny")
		who := fs.String("who", "local", "actor recorded on re-approve")
		id := fs.String("id", "", "approval id (default: first pending)")
		_ = fs.Parse(os.Args[2:])
		if *id == "" {
			var err error
			*id, err = firstPending(*base)
			if err != nil {
				fmt.Fprintf(os.Stderr, "hitl decide: %v\n", err)
				os.Exit(1)
			}
		}
		body, _ := json.Marshal(hitl.DecisionBody{Decision: *decision, Who: *who})
		resp, err := http.Post(*base+"/v1/approvals/"+*id+"/decide", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Fprintf(os.Stderr, "hitl decide: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "hitl decide: status %d\n", resp.StatusCode)
			os.Exit(1)
		}
		_, _ = fmt.Fprintf(os.Stdout, "ok %s id=%s who=%s\n", *decision, *id, *who)
	default:
		usage()
		os.Exit(2)
	}
}

func loadRequest(path string) (map[string]any, error) {
	if path == "" {
		// Minimal demo ToolDiffSummary + candidate pin.
		return map[string]any{
			"diff": map[string]any{
				"added":        []string{"evil_tool"},
				"removed":      []string{},
				"changed":      []string{"search"},
				"pin_revision": "pin-1",
				"live_hash":    "newhashdemo",
				"pin_hash":     "oldhashdemo",
			},
			"candidate": map[string]any{
				"version":   "pin-2",
				"aggregate": "newhashdemo",
			},
		}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func firstPending(base string) (string, error) {
	cli := &http.Client{Timeout: 3 * time.Second}
	resp, err := cli.Get(base + "/v1/approvals/pending")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var list []hitl.ApprovalRequest
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", fmt.Errorf("no pending approvals")
	}
	return list[0].ID, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\n  hitl serve -listen 127.0.0.1:8765\n  hitl request -base http://127.0.0.1:8765 [-file diff.json] [-timeout 60]\n  hitl decide -base http://127.0.0.1:8765 -decision approve|deny -who alice [-id ID]\n")
}
