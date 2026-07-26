# Architecture maturity — Stage 3 Slice 5.5b final validation

Status: finalized as exactly five clean T→A→M→W→G commits from immutable base `973660df6ea31112a6984a39421ad60783e706d4`. Repaired W is `d6e1a81dfedb4f8910b9340b6da0d3c2b8b8291c`; G has that commit as its sole parent and subject `refactor(fontglyph): guard resolution and shaping extraction`.

## Scope and compatibility conclusion

`internal/fontglyph/shape` now owns deterministic face plans, authored rule/fallback policy, bounded positive/load-failure caches, portable/default shaping, feature projection helpers, and run-substitution decisions. `internal/fontglyph/internal/face.Ref` projects only immutable SFNT/source/index shaping state. Root `internal/fontglyph` remains the compatibility/orchestration facade and retains concrete `ShapedGlyph`, `SimpleShaper`, `Shaper`, `FeatureShaper`, `FaceDiagnostic`, backend entry points, and the historical unexported plan projections.

The root facade converts detached values; it exposes no discovery-owned storage, source bytes, cache lease, parsed-face owner, raster resource, or native handle. Shape policy invokes load/coverage/discard hooks without its mutex held, deduplicates same-cluster work, caps both resolution and load-failure caches at `fontdesc.MaxNegativeEntries`, and clears authored slices and cache state on close. Root owns loaded raster backends and closes losing candidates immediately, then closes retained fallback backends before the primary backend. Whole clusters are tested before selection. Rules, primary, authored fallback, and embedded order are unchanged and no fallback I/O occurs during primary installation or a primary hit.

DirectWrite face creation, native analysis, native raster, color extraction, and all renderer/backend-selection ownership remain in root for Slice 5.5c. The only platform seam is `shape.PlatformFactory`; Windows returns the same concrete root `DirectWriteShaper` or `SimpleShaper` after unwrapping, preserving selection and fallback behavior.

## Exact package DAG

```text
cervterm/internal/fontglyph -> cache, discovery, shape, internal/face, fontdesc, unicodecluster, unicodeprops
cervterm/internal/fontglyph/shape -> internal/face, fontdesc, unicodecluster, unicodeprops (plus stdlib/x dependencies)
cervterm/internal/fontglyph/cache -> internal/face
cervterm/internal/fontglyph/discovery -> fontdesc
cervterm/internal/fontglyph/internal/face -> fontdesc
```

The executable guard rejects root or sibling-subsystem imports from `shape`, back-imports outside ADR-0021 (including the allowed `unicodecluster` leaf), package cycles, and obsolete root policy/shaping authority. Recursive AST inspection runs over every real root production/test source and rejects transitive aliases, embedding, inferred locals/composites, generics, function literals, and captures; adversarial fixtures mutate real production source. Exact canonical facade delegation bodies, policy budget/lifecycle bodies and tests, losing-candidate cleanup, reverse fallback close order, and cache lifecycle tests are independently body-pinned. The exact G cleanup patch across all seven production/test files is pinned at SHA-256 `ed6becd4ae19c55f89083d817fdd3049536208a919e2baf945fd222af6d53e8c`, so DirectWrite/native, raster, and color work deferred to 5.5c cannot be absorbed.

## Reproducible benchmark identity

The comparison uses Go 1.25.8 on Windows/amd64 with `GOMAXPROCS=1`, `-test.cpu=1`, `-test.benchtime=1s`, `-test.benchmem`, and exactly ten physical samples in odd `AB` / even `BA` order. No warm-up command was run or claimed (`warmup=none`).

Baseline production is immutable `973660df6ea31112a6984a39421ad60783e706d4` plus only the T characterization harness at `d0bfd354bc088093621348a817ed005cc1e1df70`. Candidate binaries and evidence use the present-tree W-semantic source rooted at lineage W `d6e1a81dfedb4f8910b9340b6da0d3c2b8b8291c`, not branch name or HEAD subject. The candidate manifest is recomputed from the physical worktree in dirty, shallow, detached, clean-G, and post-merge modes (excluding only the G-only dedicated guard source), so a coordinated clean source change with the exact G subject still fails. Every candidate `.go` file plus `go.mod`/`go.sum` in that projection is represented in the sorted LF-normalized manifest:

- base manifest: `ab85a007fa45960054d5783caa641137ca5641efffd8d19512f2cc037324ab01`;
- candidate manifest: `7aa058a776c6f57cf686b7ac675da430caf0c94f8a37abf1a813a1393bd8bee9`.

