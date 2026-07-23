# Phase 15 platform qualification

Date: 2026-07-23
Runtime candidate: `f2be778`
Qualification package: `0.10.0-phase15-rc2`
Machine-readable commands, hashes, exit dispositions and artifact identities: [`phase-15-platform-manifest.json`](phase-15-platform-manifest.json).

## Windows 11 amd64

**Result: PASS** for the Windows beta package and real GLFW/OpenGL GUI.

Environment: Windows 11 Home `10.0.26200`, build `26200`, AMD Ryzen 9 7940HX, Go 1.25.8.

The RC2 package passed:

- `release-preflight.go`: 39 PASS, 0 required failures, one expected missing-vttest warning;
- `smoke-installed-package.go`: version, build info, doctor, generated config, emoji asset, raw ConPTY capture and diagnostics;
- `daily-driver-smoke.go`: cmd, git log, pager, alternate screen, 40/100-column reflow and long-session captures;
- clean zero-exit shutdown for every capture and daily-driver case.

The initial RC1 qualification exposed repeatable Windows heap termination `0xc0000374` after successful captures. `f2be778` made `localSession.Close` concurrent/idempotent around the non-idempotent native ConPTY close. The focused 32-caller race test, full tests, a direct capture, installed-package smoke and the complete RC2 daily-driver matrix then exited cleanly.

Two real GUI runs used a bounded helper emitting ordinary text, OSC 8, OSC 133 semantic zones, and Kitty/Sixel/iTerm envelopes:

- all graphics and accessibility disabled: [screenshot](../assets/phase15-windows-disabled.png);
- all three graphics protocols plus visible-only experimental accessibility enabled: [screenshot](../assets/phase15-windows-enabled.png).

Both windows became visible, remained responsive during a five-second idle sample, retained text after all envelopes, captured a screenshot, processed `WM_CLOSE`, and exited zero without panic/fatal diagnostics. The enabled run exercised real OpenGL context/cache/placement activation; this is a bounded qualification fixture, not broad protocol conformance.

### Process comparison

Machine-readable samples: [`phase-15-process-comparison.json`](phase-15-process-comparison.json).
Reproducible Win32 metric capture harness: [`../../scripts/capture-phase15-process.py`](../../scripts/capture-phase15-process.py).

Three alternating Phase 0/candidate runs used the same 900x620 helper window. Measurement ended by forced termination so Phase 0's known non-idempotent shutdown did not bias the metrics.

| Metric (median) | Phase 0 `7d64cc9` | Candidate `f2be778` | Change |
|---|---:|---:|---:|
| Window-visible startup | 265.96 ms | 290.99 ms | +9.41% |
| Working set | 109,817,856 B | 111,230,976 B | +1.29% |
| Peak working set | 109,817,856 B | 111,230,976 B | +1.29% |
| Idle CPU, one-core scale | 0.5207% | 0.5208% | effectively unchanged |

All remain below the Phase 15 15% investigation threshold. A separate enabled graphics/accessibility run measured 313.54 ms startup, 115,851,264 B working set, 0.3125% idle CPU, and clean exit.

## Linux amd64

### Headless

**Result: PASS.** Ubuntu 24.04 under WSL2, Linux 6.6.87.2, Go 1.25.8:

- maturity gates;
- `CGO_ENABLED=0 go test ./...`;
- `CGO_ENABLED=0 go vet -unsafeptr=false ./...`;
- headless `cmd/cervterm` build, `--version`, and `--print-default-config`.

The host lacks a system emoji package; qualification supplied the same pinned packaged Noto Color Emoji asset used by the beta package rather than weakening the tests.

### Real GUI

**Result: PASS with WSLg scope.** GLFW/OpenGL tests and build passed against user-extracted Ubuntu development packages. A WSLg/X11 real window then ran with Mesa software OpenGL, all image protocols enabled, emitted text/OSC/Sixel fixture traffic, produced a [cropped screenshot](../assets/phase15-linux-wslg.png), accepted an X11 window-close event, and exited zero.

Expected platform diagnostics reported unsupported KWin blur/titlebar integration and no Segoe UI Emoji. This qualifies WSLg/X11 software-GL integration only; it does not claim broad native Linux desktop/driver support.

## macOS

**Result: SKIP for real GUI; PASS for headless compile.** Both `darwin/amd64` and `darwin/arm64` `CGO_ENABLED=0 go build ./...` passed. No macOS host or real Cocoa/OpenGL session was available, so no real-GUI, installed-font, IME, accessibility, blur, or graphics-protocol claim is made.

## Additional compile boundary

`windows/arm64`, `CGO_ENABLED=0 go build ./...` passed. No Windows ARM64 real-GUI claim is made.

## Claim boundary

Windows amd64 remains the qualified beta platform. Kitty, Sixel, iTerm images, Windows UI Automation and IME remain experimental, explicit opt-in/default-off surfaces. Linux WSLg evidence is narrow integration evidence. macOS real GUI remains unqualified. No selectable renderer/backend work is included.
