# Phase 12 accessibility close-out

Date: 2026-07-20
Disposition: **experimental default-off implementation complete; screen-reader support remains unclaimed**

## Revisions and environment

- Baseline: `820c389` (Phase 12.8 dormant lifecycle, before public activation)
- Activation candidate: `5794064` (local dev merge of Phase 12.9)
- Immutable focused-benchmark candidate: `1dfded5`; synchronized, fingerprinted process-evidence harness: `e091a84`. These commits add diagnostic/test-only code; production activation remains `5794064`.
- Host: Microsoft Windows 11 Home, version `10.0.26200`, build `26200`, 64-bit
- CPU: AMD Ryzen 9 7940HX with Radeon Graphics
- Toolchain: Go `1.25.8 windows/amd64`
- Scope/config: identical 640x480 local `cmd.exe` session; candidate measured with `scope="visible"`, first disabled and then enabled

## Focused benchmarks

Each benchmark ran three times on the same host. The table reports the median. Allocation ceilings are enforced by tests.

| Benchmark | Baseline `820c389` | Candidate `1dfded5` | Delta | Candidate allocations | Enforced ceiling |
| --- | ---: | ---: | ---: | ---: | ---: |
| 80x24 production-shaped Unicode semantic capture | 193,629 ns/op | 185,893 ns/op | -4.00% | 509,252 B/op, 587 allocs/op | 614,400 B/op, 700 allocs/op |
| Two-transition semantic event burst/coalescing | 119,716 ns/op | 123,566 ns/op | +3.22% | 2,368 B/op, 97 allocs/op | 4,096 B/op, 128 allocs/op |
| UIA COM callback property read with BSTR ownership | 678.2 ns/op | 698.2 ns/op | +2.95% | 768 B/op, 8 allocs/op | 1,024 B/op, 10 allocs/op |

Commands:

```text
go test ./internal/accessibility -run '^$' -bench '^BenchmarkAccessibility(SemanticCapture|EventCoalescing)$' -benchmem -count=3
go test -tags glfw ./internal/frontend/glfwgl -run '^$' -bench '^BenchmarkAccessibilityUIA(Callback|ProviderHelper)Read$' -benchmem -count=3
```

For the `820c389` baseline, copy `internal/accessibility/benchmark_test.go` and `internal/frontend/glfwgl/windows_uia_benchmark_windows_test.go` byte-for-byte from `1dfded5` into the detached worktree before running the commands; these are test-only fixtures and the baseline runtime remains otherwise unchanged. Candidate commands ran from a clean detached `1dfded5` worktree.

`go run ./scripts/capture-parity-baseline.go -count 3` was also run from clean detached worktrees at both revisions on the same host. Baseline → candidate medians were parser 32,348 → 34,585 ns/op (+6.92%), core reuse 35,839 → 36,561 ns/op (+2.01%), new terminal 74,789 → 72,346 ns/op (-3.27%), and render snapshot 8,968 → 9,100 ns/op (+1.47%). Allocations were unchanged and every delta remained below the 15% investigation threshold. Raw outputs are tracked in `docs/validation/phase-12-parity-baseline-comparison.txt`; focused accessibility outputs are in `docs/validation/phase-12-accessibility-benchmarks.txt`.

## Startup, memory, handles, wake, and frame evidence

Three fresh processes per configuration were polled until a responding native window existed. After a one-second post-readiness warmup, the script writes a per-run start signal. The identical `accessibilitymetrics` harness receives that signal, forces GC, measures exactly three seconds of projection wakes/frames/Go allocations, forces GC again and writes final heap/meter values. Process CPU uses the same signaled interval. Values are medians.

| Configuration | Ready | Working set | Private bytes | Go heap | Handles | CPU / 3 s | Wakes | Frames |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Phase 12.8 baseline + identical probe | 436.65 ms | 104.01 MiB | 166.29 MiB | 0.91 MiB | 478 | 15.62 ms | 7 | 7 |
| Candidate, accessibility disabled | 465.71 ms | 104.64 MiB | 166.33 MiB | 0.98 MiB | 484 | 15.62 ms | 11 | 7 |
| Candidate, accessibility enabled | 414.53 ms | 108.67 MiB | 166.57 MiB | 1.06 MiB | 531 | 31.25 ms | 11 | 7 |

All nine raw samples are tracked in `docs/validation/phase-12-accessibility-process-metrics.csv`.

