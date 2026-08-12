# macOS Platform Validation — 2026-08-12

Ad hoc community validation pass against the `platform: macOS` issue group
(#104, #105, #106, #107, #108, #109, #110, tracked by #111). This is a single
real-hardware data point, not a full qualification: see "Not verified" below
for what still needs a human tester, additional hardware, or an Apple
Developer certificate.

> **⚠️ THIS BRANCH CANNOT GO GREEN WITHOUT A MAINTAINER DECISION. ⚠️**
> The maturity gate fails on **every** CI job (Windows, macOS, Linux), not
> just the new macOS one, because this diff edits two files whose content is
> hash-pinned to closed Slice 5.5c. Nothing in the code is wrong; the pin is
> doing exactly what it was built to do. It cannot be resolved from inside
> this pass — see
> [Blocking: pinned-evidence maturity gate](#blocking-pinned-evidence-maturity-gate)
> for the verified remedy and exact digests.

## Environment

- Mac: Intel, `x86_64`
- macOS: 15.7.7 (Sequoia), build 24G720
- Display: 2560x1600 Retina (per `system_profiler SPDisplaysDataType`)
- Go: 1.25.8 per `go.mod`; built/tested with go1.26.5 (`go-version-file: go.mod` resolves the module's declared version in CI)
- Xcode Command Line Tools present; no Apple Developer Program certificate available
- Shell: `/bin/zsh` (default), `/bin/bash` (explicit)

## Bugs found and fixed on real macOS hardware

Neither of these was visible from Windows/Linux CI; both only reproduce
against real macOS system fonts and real macOS temp-directory symlinks.

1. **Apple Color Emoji never rendered in color** (`internal/fontglyph/raster/sfnt_tables.go`). The sfnt table-directory parser assumed every font file starts with a plain single-face table directory. `/System/Library/Fonts/Apple Color Emoji.ttc` is a TrueType Collection (`ttcf` header + per-face directories), so the parser silently read garbage table entries and reported zero color tables, and `sbix` bitmap extraction (already implemented for the render path) never activated. Fixed by unwrapping the `ttcf` header to the first face's own table directory before scanning. Added a synthetic-fixture regression test (`TestDetectColorTablesUnwrapsTrueTypeCollection`) so this doesn't require a real font file to catch a regression. `TestSystemColorEmojiRasterizesKnownGlyph`/`TestSystemColorEmojiRasterizesRepresentativeGlyphs`/`TestSystemColorEmojiFixtureDetection` (previously failing on this Mac) now pass.
2. **Config/background watch-set falsely duplicated entries on macOS** (`internal/frontend/glfwgl/config_watch.go`). `watchPathIdentity` only canonicalized parent-directory symlink aliases on Windows (an 8.3-short-name/case concern). macOS's `/var` → `/private/var` and `/tmp` → `/private/tmp` system symlinks hit the exact same class of problem, so a config/background-image path observed both before and after canonicalization elsewhere in the pipeline was treated as two different files — breaking the `-tags glfw ./internal/frontend/glfwgl` reload/background test suite outright (6 failing tests). Fixed by running the same parent-directory `EvalSymlinks` canonicalization on all platforms, keeping case-folding Windows-only (other platforms are case-sensitive). All 6 previously-failing tests now pass.

## Blocking: pinned-evidence maturity gate

**This branch fails `go run ./scripts/check-maturity-gates.go` and cannot be
made to pass without a maintainer governance decision. Do not merge assuming
CI will go green.**

Fix #1 above edits `internal/fontglyph/raster/sfnt_tables.go` and
`internal/fontglyph/raster/color_tables_test.go`. Both are content-hash-pinned
by closed Slice 5.5c via
`docs/validation/architecture-maturity-slice-5.5c/source-manifest-candidate.txt`,
enforced by `comparePinnedManifest` in `scripts/check-slice55c-evidence.go`
(protected scope: every path under `internal/fontglyph/`). Editing them is
therefore *guaranteed* to trip the gate.

Verified, not assumed:

- `main` at `1216cef`: `maturity gates ok` (exit 0).
- This branch at `4881480`: exit 1, with exactly two failures — the two files
  above. Nothing else in the gate is broken.
- The check is **not** platform-gated. `.github/workflows/ci.yml` runs it in
  the `windows`, `macos`, and `linux-headless` jobs alike, and the manifest
  digests are LF-normalized, so CRLF checkout changes nothing. **All three
  jobs fail identically.** The new macOS job did not cause this and is not
  uniquely affected.
- Correction to an earlier draft of this note: `scripts/check-slice31-owner.go`
  is **not** involved. It contains zero references to `fontglyph`, its
  allowlist derives from a fixed committed commit range rather than from HEAD,
  and `go run ./scripts/check-slice31-owner.go` still reports
  `Slice 3.1 owner guards ok` on this branch.

### What a maintainer would have to do

No fabricated history is required — every lineage constant the guard checks
(`baseCommit`/T/A/M/W/G) is a real commit already in `main`, and `checkHistory`
explicitly permits arbitrary descendant commits. What *is* required is
re-declaring a closed slice's pinned evidence, which is a governance call:

1. Update the two lines in `source-manifest-candidate.txt` to the post-fix
   digests:
   - `internal/fontglyph/raster/color_tables_test.go` →
     `0c2ec0e95cbf5bc04a47bd48d8094c30ff225306b4de1d17a8a8326be3747813`
   - `internal/fontglyph/raster/sfnt_tables.go` →
     `480a67792049caaf63cee82b0b696c1d99afb23cbb7609c9f3f52b4938129f88`
2. Update `artifactHashes["source-manifest-candidate.txt"]` in
   `scripts/check-slice55c-evidence.go` (currently
   `3225257f40be27a13a9022e253ed4c1b0995bf08679b1ad312e68a6b19e5d268`) to the
   manifest's new digest,
   `b5b3f43ad693144e35415f2d9628138391480b9f803414cfac7ea6474942a095`.
   That value was verified by applying the two-line patch in a throwaway
   worktree; it is only valid if the two pinned files are byte-identical to
   this branch's versions. Any further edit to either file invalidates it.

**Do not use the guard's own `-write-candidate-manifest` flag for this.** It
regenerates the entire manifest from the present working tree, which was
measured to rewrite **272 lines (161+/111-)** — only 2 of which are in the
enforced `internal/fontglyph/` scope. The other 268 are unrelated repo
evolution since Slice 5.5c closed, and rewriting them would silently destroy
the historical snapshot the slice's evidence exists to preserve.

An alternative with closer precedent: rather than re-declaring content hashes,
narrow the guard's scope — the post-G commits `0d6895f`
("fix(ci): scope font extraction successor pins") and `38486b2` established
that *amending the guard's scope* is an accepted pattern, whereas rewriting a
pinned content hash has no precedent in this repo's history.

Nothing in this pass touched any pinned manifest, allowlist, or guard script.
That was deliberate: making the gate pass is a maintainer's decision to make
explicitly, not a side effect of a bug fix.

## Automated evidence (commands run, real output)

```
go test ./...                                                         # PASS, all packages
go vet -unsafeptr=false ./...                                         # clean
go test ./internal/vt -run '^$' -fuzz=FuzzParserAdvanceDoesNotPanic -fuzztime=2s   # PASS
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1 # PASS (after fix #2 above)
go test -tags glfw ./internal/mux ./internal/frontend/glfwgl -run 'Zoom|Divider|Layout|Mux|Atlas' -count=1  # PASS
go test -race ./internal/mux -count=1                                 # PASS
go test -tags glfw ./internal/fontglyph ./internal/frontend/glfwgl ./internal/render ./internal/unicodecluster -count=1  # PASS (after fix #1 above)
go test -tags glfw ./internal/frontend/glfwgl -run 'Blur' -count=1    # PASS, incl. the darwin cgo/Objective-C NSVisualEffectView provider (blur_provider_darwin.m)
go build -tags glfw -o dist/cervterm ./cmd/cervterm                   # succeeds, Mach-O 64-bit x86_64
go build -o dist/cervterm-headless ./cmd/cervterm                     # succeeds
./dist/cervterm-headless --doctor                                     # runs, see config-path finding below
./dist/cervterm-headless --capture-vt dist/zsh.vt --capture-program /bin/zsh --capture-arg=-c --capture-arg='printf mac-pty-ok'   # byte-exact capture
```

zsh/bash PTY capture, UTF-8/CJK/emoji byte round-trip
(`café 你好 😀`), and repeated launch/close cycles (checked against
`pgrep`/`ps` for orphaned `cervterm-headless`/`zsh` processes) all behaved
correctly — no corruption, no leaked processes.

## Findings that are not bugs to fix, but worth the maintainer's attention

Lower-severity audit findings that were reviewed and deliberately deferred
(with the reasoning for each) are logged in
[`macos-platform-validation-2026-08-12-p2-log.md`](macos-platform-validation-2026-08-12-p2-log.md).

- **`--doctor` config discovery does not follow `~/Library` convention.** It reports candidates under `~/.config/cervterm/` (XDG-style), matching Linux rather than native macOS (`~/Library/Application Support/CervTerm`). This is a product decision (XDG-on-macOS is a legitimate, common choice for cross-platform CLI-adjacent tools), not something this pass changed unilaterally.
- **Emoji-coverage startup warnings are platform-blind.** On every macOS launch, the log unconditionally prints "NotoColorEmoji.ttf not found" and "Segoe UI Emoji not found" — fonts that are Linux/Windows-specific and were never expected to exist on macOS. Not incorrect, just noisy; worth gating by platform if someone wants to clean it up.

## macOS `.app` packaging (new, for #110)

Added `packaging/macos/` (`Info.plist.template`, `AppIcon.icns` generated
from the existing `packaging/assets/cervterm.svg` via `rsvg-convert` +
`iconutil`) and `scripts/package-macos.go`. `go run ./scripts/package-macos.go
-version 0.1.0-ci -outdir dist` builds a well-formed `CervTerm.app`
(verified: valid `Info.plist` via `plutil -p`, launches and reports the
injected version correctly from inside the bundle, zips cleanly).

- **Ad-hoc signing (`codesign --sign -`) is implemented but only partially verified.** codesign refuses to sign any bundle carrying a `com.apple.provenance` extended attribute — which recent macOS stamps on newly written files. The script strips xattrs first (`xattr -cr`), but on the specific Mac used for this pass, `com.apple.provenance` could not be removed even directly (`xattr -d` reports success yet the attribute persists — consistent with an MDM/endpoint-security layer enforcing it), so end-to-end ad-hoc signing wasn't confirmed on this machine. This is expected to work on an unmanaged Mac or a stock GitHub-hosted runner; that has not yet been confirmed. Pass `-adhoc-sign=false` to skip signing if you hit the same block.
- **Gatekeeper on an unsigned, quarantined `.app` was verified directly:** simulating a browser download (`xattr -w com.apple.quarantine ...`) and running `spctl -a -vv` returns `rejected / source=no usable signature`, confirming the expected pre-signing user experience.
- **Developer ID signing and notarization are explicitly out of scope** — they require an Apple Developer Program certificate this pass has no access to and is not authorized to obtain, per #110's own note.

## Not verified (needs a human, more hardware, or a certificate)

- Apple Silicon coverage (this pass was Intel-only)
- A second/external display, mixed-scale/multi-monitor behavior
- Non-US keyboard layouts, dead keys, and IME composition (Japanese/Chinese preedit/commit)
- VoiceOver navigation and other assistive-technology behavior
- **Any visual/interactive GUI verification at all.** The GLFW app was launched successfully (process stays alive, startup log sequence completes, including native-window creation) but this automated execution environment has no interactive logged-in desktop session that composites into `screencapture` output — captures show only the desktop picture and menu bar, with no Dock, no menu-bar status items, and no app window, regardless of how long the app had been running. That's an environment limitation, not evidence of an app bug: font rendering quality, glyph alignment, blur appearance, window chrome, Retina sharpness, and zoom/divider behavior were exercised only through the automated Go test suites above, not eyeballed.
- Developer ID signing / notarization ticket stapling
- Apple Silicon and Intel universal binary strategy for release (this build is Intel-only; no `lipo` universal binary was produced)