Compiled binaries use `go test -c -trimpath`; the GLFW proxy uses `-tags glfw`. `benchmark-binaries.txt` carries exact compile and run records while the executable guard independently pins all eight binary digests, including the rebuilt candidate fontglyph and GLFW binaries.

Raw LF artifact SHA-256 values:

- `benchmark-binaries.txt`: `51e3b8332d15566048f99febf9d0e9a6cf7e23ec03f67aca93346131d895351e`;
- `benchmarks-base.txt`: `9248acb0a46c6a2e45cd409bf5bb384b5e355ca4d0a865a4e39628043f456761`;
- `benchmarks-candidate.txt`: `3c49404a19ab113b8a68353ab69a2eba1ee42f2e7abc6f6a3a3eedd7b4a64038`;
- `benchmarks-interleaved.txt`: `2fd35cd67b5bcde0edcfd0892584bac52b7611958fce6bd1f8aa47b1f8c6e5dc`;
- `platform-gates.txt`: `179b950bbfc577f21f1bf63e8061c58ff4755c794fbc0e4d93418fb162acf36a`;
- `scope-and-commits.txt`: `75f9e2bb0ac4423a5259ca6c10d9d9e30390d3241830a586ee685f9606dd681e`.

## Ten-sample results

Acceptance is candidate median `ns/op` no more than 3% above base, with no increase in worst-sample `B/op` or `allocs/op`.

| Benchmark | Base median ns/op | Candidate median ns/op | Delta | Base max B/op / allocs | Candidate max B/op / allocs | Result |
|---|---:|---:|---:|---:|---:|---|
| `BenchmarkL402ResolvePrimaryPlans` | 2,454.0 | 1,857.5 | -24.307253% | 2,264 / 56 | 2,240 / 39 | PASS |
| `BenchmarkL402SimpleShapeASCII` | 222.35 | 210.85 | -5.172026% | 1,856 / 3 | 1,856 / 3 | PASS |
| `BenchmarkL402FallbackResolutionHit` | 70.865 | 32.585 | -54.018204% | 0 / 0 | 0 / 0 | PASS |
| `BenchmarkPhase15TerminalStartupMemory` | 10,817.5 | 10,733.0 | -0.781142% | 125,344 / 4 | 125,344 / 4 | PASS |
| `BenchmarkPhase13TextOnlySnapshot` | 8,831.5 | 8,874.5 | +0.486894% | 0 / 0 | 0 / 0 | PASS |
| `BenchmarkPhase13DisabledDraw` | 45,676.5 | 46,201.0 | +1.148293% | 0 / 0 | 0 / 0 | PASS |

The startup and frame rows are existing headless proxies. They do not create a window, GL context, native font object, atlas, or presented frame; no GUI claim is made.

## Validation and platform evidence

Passed on Windows/amd64:

- `go test ./...`;
- focused/race `fontdesc`, root `fontglyph`, `shape`, `discovery`, `cache`, and `internal/face`;
- `go test -tags glfw ./...`;
- `go vet -unsafeptr=false ./...`;
- `go list -deps -test ./internal/fontglyph/...`;
- glyph-output, DirectWrite selection/fallback, lazy fallback, exact-close, feature/style, run shaping, cache budget, color/raster retention, startup, and frame fixtures included by those suites.

Windows amd64/arm64, Darwin amd64/arm64, and Linux amd64/arm64 full-package compile gates passed with CGO disabled. Linux execution was reprobed through the default WSL2 `docker-desktop` distribution but unavailable because it has no Go toolchain; it is not relabeled as execution. No Darwin, Windows arm64, native-window, or GUI execution claim is made.

## Guard modes

The dedicated guard supports and self-tests dirty pre-G W, an exact dirty G-amend projection, clean G, merged/full-history descendants, and detached/depth-1 present-tree modes. Historical checks require the exact T/A/M/W parent chain, subjects, path sets, sole G identity, G ancestry, cleanup patch, and the exact union of committed-plus-dirty G paths when objects are available. Present-tree DAG, API, recursive real-source retention, exact delegation/lifecycle body pins, physical block binding, exact ten-sample AB/BA projections, manifest recomputation, compile/binary identity, raw hashes, and adversarial fixtures remain mandatory regardless branch or history depth.

No push or merge is part of this evidence.
