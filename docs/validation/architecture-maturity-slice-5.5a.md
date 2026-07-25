# Architecture maturity — Stage 3 Slice 5.5a final validation

Status: closed as exactly `T -> A -> M -> W -> G` from `320deef1ecb16db212cfee692128591359bebc70`. Immutable W is `295ef3f847c2be13f20fee250aeb06d39b67ccc7`; G is the unique full-history commit with that sole parent and subject `refactor(fontglyph): guard discovery and cache extraction`.

## Scope and compatibility conclusion

Slice 5.5a keeps bounded font discovery/index ownership in `internal/fontglyph/discovery`, parsed-face cache ownership in `internal/fontglyph/cache`, and parsed/native resource ownership in `internal/fontglyph/internal/face`. The root `internal/fontglyph` package remains the compatibility/orchestration facade.

The repaired facade deliberately restores the concrete legacy root identities rather than aliases:

- concrete unexported `faceInfo` has its historical five fields and zero methods;
- concrete `FontIndexDiagnostics` and `FontResolution` retain their historical field order/types, `%T` identity, and zero-method value sets while converting internally from discovery-owned results;
- concrete `FontIndex` exposes exactly pointer methods `Diagnostics` and `Lookup`;
- no root `Faces` method exposes discovery-owned mutable authority;
- `BuildFontIndex`, `ResolveSystemFont`, `loadSystemFontIndex`, `systemFontDirs`, cache-key and canonical-source signatures remain compile-pinned;
- root lookup/private bridge results and discovery `Lookup`/`Faces` results are detached values.

The permanent guard `scripts/check-slice55a-evidence.go` pins canonical AST hashes for the concrete root type/field/method inventory, internal conversions, detached discovery methods and tests, deterministic equal-`lastUsed` lexical eviction and test, cache source clearing, deferred eviction owner closure, outside-lock callbacks, reverse backend close, and retained closed-backend source-byte accounting. It also owns an immutable expected table for every base/candidate benchmark binary digest and every exact compile/run target record. Its adversarial self-fixtures exercise aliases, missing/duplicate/reordered raw samples, source-manifest mutations, arbitrary valid coordinated digest replacement, discovery/cache target swaps, benchmark metadata drift, lexical reversal, under-lock closers, retained source references/bytes, forward close, early lease release, and discovery-owned storage aliases. The guard is invoked by `scripts/check-maturity-gates.go`, so present-tree invariants remain active after merge and in shallow CI even when historical objects are unavailable.

## Unchanged budgets

- discovery: 20,000 files, 65,536 faces, 256 faces per file;
- parsed cache: 128 faces and 256 MiB retained source bytes;
- cache hit: exactly one pin per lease and idempotent close;
- repaired eviction: equal-age victims use lexical source/index key order;
- cache/native callbacks run after manager state is detached and the manager mutex is unlocked.

## Reproducible benchmark identity

The comparison uses Go 1.25.8 on Windows/amd64 with `GOMAXPROCS=1`, `-test.cpu=1`, `-test.benchtime=1s`, `-test.benchmem`, 10 interleaved samples in odd `AB` / even `BA` order. **No warm-up command was run or claimed** (`warmup=none`).

Baseline production is immutable `320deef1ecb16db212cfee692128591359bebc70`. Because the L4-02 benchmark harness did not exist in that production tree, the baseline overlays only `internal/fontglyph/discovery_cache_characterization_test.go` from immutable T `6efd6cd8e6df21a57886257552c31fd76be7c533`; the production/harness split is explicit in `benchmark-binaries.txt`. Candidate production and benchmark source are immutable W `295ef3f847c2be13f20fee250aeb06d39b67ccc7`; `source-manifest-candidate.txt` is generated from that exact tree and remains pinned in detached, depth-1, post-G, and post-merge validation.

Every `.go` file plus `go.mod`/`go.sum` is represented as a sorted line `<normalized-content-sha256>  <slash-path>`. Content normalization converts CRLF or bare CR to LF. The complete manifest-file SHA-256 values are:

- base production plus named T harness overlay: `f06959254d1a16a107eac3def00804f087902c780db5f52a1f182e3a8ae56ea9`;
- immutable candidate W production plus benchmark source: `73de78a927b3e29210306c641ed343093e6554655a528b0ca0f01aaf17ca0d6e`.

The compiled test binaries were produced with `go test -c -trimpath`; the GLFW proxy binary additionally used `-tags glfw`. `benchmark-binaries.txt` contains one exact compile record and one exact run record per side/package/binary, with explicit package, GOOS, GOARCH, tags, `GOMAXPROCS`, warm-up disposition, run pattern, benchmark pattern, `benchmem`, benchtime, count, and CPU. The executable guard independently pins all nine binary SHA-256 values and the complete expected record set; the evidence file cannot redefine those expectations. Reproduction is:

1. Create a detached worktree at `320deef1ecb16db212cfee692128591359bebc70`.
2. Overlay the single named harness with `git show 6efd6cd8e6df21a57886257552c31fd76be7c533:internal/fontglyph/discovery_cache_characterization_test.go`.
3. Create a detached candidate worktree at immutable W `295ef3f847c2be13f20fee250aeb06d39b67ccc7`; generate both normalized source manifests and verify their pinned file SHA-256 values.
4. Build every package test binary using its exact compile record in `benchmark-binaries.txt`; on a host-compatible Windows/amd64 toolchain, verify the resulting binary against the independently pinned guard digest. For non-host targets, use the pinned source manifest plus exact platform/package command record.
5. Execute each binary with the exact environment and arguments in its run record, alternating AB/BA by sample number; no warm-up command is permitted.
6. Run `go run ./scripts/check-slice55a-evidence.go` to parse every exact record and raw binding, verify source/binary/target identities, recompute medians and worst allocations, and enforce the thresholds.

