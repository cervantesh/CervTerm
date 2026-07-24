# ADR: Generate closed configuration and action authorities

## Status

Accepted

## Date

2026-07-23

## Relationship

Amends ADR-0002 and ADR-0003 only for closed-vocabulary authority. Their v1 permissive configuration decoder, legacy action callbacks, compatibility behavior and precedence remain authoritative until separately retired.

## Context

Configuration leaves, action kinds, Teal declarations, scopes, decoders, diagnostics and runtime setters can drift when represented by repeated strings and handwritten switches.

## Decision

Define one declarative catalog for each closed vocabulary. Generation emits typed Go identifiers, schema metadata, **strict v2-only** decoding, Teal/public declarations, scope/provenance tables and parity tests. V1 continues through the behaviorally frozen permissive decoder, and legacy action callbacks retain ADR-0003 compatibility. Generated output is deterministic and checked in. Runtime extension points remain explicit typed interfaces; unknown strict-v2 catalog values fail closed. Generated code does not own runtime state.

## Consequences

Adding a closed value changes one catalog and regenerated artifacts. Build gates reject stale output, duplicates and switch drift.

## Rejected alternatives

Stringly typed maps, reflection over runtime structs, or independent handwritten authorities.

## Rollback

Revert consumers before generated output/catalog. Previously accepted config/action syntax remains readable during migration.
