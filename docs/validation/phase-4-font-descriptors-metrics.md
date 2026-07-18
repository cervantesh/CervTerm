# Phase 4 Font Descriptors, Fallback, Features, and Metrics Validation

## Scope

Phase 4 adds deterministic, bounded WezTerm-inspired font configuration without renderer selection: startup installation ownership, primary descriptors and real styles, lazy whole-cluster fallback/rules, OpenType feature projection, fixed-grid metrics, complete cache identity, bounded retained contexts, and redacted diagnostics.

## Delivery

| Slice | Merge | Evidence |
|---|---|---|
| 4.1 bounded installation foundations | PR #149 / `903f4f2` | cache/index bounds, startup transaction, safe-font seam, two-level key foundation |
| 4.2 primary descriptors and styles | PR #150 / `39a97cb` | strict v2 schema, deterministic face ranking, real/synthetic four-style routing |
| 4.3 lazy fallback and rules | PR #151 / `0e501b6` | whole-cluster rule/primary/fallback/embedded order and symbol classes |
| 4.4 shaping features | PR #152 / `0c0f488` | compatibility projection, explicit feature precedence, shaping identity |
| 4.5 metrics and context bounds | PR #153 / `03efcc5` | fixed-grid projection, 64-context LRU/pins, per-context negative budgets |
| 4.6 diagnostics and qualification | this closeout | doctor, support/docs, installed package, performance and platform evidence |

## Contracts and evidence

- `internal/fontdesc`: immutable descriptor/style/rule/feature/metric vocabulary, canonical payloads, complete `FontEnvironmentKey` and `ResolvedFaceKey`, and hard resource limits.
- `internal/fontglyph`: deterministic top-K discovery, 128-face/256-MiB pin-aware parsed cache, real-style resolution, lazy whole-cluster fallback, DirectWrite/portable feature projection, and exact-close ownership.
- `internal/config`: explicit-v2 replacement/merge/unset/provenance/CLI semantics for descriptors, fallback, rules, features and metrics. Unversioned/v1 remains shorthand-compatible; every advanced font field is restart-scoped.
- `internal/frontend/glfwgl`: startup prepares font resources before mux/PTY adoption; metric canvases keep advances fixed; all rune/cluster/run/color/fallback atlas entries use environment plus resolved-face identity; two 2048² pages remain fixed; context admission is prepare/commit/abort transactional.
- `cmd/cervterm/doctor.go`: reports effective feature/metric projection, natural/projected cells, concrete path-free family/subfamily metadata for four primary styles, synthetic modes, representative content source tier/rule index, raster/shaper capability, safe-mode suppression and hard limits. Font paths, cache keys and backend errors are redacted. Arbitrary active-terminal selections and active-process cache/context counts remain explicitly unavailable rather than fabricated.

## Windows installed qualification

Reference: Windows 11 `windows/amd64`, AMD Ryzen 9 7940HX, Go 1.25.8, runtime merge `03efcc5`.

- Packaged `0.0.0-phase4-local` zip passed release preflight (39 pass, 0 required failures, one expected missing-vttest warning), installed-package smoke, and daily-driver smoke.
- The known ConPTY capture exit `0xc0000374` was accepted only after required VT/log artifacts and test markers validated, matching the recorded baseline behavior.
- Installed JetBrainsMono Nerd Font resolved `Regular` (400), `Bold` (700), `Italic` (400), and `Bold Italic` (700) with `synthetic=none`; the normal selection is explicitly not ExtraLight. Doctor reported DirectWrite feature capability and natural/projected cells `11x28/21`. Representative content diagnostics selected Powerline/Nerd/CJK/emoji rule tiers with path-free family/subfamily metadata.
- GUI smoke rendered centered punctuation/ligatures (`->`, `=>`, `!=`, `===`), aligned primary/CJK/emoji baselines, Powerline and Nerd Font PUA, CJK through Microsoft YaHei UI, and color emoji through Segoe UI Emoji. A mixed-size split verified pane-local zoom and sibling stability. Evidence: [font qualification](../assets/phase4-font-qualification-windows.png) and [mixed-size panes](../assets/phase4-pane-zoom-windows.png). Cross-monitor DPI was explicitly skipped because no second monitor was available; automated DPI/context-identity tests passed.
- The package-bundled Noto Color Emoji file is distribution material, not silently installed or added as a private discovery root. Qualification therefore uses installed system fallback families and does not claim otherwise.
- Redacted text evidence: [installed doctor selections](phase-4-font-doctor.txt) and [package/gate smoke summary](phase-4-package-smoke.txt). Full generated logs/packages remain untracked under `dist/`.

