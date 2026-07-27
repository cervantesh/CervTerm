# Architecture maturity — Stage 3 Slice 5.5c final validation

Status: retained in the sole G commit over the exact clean T→A→M→W lineage from immutable base `504f5f54187ef9eb696bd14bd90a8c18940fb586`. W is `7df9952db6d35a031da783479eb4d5c7f7d30ba3`; G has W as its sole parent and exact subject `refactor(fontglyph): guard raster color and platform extraction`.

## Scope and compatibility conclusion

`internal/fontglyph/raster` owns portable outline/subpixel paint, bitmap strikes, SVG extraction/rasterization, COLRv0/v1 paint including variation and composite modes, and color-table parsing. `internal/fontglyph/platform` owns DirectWrite analysis, shaping, font-face/native raster construction, ABI-private COM layouts, and reverse native-resource close. The root `internal/fontglyph` package remains the concrete compatibility/orchestration facade: `RasterizedGlyph`, `OpenTypeBackend`, `ColorTables`, the COLR concrete value surface, `DirectWriteShaper`, backend selection, cache leases, and cross-subsystem construction retain their root identities.

Obsolete root parser/paint/native implementation files are removed. Root retains only explicit conversions and small compatibility delegates where concrete root APIs require them. The malformed-CPAL and overflowing sbix offset cases found by the new portable fuzz target now reject safely without changing valid fixture output.

The nine redistributable font fixtures remain exactly once under the shared `internal/fontglyph/testdata` authority because discovery, root characterization, and the raster package all consume them. Raster tests use `../testdata`; duplicated `internal/fontglyph/raster/testdata` copies are removed. Bitmap, SVG text/gradient, COLR v0/v1 gradient, variation, composite, shaped-color, real Noto subset, pixel-hash, and portable fuzz coverage remain active. DirectWrite collection face-index, font-face shaping, feature argument, ABI, analysis, bridge, raster, fallback, and root facade coverage remains active on Windows.

## Exact ADR-0021 package DAG

```text
internal/fontglyph -> discovery, cache, shape, raster, platform, internal/face

discovery -> fontdesc
cache -> internal/face
shape -> internal/face, fontdesc, unicodecluster, unicodeprops
raster -> internal/face, fontdesc, unicodecluster, unicodeprops
platform -> internal/face, fontdesc, unicodecluster, unicodeprops
internal/face -> fontdesc
```

The listed local edges are the complete allowlist. Public subsystem packages never import root or one another; raster/platform never import renderer, GL, or GLFW packages. `go list -deps -test ./internal/fontglyph/...` passes.

The permanent `scripts/check-slice55c-evidence.go` guard validates the DAG; recursive root anti-retention; exact root field types, exported method signatures, and ABI tests; canonical ownership; semantic fuzz routing; DirectWrite constructor validation; fixture/testdata SHA-256 manifests; all eight benchmark binaries with each hash bound to its own compile row; per-run monotonic timestamp and unique nonce witnesses for the exact ABBA projections; exact ten-sample medians/allocations; exact T/A/M/W history; future G identity; and the cleanup hash. Shallow non-PR history fails closed with a fetch/unshallow requirement. A depth-1 synthetic PR merge is accepted only when the event G/base, raw merge parents, and present-tree manifests all match; forged G-equals-base and forged-parent fixtures are rejected.

The semantic cleanup diff under `internal/fontglyph` is pinned at SHA-256 `9a07bbf9e544ba50789083adda46849209a0393016121a3c1a51dd170277c5af`. Slice 5.5a and 5.5b guards recognize this explicit successor while retaining their historical manifests, body pins, path/chain identities, budgets, ownership, and cleanup hashes.

## Reproducible binary and source identity

The comparison uses Go 1.25.8 on Windows/amd64 with `GOMAXPROCS=1`, `-test.cpu=1`, `-test.benchtime=1s`, `-test.benchmem`, no warm-up, and exactly ten physical samples in odd AB/even BA order. Every physical execution carries a strictly increasing `Stopwatch` tick and unique GUID nonce in the retained interleaved transcript while base/candidate projections remain byte-exact. Baseline is immutable T `8074203cfdc516be4c887c8c4ec800b187cd85bf`; candidate is the present W-semantic physical tree. Test binaries use `go test -c -trimpath`; GLFW uses `-tags glfw`.

