# Phase 15 close-out

Date: 2026-07-23

Runtime candidate: `d468089`

Pull request: [#213](https://github.com/cervantesh/CervTerm/pull/213)

## Disposition

**PASS — Phase 15 implementation and bounded qualification are complete.**

This closes release hardening without broadening any experimental feature's default or support claim. Renderer selection remains excluded.

## Required-item closure

| Item | Result | Evidence |
| --- | --- | --- |
| P15-02 Migration | PASS | Sanitized v1 corpus, paired v2 templates, semantic equivalence and source-immutability tests in [`phase-15-config-migrations.md`](phase-15-config-migrations.md). |
| P15-03 Security | PASS | Full tests/vet, focused race suites, all repository fuzz targets, recovery probes, `govulncheck`, and GitHub CodeQL run [`30048191712`](https://github.com/cervantesh/CervTerm/actions/runs/30048191712) with zero results/open alerts; see [`phase-15-security-accessibility.md`](phase-15-security-accessibility.md). |
| P15-04 Performance | PASS | Startup, RSS, idle process/CPU, input, resize, font, tabs/windows, metadata, graphics and packaged-GUI measurements satisfy the accepted budgets in [`phase-15-performance.md`](phase-15-performance.md). |
| P15-05 Platform qualification | PASS | Windows headless/real GUI and Linux headless/WSLg evidence passes the scoped matrix; macOS real GUI is an explicit prerequisite SKIP, not an inferred pass. See [`phase-15-platform-qualification.md`](phase-15-platform-qualification.md). |
| P15-06 Release checkpoint | PASS | Clean package/install, installed-binary smoke, maturity gate, default-off rollback and required Windows/Linux CI run [`30048191106`](https://github.com/cervantesh/CervTerm/actions/runs/30048191106) passed. See [`phase-15-release-readiness.md`](phase-15-release-readiness.md). |

The machine-readable source of truth is [`phase-15-evidence.json`](phase-15-evidence.json). The support-matrix consistency and release preflight scripts reject missing, contradictory, stale, or path-escaped evidence.

## Qualification findings resolved

- Repeated/concurrent Windows ConPTY close is synchronized and idempotent. The real-process capture no longer terminates with native heap corruption.
- Config, workspace-layout and transient-image recovery preserve last-valid state and release provisional resources without publishing partial candidates.
- Public logs, panic output, capability reports and image diagnostics remain value-free/path-redacted under the Phase 15 leak oracle.
- Accessibility passes automated visible-only privacy, stable identity, coalescing, race and real-GUI lifecycle checks; no assistive-technology support claim is made.
- Windows iTerm and Linux WSLg Kitty/Sixel have scoped visual/protocol evidence. Windows ConPTY filtering remains an explicit environment boundary.

## Residual boundaries

These are accepted qualification limits, not hidden passes:

- macOS real-GUI validation is `SKIP` until a Cocoa/OpenGL host is available;
- Windows UIA has no Narrator/NVDA/JAWS validation and retains `support_claim=none`;
- Windows IME JCK coverage remains skipped and IME stays experimental/default-off;
- terminal graphics remain experimental/default-off and protocol/platform support claims remain bounded by the support matrix;
- renderer selection and remote/domain work remain out of scope.

## Rollback and release policy

- Keep experimental graphics, IME, accessibility and native notification flags disabled by default.
- Disable an opt-in feature and restart to return to the qualified text-only path.
- Preserve v1 loading, compatibility adapters and last-valid candidate rollback through at least one stable release.
- A beta checkpoint may be published after the final PR-head required checks pass and the branch is merged. Broader defaults or support claims require new platform evidence rather than documentation-only promotion.
