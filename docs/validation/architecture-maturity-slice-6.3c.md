# Architecture Maturity Slice 6.3c — App Script/Native Preparatory Delegation

Date: 2026-07-24
Execution predecessor: Slice 6.3b PR #216, merge `7656960`
Branch: `arch/l1-01c-app-script-native-prep`
Finding state: L1-01 remains **partial**; formal closure is deferred to Slice 6.3d after L1-02 through L1-06 and the 6.3d execution predecessor.

## Scope

Slice 6.3c delegates pane-bound script host routing, script callback/deferred-event/projection ordering, and native IME/accessibility activation ordering beneath unchanged App entry points. Controllers are private and per projection. `App` remains authoritative for the script runtime, config, mux/pane state, pending resize/scroll maps, timers, status and overlays, plus the native window, projection bundle, WndProc, IME/accessibility objects, adoption and rollback resources. Controller values contain zero dynamic `App` backreferences: script host stores only a pane ID and initialized scalar, while lifecycle and native controllers are zero-field values. No lifecycle ownership changes in this preparatory slice.

Default-off IME/accessibility behavior, initial and child activation order, rollback order, exact script event filtering/error reporting, deferred resize-before-scroll ordering, timer active-projection routing, and detached config/snapshot boundaries are characterized. Existing known defects remain assigned to their original findings and expiry slices; this slice adds only the temporary `L1-01` facade TODO expiring at 6.3d.

## Relations and disposition

- `execution_predecessor`: Slice 6.3b / PR #216 / merge `7656960`.
- `semantic_depends_on`: none for preparatory parity delegation; L1-02, L1-03, L1-04, L1-05 and L1-06 before formal L1-01 closure.
- Disposition: Slice 6.3c is **complete only as preparatory work**. L1-01 is not closed and the Phase 6 success criteria remain open.

## Exact changed-path allowlist

No generated exceptions. The T-through-G slice is restricted to:

```text
docs/architecture-maturity/implementation-plan.md
docs/architecture.md
docs/validation/architecture-maturity-slice-6.3c.md
docs/validation/architecture-maturity-slice-6.3c/benchmarks-base.txt
docs/validation/architecture-maturity-slice-6.3c/benchmarks-candidate.txt
docs/validation/architecture-maturity-slice-6.3c/gates.txt
docs/validation/architecture-maturity-slice-6.3c/scope-and-commits.txt
internal/frontend/glfwgl/action_bindings.go
internal/frontend/glfwgl/action_executor.go
internal/frontend/glfwgl/app.go
internal/frontend/glfwgl/app_bell.go
internal/frontend/glfwgl/app_callbacks.go
internal/frontend/glfwgl/app_host.go
internal/frontend/glfwgl/app_loop.go
internal/frontend/glfwgl/app_mux.go
internal/frontend/glfwgl/app_mux_test.go
internal/frontend/glfwgl/app_overlay.go
internal/frontend/glfwgl/app_script_native_characterization_test.go
internal/frontend/glfwgl/app_status.go
internal/frontend/glfwgl/command_palette.go
internal/frontend/glfwgl/events_glfw.go
internal/frontend/glfwgl/initial_projection.go
internal/frontend/glfwgl/mouse_bindings.go
internal/frontend/glfwgl/native_capability_controller.go
internal/frontend/glfwgl/native_capability_controller_app.go
internal/frontend/glfwgl/native_capability_controller_test.go
internal/frontend/glfwgl/projection_factory_glfw.go
internal/frontend/glfwgl/projection_ime_windows_test.go
internal/frontend/glfwgl/reload.go
internal/frontend/glfwgl/script_host_controller.go
internal/frontend/glfwgl/script_host_controller_app.go
internal/frontend/glfwgl/script_host_controller_test.go
internal/frontend/glfwgl/script_lifecycle_controller.go
internal/frontend/glfwgl/script_lifecycle_controller_app.go
internal/frontend/glfwgl/script_lifecycle_controller_test.go
scripts/check-maturity-gates.go
```

## Atomic commits

| Class | Commit | Purpose |
|---|---|---|
| T | `412a5ce` | Characterize script and native lifecycle parity and retained defects. |
| A | `43afad3` | Add private unwired script/native controller seams and fake-port traces. |
| M | `5d9628c` | Mechanically split App script/native adapters. |
| W | `7787b49` | Wire App entry points with allocation-neutral, operation-scoped controller ports. |
| G | pending | `refactor(frontend): guard script and native controller delegation`; guards, benchmarks, evidence and minimal docs. |

The static maturity gate validates the immutable T→A→M ancestry/subjects and discovers W by its exact subject and direct M parent, allowing W to be safely rewritten during repair. Before G is committed, it requires `HEAD == W` and validates the union of the immutable `base..W` range plus tracked and nonignored untracked worktree paths. Once G exists on `arch/l1-01c-app-script-native-prep`, it requires `HEAD == G` and a clean nonignored worktree. On merge/main history it validates only the immutable `base..G` slice range, so later unrelated main commits are outside this scope and are not rejected. A history-limited shallow CI checkout falls back to the documented sequence, allowlist and explicit partial/6.3d closure contract.