All eight binary digests are independently pinned to their exact local compile records. Source manifests include every `.go` file, `go.mod`, `go.sum`, the Windows-host WSL runner, all nine shared raster fixture/license/provenance files, and the retained raster fuzz corpus input. Source text is LF-normalized; fixture/testdata bytes use raw exact SHA-256. Only the 5.5c guard itself is excluded so clean/pre-G/merged/shallow present-tree validation remains stable:

- base source manifest: `94a9328d11d8e83fc4132f39b8ba1c985a43c269017befde2359e72107110912`;
- candidate source manifest: `4f4c46f31d6d4c2c8d69827941a6c4a6fa87e005faf2c2759b54a6848c2446b9`.

Raw LF artifact hashes:

- `benchmark-binaries.txt`: `4172e5bf92458a2049191e92ba260821a920c2a6edd87f2d6eca98003bd987b8`;
- `benchmark-summary.txt`: `7c17132af88bd65e8560b651dc6343513fdccbd8835fe308e1c9b6b7f0f774cc`;
- `benchmarks-base.txt`: `384cafde996601e0d1d61bf4befb3eb3663caef2293c10f00e9ffaee3d9ffe64`;
- `benchmarks-candidate.txt`: `2fe62d89d011726eb447fa84621597a943eda980e538c1495a30ef864dbe98d6`;
- `benchmarks-interleaved.txt`: `75274d589fd0e40ab86b48e9eec7c59084acb21ecf723020260b88a3b91c9b5e`;
- `directwrite-allocation-benchmark.txt`: `1c3b640b804bf5ebda6d21d3cec520bc466cac05a144c9b2f5e121657488befc`;
- `gates.txt`: `531e5e763bea032061e85864603cae9f0832cc7507be694e979cf13483fb86cd`;
- `platform-gates.txt`: `be8bcaa98762ad0d10d19b22dd82cec845295eb457bd6f54612c4de04adff570`;
- `scope-and-commits.txt`: `a909af46b9ecb5764bbcc22fc7659da72e369ca6cda46aac5cd3dbeb431f7459`.

## Ten-sample results

Acceptance is candidate median `ns/op` no more than 3% above base, with no increase in worst-sample `B/op` or `allocs/op`.

| Benchmark | Base median ns/op | Candidate median ns/op | Delta | Base max B/op / allocs | Candidate max B/op / allocs | Result |
|---|---:|---:|---:|---:|---:|---|
| `BenchmarkL402RasterColorGlyph` | 635,644.5 | 636,569.5 | +0.145522% | 284,049 / 528 | 284,049 / 528 | PASS |
| `BenchmarkPhase15TerminalStartupMemory` | 10,532.0 | 10,490.5 | -0.394037% | 125,344 / 4 | 125,344 / 4 | PASS |
| `BenchmarkPhase13TextOnlySnapshot` | 8,720.5 | 8,760.0 | +0.452956% | 0 / 0 | 0 / 0 | PASS |
| `BenchmarkPhase13DisabledDraw` | 45,493.0 | 45,489.5 | -0.007693% | 0 / 0 | 0 / 0 | PASS |

Startup and frame rows are headless proxies; they do not claim native-window, GL-context, atlas-upload, or presented-frame execution.

The retained DirectWrite allocation benchmark records five samples per path: the historical two-result projection uses 976 B/op and 12 allocs/op, while the one-result root-concrete path uses 784 B/op and 11 allocs/op. Output equivalence is separately asserted.

## Validation and platform evidence

Passed on Windows/amd64: full tests, full race tests, full GLFW-tagged tests, `go vet -unsafeptr=false`, package-DAG listing, exhaustive raster/color fixtures, DirectWrite ABI/native tests, exact SHA-256 pixel characterization, and a 10-second portable fuzz smoke with retained malformed-sbix regression corpus.

Windows amd64/arm64, Linux amd64/arm64, and Darwin amd64/arm64 full-package CGO-disabled compile gates pass. The retained Linux execution commands are Windows-host PowerShell commands that invoke explicit `Ubuntu-24.04`, pass the repository path as an argument, convert it with safely quoted `wslpath -a -u -- "$1"`, and execute native guest `go test`. Both commands were actually invoked; this host's Ubuntu-24.04 guest has no Go toolchain, so the evidence records `UNAVAILABLE` with exit 1 and does not claim Linux execution passed.

Slice 5.5a, Slice 5.5b, Slice 5.5c, maturity, and `git diff --check 504f5f54187ef9eb696bd14bd90a8c18940fb586..HEAD` guards pass. The work is finalized as exactly five local commits; no push or merge is part of this work.
