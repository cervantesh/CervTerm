# Phase 14 Slice 14.14 — Dormant Public Graphics Configuration

Date: 2026-07-22
Base: `0ee3ecf`

Published independent strict-v2 `graphics.sixel.enabled` and `graphics.iterm.enabled` intent. Both default to false, inherit restart scope, support source-graph composition and CLI override, reject runtime override, and reuse the existing lower-only shared graphics limits. V1 behavior remains unchanged. Diff, template, Lua/Teal declarations/examples, doctor output and getting-started documentation expose the two fields and their dormant status.

The GLFW frontend deliberately ignores both fields in this slice. With Kitty disabled, either/both configured flags still produce literal-nil image limits, no stores/scheduler/cache/options/deadline, and no activation. With Kitty enabled, only Kitty reaches mux/cache activation. Transactional Sixel/iTerm activation remains deferred to Slice 14.15.

Validation passed:

- focused config/script/doctor/frontend tests;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import, maturity and diff gates;
- independent slice review (Decisions/Cross-slice OK) and peer comparison against every Kitty configuration surface.

Tests cover defaults, strict decode, wrong types, V1 rejection/golden compatibility, includes/environment/profile/CLI precedence, leaf/table unset and provenance, schema CLI/restart/runtime capabilities, lower-only limits, diff schema order/count, template, Teal, doctor and frontend dormancy.

A standalone ten-sample disabled-frame run measured median 7.962 ns/op, 0 B/op and 0 allocs/op. A contemporaneous clean-base `0ee3ecf` run measured 7.756 ns/op; the +2.66% paired difference remains inside the 3% gate with no new allocation or wake. Host-level drift relative to the older artifact affects both worktrees.
