package hitl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/core"
)

func TestHTTPStubApproveAndDeny(t *testing.T) {
	srv := NewServer()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	diff := core.ToolDiffSummary{
		Added:       []string{"evil"},
		Removed:     []string{},
		Changed:     []string{"search"},
		PinRevision: "pin-1",
		LiveHash:    "live",
		PinHash:     "pin",
	}
	cand := core.Pin{Version: "pin-2", Aggregate: "live"}

	type outcome struct {
		ver string
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		ver, err := srv.RequestApproval(context.Background(), diff, cand)
		ch <- outcome{ver, err}
	}()

	deadline := time.Now().Add(2 * time.Second)
	var id string
	for time.Now().Before(deadline) && id == "" {
		resp, err := http.Get(ts.URL + "/v1/approvals/pending")
		if err != nil {
			t.Fatal(err)
		}
		var list []ApprovalRequest
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			_ = resp.Body.Close()
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if len(list) == 1 {
			if list[0].Diff.Added[0] != "evil" || list[0].Diff.PinRevision != "pin-1" {
				t.Fatalf("payload not ToolDiffSummary: %+v", list[0].Diff)
			}
			id = list[0].ID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("pending request never appeared")
	}

	body, _ := json.Marshal(DecisionBody{Decision: "approve", Who: "carol"})
	resp, err := http.Post(ts.URL+"/v1/approvals/"+id+"/decide", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	o := <-ch
	if o.err != nil || o.ver != "pin-2" {
		t.Fatalf("approve: ver=%s err=%v", o.ver, o.err)
	}
	if srv.Actor() != "carol" {
		t.Fatalf("who=%s", srv.Actor())
	}

	// deny path
	ch2 := make(chan outcome, 1)
	go func() {
		ver, err := srv.RequestApproval(context.Background(), diff, cand)
		ch2 <- outcome{ver, err}
	}()
	id = ""
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && id == "" {
		resp, err := http.Get(ts.URL + "/v1/approvals/pending")
		if err != nil {
			t.Fatal(err)
		}
		var list []ApprovalRequest
		_ = json.NewDecoder(resp.Body).Decode(&list)
		_ = resp.Body.Close()
		if len(list) == 1 {
			id = list[0].ID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	body, _ = json.Marshal(DecisionBody{Decision: "deny", Who: "dave"})
	resp, err = http.Post(ts.URL+"/v1/approvals/"+id+"/decide", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	o = <-ch2
	if o.err != ErrDenied {
		t.Fatalf("deny err=%v", o.err)
	}
}

func TestHTTPStubTimeoutIsDeny(t *testing.T) {
	srv := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	_, err := srv.RequestApproval(ctx, core.ToolDiffSummary{Added: []string{"x"}}, core.Pin{Version: "p2"})
	if err != ErrTimeout {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}
