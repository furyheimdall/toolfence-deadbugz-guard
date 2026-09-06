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
