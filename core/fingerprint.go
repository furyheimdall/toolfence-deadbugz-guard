package core

import (
	"path/filepath"
	"sort"
	"strings"
)

// InventoryEnvKeys are the env names hashed into config_fingerprint.
// These change the advertised tool inventory (GitHub toolsets / --tools
// equivalents and credential-scope labels). Secret-bearing names are
// never included even if a caller passes them in Env.
var InventoryEnvKeys = []string{
	"DEADBUGZ_CREDENTIAL_SCOPE",
	"GITHUB_CONTENT_FILTER",
	"GITHUB_DYNAMIC_TOOLSETS",
	"GITHUB_READ_ONLY",
	"GITHUB_SCOPES",
	"GITHUB_TOOLS",
	"GITHUB_TOOLSETS",
	"GH_SCOPES",
	"MCP_TOOLS",
	"MCP_TOOLSETS",
}

var inventoryEnvSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(InventoryEnvKeys))
	for _, k := range InventoryEnvKeys {
		m[k] = struct{}{}
	}
	return m
}()

var inventoryCSVEnv = map[string]struct{}{
	"GITHUB_TOOLS":    {},
	"GITHUB_TOOLSETS": {},
	"MCP_TOOLS":       {},
	"MCP_TOOLSETS":    {},
}

var inventoryCSVFlags = map[string]struct{}{
	"--tools":    {},
	"--toolsets": {},
}

// NewConfigIdentity builds a fingerprint input from argv after `--` and
// a raw environ list (`KEY=value`). Only InventoryEnvKeys are kept.
func NewConfigIdentity(argv []string, environ []string) ConfigIdentity {
	return ConfigIdentity{
		ServerName: ServerNameFromArgv(argv),
		Argv:       append([]string(nil), argv...),
		Env:        SubsetEnv(environ),
	}
}

// ServerNameFromArgv is the basename of argv[0] (the wrapped server).
func ServerNameFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return filepath.Base(argv[0])
}

// SubsetEnv keeps inventory keys from a `KEY=value` environ list.
func SubsetEnv(environ []string) map[string]string {
	out := make(map[string]string)
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		if _, keep := inventoryEnvSet[k]; !keep {
			continue
		}
		if secretEnvKey(k) {
			continue
		}
		if _, csv := inventoryCSVEnv[k]; csv {
			v = sortCSV(v)
		}
		out[k] = v
	}
	return out
}

// ConfigFingerprint = hash(stable(argv_after_separator) || subset(env)).
// Encoding is canonical JSON of {"argv":[...],"env":{...}} so key order
// in env does not matter. Known --tools / --toolsets CSV values are sorted.
func ConfigFingerprint(argv []string, env map[string]string) string {
	h, err := hashCanonical(map[string]any{
		"argv": StableArgv(argv),
		"env":  stableEnv(env),
	})
	if err != nil {
		return ""
	}
	return h
}

// ComputePinID = hash(server_name || config_fingerprint || canonical_tools_hash).
func ComputePinID(serverName, configFingerprint, canonicalToolsHash string) string {
	h, err := hashCanonical(map[string]any{
		"canonical_tools_hash": canonicalToolsHash,
		"config_fingerprint":   configFingerprint,
		"server_name":          serverName,
	})
	if err != nil {
		return ""
	}
	return h
}

// BindPinIdentity fills ServerName, ConfigFingerprint, and PinID on p.
func BindPinIdentity(p *Pin, cfg ConfigIdentity) {
	if p == nil {
		return
	}
	name := cfg.ServerName
	if name == "" {
		name = ServerNameFromArgv(cfg.Argv)
	}
	fp := ConfigFingerprint(cfg.Argv, cfg.Env)
	p.ServerName = name
	p.ConfigFingerprint = fp
	p.PinID = ComputePinID(name, fp, p.Aggregate)
}

// PinToolsWithConfig is PinTools plus #21 identity fields.
func PinToolsWithConfig(tools []ToolDef, version string, cfg ConfigIdentity) Pin {
	p := PinTools(tools, version)
	BindPinIdentity(&p, cfg)
	return p
}

// (cfg ConfigIdentity) Fingerprint is the hash for this identity.
func (cfg ConfigIdentity) Fingerprint() string {
	return ConfigFingerprint(cfg.Argv, cfg.Env)
}

// StableArgv is a deterministic copy of argv after `--`. Inventory CSV
// flags (--tools, --toolsets) have their comma-separated values sorted
// so `repos,issues` and `issues,repos` share a fingerprint.
func StableArgv(argv []string) []string {
	out := append([]string(nil), argv...)
	for i := 0; i < len(out); i++ {
		flag, val, inline := splitArgvFlag(out[i])
		if _, ok := inventoryCSVFlags[flag]; !ok {
			continue
		}
		if inline {
			out[i] = flag + "=" + sortCSV(val)
			continue
		}
		if i+1 < len(out) && !strings.HasPrefix(out[i+1], "-") {
			out[i] = flag + "=" + sortCSV(out[i+1])
			out = append(out[:i+1], out[i+2:]...)
		}
	}
	return out
}

func stableEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if k == "" || secretEnvKey(k) {
			continue
		}
		if _, ok := inventoryEnvSet[k]; !ok {
			// Callers may pass an already-subsetted map; still drop
			// anything outside the allowlist so tokens cannot leak in.
			continue
		}
		if _, csv := inventoryCSVEnv[k]; csv {
			v = sortCSV(v)
		}
		out[k] = v
	}
	return out
}

func splitArgvFlag(s string) (flag, val string, inline bool) {
	if !strings.HasPrefix(s, "--") {
		return "", "", false
	}
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

func sortCSV(s string) string {
	parts := strings.Split(s, ",")
	trimmed := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		trimmed = append(trimmed, p)
	}
	sort.Strings(trimmed)
	return strings.Join(trimmed, ",")
}

func secretEnvKey(k string) bool {
	u := strings.ToUpper(k)
	for _, needle := range []string{
		"TOKEN", "SECRET", "PASSWORD", "PASSWD",
		"AUTHORIZATION", "API_KEY", "PRIVATE_KEY",
	} {
		if strings.Contains(u, needle) {
			return true
		}
	}
	return false
}
