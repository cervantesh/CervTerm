# Phase 15 release readiness

Date: 2026-07-23

Runtime candidate: `d468089`

## Result

**PASS for the Phase 15 beta checkpoint.**

## Gates

- Required CI run [`30048191106`](https://github.com/cervantesh/CervTerm/actions/runs/30048191106): Windows `test` PASS and Ubuntu `linux-headless` PASS.
- CodeQL run [`30048191712`](https://github.com/cervantesh/CervTerm/actions/runs/30048191712): Actions, C/C++, Go and Python PASS with zero results; PR ref has zero open code-scanning alerts.
- Local full/GLFW tests, vet, maturity, race, fuzz, recovery and `govulncheck`: PASS; see the linked Phase 15 evidence documents.
- Windows beta package `v0.10.0-beta.1`: PASS.
- Installed-package version, build-info, doctor, real ConPTY echo and clean shutdown smoke: PASS.
- Experimental graphics, IME, accessibility and native notification features remain disabled by default.

The local package checksum and exact commands are recorded in [`phase-15-package-summary.json`](phase-15-package-summary.json). It is a qualification artifact, not the release artifact: the release workflow must rebuild from the merged tag and publish its own checksums.

## Release conditions

1. Final PR-head required checks must pass after evidence close-out.
2. Merge PR #213 without changing the qualified runtime candidate.
3. Tag the merged commit through the repository's release workflow.
4. Verify release assets, version metadata, checksums and installed-package smoke from the published archive.
5. Retain compatibility adapters and default-off rollback until at least one stable release.

A failing final check, package rebuild, version assertion or published-asset smoke blocks publication. It does not justify weakening the gate or changing a support claim.