Reproduce the process samples with identically built baseline/candidate executables and minimal matching configs:

```powershell
# Build both revisions with the identical diagnostic-only harness.
go build -tags "glfw accessibilitymetrics" -o ./tmp/cervterm-candidate.exe ./cmd/cervterm

./scripts/measure-accessibility-process.ps1 `
  -BaselineExe ./tmp/cervterm-baseline.exe -BaselineConfig ./tmp/baseline.lua `
  -CandidateExe ./tmp/cervterm-candidate.exe -DisabledConfig ./tmp/disabled.lua `
  -EnabledConfig ./tmp/enabled.lua -RuntimeMetricsDir ./tmp/runtime-metrics `
  -Iterations 3 -WarmupSeconds 1 -IdleSeconds 3
```

The baseline config was:

```lua
return { shell = { program = "cmd.exe", args = { "/k" } }, window = { width = 640, height = 480 } }
```

The two candidate configs were identical except for the explicit boolean:

```lua
return {
  config_version = 2,
  accessibility = { enabled = false, scope = "visible" }, -- true for enabled run
  shell = { program = "cmd.exe", args = { "/k" } },
  window = { width = 640, height = 480 },
}
```

The detached baseline used `820c389` plus only the identical `accessibilitymetrics` probe calls/files from `e091a84`; no Phase 12.9 activation/runtime code was copied. Candidate executable, probe and synchronized script came directly from `e091a84`.

Interpretation:

- Default-off startup varied by +6.65%, below the 15% investigation threshold; working-set delta was +0.63 MiB and post-GC Go-heap delta was +0.07 MiB.
- Enabled startup improved within run noise. The +4.66 MiB working set, +0.28 MiB private bytes, +0.15 MiB post-GC heap and +53 handles are accepted for an explicit experimental opt-in because Windows loads UI Automation and owns a provider/router graph per visible projection.
- All configurations produced 7 frames during the exact signaled three-second interval. Disabled and enabled both produced 11 wakes versus 7 at baseline, so activation itself added no wake differential and no frame cadence; the bounded four-wake difference remains within the default-off experimental budget.
- CPU samples are scheduler-quantized and noisy; disabled matched baseline at 15.62 ms and enabled used one additional 15.62 ms quantum. Repaint-only suppression, no-pending refresh and damage-driven redraw tests corroborate the process counters.

## Assistive-technology matrix

| Tool | Version | Result | Reason |
| --- | --- | --- | --- |
| Windows Narrator | 10.0.26100.8521 | SKIP | Installed, but no supervised interactive screen-reader qualification was performed in this implementation session. |
| NVDA | unavailable | SKIP | Executable not installed. |
| Accessibility Insights / Inspect | unavailable | SKIP | Executable not installed. |
| UIA Verify | unavailable | SKIP | Executable not installed. |
| macOS NSAccessibility | n/a | SKIP | No adapter or macOS GUI support claim. |
| Linux AT-SPI | n/a | SKIP | No adapter or Linux GUI support claim. |

The detailed Narrator/NVDA scenario matrix remains in `docs/validation/phase-12-accessibility-qualification.md`. SKIP is never PASS and cannot justify default-on behavior.

## Final automated gates

- Full tests, tagged GLFW tests, focused race suites, vet with the repository unsafe-pointer policy, maturity gates, and Windows tagged build: PASS.
- Beta package, release preflight, and installed-package smoke: PASS; existing optional vttest/capture warnings remain documented.
- Windows 386 tagged link: SKIP because this host has only x86_64 MinGW libraries; pointer-width ABI invariants remain tested and no 386 claim is made.
- Independent activation and close-out code/coverage reviews were completed; findings and revision evidence are recorded in `docs/validation/phase-12-review-disposition.md`.

## Architecture evidence

Accepted ADR 0013 is tracked at `.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0013-expose-bounded-accessibility-snapshots-through-a-projection-owned-native-capability.md` and mirrored in active T50 state. The tracked drift result is `docs/validation/phase-12-architecture-drift.md`; no architecture direction, persistence, external domain or default policy changed during close-out.

## Support decision

Phase 12 implementation is closed for the current scope. Keep:

```lua
accessibility = { enabled = false, scope = "visible" }
```

Users may opt in on Windows for controlled testing. No Narrator, NVDA, cross-platform accessibility, or default-on support claim is made. A default change requires a new explicitly approved slice with real assistive-technology PASS evidence.
