package audit

// AllowedFields is the redaction-safe allowlist for JSONL records.
// Anything else (secrets, raw tool arguments, inputSchema, tokens) is dropped.
//
// Hash transitions use oldHash / newHash. Re-approve (Chief G) also records
// who / when.
var AllowedFields = []string{
	"event",
	"when",
	"who",
	"oldHash",
	"newHash",
	"pin_revision",
	"pin_hash",
	"live_hash",
	"added",
	"removed",
	"changed",
	"reason_code",
	"decision",
	"approved_version",
}

var allowedSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedFields))
	for _, k := range AllowedFields {
		m[k] = struct{}{}
	}
	return m
}()

// DeniedFieldNames are documented examples that must never be persisted.
var DeniedFieldNames = []string{
	"tool_args",
	"arguments",
	"inputSchema",
	"authorization",
	"token",
	"secret",
	"password",
	"api_key",
	"raw",
}

func redact(event string, fields map[string]any) map[string]any {
	out := make(map[string]any, len(AllowedFields))
	out["event"] = event
	if fields == nil {
		return out
	}
	for k, v := range fields {
		if _, ok := allowedSet[k]; !ok {
			continue
		}
		if k == "event" {
			continue
		}
		switch k {
		case "added", "removed", "changed":
			out[k] = asNameList(v)
		default:
			out[k] = v
		}
	}
	return out
}

func asNameList(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}
}
