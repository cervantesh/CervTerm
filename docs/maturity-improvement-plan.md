# CervTerm Maturity Improvement Plan

Date: 2026-07-10  
Baseline: [`docs/project-maturity-analysis.md`](project-maturity-analysis.md)  
Target: raise the least mature areas from experimental/beta to repeatable beta gates.

## Scope

This plan attacks the lowest-scoring maturity areas from the maturity analysis:

| Area | Baseline | Target |
|---|---:|---:|
| Product/UX readiness | 5.5 | 6.5 |
| Operations and diagnostics | 6.5 | 7.2 |
| Security and supply chain | 6.5 | 7.2 |
| Packaging and installability | 7.0 | 7.6 |
| Maintainability | 7.0 | 7.5 |
| GUI/PTY integration verification | implicit gap | explicit smoke gate |

Non-goals for this slice:

- Buying/configuring an Authenticode certificate.
- Promoting WiX/MSI to the primary install path.
- Claiming daily-driver stability.
- Full visual screenshot diffing in CI.

## Definitions

### DoE — Definition of Exploration

A maturity slice is ready to design only when exploration has captured:

1. The affected maturity areas and current score.
2. Existing repo evidence: docs, scripts, CI, tests, release artifacts.
3. Constraints that cannot be solved inside the slice, such as signing certificates or external installer policy.
4. The smallest useful improvement that can be verified automatically.
5. The risk of doing nothing.

### DoDm — Definition of Done for Maturity

A maturity improvement is done only when:

1. It has an executable or reviewable gate, not only prose.
2. The gate runs locally and/or in CI.
3. User-facing docs explain what changed and how to diagnose failures.
4. Release/package preflight knows about the new requirement when relevant.
5. The change is verified with tests or script execution.
6. Residual risks are documented instead of hidden.

## Workflow

### 1. Exploration

Findings:

- GUI/PTY integration was the weakest automated verification surface.
- Troubleshooting existed implicitly through `--log-file`, but no user-facing diagnostic workflow existed.
- Release trust depended on checksums and GitHub attestations, but the trust model was not documented clearly.
- Packaging worked, but clean installed package smoke was not encoded as a reusable script/gate.
- Maintainability had a documented line-count guardrail, but no automated guard.
- Version examples had drifted after `v0.2.0-beta.1`.

Exploration exit criteria:

- Weak areas mapped to concrete repo surfaces.
- First slice chosen without requiring external secrets or policy decisions.

### 2. Design

Design choice: create a lightweight maturity gate layer around the existing beta pipeline.

Components:

1. `docs/troubleshooting.md` — user/debug operator workflow.
2. `docs/release-trust.md` — release authenticity and unsigned-beta expectations.
3. `scripts/smoke-installed-package.ps1` — reusable clean-package smoke for Windows zip.
4. `scripts/check-maturity-gates.go` — cross-platform repository maturity guard.
5. CI integration — run maturity gates and installed-package smoke.
6. Release preflight integration — require docs and smoke script presence.

Design exit criteria:

- No external secrets required.
- Existing release path remains portable zip based.
- New checks are deterministic and fast.

### 3. Plan

Implementation order:

1. Add troubleshooting and release trust docs.
2. Add maturity gates script:
   - required maturity docs exist;
   - stale beta version examples are not present in user docs;
   - production Go files over 500 lines are either absent or listed as explicit known exceptions;
   - release trust docs mention checksums, attestations, and signing status.
3. Add installed package smoke script:
   - extract or reuse a Windows zip;
   - run `--version`, `--build-info`, and `--print-default-config`;
   - run `--capture-vt` against `cmd.exe`;
   - assert bundled `NotoColorEmoji.ttf` and license exist;
   - fail if emoji coverage warnings appear during smoke.
4. Wire CI to run:
   - maturity gates;
   - package preflight;
   - installed package smoke on Windows package artifact.
5. Update stale version docs to `<tag>` or `v0.2.0-beta.1` where appropriate.
6. Verify locally.

### 4. Implementation

Implemented in this slice:

- Troubleshooting docs for logs, config, emoji fonts, unsigned Windows binaries, and VT capture.
- Release trust docs for prerelease status, checksums, GitHub attestations, unsigned beta binaries, and future signing/SBOM work.
- Maturity gate script.
- Installed-package smoke script.
- CI and release preflight hooks.
- Version drift cleanup.

### 5. Review

Review checklist:

- `go test ./...`
- `go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1`
- `go run ./scripts/check-maturity-gates.go`
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-beta.ps1 -Version v0.2.0-beta.1 -OutDir dist`
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release-preflight.ps1 -Version v0.2.0-beta.1 -OutDir dist -WindowsZip dist/cervterm-v0.2.0-beta.1-windows.zip`
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/smoke-installed-package.ps1 -ZipPath dist/cervterm-v0.2.0-beta.1-windows.zip`

## Residual Risks

- Authenticode signing remains deferred until a certificate exists.
- MSI/WiX remains a template until product install policy is decided.
- GUI visual regression is still not screenshot-diffed in CI.
- The current line-count gate allows two known `internal/fontglyph` exceptions; future work should split them.

## Next Slice

The next maturity slice should implement a daily-driver correctness gate using replayable sessions for:

1. PowerShell prompt and colored output.
2. `cmd.exe` prompt and command output.
3. A pager or `git log` style output.
4. Full-screen alternate-screen behavior.
5. Resize/reflow with wide/emoji text.
