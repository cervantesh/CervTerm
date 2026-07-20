# Phase 9 Validation — Windows, Workspaces, and Fresh-Session Layout Persistence

Phase 9 implements one process-owned mux across native windows, named local workspaces, transactional tab/pane transfer, typed window/workspace actions, and opt-in layout-only persistence.

## Persistence contract

- Version 1 stores ordered workspace/window/tab/split topology, native logical bounds, active/focused indices, semantic CWD, trusted user-authored local launch intent, and scalar appearance intent.
- Runtime IDs, environment maps, dedicated credential fields, PTY handles/processes, terminal cells, scrollback, renderer selection, and live sessions have no representation. Program arguments are trusted launch data and are not claimed to be secret-redacted.
- Restore always creates fresh local sessions. It never claims process continuity.
- The complete layout is decoded and normalized before windows or PTYs are created. Native projections remain hidden until all native resources, config/Teal publication, mux sessions, topology, and reader ownership commit.
- Missing, corrupt, unsafe, future-version, or unavailable-scheme state falls back to one fresh window. Off-screen bounds recover to current monitors and unavailable CWD falls back deterministically while retaining valid topology. Failed save/restore preserves the last usable state.
- The final usable layout is saved before final-window teardown; non-final topology/focus transitions save the remaining state through atomic replacement.

## Automated evidence

- Mux randomized/model tests cover globally unique windows/tabs/panes, cross-window transfer, workspace membership/focus, exact split ratios, fresh launch capture, registry ownership, rollback, and no duplicate spawn/close.
- Layout codec/store tests cover strict limits, deterministic encoding, corruption/future versions, symlink/hardlink/directory safety, staged replacement, durability, and rollback.
- Bounds tests cover monitor removal, off-screen recovery, DPI metadata, clamping, and deterministic fallback.
- GLFW fake-host tests cover hidden batch preparation, reversible bind/unbind, partial failure cleanup, final addressed activation, workspace visibility, bounds/appearance export, atomic save/load round-trip, and final-window preservation.
- GitHub Windows/Linux default CI and CodeQL passed for PRs #198–#202; GLFW-tagged and race suites plus maturity gates were run as local qualification evidence.

## Platform qualification

Windows native multi-window and multi-monitor interaction still requires the manual matrix in `docs/manual-verification.md`. Linux is qualified headlessly. macOS/Linux GUI behavior is not claimed without a native runner.
