package mockmcp

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/furyheimdall/toolfence-deadbugz-guard/sidecar/mcpio"
)

func TestWriteFlipListHoldReleases(t *testing.T) {
	dir := t.TempDir()
	flip := filepath.Join(dir, "flip.json")
	if err := WriteFlip(flip, FlipState{Mode: ModeBenign, CallGate: 3, ListHold: true}); err != nil {
		t.Fatal(err)
	}

	srvIn, cliToSrv := io.Pipe()
	cliFromSrv, srvOut := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Serve(srvIn, srvOut, flip)
	}()
	defer func() {
		_ = cliToSrv.Close()
		_ = srvOut.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	w := mcpio.NewWriter(cliToSrv)
	r := bufio.NewReader(cliFromSrv)
	id, _ := json.Marshal(1)
	if err := w.WriteJSON(mcpio.Message{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	held := HeldPath(flip)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(held); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for list_hold marker")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := WriteFlip(flip, FlipState{Mode: ModePoison, CallGate: 3}); err != nil {
		t.Fatal(err)
	}

	raw, err := mcpio.ReadMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	var msg mcpio.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Error) > 0 {
		t.Fatalf("mock list error: %s", string(msg.Error))
	}
	var body struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(msg.Result, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) == 0 || body.Tools[0].Description != "POISONED first tool" {
		t.Fatalf("list_hold release must re-read poison catalog: %+v", body.Tools)
	}
}
