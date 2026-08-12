# macOS Platform Validation — 2026-08-12

Ad hoc community validation pass against the `platform: macOS` issue group
(#104, #105, #106, #107, #108, #109, #110, tracked by #111). This is a single
real-hardware data point, not a full qualification: see "Not verified" below
for what still needs a human tester, additional hardware, or an Apple
Developer certificate.

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

**Note for whoever merges this:** both files are referenced by this
repository's `scripts/check-maturity-gates.go` pinned-evidence guards
(Slice 3.1's `check-slice31-owner.go` allowlist and Slice 5.5c's
`source-manifest-candidate.txt` hash pin). The maturity gate will fail on
this diff until those manifests are regenerated/extended — that's the
gate's own historical-slice governance working as designed, not a new
problem introduced here. I did not touch the pinned manifests myself since
re-declaring a closed slice's evidence is a maintainer decision.

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