## Performance and resource evidence

Contemporaneous five-run baseline/candidate comparison and GUI resource scenarios are recorded in [`docs/parity-baseline.md`](../parity-baseline.md). Parser/core/render hot-path timings and allocations remain below the 15% investigation threshold; ordinary startup readiness/memory also remain below it. Plain ASCII idle reported 27.6 MiB Go heap; first Powerline/Nerd/CJK/emoji fallback reported 65.8 MiB; 40 rapid zoom steps plus reset reported 56.2 MiB after GC and remained within the deterministic 64-context policy proven by boundary tests. The fixed atlas remains two 2048² RGBA pages (32 MiB). Bounded system-font discovery raises sampled startup CPU by about 45% and is retained as an explicit optimization target; that CPU observation is not described as below threshold.

## Platform matrix

| Platform | Status | Evidence / boundary |
|---|---|---|
| Windows 11 amd64 GUI | Qualified | packaged doctor, visual capture, installed and daily-driver smokes |
| Linux amd64 headless | Qualified for build/tests only | CI headless tests/package; no GUI/font rendering support claim |
| macOS GUI | Not qualified | compile coverage only; manual installed-font/style/fallback matrix remains open |
| Linux GUI (X11/Wayland) | Not qualified | no compositor/font-stack manual matrix; headless evidence must not be overstated |

## Verification commands

```text
go run ./scripts/check-maturity-gates.go
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go test ./internal/vt -run '^$' -fuzz=FuzzParserAdvanceDoesNotPanic -fuzztime=2s
go test -race ./internal/fontdesc ./internal/fontglyph ./internal/config ./internal/mux -count=1
tl check --include-dir docs/examples docs/examples/cervterm.tl
govulncheck ./...
python -m json.tool docs/parity-support-matrix.json
go run ./scripts/capture-parity-baseline.go -count 3 -out dist/phase4-parity-candidate.txt
go run ./scripts/package-beta.go -version 0.0.0-phase4-local -outdir dist/phase4-closeout
go run ./scripts/release-preflight.go -version 0.0.0-phase4-local -outdir dist/phase4-closeout -windows-zip dist/phase4-closeout/cervterm-0.0.0-phase4-local-windows.zip
go run ./scripts/smoke-installed-package.go -zip dist/phase4-closeout/cervterm-0.0.0-phase4-local-windows.zip -expected-version 0.0.0-phase4-local
go run ./scripts/daily-driver-smoke.go -cervterm dist/phase4-closeout/cervterm-0.0.0-phase4-local-windows/cervterm.exe -workdir dist/phase4-closeout/daily-driver-smoke
```

Windows race qualification initially exposed Go checkptr rejecting foreign COM callback `uintptr` values when they re-entered Go. The established DirectWrite analysis callbacks are now narrowly marked `//go:nocheckptr`; the exact planned race command passes without disabling race instrumentation.

## Rollback and residual scope

- `--safe-fonts` restores embedded Go Mono, clears advanced descriptors/fallback/rules/features, and restores natural metrics after restart.
- Removing explicit-v2 advanced fields restores shorthand behavior; no persistence migration exists.
- Representative content probes expose path-free selected tier/rule metadata; arbitrary active-terminal content selections and live atlas/cache occupancy are not exposed by diagnostic-only commands. A future active-runtime diagnostics channel would require a separately approved aggregate snapshot seam.
- Renderer/backend selection remains explicitly excluded.
