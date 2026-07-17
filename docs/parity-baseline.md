# WezTerm-Inspired Parity Baseline

Baseline commit: `7d64cc9`  
Captured: 2026-07-16  
Reference host: Windows 11, `windows/amd64`, AMD Ryzen 9 7940HX, Go 1.25.8

This baseline separates correctness invariants from machine-dependent observations. A later phase must investigate a material regression; it must not silently update this document to normalize one.

## Reproduce the Automated Baseline

```bash
go run ./scripts/capture-parity-baseline.go -count 3
go run ./scripts/capture-parity-baseline.go -full -count 3
```

The report defaults to `dist/parity-baseline.txt` and records commit, host, Go version, commands, tests, and benchmark output. `dist/` reports are evidence artifacts and are not committed.

## Hot-Path Observations

| Metric | Baseline range | Required invariant |
|---|---:|---|
| VT parser throughput | 32.3–32.9 us/op; 0.67–0.68 MB/s | `0 B/op`, `0 allocs/op` |
| Core reuse | 32.1–32.6 us/op | `0 B/op`, `0 allocs/op` |
| New terminal comparison | 49.8–51.6 us/op | comparison only; 156,208 B/op and 4 allocs/op observed |
| Render snapshot capture | 2.057–2.062 us/op | `0 B/op`, `0 allocs/op` |

Timing is host-dependent. A repeatable median regression above 15% on the same host requires profiling or explicit acceptance. Allocation invariant regressions block merge.

## GUI Startup and Memory Observation

A clean `-tags glfw` executable from the baseline commit was launched with the default configuration and sampled five seconds after the native window appeared:

| Metric | Observation |
|---|---:|
| Native window ready | 1,278 ms |
| Working set | 128.2 MiB |
| Private bytes | 201.4 MiB |
| Process CPU consumed by sample time | 0.797 s |

Reproduction on Windows PowerShell:

```powershell
go build -tags glfw -o dist/cervterm-baseline.exe ./cmd/cervterm
$sw = [Diagnostics.Stopwatch]::StartNew()
$p = Start-Process .\dist\cervterm-baseline.exe -PassThru
while ($p.MainWindowHandle -eq 0) { Start-Sleep -Milliseconds 50; $p.Refresh() }
$sw.ElapsedMilliseconds
Start-Sleep -Seconds 5
$p.Refresh()
$p | Select-Object WorkingSet64, PrivateMemorySize64, CPU
Stop-Process -Id $p.Id
```

These numbers are observations, not universal limits. Compare on the same host/configuration. A repeatable startup or memory regression above 15% requires explanation.

## Installed-Package Smoke

The baseline Windows zip `cervterm-0.9.0-phase0-windows.zip` was built from `7d64cc9` and passed version, build-info, doctor, bundled-file, logging, and raw ConPTY capture validation with:

```bash
go run ./scripts/package-beta.go -version 0.9.0-phase0 -outdir dist
go run ./scripts/smoke-installed-package.go -zip dist/cervterm-0.9.0-phase0-windows.zip -expected-version 0.9.0-phase0
```

The capture child reported the pre-existing Windows exit-status warning `0xc0000374`; the reusable smoke gate accepted the run after validating the required VT and log artifacts. This warning must not be presented as a new Phase 0 regression.

## Atlas and Idle Contracts

- The shared atlas owns exactly two `2048 x 2048` RGBA texture pages: **32 MiB GPU texture storage** before driver overhead. All pane/font-size contexts share this bounded pool.
- `TestAtlasContextConfiguresFixedPoolOnce` protects the fixed two-page pool.
- Damage-driven mode waits up to 500 ms when there is no blink, notice, stats panel, or timer: at most a **2 Hz self-heal wake**, not a forced 2 FPS repaint.
- A steady idle terminal repaints only for visible damage or cursor phase changes. With cursor blink and stats disabled, expected visible frame rate is zero.
- `TestNextWake` protects blink, timer, notice, and idle wait boundaries.

Focused checks:

```bash
go test -tags glfw ./internal/frontend/glfwgl -run 'Test(AtlasContextConfiguresFixedPoolOnce|NextWake)$' -count=1
go test ./internal/render -run '^$' -bench BenchmarkCaptureReuse -benchmem -count=3
```

## Per-Phase Comparison Policy

Every roadmap phase records:

1. baseline commit and candidate commit;
2. identical host/configuration where timing is compared;
3. correctness and allocation invariants;
4. median of at least three benchmark runs;
5. startup/memory sample for font, window, tab, accessibility, or image phases;
6. idle wake/frame evidence for frontend loop changes;
7. explanation and explicit approval for any accepted regression.

## Current Capability Source

`docs/parity-support-matrix.json` is the machine-readable feature/status source. Update it in the same PR that changes a feature from `planned` to `partial` or `supported`.

Completed phase evidence:
- [Phase 1 — Typed Action Engine](validation/phase-1-typed-action-engine.md)
