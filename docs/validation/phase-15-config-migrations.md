# Phase 15.2 — Real-User Configuration Migration

Implementation commit: `5978c5a`

## Result

`PASS` on Windows 11 `windows/amd64` with Go 1.25.8.

## Corpus

- `cervterm-v1-daily-driver`: sanitized owner-provided CervTerm v1 configuration with exact effective v2 equivalence.
- `wezterm-daily-driver`: sanitized owner-provided WezTerm configuration with explicit supported-value golden and a manifest of unsupported/excluded surfaces.

Every corpus file is scanned for private path, username, script-host and secret sentinels. The exact case IDs and source kinds are mandatory.

## Safety and semantics

- Loading the copied v1 source and paired v2 template produces deeply equal effective CervTerm configuration.
- The loader preserves the complete source-directory inventory, every file byte, and file mode; a generated migration neighbor fails the test.
- The WezTerm source is never executed by CervTerm.
- The WezTerm v2 translation is checked against an explicit golden covering initial grid, decorations/titlebar, per-side padding, all three opacity controls, font/fallback/line height, colors, scrollback/scrollbar, tab bar, cursor, bell and FPS.
- Renderer selection, child-process clipboard callbacks, update/status callbacks and side-effectful mouse behavior are documented as non-equivalent rather than silently dropped.

## Verification

- `go test ./internal/config -run '^TestUserMigrationCorpus$' -count=1` — PASS.
- `go test ./internal/config ./internal/script -count=1` — PASS.
- `go test -race ./internal/config ./internal/script -count=1` — PASS.
- `go test ./... -count=1` — PASS.
- `go test -tags glfw ./... -count=1` — PASS.
- `go run ./scripts/check-maturity-gates.go` — PASS.
- `git diff --check` — PASS.
- Independent migration/privacy review — PASS after adding the explicit cross-terminal golden, corpus-wide sanitization, full directory immutability, exact case inventory, and complete opacity assertions.

## User contract

CervTerm never rewrites either source. The published guide requires creating and reviewing a separate v2 file with doctor before the user replaces an active configuration.
