# ADR: Version configuration composition and provenance

## Status
Proposed

## Date
2026-07-16

## Context
CervTerm currently loads one Lua/Teal configuration and swaps validated reload state atomically. Parity work needs includes, profiles, CLI/runtime overrides, migrations, dependency watching, and diagnostics without making precedence implicit or allowing a failed source to replace the last valid runtime.

## Decision to Make
Choose the versioning, merge, provenance, path-security, and reload model before Phase 2 implementation.

## Candidate Direction
`defaults < includes < primary config < selected profile/environment < CLI < per-window runtime override`, with canonical local paths, cycle detection, field-level provenance, explicit schema migrations, and full candidate validation before swap.

## Constraints
- Existing configurations retain behavior or receive actionable migration errors.
- Remote includes are out of scope.
- Unknown fields remain errors.
- Go schema, Lua mapping, Teal types, template, validation, and docs change together.
- Reload failure preserves the previous config, bindings, and runtime.

## Evidence Required for Acceptance
Precedence and cycle examples, migration golden strategy, path trust analysis, source-location diagnostics, and live-versus-restart diff semantics.

## Reversal Signals
A simpler composition model meets the same provenance/reload guarantees, or Lua module semantics make field-level merging unsafe.
