# Architecture Maturity Slice 6.3a — App Action/Input Preparatory Delegation

Date: 2026-07-24
Execution predecessor: Phase 0 PR #214, merge `fba4e9c`
Branch: `arch/l1-01a-app-action-input-prep`
Finding state: L1-01 remains **partial**; formal closure is Slice 6.3d after L1-02 through L1-06.

## Scope and constraints

This slice moves route coordination only. `App` retains every mutable binding, modal, search, mouse, pane, mux, config, script and native value plus concrete action effects. Controllers are private, per-projection, and contain only narrow concern ports. Character/IME routing and GLFW callback registration/lifetime remain unchanged.

Known defect `L4-05` (unbound Ctrl+V is dropped) is characterized and expires at Slice 2.2; it is not fixed here. Fixed input ordering is marked `L1-06`, expiring at Slice 6.1b. The temporary concrete-action command port expires at Slice 6.3d.

## Required relations

- `execution_predecessor`: Phase 0 / PR #214.
- `semantic_depends_on`: none for preparatory delegation; L1-02, L1-03, L1-04, L1-05 and L1-06 before formal L1-01 closure.

## Changed-path allowlist

No generated exceptions. The PR must contain only:

```text
.gitattributes
docs/architecture.md
docs/validation/architecture-maturity-slice-6.3a.md
docs/validation/architecture-maturity-slice-6.3a/*.txt
internal/frontend/glfwgl/action_bindings.go
internal/frontend/glfwgl/action_bindings_test.go
internal/frontend/glfwgl/action_controller.go
internal/frontend/glfwgl/action_controller_test.go
internal/frontend/glfwgl/action_executor.go
internal/frontend/glfwgl/action_window_context.go
internal/frontend/glfwgl/action_window_test.go
internal/frontend/glfwgl/app.go
internal/frontend/glfwgl/app_callbacks.go
internal/frontend/glfwgl/input_controller.go
internal/frontend/glfwgl/input_controller_test.go
internal/frontend/glfwgl/mouse_bindings_test.go
internal/frontend/glfwgl/projection_factory_glfw.go
```

## Atomic commit sequence

| Class | Commit | Purpose |
|---|---|---|
| T | `80361d3` | Characterize key/action/mouse precedence and retained L4-05 defect. |
| A | `7402b36` | Add private, unwired typed controller seams and fake-port traces. |
| M | `3e85d76` | Mechanically split legacy route bodies into named App adapters. |
| W | `d6c6d5a` | Wire App entry points and per-projection controller construction. |
| G | pending at capture | Remove legacy duplicates; add aggregate/initialization/allocation guards, evidence and documentation. |

Every committed stage passed its focused GLFW package tests before the next stage.

## Behavior and architecture evidence

- Action validation, registry lookup, target resolution, pane existence, per-child active-projection refresh and first-error stop are pinned.
- Key order is suppression clear → modal → search → reload → named table → root script → builtin → selection copy → terminal.
- Button, cursor and wheel ordering/short-circuiting are pinned with fake ports; modal capture precedes cursor lookup.
- Focus is native record → blur cleanup when applicable → script → terminal report.
- Character/IME callback remains direct through `routeGLFWChar`.
- Owner and child controllers are distinct; lazy construction is idempotent.
- Every individual port has at most five methods. The temporary input aggregate is explicitly frozen at 36 methods and expires with the App facade work; silent surface growth fails tests.
- Controller fields reject concrete `*App`, `*Mux`, `*glfw.Window`, `*script.Runtime`, maps and function bags.
- Fake-controller and full warmed App routes assert zero allocations.

## Performance disposition

### Canonical unrelated parity benchmarks

`internal/vt` tree `eef44d3e302b409c234fe4e091d12b898fa81410` and `internal/render` tree `c80a2d0d98d48f03624a47450c4cb6b2c35055cf` are byte-identical between base and candidate. Two ten-sample runs were executed in opposite orders:

| Benchmark | Base first → candidate second | Candidate first → base second | 20-sample pooled candidate delta |
|---|---:|---:|---:|
| VT parser | +4.00% | +0.58% | +2.64% |
| Core reuse | +7.49% | -0.23% | +3.27% |
| New terminal | +17.20% | -3.58% | +3.35% |
| Render reuse | -0.35% | +4.79% | +1.53% |

The direction changes with run order while the measured source trees are identical; this is classified as host/thermal variance, not a slice-caused regression. Allocations remain unchanged. Raw captures are committed in this directory.

### Direct frontend dispatch

A temporary identical benchmark file was run against base and candidate, candidate first then base, ten samples each:

| Route | Base median | Candidate median | Absolute delta | Allocations |
|---|---:|---:|---:|---:|
| Unhandled key | 13.38 ns | 21.65 ns | +8.27 ns | 0 → 0 |
| Non-pane action | 45.845 ns | 49.715 ns | +3.87 ns | 0 → 0 |

The interface dispatch cost is measured and explained, remains below 10 ns on synthetic no-effect routes, adds no allocation, and is accepted as non-material for this architecture seam. Full warmed App allocation guards remain zero. Any future growth or allocation fails the guard and the aggregate adapter expires rather than becoming permanent.

## Verification

Passed on the candidate:

```text
go run ./scripts/check-maturity-gates.go
go test ./...
go vet -unsafeptr=false ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go test -race -tags glfw ./internal/frontend/glfwgl -count=1
go run ./scripts/check-phase15-recovery.go -race
```

Independent claim verification found all ten parity/ownership claims verified. Initial adversarial review found no behavior drift and required this G commit, durable allowlist, initialization/aggregate/production-allocation guards, and paired performance disposition before merge.