## Static and runtime guards

- Static AST guard fixes complete controller fields, every private port method count, aggregate budgets (`scriptHost=21`, `scriptLifecycle=14`, `nativeCapability=8`), and a five-method maximum per port.
- Static and reflection guards reject App, mux, GLFW window, script runtime, projection bundle, WndProc, GL/GPU, prepared/resource, map, callback/function-bag and channel ownership in controller fields or port signatures.
- Each controller carries exactly one `TODO(L1-01; expires Slice 6.3d)` facade-adapter expiry.
- Runtime characterization pins script/native ordering, failure/rollback behavior, detached config/pending values, distinct owner/child adapters, pane-zero compatibility behavior and zero-allocation direct routes.
- `App` remains the only lifecycle/resource authority. Ports are operation-scoped; controllers have zero dynamic `App` backreferences, and `paneHost` retains its stable pane route without allocation.

## Direct same-host performance evidence

Windows/amd64, AMD Ryzen 9 7940HX, Go 1.25.8. The repaired candidate direct routes were rerun in seven `-benchtime=500ms` samples. Base was not rerun because the direct benchmark semantics did not change; `benchmarks-base.txt` remains the prior same-host `origin/main` (`7656960`) capture. The FireOutput rows are a base-compatible overlay comparison within the candidate process: direct `Runtime.FireOutput(newPaneHost(...))` versus the active `App.fireScriptOutput` route, with equivalent setup outside the timed loop.

| Direct benchmark | Comparison median | Candidate median | Candidate delta | Candidate allocation |
|---|---:|---:|---:|---:|
| No-runtime App lifecycle pass | origin/main 9.579 ns/op | 18.570 ns/op | +8.991 ns, +93.86% | 0 B/op, 0 allocs/op |
| Warmed pane script-host reads | origin/main 3.696 ns/op | 6.212 ns/op | +2.516 ns, +68.07% | 0 B/op, 0 allocs/op |
| Default-disabled native IME + accessibility | origin/main 5.672 ns/op | 5.718 ns/op | +0.046 ns, +0.81% | 0 B/op, 0 allocs/op |
| FireOutput direct production baseline | candidate overlay 7,897 ns/op | 8,041 ns/op active App route | +144 ns, +1.82% | 11,960 B/op, 95 allocs/op on both; allocation delta 0 |
| Phase13 disabled draw | origin/main 43,508 ns/op | 44,603 ns/op (prior capture) | +1,095 ns, +2.52% | 0 B/op, 0 allocs/op |
| Phase13 disabled frame | origin/main 7.020 ns/op | 7.198 ns/op (prior capture) | +0.178 ns, +2.54% | 0 B/op, 0 allocs/op |
| Presentation rejected admission | origin/main 24.070 ns/op | 24.070 ns/op (prior capture) | 0.000 ns, 0.00% | 0 B/op, 0 allocs/op |
| Accessibility UIA helper read | origin/main 176.900 ns/op | 178.400 ns/op (prior capture) | +1.500 ns, +0.85% | 352 B/op, 2 allocs/op on both |
| Accessibility UIA callback read | origin/main 505.400 ns/op | 504.500 ns/op (prior capture) | -0.900 ns, -0.18% | 768 B/op, 8 allocs/op on both |

The changed direct routes expose measured scalar-routing/ordering overhead while keeping their no-runtime/default-disabled paths allocation-free. The real FireOutput route is not allocation-free, but its active-versus-direct allocation delta is exactly zero (both measure 11,960 B/op and 95 allocs/op); only the timing median differs. Phase13, presentation and UIA rows retain their prior raw captures because their benchmark semantics and measured subsystems were unchanged by the repair. Raw captures are in the matching evidence directory.

## Verification

The post-repair focused, full, race, maturity, allocation-regression and whitespace commands and their exact results are recorded in `architecture-maturity-slice-6.3c/gates.txt`. Scope and final W identity are in `scope-and-commits.txt`.

| Gate | Result |
|---|---|
| `go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1` | PASS |
| `go test ./...` | PASS |
| `go test -race -tags glfw ./internal/frontend/glfwgl -count=1` | PASS |
| `go run ./scripts/check-maturity-gates.go` | PASS |
| `TestPaneHostRealFireOutputAddsNoAllocationBeyondWarmedBaseline` | PASS; active/baseline allocation delta 0 |
| `git diff-index --check HEAD --` | PASS; only line-ending advisories |
| Direct candidate benchmark, seven samples | PASS; raw capture recorded |

The broader vet, fuzz, recovery, packaging and daily-driver runs from the earlier pre-repair capture are not presented as post-repair reruns. Real-GUI qualification remains `UNRUN`: this preparatory slice does not intentionally change native feature defaults, window/resource ownership, geometry, pixels, renderer selection or support claims. Existing default-off native, GLFW integration, rollback, accessibility and IME tests remain automated coverage; manual real-GUI qualification is deferred to the formal App closure gate. No release, changelog, deployment or commit was produced.
