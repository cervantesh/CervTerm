# Architecture Maturity Slice 6.3b — App Render/Reload Preparatory Delegation

Date: 2026-07-24
Execution predecessor: Slice 6.3a PR #215, merge `93168eb`
Branch: `arch/l1-01b-app-render-reload-prep`
Finding state: L1-01 remains **partial**; formal closure is Slice 6.3d after L1-02 through L1-06.

## Scope

This slice delegates projection presentation order and reload request/watch/dispatch order beneath unchanged App entry points. `App` retains renderer, atlas, GPU resources, pane/draw scratch, damage/accounting, pending state, watches, workers, generations, config/runtime/bundle ownership and the full activation transaction.

The exact frame order remains tick → fresh clock read → ready or throttle → candidate-geometry begin/body/deferred finish → EndFrame. Damage clearing, presentation recording, metering and redraw acknowledgement remain outside `withCurrent`. Reload remains result drain → pending check → two-worker cap → consume → source check → report/start; failure time is read only in the missing-source branch.

Known defects remain explicit: projection-global pane scratch/damage identity (`L1-06`, expires 6.1a), untyped reload states (`L1-06`, expires 6.1c), child-local reload ownership (`L1-02`, expires 3.4), and preparatory App adapters (`L1-01`, expires 6.3d).

## Relations

- `execution_predecessor`: Slice 6.3a / PR #215.
- `semantic_depends_on`: none for preparation; L1-02 through L1-06 before formal L1-01 closure.

## Changed-path allowlist

No generated exceptions:

```text
docs/architecture.md
docs/validation/architecture-maturity-slice-6.3b.md
docs/validation/architecture-maturity-slice-6.3b/*.txt
internal/frontend/glfwgl/app.go
internal/frontend/glfwgl/app_damage_test.go
internal/frontend/glfwgl/app_draw.go
internal/frontend/glfwgl/app_draw_characterization_test.go
internal/frontend/glfwgl/app_loop.go
internal/frontend/glfwgl/presentation_gate_test.go
internal/frontend/glfwgl/projection_factory_glfw.go
internal/frontend/glfwgl/reload.go
internal/frontend/glfwgl/reload_background_async_test.go
internal/frontend/glfwgl/reload_controller.go
internal/frontend/glfwgl/reload_controller_app.go
internal/frontend/glfwgl/reload_controller_test.go
internal/frontend/glfwgl/reload_test.go
internal/frontend/glfwgl/render_controller.go
internal/frontend/glfwgl/render_controller_test.go
```

## Atomic commits

| Class | Commit | Purpose |
|---|---|---|
| T | `8c85d5f` | Characterize presentation, panic cleanup, reload dispatch and known defects. |
| A | `9bcfef1` | Add private unwired render/reload ordering seams. |
| M | `15c03f8` | Mechanically split App render/reload adapters. |
| W | `c4d1d83` | Wire per-projection controllers with exact clock/GL/reload timing. |
| G | pending at capture | Add expiry, evidence, documentation and final guards. |

## Guards

- Every port is private, listed, at most five methods and counted against a fixed aggregate budget.
- Controller fields are interface-only and reject App, mux, GLFW window, renderer, runtime, config/prepared resources, maps and function bags.
- Pending reload state remains App-owned through a narrow port; no controller owns a mutable generation or resource.
- Owner/child controllers are distinct; eager and lazy initialization is idempotent.
- Rejected render admission and inactive/capped/dispatch reload controller paths allocate zero.
- Panic cleanup finishes candidate geometry without calling EndFrame.

## Performance

Identical built-in benchmarks were run candidate first and base second with ten samples on Windows/amd64:

| Benchmark | Base median | Candidate median | Delta | Allocation |
|---|---:|---:|---:|---:|
| Phase13 disabled draw | 44,583.5 ns | 45,008 ns | +0.95% | 0 B / 0 allocs unchanged |
| Phase13 disabled frame | 7.3025 ns | 7.289 ns | -0.18% | 0 B / 0 allocs unchanged |

Identical temporary App-entry benchmarks directly compared base `93168eb` with the candidate, candidate first and base second:

| Entry path | Base median | Candidate median | Absolute overhead | Allocation |
|---|---:|---:|---:|---:|
| Draw bracket dispatch (panic-cleanup harness) | 1,867.5 ns | 1,929.5 ns | +62 ns | unchanged |
| Active reload request | 0.51145 ns | 4.406 ns | +3.895 ns | 0 B / 0 allocs |
| Inactive reload apply | 3.034 ns | 5.966 ns | +2.932 ns | 0 B / 0 allocs |

A same-binary dual oracle directly exercised rejected, admitted and continuous render admission. Interface delegation adds 3.107 ns, 6.856 ns and 6.699 ns respectively, with 0 B/0 allocations. Percentage deltas are large only because the synthetic no-effect baselines are 1.3–3.3 ns; the measured absolute cost is under 7 ns and the real disabled draw benchmark remains +0.95%. This is explained, reviewed as non-material structural dispatch overhead, and bounded by zero-allocation/port-budget/expiry guards rather than hidden.

All raw LF captures are committed in this directory. There is no unexplained material regression and no allocation regression.

## Verification

Required before merge:

```text
go run ./scripts/check-maturity-gates.go
go test ./...
go vet -unsafeptr=false ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go test -race -tags glfw ./internal/frontend/glfwgl -count=1
go run ./scripts/check-phase15-recovery.go -race
```

Windows real-GUI qualification is `UNRUN` for this structural slice: no native geometry, pixels, renderer selection, support or default behavior intentionally changes. Existing GLFW integration/fault tests and the disabled draw/frame benchmarks are the blocking evidence; a manual on-demand/continuous and successful/failed reload smoke remains recommended before the later formal App closure.
