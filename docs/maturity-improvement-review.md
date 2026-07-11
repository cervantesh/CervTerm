# CervTerm Maturity Improvement Review

Date: 2026-07-10  
Implementation commit: `b9814be`  
Plan: [`docs/maturity-improvement-plan.md`](maturity-improvement-plan.md)

## Scope Reviewed

This review covers the first maturity-improvement slice for the least mature areas identified in [`docs/project-maturity-analysis.md`](project-maturity-analysis.md):

- Product/UX readiness.
- Operations and diagnostics.
- Security and supply chain communication.
- Packaging and installability.
- Maintainability guardrails.
- GUI/PTY integration smoke coverage.

## What Changed

### Documentation

Added:

- [`docs/maturity-improvement-plan.md`](maturity-improvement-plan.md)
- [`docs/troubleshooting.md`](troubleshooting.md)
- [`docs/release-trust.md`](release-trust.md)
- [`docs/project-maturity-analysis.md`](project-maturity-analysis.md)

Updated:

- `README.md`
- `docs/release-packaging.md`
- `packaging/winget/README.md`
- `packaging/wix/README.md`
- Windows version metadata examples

### Executable gates

Added:

- `scripts/check-maturity-gates.go`
- `scripts/smoke-installed-package.ps1`

Integrated:

- CI runs maturity gates on Windows and Linux jobs.
- CI runs installed-package smoke after building the Windows zip.
- Release workflow runs maturity gates before packaging.
- Release workflow runs installed-package smoke before uploading Windows artifacts.
- Release preflight now checks for maturity docs and smoke/gate scripts.

## DoE Review

The Definition of Exploration was satisfied:

1. Weak areas were identified from the maturity analysis.
2. Existing repo evidence was inspected: CI, release workflow, release preflight, docs, PTY, GLFW, and scripts.
3. External constraints were separated from this slice: Authenticode certificate, MSI policy, and full GUI screenshot diffing remain out of scope.
4. The smallest useful executable improvement was chosen: maturity gates plus clean installed-package smoke.
5. Risk of doing nothing was documented: GUI/PTY/package regressions could ship without a reusable smoke gate, and users lacked troubleshooting/release trust guidance.

## DoDm Review

The Definition of Done for Maturity was satisfied:

1. The change added executable gates, not only prose.
2. Gates run locally and in CI.
3. User-facing docs now explain troubleshooting and release trust.
4. Release preflight knows about the new maturity docs/scripts and required package contents.
5. Verification was run locally and remotely.
6. Residual risks are documented in the plan.

## Local Verification

Commands run successfully:

```bash
go test ./...
go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/check-maturity-gates.go
```

Windows package verification run successfully:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-beta.ps1 -Version v0.2.0-beta.1 -OutDir dist
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release-preflight.ps1 -Version v0.2.0-beta.1 -OutDir dist -WindowsZip dist/cervterm-v0.2.0-beta.1-windows.zip
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/smoke-installed-package.ps1 -ZipPath dist/cervterm-v0.2.0-beta.1-windows.zip -ExpectedVersion v0.2.0-beta.1
```

Observed preflight result:

```text
Release preflight summary: 30 pass, 0 required fail, 0 warning
```

Observed installed-package smoke result:

```text
installed package smoke passed: dist/cervterm-v0.2.0-beta.1-windows.zip
```

## Remote CI Review

GitHub Actions after `b9814be`:

- `main`: CI success, run `29138737256`.
- `dev`: CI success, run `29138737990`.

## Maturity Impact

Expected maturity movement after this slice:

| Area | Before | After | Reason |
|---|---:|---:|---|
| Product/UX readiness | 5.5 | 6.2 | Troubleshooting and package smoke create a clearer user path. |
| Operations and diagnostics | 6.5 | 7.1 | Diagnostics docs and smoke workflow make failures reproducible. |
| Security and supply chain | 6.5 | 7.0 | Release trust model documents checksums, attestations, and unsigned beta status. |
| Packaging/installability | 7.0 | 7.5 | Installed zip smoke is now reusable and CI-gated. |
| Maintainability | 7.0 | 7.3 | Line-count exceptions are explicit and future large files fail the gate. |
| GUI/PTY verification | weak | better | Windows package smoke exercises the built exe, config output, logging, bundled font, and `--capture-vt` via `cmd.exe`. |

## Residual Risks

Still open:

- No Authenticode signing yet.
- No SBOM or dependency vulnerability scan yet.
- No screenshot diffing or full GUI visual regression gate yet.
- No broad daily-driver TUI capture matrix yet.
- Two `internal/fontglyph` production files remain over 500 lines as documented exceptions.

## Recommended Next Slice

Implement the daily-driver correctness gate:

1. Add replayable captures for PowerShell, cmd, pager/git-log style output, alternate screen, and resize/reflow.
2. Add a local/CI script that replays captures and verifies expected screen snapshots.
3. Use the resulting gate to raise terminal correctness and GUI/PTY confidence before the next beta release.
