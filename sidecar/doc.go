// Package sidecar is the Deadbugz guard process adapter (#7).
//
// Primary attach: stdio wrap
//
//	deadbugz-guard -- <server>
//
// The wrap observes tools/list against a pin and, when call_gate=3,
// evaluates the Deadbugz path (pin → diff → call gate) before tools/call.
//
// It uses the merged #12 core.Gate / MemoryGate / PinTools types.
// Do not redefine those seats here.
//
// Beside attach is docker-compose / mcp-gateway. AGT is a juxtaposition
// reference only — this package does not copy AGT names or code.
package sidecar
