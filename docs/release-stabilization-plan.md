# CervTerm Release Stabilization Plan

## Goal

Stabilize the current Unicode/emoji rendering work, verify CI and local behavior, package a beta build, and publish a clean release path using `dev` for integration and `main` for stable releases.

## Current State

- `main` and `dev` both point to commit `d42b0ff`.
- Unicode cluster handling, emoji glyph fallback, visual captures, diagnostics, and checker scripts are implemented.
- The next work is verification, packaging, clean-install testing, and beta release publication.

## Phase 1 — Verify remote CI

### Objective

Confirm GitHub Actions are green for both `main` and `dev`.

### Steps

1. Open the GitHub Actions page for the repository.
2. Check workflow runs for `main`.
3. Check workflow runs for `dev`.
4. Verify these surfaces:
   - normal Go tests
   - GLFW-tagged tests
   - Windows build
   - release workflow, if triggered or manually runnable

### Success Criteria

- All required checks are green, or every failure has a documented cause and owner.
- No failing check is ignored before packaging.

## Phase 2 — Run local smoke tests

### Objective

Validate the real application binary, not only unit tests.

### Steps

1. Build a fresh CervTerm executable.
2. Launch CervTerm locally.
3. Inspect the diagnostics log.
4. Confirm no unexpected emoji font warnings appear when `NotoColorEmoji.ttf` is available.
5. Exercise visual fixtures:
   - flags
   - ZWJ/person emoji
   - keycaps
   - 100 emoji grid
   - complete emoji grid
6. Confirm `NotoColorEmoji.ttf` is found from the expected runtime path, especially `dist/font-sources/` or the installed package layout.

### Success Criteria

- CervTerm opens normally.
- Flags render as flags, not regional indicator letters.
- ZWJ/person emoji render without obvious degradation.
- Keycaps are not clipped.
- Warnings are written to logs/checkers only, not into terminal output.

## Phase 3 — Run automated emoji verification

### Objective

Ensure semantic cluster coverage and glyph rendering coverage have not regressed.

### Commands

```bash
go test ./...
go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/check-emoji-coverage.go .tmp/emoji-test-latest.txt
go run ./scripts/check-emoji-glyphs.go .tmp/emoji-test-latest.txt
```

### Success Criteria

- All Go tests pass.
- Cluster checker reports full coverage for fully-qualified emoji sequences.
- Glyph checker reports all sequences rasterized, visible, and using color glyph paths.
- All flags are routed through Noto Color Emoji.

## Phase 4 — Prepare beta release artifacts

### Objective

Generate a reproducible beta package suitable for testing outside the development tree.

### Steps

1. Choose a beta version, for example `v0.2.0-beta.1`.
2. Run release preflight.
3. Generate Windows resources.
4. Build the release executable.
5. Generate installer/package artifacts.
6. Confirm package contents include:
   - CervTerm executable
   - Windows manifest
   - icon/resources
   - bundled font sources, especially `NotoColorEmoji.ttf`
   - default config/documentation needed for first run

### Success Criteria

- Release artifacts are generated successfully.
- The package is reproducible from committed scripts.
- The package contains all runtime dependencies needed for emoji fallback.

## Phase 5 — Test clean installation

### Objective

Verify behavior as an end user, outside the repo checkout.

### Steps

1. Install from the generated package.
2. Launch CervTerm from the installed location.
3. Confirm default config loading works.
4. Confirm diagnostics logging works.
5. Re-run visual emoji smoke checks.
6. Confirm font fallback still finds bundled/installed `NotoColorEmoji.ttf`.
7. Confirm warnings remain non-intrusive.

### Success Criteria

- Installed CervTerm launches successfully.
- Emoji and flags render correctly outside the repo.
- No runtime dependency silently relies on development-only paths.

## Phase 6 — Publish beta release

### Objective

Publish a consumable GitHub beta release.

### Steps

1. Create and push the release tag:

```bash
git tag v0.2.0-beta.1
git push origin v0.2.0-beta.1
```

2. Create a GitHub Release for the tag.
3. Upload generated artifacts.
4. Include release notes covering:
   - Unicode cluster architecture
   - emoji glyph fallback
   - Noto Color Emoji flag handling
   - checker scripts
   - diagnostics/log warnings
   - known limitations
   - Windows support status

### Success Criteria

- GitHub Release exists.
- Artifacts are attached.
- Release notes clearly describe what changed and how to verify it.

## Phase 7 — Normalize branch flow

### Objective

Keep future work organized after this release.

### Recommended Flow

- `main`: stable/release branch.
- `dev`: integration branch.
- feature branches: branch from `dev`.
- PRs: merge feature branches into `dev`.
- release promotion: merge `dev` into `main` only after CI, smoke tests, and packaging pass.

### Success Criteria

- Future development starts from `dev`.
- `main` only receives verified release-ready work.
- Release work is traceable through tags and GitHub Releases.

## Immediate Next Action

Start with Phase 1: check GitHub Actions for `main` and `dev`. If CI is green, proceed directly to Phase 2 and Phase 3 local verification.
