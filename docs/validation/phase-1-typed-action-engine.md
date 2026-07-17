# Phase 1 Validation — Typed Action Engine

- Validated implementation commit: `bac45c72413c55a066dd1183c0f8a6527cff3f9c`
- Functional baseline: `7d64cc9`
- Validated: 2026-07-17 on Windows 11, `windows/amd64`, Go 1.25.8

## Delivery Evidence

| Slice | Pull request | Merge commit | Result |
|---|---|---|---|
| Typed values, registry, codec, sequences | [#123](https://github.com/cervantesh/CervTerm/pull/123) | `5b59c24` | Supported |
| Frontend executor and typed built-ins | [#124](https://github.com/cervantesh/CervTerm/pull/124) | `a12ba0f` | Supported |
| Lua/Teal actions and callback adapter | [#125](https://github.com/cervantesh/CervTerm/pull/125) | `132dd1f` | Supported |
| Final precedence pipeline; legacy handlers removed | [#126](https://github.com/cervantesh/CervTerm/pull/126) | `bac45c7` | Supported |

The implementation conforms to accepted [ADR-0003](../../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0003-use-a-typed-action-model.md).

## Acceptance Mapping

- `internal/action` is toolkit-neutral and owns immutable action values, validation, stable IDs, deterministic metadata, semantic targets, trigger policies, bounded JSON codecs, callback metadata, and stop-on-error sequences.
- The GLFW executor resolves focused/origin pane targets at execution time and preserves mux lifecycle, redraw, scroll, zoom, clipboard, search, and reload behavior.
- The key pipeline remains: active modal/search activation, reserved reload, user typed/callback binding, built-in table, selection-dependent Ctrl+C, then terminal encoding.
- Built-ins no longer depend on GLFW values inside action definitions. The final fixed clipboard, mux, zoom, and scroll handler adapters were removed.
- `cervterm.action` exposes immutable validated Lua values and constructors. Teal declarations mirror the API. Existing function callbacks remain runtime-local, main-thread-only, label-aware, one-second-watchdog protected, and non-serializable.
- Registry enumeration is deterministic and can feed later command-palette work without importing frontend types.

## Behavioral Evidence

Automated tests cover:

- valid/invalid arguments, duplicate IDs, deterministic enumeration, unknown fields/actions, bounded payloads, and JSON round trips;
- nested sequence ordering, bounds, validation, and stop at the first error;
- focused/origin target resolution, stale/missing targets, pane-local zoom, split/focus/close lifecycle, and one-notice error classification;
- exact PTY bytes and no-leak behavior for key press/repeat, scroll, mux, zoom, clipboard, selection Ctrl+C, and mouse normalization;
- search and reload precedence, user override precedence, character callback suppression, non-consuming repeat fallthrough, and modal input capture;
- Lua constructor errors, immutable userdata, labels/discovery, callback timeout/recovery, reload-local callback identity, and Teal compilation.

## Verification Commands

All commands passed from the validated tree:

```bash
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./internal/action ./internal/script -count=1
go test -race -tags glfw ./internal/frontend/glfwgl -count=1
go vet -unsafeptr=false ./...
go run ./scripts/check-maturity-gates.go
```

CI and CodeQL also passed for PRs #123 through #126.

## Performance Comparison

A paired five-run benchmark was executed on the same host against baseline `7d64cc9` and candidate `bac45c7`. Values are medians; timing deltas stay below the 15% investigation threshold and allocation invariants are unchanged.

| Benchmark | `7d64cc9` | `bac45c7` | Delta | Allocation invariant |
|---|---:|---:|---:|---|
| VT parser throughput | 41,274 ns/op | 41,788 ns/op | +1.2% | 0 B/op, 0 allocs/op |
| Core reuse | 41,069 ns/op | 42,823 ns/op | +4.3% | 0 B/op, 0 allocs/op |
| New terminal comparison | 82,207 ns/op | 78,866 ns/op | -4.1% | unchanged: 156,208 B/op, 4 allocs/op |
| Render snapshot capture | 2,191 ns/op | 2,207 ns/op | +0.7% | 0 B/op, 0 allocs/op |

The action work does not alter VT parsing or snapshot capture; these benchmarks guard against unrelated hot-path regressions. Dispatch correctness and race evidence are covered by the targeted suites above.

## Rollback and Residual Scope

The old fixed-handler adapters were removed only after full pipeline parity passed. Rollback is therefore the PR sequence in reverse order; no config migration or persistent data is involved. Advanced key tables, leaders/chords, typed mouse bindings, pane resize/swap/move, tabs, windows, and workspaces remain assigned to later roadmap phases rather than being claimed by Phase 1.
