# Phase 15.3 — Recovery and Redaction

Implementation commit: `1a7335a`

## Result

`PASS` on Windows 11 `windows/amd64` with Go 1.25.8.

## Recovery boundaries

- Failed config reload candidates preserve the active config/runtime and keep repair dependencies watched.
- Invalid, future, unsafe, or corrupt layout state becomes a value-free `invalid-or-unavailable` disposition and is non-authoritative, allowing the existing fresh-window path.
- Transient context-local image activation publishes old-or-new, closes provisional caches/mux state in reverse order, retries uploads on the bounded schedule, and closes idempotently in the owning context.
- No persistent image cache or disk format was added.

## Redaction

- Diagnostic setup no longer logs its private output path.
- Setup failures return a fixed typed/staged classification without the OS path or source error value.
- Panic logs classify only the fixed entry context and value class; raw panic values are omitted.
- Stack function names and line numbers remain useful while Windows and Unix source paths become `<source>/<basename>:<line>`.
- User-facing crash text is fixed and value-free.

## Enforced gate

`scripts/check-phase15-recovery.go` first enumerates and verifies every required test name, preventing an unmatched `-run` expression from passing. It then executes logging, layout, config reload, fresh restore fallback, image activation rollback, upload retry, and cache-close suites. `-race` runs the same exact inventory under the race detector. Windows CI executes the focused race gate.

## Verification

- `go run ./scripts/check-phase15-recovery.go` — PASS.
- `go run ./scripts/check-phase15-recovery.go -race` — PASS.
- `go test ./... -count=1` — PASS.
- `go test -tags glfw ./... -count=1` — PASS.
- `go run ./scripts/check-maturity-gates.go` — PASS and requires the CI recovery gate.
- `git diff --check` — PASS.
- Independent security/recovery review — PASS after repairing setup-error leakage, false-positive regex matching, CI execution, fresh fallback coverage, and transient cache retry/close coverage.

## Rollback

Revert the bounded logging formatter and pure restore-load policy together. Existing config, layout and image transactions remain independently safe; no user data migration is involved.
