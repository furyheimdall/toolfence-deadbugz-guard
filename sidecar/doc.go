// Package sidecar is the Deadbugz guard process adapter (#7).
//
// Primary attach: stdio wrap
//
//	deadbugz-guard -- <server>
//
// The wrap hashes/diffs every tools/list (including a guard-owned refresh
// after notifications/tools/list_changed) before the host sees the listing.
// Host list_changed / deferred-tool refresh is not a security boundary.
// When call_gate=3, the same pin → diff path runs before tools/call.
//
// Issue #20: IDE spawn has no TTY. First pin / re-pin is headless
// (flag / env / approve-file / HTTP GET / `deadbugz-guard approve`).
// Missing or tampered pins stay fail-closed; there is no silent re-pin.
//
// It uses the merged #12 core.Gate / MemoryGate / PinTools types.
// Do not redefine those seats here.
//
// Beside attach is docker-compose / mcp-gateway. AGT is a juxtaposition
// reference only — this package does not copy AGT names or code.
package sidecar
