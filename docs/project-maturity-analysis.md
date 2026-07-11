# CervTerm Project Maturity Analysis

Date: 2026-07-10  
Branch: `main`  
Commit: `f3a668621d66bbb8d5afa37dc7967b0414ebda55`  
Release assessed: [`v0.2.0-beta.1`](https://github.com/cervantesh/CervTerm/releases/tag/v0.2.0-beta.1)

## Executive Summary

CervTerm is at an early beta maturity level: it has a credible architecture, meaningful automated verification, green CI on `main` and `dev`, reproducible prerelease artifacts, and a published beta release with checksums. It is no longer just a prototype, but it is not yet a low-risk daily-driver terminal.

Overall maturity score: **7.1 / 10**

Interpretation:

- **Architecture maturity:** high for a pre-1.0 terminal.
- **Release maturity:** strong beta-level workflow.
- **Correctness maturity:** promising, but still needs broader VT/vttest and GUI/runtime coverage.
- **Product maturity:** beta/experimental; not ready to position as stable daily-driver software.

## Evidence Reviewed

### Repository and release state

- `origin/main` and `origin/dev` both point to `f3a668621d66bbb8d5afa37dc7967b0414ebda55`.
- Release `v0.2.0-beta.1` is published as a prerelease.
- Release assets include:
  - `cervterm-v0.2.0-beta.1-windows.zip`
  - `cervterm-v0.2.0-beta.1-linux-headless-amd64.zip`
  - `SHA256SUMS.txt`

### CI and automated verification

Recent GitHub Actions results:

- CI on `main`: success.
- CI on `dev`: success.
- Release workflow for `v0.2.0-beta.1`: success.

Local verification executed during assessment:

```bash
go test ./...
go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
```

Both passed.

Coverage snapshot from `go test ./... -coverprofile`:

- Total statement coverage: **68.8%**
- Strong packages:
  - `internal/core`: 81.4%
  - `internal/input`: 86.8%
  - `internal/render`: 100.0%
  - `internal/selection`: 94.6%
  - `internal/theme`: 100.0%
  - `internal/vt`: 82.0%
- Weak or intentionally hard-to-unit-test surfaces:
  - `cmd/cervterm`: 0.0%
  - `internal/frontend/glfwgl`: 0.0%
  - `internal/pty`: 0.0%

### Project size snapshot

- Go files: 110
- Go test files: 45
- Total Go LOC: about 14,140
- Documentation Markdown files under `docs/`: 12
- Scripts: 12
- Packaging files: 11

Largest Go files:

- `internal/fontglyph/backend.go`: 585 lines
- `internal/fontglyph/color_colr_render.go`: 521 lines
- `internal/core/terminal.go`: 485 lines
- `internal/fontglyph/color_colr_test.go`: 474 lines
- `internal/core/terminal_test.go`: 465 lines

The roadmap guardrail says touched source files should remain under 500 lines where practical. Two files currently exceed that threshold.

## Maturity Scores by Area

| Area | Score | Assessment |
|---|---:|---|
| Architecture and layering | 8.0 | Clear separation among core, VT parser, PTY, rendering snapshots, GLFW frontend, config, and font glyph backend. Good direction for future mux/domain growth. |
| Terminal correctness | 7.0 | Many essential VT, input, mouse, scrollback, resize, Unicode, and alternate-screen features exist. Still needs broader authoritative `vttest` coverage and more TUI app validation. |
| Unicode, emoji, and font rendering | 7.5 | Strong semantic cluster model, generated Unicode properties, font fallback, Noto flag handling, color glyph raster paths, diagnostics, and checkers. Complexity is high and cross-platform coverage still needs expansion. |
| Test and verification strategy | 7.5 | Good unit coverage for core packages, parser fuzz smoke, golden VT fixtures, emoji checkers, release preflight, CI. Weakest areas are GUI runtime, PTY integration, and end-to-end installed-app testing. |
| CI and release automation | 8.0 | Green `main`/`dev`, tag-triggered release workflow, Windows and Linux-headless artifacts, checksums, provenance attestations, prerelease publication. Signing/MSI/winget finalization remains deferred. |
| Documentation | 8.0 | README, architecture, roadmaps, release notes, vttest docs, emoji research, packaging notes, screenshots, and release stabilization plan are strong for beta. Some examples still reference older `0.1.0` naming/version conventions. |
| Packaging and installability | 7.0 | Windows portable zip is usable and now includes bundled Noto Color Emoji. Linux headless zip validates non-GUI surfaces. No signed installer, stable winget submission, or MSI yet. |
| Operations and diagnostics | 6.5 | `--log-file`, default diagnostic logging, panic capture, and checker warnings exist. Needs a clearer support/debug workflow for user bug reports and runtime environment capture. |
| Security and supply chain | 6.5 | GitHub checksums and provenance attestations are good. Still lacks Authenticode signing, SBOM, dependency vulnerability scanning, and explicit release trust documentation. |
| Product/UX readiness | 5.5 | App is launchable and visually demonstrable, but README correctly says it is not a finished daily-driver. Missing mature installer UX, config discovery polish, broad GUI smoke automation, and user-facing stability guarantees. |
| Maintainability | 7.0 | Package boundaries are good and docs explain decisions. Main risk is complexity concentration in `internal/fontglyph`, plus a few files over the stated line-count guardrail. |

## Strengths

1. **Good architectural foundation**
   - Core terminal state is independent from renderer, PTY, and frontend concerns.
   - Renderer-neutral snapshots give room for future frontends or rendering backends.
   - Font/platform-specific logic is isolated in `internal/fontglyph`.

2. **Unusually strong Unicode/emoji work for this project stage**
   - Generated Unicode data.
   - Cluster-aware width/rendering model.
   - Noto Color Emoji fallback for flags.
   - Glyph coverage checkers.
   - Non-intrusive warning strategy.

3. **Credible beta release pipeline**
   - CI is green on `main` and `dev`.
   - Tag-triggered release workflow works.
   - Published prerelease exists with assets and checksums.
   - Local preflight validates package contents.

4. **Good documentation discipline**
   - Architecture, roadmap, packaging, vttest, emoji, and release documents exist.
   - Known limitations are explicitly documented.

5. **Healthy core test posture**
   - `core`, `vt`, `input`, `selection`, `render`, and `theme` have meaningful coverage.
   - Parser fuzz smoke and replay-style fixtures reduce regression risk.

## Main Risks

1. **GUI/runtime coverage gap**
   - `internal/frontend/glfwgl` reports 0% coverage in normal coverage mode.
   - There are compile checks and some tagged tests, but not enough automated end-to-end GUI behavior validation.

2. **PTY integration coverage gap**
   - `internal/pty` has 0% statement coverage in the coverage report.
   - PTY behavior is OS-specific and must be tested by smoke/integration scripts, but those are not yet a hard CI gate beyond targeted release flows.

3. **Fontglyph complexity risk**
   - `internal/fontglyph/backend.go` is 585 lines and `color_colr_render.go` is 521 lines.
   - The package handles complex cross-platform font behavior; future changes should avoid further concentration.

4. **Unsigned Windows distribution**
   - Portable zip is acceptable for beta, but Windows trust/user friction will remain high without Authenticode signing or a trusted installer/channel.

5. **Compatibility is not yet daily-driver proven**
   - Important VT features exist, but broad interactive `vttest`, real TUI app fixtures, shell matrix testing, and long-running sessions are still limited.

6. **Version/documentation drift**
   - Some docs/examples still mention `0.1.0-beta.1` even though the current release is `v0.2.0-beta.1`.
   - This is minor but should be cleaned before broader users arrive.

## Readiness Classification

### Current classification

**Beta-quality technical foundation / experimental user release.**

CervTerm is mature enough for:

- developer testing,
- architecture validation,
- controlled beta releases,
- Unicode/emoji regression verification,
- packaging pipeline iteration,
- early user feedback from technically comfortable users.

CervTerm is not yet mature enough for:

- marketing as a stable daily-driver terminal,
- non-technical Windows users without warnings about unsigned binaries,
- production reliance for all TUI workflows,
- broad cross-platform GUI support.

## Recommended Next Milestones

### Milestone 1 — Daily-driver correctness gate

Goal: prove common terminal apps behave correctly.

Recommended work:

1. Expand `vttest` automation beyond startup/menu capture.
2. Add scripted smoke fixtures for:
   - cmd.exe
   - cmd
   - Git interactive output
   - vim or a comparable full-screen TUI
   - less/pager behavior
3. Add regression fixtures for:
   - scroll regions
   - alternate screen
   - mouse mode routing
   - resize/reflow
   - wide/emoji cells inside TUI layouts

Exit criteria:

- No major visual corruption in common shell/TUI workflows.
- CI or local preflight can replay representative captures.

### Milestone 2 — GUI smoke automation

Goal: reduce risk in the GLFW/OpenGL frontend.

Recommended work:

1. Add a deterministic smoke capture path for the installed Windows zip.
2. Automate screenshots for a small fixed fixture matrix.
3. Compare screenshots with tolerances or at minimum publish them as CI artifacts.
4. Add startup/log assertion tests that confirm diagnostics warnings are non-intrusive.

Exit criteria:

- GUI startup and basic rendering are validated automatically on Windows CI or a documented local gate.

### Milestone 3 — Packaging trust and installation path

Goal: make install experience less experimental.

Recommended work:

1. Finalize portable winget manifest for `v0.2.0-beta.1` or the next beta.
2. Decide Authenticode signing path.
3. Decide whether MSI/WiX is worth enabling before 1.0.
4. Add SBOM and dependency vulnerability scanning.
5. Add release trust docs explaining checksums, attestations, and signing status.

Exit criteria:

- Users can install/update through a known channel.
- Release authenticity story is clear.

### Milestone 4 — Maintainability hardening

Goal: keep the architecture from collapsing under rendering complexity.

Recommended work:

1. Split `internal/fontglyph/backend.go` below 500 lines.
2. Split `internal/fontglyph/color_colr_render.go` below 500 lines.
3. Add package-level docs for `internal/fontglyph` explaining responsibilities and invariants.
4. Add a line-count CI check matching the roadmap guardrail.

Exit criteria:

- No touched production Go file exceeds the documented guardrail without an explicit exception.

### Milestone 5 — Documentation cleanup

Goal: keep beta users from seeing stale instructions.

Recommended work:

1. Replace old `0.1.0-beta.1` examples with `v0.2.0-beta.1` or `<tag>` placeholders.
2. Add a `docs/troubleshooting.md` page for:
   - logs,
   - missing Noto Color Emoji,
   - unsigned Windows binary warnings,
   - config loading,
   - capture-vt diagnostics.
3. Add a short compatibility matrix.

Exit criteria:

- A new beta user can install, run, configure, diagnose, and report issues from documentation alone.

## Suggested Next Action

The best next concrete step is:

**Add a daily-driver correctness gate document and turn 3–5 representative TUI/shell sessions into replayable fixtures.**

This directly addresses the biggest remaining maturity gap: confidence that the terminal behaves correctly beyond unit-level parser/core behavior and curated emoji/rendering checks.
