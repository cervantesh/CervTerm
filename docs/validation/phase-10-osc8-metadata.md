# Phase 10.1 Validation — Bounded OSC 8 Metadata

This slice parses and retains OSC 8 hyperlink metadata without adding any external side effect.

## Contract

- OSC 8 BEL/ST forms are accumulated by the existing 64 KiB all-or-nothing collector; malformed, truncated, invalid-UTF-8, control-bearing, oversized URI/parameter, duplicate-id, and schemeless opens mutate nothing.
- Cells carry a 16-bit pane-local identity while remaining 32 bytes on amd64. URI strings live in a table bounded to 4096 entries, 2048 URI bytes and 1024 parameter bytes.
- Referenced identities are never evicted. Unreferenced slots are reclaimed safely; IDs are reused only after no active/scrollback cell can reference them.
- Printed spaces and both halves of wide glyphs retain identity. Erase/insert/clear blanks do not. Copy, scrollback, reflow, resize and combining operations preserve identity.
- Primary and alternate screens own isolated tables/current spans; reset clears only the active screen state.
- Render and quick-select snapshots project detached, visible, resolvable URI metadata. Hyperlink-only changes participate in row damage.
- URI schemes are syntactically parsed as metadata only. Scheme allowlisting and opening remain the explicit frontend policy work of Slice 10.2.

## Evidence

Focused core/VT/render/mux tests cover parser terminators/chunks/atomicity/limits, table capacity and referenced-entry behavior, 32-byte cell size, wide/erase/reflow/scrollback/alternate/reset lifetime, snapshot detachment and row damage. No code in this slice invokes the OS URL opener or a native API.
