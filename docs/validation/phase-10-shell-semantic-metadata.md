# Phase 10.3 Validation — Shell Semantic Metadata

This slice adds bounded metadata-only OSC 133/633 shell semantics. It does not add actions, scripting APIs, notifications, or external effects.

## Contract

- Complete BEL/ST OSC 133 and 633 markers map `A/B/C/D` to prompt/input/output/none; OSC 633 `E` marks input while discarding its command payload. Unknown markers and properties are ignored.
- Recognized payloads are UTF-8/control validated and capped at 1024 bytes before any state mutation. Truncated collector payloads remain all-or-nothing.
- `SemanticKind` occupies the final byte of the existing cell layout; `Cell` remains 32 bytes on amd64 and hyperlink capacity is unchanged.
- Printed spaces, lead cells, wide continuations and combining bases inherit the active kind. Erase/insert/clear blanks carry none. Cell copy, scrollback, reflow and resize preserve metadata.
- Primary and alternate screens retain isolated active marker state; reset clears the active state.
- Visible semantic runs are projected as detached, capped `SemanticZone` values through render and mux snapshots. Projection reports truncation at 4096 zones and semantic-only changes participate in row damage.
- Shell command lines, status values, CWD/property payloads and arbitrary OSC fields are never retained or logged.

## Evidence

Focused core tests cover the 32-byte budget, cluster propagation, erasure, scrollback/reflow/resize, alternate/reset behavior and bounded projection. VT tests cover BEL/ST/chunking, OSC 133/633 transitions, unknown/property markers, payload non-retention and malformed/oversized atomicity. Render and mux tests cover detached projection and damage behavior.
