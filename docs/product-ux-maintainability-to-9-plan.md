# Product/UX and Maintainability Plan to 9/10

Date: 2026-07-11  
Baseline commit: `9ec7dd3`

## Goal

Raise CervTerm's two weaker maturity dimensions to 9/10:

- Product / UX final: 6.2 -> 9.0
- Maintainability: 7.3 -> 9.0

The target is not only more documentation. A 9/10 score requires executable gates, user-facing diagnostics, support workflows, CI checks, and reduced code-evolution risk.

## Product / UX: Target 9/10

CervTerm reaches Product/UX 9 when users can install, start, configure, diagnose, and report issues without maintainer hand-holding, and releases are backed by trustworthy package verification.

### UX-1: Installer and installation trust

Actions:

1. Publish a trusted install path, preferably winget for the portable zip.
2. Add Authenticode signing when a certificate is available.
3. Keep SHA256 checksums and GitHub artifact attestations mandatory.
4. Document install, update, uninstall, config, logs, and trust verification.

Definition of Done:

- README has short installation instructions.
- Release trust docs explain checksums, attestations, and signing status.
- Release preflight fails when required package trust artifacts are missing.
- Signing becomes a hard gate once certificates/secrets are available.

Expected score movement: 6.2 -> 7.0.

### UX-2: Onboarding and diagnostics

Actions:

1. Add `cervterm --doctor`.
2. Add a config-path discovery command or clear docs for config discovery.
3. Add `docs/getting-started.md`.
4. Add issue templates that request `--doctor`, log paths, config, screenshots, and capture files.
5. Add `SUPPORT.md`.

Definition of Done:

- A new user can install, run, configure, diagnose, and file a useful bug report from docs alone.
- `--doctor` reports version, platform, config path, candidate config paths, log path, shell, environment hints, and warnings.
- Bug templates request actionable diagnostics.

Expected score movement: 7.0 -> 7.6.

### UX-3: Daily-driver correctness gate

Actions:

1. Add replayable smoke captures for `cmd.exe`, PowerShell, git/pager output, alternate screen, resize/reflow, scrollback, and Unicode/emoji layouts.
2. Add `scripts/daily-driver-smoke.ps1` and/or integration-tag Go tests.
3. Make failures produce logs/captures as artifacts.
4. Include the smoke in release preflight once stable.

Definition of Done:

- Common shell/TUI workflows have automated evidence.
- CI or release preflight exercises the matrix.
- Regressions in basic daily-driver behavior fail before release.

Expected score movement: 7.6 -> 8.2.

### UX-4: GUI visual regression

Actions:

1. Add deterministic render fixtures.
2. Capture screenshots for ASCII, ANSI colors, box drawing, emoji, flags, wide chars, alternate screen, cursor, and selection.
3. Compare screenshots with tolerances or publish them as CI artifacts.

Definition of Done:

- Windows CI or a documented local gate validates startup and basic rendering.
- Important visual regressions are visible before release.

Expected score movement: 8.2 -> 8.6.

### UX-5: Feedback loop and support policy

Actions:

1. Maintain issue templates for bug, rendering bug, install problem, and feature request.
2. Maintain `SUPPORT.md`.
3. Document supported platforms, shells, GPU expectations, and known limitations.
4. Keep changelog entries user-oriented.

Definition of Done:

- Bug reports contain enough context to reproduce.
- Users know what is supported and what is experimental.
- The project has a clear triage path.

Expected score movement: 8.6 -> 9.0.

## Maintainability: Target 9/10

CervTerm reaches Maintainability 9 when complexity is bounded, architectural contracts are executable, dependency/security drift is automated, and risky subsystems are decomposed enough for future changes.

### M-1: Complexity budget automation

Actions:

1. Extend `scripts/check-maturity-gates.go` for required docs/scripts, line-count limits, stale version drift, and security automation files.
2. Add CI gates for `go vet`, `govulncheck`, and maturity gates.
3. Keep large-file exceptions explicit and reviewed.
4. Add linter/complexity tooling later if it proves stable on developer machines.

Definition of Done:

- CI fails for new large production files without explicit exception.
- CI runs static and vulnerability checks.
- Required maturity/support docs cannot silently disappear.

Expected score movement: 7.3 -> 7.8.

### M-2: Refactor `internal/fontglyph`

Actions:

1. Split `internal/fontglyph/backend.go` and `internal/fontglyph/color_colr_render.go` into smaller responsibility-focused files.
2. Preserve existing public behavior and tests.
3. Add tests around new boundaries.

Possible split:

```text
internal/fontglyph/
  backend.go
  backend_fallback.go
  backend_cache.go
  backend_diagnostics.go
  color_colr_paint.go
  color_colr_gradients.go
  color_colr_transforms.go
```

Definition of Done:

- No production Go file exceeds 500 lines unless explicitly documented.
- Tests and emoji smoke checkers pass.
- No expected rendering behavior changes.

Expected score movement: 7.8 -> 8.3.

### M-3: Contract tests between layers

Actions:

1. Add contracts for VT bytes -> parser events -> terminal state.
2. Add contracts for core state -> render snapshots.
3. Add contracts for PTY resize -> terminal resize.
4. Add contracts for Unicode cluster -> width model -> render cells.
5. Add contracts for frontend input normalization -> internal input events.

Definition of Done:

- Changing one layer breaks tests if it violates a documented boundary.
- Contracts are documented in `docs/architecture.md`.
- Fixtures are small and readable.

Expected score movement: 8.3 -> 8.6.

### M-4: Architecture decision records

Actions:

Add ADRs for:

1. Core/renderer separation.
2. Unicode cluster and width model.
3. Font fallback strategy.
4. Portable zip vs MSI/installer strategy.
5. Diagnostics/logging strategy.
6. PTY/integration testing strategy.

Definition of Done:

- Major architectural decisions have durable context.
- New large changes must reference or update an ADR.

Expected score movement: 8.6 -> 8.8.

### M-5: Dependency/security automation

Actions:

1. Add Dependabot for Go modules and GitHub Actions.
2. Add `govulncheck` to CI.
3. Generate an SBOM during releases.
4. Add a license/dependency check if a stable local tool is selected.

Definition of Done:

- Known Go vulnerabilities fail CI.
- Dependency update PRs are automatic.
- Releases include an SBOM artifact.

Expected score movement: 8.8 -> 9.0.

## Recommended Sprint Order

### Sprint 1: High-impact low-risk foundation

- Save this plan.
- Add `--doctor`.
- Add getting-started docs.
- Add issue templates and `SUPPORT.md`.
- Add Dependabot and `govulncheck`.
- Extend maturity gates.

Expected result:

- Product/UX: 6.2 -> about 7.4.
- Maintainability: 7.3 -> about 8.0.

### Sprint 2: Daily-driver correctness

- Daily-driver smoke matrix.
- PTY/script fixtures.
- Release preflight integration.

Expected result:

- Product/UX: 7.4 -> about 8.2.
- Maintainability: 8.0 -> about 8.4.

### Sprint 3: Structural refactor

- Split `fontglyph` concentration points.
- Add contract tests.
- Add ADRs.

Expected result:

- Product/UX: 8.2 -> about 8.5.
- Maintainability: 8.4 -> about 8.8.

### Sprint 4: Product trust and final polish

- Authenticode signing.
- winget stable submission.
- GUI screenshot regression.
- SBOM release artifacts.
- Support policy hardening.

Expected result:

- Product/UX: 8.5 -> 9.0.
- Maintainability: 8.8 -> 9.0.

## Declaration Criteria for 9/10

Declare Product/UX 9 only when:

- Trusted install path exists.
- `--doctor` and support docs are mature.
- Daily-driver smoke is CI/preflight gated.
- GUI smoke/visual regression exists.
- Issue reports are actionable by default.

Declare Maintainability 9 only when:

- Complexity and security gates run in CI.
- Large risky files are split or explicitly justified.
- `fontglyph` risk is reduced.
- Cross-layer contract tests exist.
- ADRs preserve architectural intent.
- Dependency/security drift is automated.
