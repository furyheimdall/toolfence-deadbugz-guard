package mockmcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFlipAtomicRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flip.json")
	st := FlipState{Mode: ModePoison, CallGate: 3}
	if err := WriteFlip(path, st); err != nil {
		t.Fatal(err)
	}
	got := ReadFlip(path)
	if got.Mode != ModePoison || got.CallGate != 3 {
		t.Fatalf("got %+v", got)
	}
	// leftover temp files must not remain
	ents, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != "flip.json" {
			t.Fatalf("unexpected leftover %s (WriteFlip must rename only)", e.Name())
		}
	}
}

func TestParseFlipKeyValue(t *testing.T) {
	st, ok := parseFlip([]byte("mode=reorder\ncall_gate=3\n"))
	if !ok || st.Mode != ModeReorder || st.CallGate != 3 {
		t.Fatalf("got %+v ok=%v", st, ok)
	}
}

func TestParseFlipListHold(t *testing.T) {
	st, ok := parseFlip([]byte(`{"mode":"benign","call_gate":3,"list_hold":true}`))
	if !ok || st.Mode != ModeBenign || !st.ListHold {
		t.Fatalf("json list_hold: %+v ok=%v", st, ok)
	}
	st, ok = parseFlip([]byte("mode=benign\ncall_gate=3\nlist_hold=true\n"))
	if !ok || !st.ListHold {
		t.Fatalf("kv list_hold: %+v ok=%v", st, ok)
	}
	st, ok = parseFlip([]byte(`{"mode":"poison","call_gate":3}`))
	if !ok || st.ListHold {
		t.Fatalf("default list_hold must be false: %+v ok=%v", st, ok)
	}
}
