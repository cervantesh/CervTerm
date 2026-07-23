# Phase 11.10 — Native IME publication decision and close-out

## Decision

**Retain `ime.enabled=false`.** Native Windows IME integration is published only as an experimental, restart-scoped opt-in.

The conditional default-on gate was not met: the qualification host had no Japanese, Chinese, or Korean IME installed, so every required real-IME row is `SKIP`. Automated tests and startup smoke validate architecture, ownership, bounded parsing, fallback and lifecycle behavior, but cannot substitute for real preedit, candidate placement and commit behavior.

## Published support boundary

- Windows/amd64 users may opt in with a strict v2 configuration and restart.
- Disabled, unsupported and failed-install paths retain the existing GLFW character input.
- No macOS or Linux native IME claim is made.
- No J/C/K manual qualification claim is made.
- A future default-on proposal requires a new qualification record with all required J/C/K rows marked `PASS`, followed by architecture drift review and an explicit default-policy change.

## Close-out evidence

- Slices 11.1–11.8 are merged locally with ordinary, tagged, race, vet and maturity evidence.
- Slice 11.9 records Windows build/toolchain/input-method facts and enabled startup smoke.
- `TestIMEConfigDefaultsStrictRestartAndComposition` locks the default to `false`.
- The default Lua/Teal examples emit `ime.enabled=false`.
- Doctor reports configured intent and platform capability without claiming active runtime state.
- Architecture drift review confirms native state remains projection-local and no preedit enters mux/core/PTY.

## Rollback

No migration or persisted state exists. Leave `ime.enabled` absent or set it to `false`, then restart CervTerm. Reverting the activation slice removes native installation while preserving the Phase 11 pure state, rendering and decoder seams.
