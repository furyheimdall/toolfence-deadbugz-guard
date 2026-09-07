package sidecar

import (
	"os"
	"strconv"
	"strings"
)

// Config is the wrap/plugin adapter surface (issue #7 / #20).
type Config struct {
	PinPath      string
	AuditPath    string
	HITLEndpoint string
	CallGate     int // Deadbugz path; 3 = pin + diff + call gate
	// ServerArgv is argv after `--` (the wrapped server). Used for
	// config_fingerprint (#21). Empty keeps the legacy tools-only gate.
	ServerArgv []string
	// ServerName selects ~/.deadbugz/pins/<name>.json when PinPath is empty.
	ServerName string
	// Approve, when true, installs the first pin from a live tools/list if the
	// pin file is missing. Drift and tampered pins stay fail-closed.
	Approve bool
	// ApproveFile is a one-shot token (contents approve|yes|1). Consumed after
	// a successful write. Works for PIN_MISSING (missing file) and DIFF.
	ApproveFile string
	// ApproveHTTP is a GET URL that may return {"decision":"approve"} for
	// PIN_MISSING bootstrap only (no TTY, no long-poll).
	ApproveHTTP string
}

// DefaultCallGate is Chief E / issue #7: call_gate=3.
const DefaultCallGate = 3

// ConfigFromEnv fills empty fields from PIN_PATH, AUDIT_PATH, HITL_ENDPOINT,
// CALL_GATE, SERVER_NAME / DEADBUGZ_SERVER_NAME, and DEADBUGZ_APPROVE*.
func ConfigFromEnv(c Config) Config {
	if c.PinPath == "" {
		c.PinPath = os.Getenv("PIN_PATH")
	}
	if c.AuditPath == "" {
		c.AuditPath = os.Getenv("AUDIT_PATH")
	}
	if c.HITLEndpoint == "" {
		c.HITLEndpoint = os.Getenv("HITL_ENDPOINT")
	}
	if c.ServerName == "" {
		c.ServerName = firstEnv("DEADBUGZ_SERVER_NAME")
	}
	if c.ApproveFile == "" {
		c.ApproveFile = os.Getenv("DEADBUGZ_APPROVE_FILE")
	}
	if v := os.Getenv("DEADBUGZ_APPROVE"); v != "" {
		boot, file, httpURL := parseApproveEnv(v)
		if boot {
			c.Approve = true
		}
		if c.ApproveFile == "" {
			c.ApproveFile = file
		}
		if c.ApproveHTTP == "" {
			c.ApproveHTTP = httpURL
		}
	}
	if c.CallGate == 0 {
		if v := os.Getenv("CALL_GATE"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				c.CallGate = n
			}
		}
	}
	if c.CallGate == 0 {
		c.CallGate = DefaultCallGate
	}
	return Normalize(c)
}

// Normalize expands ~ on local paths and fills a per-server pin path.
func Normalize(c Config) Config {
	c.PinPath = ExpandHome(c.PinPath)
	c.AuditPath = ExpandHome(c.AuditPath)
	c.ApproveFile = ExpandHome(c.ApproveFile)
	c.ServerName = SanitizeServerName(c.ServerName)
	if c.PinPath == "" && c.ServerName != "" {
		c.PinPath = DefaultPinPath(c.ServerName)
	}
	return c
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func parseApproveEnv(v string) (bootstrap bool, file, httpURL string) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "0", "false", "no", "off":
		return false, "", ""
	case "1", "true", "yes", "on", "approve":
		return true, "", ""
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return false, "", v
	}
	return false, v, ""
}
