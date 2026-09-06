package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

const (
	EventPinCreated   = "pin_created"
	EventDiffDetected = "diff_detected"
	EventBlocked      = "blocked"
	EventApproved     = "approved"
	EventDenied       = "denied"
)

// JSONLAuditor is an append-only local audit log. No remote shipping.
type JSONLAuditor struct {
	path string
	mu   sync.Mutex
}

// NewJSONLAuditor writes redacted events to path (created if missing).
func NewJSONLAuditor(path string) *JSONLAuditor {
	return &JSONLAuditor{path: path}
}

// Path returns the log file path.
func (a *JSONLAuditor) Path() string { return a.path }

// Record appends one JSON object. Only AllowedFields are written.
func (a *JSONLAuditor) Record(event string, fields map[string]any) {
	if a == nil || a.path == "" {
		return
	}
	rec := redact(event, fields)
	if _, ok := rec["when"]; !ok {
		rec["when"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	line = append(line, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(line)
	_ = f.Close()
}

// Tail returns the last n JSON objects (n<=0 means 10).
func Tail(path string, n int) ([]map[string]any, error) {
	if n <= 0 {
		n = 10
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		t := sc.Text()
		if t == "" {
			continue
		}
		lines = append(lines, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]map[string]any, 0, len(lines))
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
