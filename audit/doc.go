// Package audit is the #8 local append-only JSONL hook.
//
// Events: pin_created, diff_detected, blocked, approved, denied.
// Chief requires at least pin / diff / block / re-approve, and
// oldHash / newHash on hash transitions (who / when on re-approve).
//
// Record persists only AllowedFields. Secrets, raw tool arguments,
// inputSchema, and tokens are dropped — see DeniedFieldNames.
//
// CLI: go run ./cmd/audit -path audit.jsonl tail -n N
package audit
