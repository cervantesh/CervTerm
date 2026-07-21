# CervTerm Phase 13 Context — Bounded Image Model and Kitty Graphics

Date: 2026-07-20
Baseline intent: Phase 12 closed at `e1ed32c`; Phase 13 Slice 13.0a starts from clean local `dev` at `65f9000` after mandatory Hermes-only Agentic Stack onboarding. The dirty primary `fix/windows-version-resource-from-tag` worktree at `61ece0e` remains untouched.

Objective: implement a protocol-neutral bounded pane-local image resource/placement model and a default-off direct-data Kitty APC subset while preserving text-only `core.Cell`, mux pane identity, detached render snapshots, owner-thread terminal/mux/GL mutation, and the OpenGL-only renderer direction.

In scope: bounded APC/DCS framing; pane/global budgets and atomic image transactions; primary/history/alternate placement lifecycle; detached image projection/acquisition; bounded direct RGB/RGBA/zlib/PNG Kitty transmit/place/delete/query; context-local GL cache/drawing; strict v2 default-off restart-scoped config; security/fuzz/performance/manual qualification.

Excluded: renderer selection, Sixel, iTerm, animation, filesystem/temp/shared-memory transport, persistent image cache, default-on activation.

Authority: Phase 13 preflight, accepted ADR 0014, Phase 13 guardrails, reviewed feature design, and the independently reviewed implementation plan. Tracked proposed ADR-0006 is superseded in substance by accepted ADR 0014.
