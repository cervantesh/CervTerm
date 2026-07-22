# CervTerm Phase 14 Context — Bounded Sixel and iTerm Inline Images

Date: 2026-07-23
Baseline: Phase 13 closed at local `dev` commit `be30c58`. Slice 14.0 uses a clean dedicated worktree. The user's dirty primary `fix/windows-version-resource-from-tag` worktree remains untouched.

Objective: add independently enabled experimental static Sixel DCS and iTerm OSC 1337 direct-inline PNG adapters over the accepted Phase 13 image model while preserving text-only `core.Cell`, pane identity, detached snapshots, owner-thread terminal/mux publication, context-local OpenGL ownership and renderer-selection exclusion.

In scope: canonical anchors; internal ID partition; ephemeral final-placement retirement; exact selected DCS and streaming selected OSC; pure bounded protocol adapters/workers; one shared scheduler; strict-v2 restart-scoped default-off config; transactional any-protocol activation; security/fuzz/performance/manual qualification.

Excluded: C1 control forms, cursor effects, protocol replies, external file/path/URL/download/write modes, animation, broad protocol formats, renderer/backend selection, stable support claims before qualification.

Authority: ADR-0014 and accepted ADR-0016; Phase 14 preflight/guardrails; independently verified feature design and implementation plan.
