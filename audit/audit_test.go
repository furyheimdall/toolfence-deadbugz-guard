package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	a := NewJSONLAuditor(path)
	for i := 0; i < 5; i++ {
		a.Record(EventPinCreated, map[string]any{"who": "t", "oldHash": "", "newHash": "h"})
	}
	// Append-only: rewrite must not truncate.
	a.Record(EventDiffDetected, map[string]any{
		"who": "t", "oldHash": "aaa", "newHash": "aaa", "added": []string{"x"},
	})

	all, err := Tail(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("len=%d want 6", len(all))
	}
	last, err := Tail(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 2 {
		t.Fatalf("tail n=2 len=%d", len(last))
	}
	if last[1]["event"] != EventDiffDetected {
		t.Fatalf("last event %v", last[1]["event"])
	}
}

func TestRedactionDropsSecretsAndRawArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	a := NewJSONLAuditor(path)
	a.Record(EventBlocked, map[string]any{
		"who":           "alice",
		"oldHash":       "old",
		"newHash":       "old",
		"tool_args":     map[string]any{"password": "hunter2"},
		"authorization": "Bearer secret",
		"inputSchema":   `{"api_key":"k"}`,
		"token":         "abc",
		"added":         []string{"evil"},
	})
	rows, err := Tail(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	for _, banned := range DeniedFieldNames {
		if _, ok := rows[0][banned]; ok {
			t.Fatalf("redaction leaked %s", banned)
		}
	}
	if rows[0]["who"] != "alice" {
		t.Fatalf("who=%v", rows[0]["who"])
	}
	if rows[0]["oldHash"] != "old" || rows[0]["newHash"] != "old" {
		t.Fatalf("hashes %+v", rows[0])
	}
}

func TestRequiredEventsAndHashTransition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	a := NewJSONLAuditor(path)
	a.Record(EventPinCreated, map[string]any{"who": "ops", "oldHash": "", "newHash": "hash1"})
	a.Record(EventDiffDetected, map[string]any{"who": "ops", "oldHash": "hash1", "newHash": "hash1", "added": []string{"evil"}})
	a.Record(EventBlocked, map[string]any{"who": "ops", "oldHash": "hash1", "newHash": "hash1"})
	a.Record(EventApproved, map[string]any{
		"who": "alice", "when": "2026-09-06T00:00:00Z",
		"oldHash": "hash1", "newHash": "hash2",
	})

	rows, err := Tail(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{EventPinCreated, EventDiffDetected, EventBlocked, EventApproved}
	if len(rows) != len(want) {
		t.Fatalf("len=%d", len(rows))
	}
	for i, ev := range want {
		if rows[i]["event"] != ev {
			t.Fatalf("row %d event %v", i, rows[i]["event"])
		}
	}
	re := rows[3]
	if re["who"] != "alice" || re["when"] == nil || re["oldHash"] != "hash1" || re["newHash"] != "hash2" {
		t.Fatalf("re-approve fields %+v", re)
	}
}

func TestAppendDoesNotTruncateExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("{\"event\":\"pin_created\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewJSONLAuditor(path)
	a.Record(EventDenied, map[string]any{"who": "bob", "oldHash": "x", "newHash": "x"})
	rows, err := Tail(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["event"] != EventPinCreated || rows[1]["event"] != EventDenied {
		t.Fatalf("%+v", rows)
	}
}
