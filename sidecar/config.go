package sidecar

import (
	"os"
	"strconv"
)

// Config is the wrap/plugin adapter surface (issue #7).
type Config struct {
	PinPath      string
	AuditPath    string
	HITLEndpoint string
	CallGate     int // Deadbugz path; 3 = pin + diff + call gate
	// ServerArgv is argv after `--` (the wrapped server). Used for
	// config_fingerprint (#21). Empty keeps the legacy tools-only gate.
	ServerArgv []string
}

// DefaultCallGate is Chief E / issue #7: call_gate=3.
const DefaultCallGate = 3

// ConfigFromEnv fills empty fields from PIN_PATH, AUDIT_PATH, HITL_ENDPOINT, CALL_GATE.
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
	return c
}