Raw LF-normalized capture file SHA-256 values after removing only Go's CPU-label trailing padding (sample headers, commands, benchmark rows, values, and physical order are retained exactly) are:

- `benchmarks-base.txt`: `2ed8d9d91f5292e4fca959c0a8c6ac4c518ac3142a0e109fb507e97d6056ee97`;
- `benchmarks-candidate.txt`: `b0178b0f58855064510f6b54c81f351037f58394c957bc402db39db3438ebb07`;
- `benchmarks-interleaved.txt`: `2dbe3b4cd7b1539d3560a114a74d7f2e698a0e17c9d13e1710afa574b124a5e3`.

Additional LF artifact integrity metadata pinned by the executable guard:

- `benchmark-binaries.txt`: `7f50b1b2fef1e11e17d9107362bcd743a8e55c19d1aa7b755e4188e191f1d7e1`;
- `platform-gates.txt`: `d5606957fd718b30a19c289ebcee913a71ad17ddf56beb482aba500367377c39`.

## Recomputed ten-sample results

The accepted gate is candidate median `ns/op` no more than 3% above base and no increase in worst-sample `B/op` or `allocs/op`.

| Benchmark | Honest scope | Base median ns/op | Candidate median ns/op | Delta | Base max B/op / allocs | Candidate max B/op / allocs | Result |
|---|---|---:|---:|---:|---:|---:|---|
| `BenchmarkL402DiscoveryTopK` | bounded path-selection implementation | 38,539,514.5 | 38,700,029.0 | +0.416493% | 5,762,088 / 20,094 | 5,762,088 / 20,094 | PASS |
| `BenchmarkL402BuildIndexGoMono` | one-file discovery/index startup component | 795,408.0 | 790,212.5 | -0.653187% | 894,794 / 203 | 894,778 / 203 | PASS |
| `BenchmarkL402CacheHitLease` | parsed-face cache hit and lease release | 73.320 | 60.970 | -16.843972% | 56 / 2 | 56 / 2 | PASS |
| `BenchmarkPhase15TerminalStartupMemory` | headless `NewTerminalWithHistory(120,32,4096)` startup-memory proxy | 10,848.5 | 10,747.0 | -0.935613% | 125,344 / 4 | 125,344 / 4 | PASS |
| `BenchmarkPhase13TextOnlySnapshot` | headless reusable 120x40 text snapshot projection | 8,936.0 | 8,925.5 | -0.117502% | 0 / 0 | 0 / 0 | PASS |
| `BenchmarkPhase13DisabledDraw` | headless context-free 120x40 text-grid row-walk proxy | 46,930.5 | 45,652.0 | -2.724241% | 0 / 0 | 0 / 0 | PASS |

The startup and text-frame rows are intentionally narrow existing repository proxies; each is a **headless proxy**, not an end-to-end application measurement. They do not start the process, create a window, create a GL context, load a glyph atlas, or present a frame. **No GUI claim** is made. The row-walk uses the existing blank-cell fixture; it is evidence for stable headless text-grid traversal/allocation only.

## Package map

```text
cervterm/internal/fontglyph -> fontdesc, cache, discovery, internal/face, unicodecluster, unicodeprops, and existing external/standard leaves
cervterm/internal/fontglyph/cache -> internal/fontglyph/internal/face
cervterm/internal/fontglyph/discovery -> fontdesc
cervterm/internal/fontglyph/internal/face -> no CervTerm-local import (fontdesc is the only allowed future stable leaf)
```

The static gate scans production imports, rejects subsystem back-imports/sibling authority, and runs `go list -deps -test ./internal/fontglyph/...`.

## Platform evidence

`platform-gates.txt` retains candidate execution on Windows and actual CGO-disabled Linux/amd64 test-binary execution inside WSL2 Ubuntu 24.04 for the repaired root contracts plus complete cache/discovery/internal-face packages. A complete Linux root-package run was not relabeled: it was excluded after WSL's absent host emoji font inventory caused three environment-dependent selection failures. Darwin amd64/arm64 and Windows amd64/arm64 `./...` checks are compile-only and labeled as such. No GUI, window-system, native-font, Darwin execution, or Windows arm64 execution claim is made.

## Evidence modes and closed chain

- **Dirty pre-G:** active branch HEAD is exactly immutable W and dirty paths equal the G path allowlist; this mode was exercised before creating G.
- **Post-G:** the unique G has rewritten W as sole parent, the exact subject/path set, and a clean active worktree.
- **Merged/full history:** exact parent+subject lookup spans all refs, requires cardinality one, and rejects duplicate G siblings while retaining the final identity/path union.
- **Detached/depth-1 shallow:** present-tree guards, pinned immutable-W source-manifest integrity, binary/raw bindings, exact samples, docs, and package checks remain mandatory while unavailable historical objects are not invented.

The chain is closed. No push or merge is part of this evidence.
