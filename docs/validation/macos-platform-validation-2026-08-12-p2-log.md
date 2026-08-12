# macOS Platform Validation — deferred P2 log (2026-08-12)

Companion to [`macos-platform-validation-2026-08-12.md`](macos-platform-validation-2026-08-12.md).

These are findings from the adversarial audit of `ci/macos-hosted-runner` that
were adjudicated as P2 — nitpick, theoretical, or pre-existing — and
deliberately **not** fixed in that pass. None is a correctness, safety, or
security defect. Each entry records why it stays deferred so a later change can
pick it up without re-deriving the analysis.

**Why none of these were fixed in code:** the two `internal/fontglyph/raster/`
files are hash-pinned by closed Slice 5.5c and already require a maintainer
re-declaration (see the blocking section of the companion document). Every
additional edit to them invalidates the digests verified for that
re-declaration and enlarges what the maintainer is asked to re-declare, for no
correctness benefit. Cosmetic hardening is not worth that cost.

---

## P2-1 — TTC header accepts structurally truncated collections

**File:** `internal/fontglyph/raster/sfnt_tables.go`, `sfntDirectoryOffset`

`numFonts` (bytes 8:12 of the `ttcf` header) is validated only as nonzero. The
code does not verify that the TTC offset table actually contains `numFonts`
entries before reading the first one.

**Confirmed P2, not upgradeable.** This is *not* an out-of-bounds read:

- The first offset entry always sits at bytes 12:16, and `len(fontData) < 16`
  is rejected before that read, so entry 0 is always in bounds.
- `numFonts` is never used as a loop bound or an index — only as a sanity
  check — so an inflated value cannot drive a read past the buffer.
- The returned offset is separately bounds-checked
  (`uint64(offset)+12 > uint64(len(fontData))`), and `int(offset)` cannot go
  negative even on a 32-bit platform: that check already constrains `offset` to
  at most `len(fontData)-12`, and `len` is a non-negative `int`.

The only consequence is that a malformed file declaring `numFonts = 5` while
carrying one entry is parsed as a single-face collection instead of being
rejected. Behavior is deterministic and bounded either way.

**If picked up later:** require
`uint64(12) + uint64(numFonts)*4 <= uint64(len(fontData))` before reading entry
0, and add a fixture asserting `ErrInvalidFontData` for an over-declared
`numFonts`.

---

## P2-2 — `numTables < 0` is dead code

**File:** `internal/fontglyph/raster/sfnt_tables.go`, `listSFNTTables`

`numTables` is produced by `int(binary.BigEndian.Uint16(...))`, so it is always
in `[0, 65535]` and the `numTables < 0` guard can never fire. Harmless and
**pre-existing** — it is unchanged from the `main` version of this function and
was merely carried along by the TTC refactor. Worth removing only if the file
is being touched for another reason.

There is no overflow risk behind it: `65535 * 16` is ~1 MiB, nowhere near
`int` overflow on any supported platform, and the `dirEnd > len(fontData)`
check bounds the loop.

---

## P2-3 — `watchPathIdentity` is now CWD-dependent for relative paths on all platforms

**File:** `internal/frontend/glfwgl/config_watch.go`

Extending the canonicalization off Windows also extends the `filepath.Abs`
call, so a relative watch path's identity now depends on the process working
directory on macOS and Linux too. If the CWD changed between two
`normalizeWatchPaths` calls, the same relative path could yield two identities.

**P2 and pre-existing in kind:** identical behavior has shipped on Windows
since `032bd61`, and in practice watch paths reaching this function are already
absolute. Deduplication returns the *original* path string regardless, so a
relative path still stats correctly; the only exposure is a missed dedup.

---

## P2-4 — No canonicalization when the parent directory does not exist yet

**File:** `internal/frontend/glfwgl/config_watch.go`

`filepath.EvalSymlinks` fails for a parent directory that does not exist, in
which case the path is left uncanonicalized. Verified directly on macOS:
`/tmp/does-not-exist-xyz/config.lua` stays as-is, while
`/tmp/<existing>/config.lua` correctly resolves to `/private/tmp/...`.

**P2, pre-existing on Windows, and self-correcting:** the miss only lasts until
the directory exists, and the failure mode is a redundant watch entry rather
than a dropped one. Noted in the function's doc comment.

---

---

## P2-5 — `package-macos.go`: concurrent runs race on shared staging path

**File:** `scripts/package-macos.go`, `packageMacOS`

Every invocation targeting a given `-outdir` builds and signs at the same
`<outdir>/CervTerm.app` path and removes it on start. Two concurrent runs
(different versions, or CI + a local run) can delete or mutate the bundle
mid-build of the other, producing a mismatched or corrupted archive.

**Confirmed real, deferred rather than fixed.** This is new code (no pinned-
manifest cost to fixing it), but the credible remedies — a unique per-run temp
staging directory with an atomic rename into `-outdir`, or a lock file — both
touch the path that the manually-verified `xattr -cr`/`codesign` steps operate
on, and add stale-lock/cleanup semantics. Poor trade for a script one
operator or one CI job runs at a time today. Tracked as a proposal issue (see
the companion document) rather than fixed in this pass.

---

## Not in this log

The audit's P1 findings on both branches were confirmed as genuine and handled
outside this log, not deferred silently:

- **CI/fixes branch:** parent-directory alias collapsing (`watchPathIdentity`)
  can drop a real reload dependency — documented as a known limitation in the
  function's doc comment, with a follow-up improvement proposed separately.
- **Packaging branch:** invalid `CFBundleVersion`/`CFBundleShortVersionString`,
  missing `runtime.GOOS` guard, and discarded zip-close errors — all three were
  fixed in code (see `scripts/package-macos.go`'s `fix(macOS): valid plist
  versions, GOOS guard, propagated zip errors` commit).
