# Configuration Compatibility and Deprecation Policy

CervTerm is pre-1.0, but configuration changes still require an explicit compatibility contract.

## Compatibility Rules

1. Existing valid configuration keeps the same behavior unless a release note identifies a deliberate breaking correction.
2. New fields are optional and default to the behavior from the preceding release.
3. Unknown fields remain actionable errors; they are not silently ignored.
4. A reload candidate is parsed and validated completely before replacing the active config, bindings, or script runtime.
5. A failed reload preserves the last valid runtime and reports source path and field context.
6. Every public field changes the Go schema, Lua loader, Teal declarations, generated template, validation, reload classification, tests, and docs together.
7. Platform-specific fields report unsupported/incompatible capability without terminating when a safe fallback exists.

## Schema Versions and Migrations

Schema versioning begins in parity-roadmap Phase 2 after ADR-0002 is accepted. Until then, the existing unversioned schema is treated as v1.

A future migration must:

- be deterministic and testable with golden input/output fixtures;
- preserve user intent rather than only parse successfully;
- emit a concise explanation for renamed, split, or removed fields;
- never rewrite the source file without an explicit user command;
- retain the previous runtime when migration or validation fails.

## Deprecation Lifecycle

1. Introduce the replacement and document equivalent behavior.
2. Emit a source-located warning for at least one checkpoint release when practical.
3. Keep old and new names mutually exclusive when accepting both would be ambiguous.
4. Remove the old form only in a documented breaking release or schema-version transition.
5. Update `CHANGELOG.md`, templates, examples, migration fixtures, and `docs/parity-support-matrix.json` in the removal PR.

Security fixes may shorten this lifecycle. The release note must explain the threat and the safer replacement.

## Merge Gate

A configuration PR is incomplete if it lacks any applicable schema, Lua, Teal, template, validation, reload, compatibility, test, or documentation update. Reviewers should use this list as a blocking checklist.
