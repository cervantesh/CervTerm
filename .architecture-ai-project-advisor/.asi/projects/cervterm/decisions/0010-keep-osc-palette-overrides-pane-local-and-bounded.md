# ADR-0010: Keep OSC palette overrides pane-local and bounded

- **Status:** Accepted
- **Date:** 2026-07-17
- **Gate:** Phase 3

## Context

OSC 4/10/11 sequences mutate presentation state and can emit PTY replies. Global config ownership would leak across panes and conflict with live scheme reload; unbounded mutations/replies create resource and side-effect risk.

## Decision

1. OSC palette state belongs to each terminal core instance/pane, never global config, mux, or renderer state.
2. Bound overrides to default foreground/background and indexed slots 0–255.
3. Render precedence is configured palette < pane-local OSC override < explicit truecolor. Reload changes the base while retained OSC state remains until reset or pane exit.
4. Support OSC 4 set/query, OSC 10/11 set/query, and OSC 104/110/111 reset. Ignore invalid, unsupported, malformed, or overlong payloads atomically.
5. Queries return canonical `rgb:RRRR/GGGG/BBBB` through the existing bounded reply path to the originating pane only.
6. Core/parser expose logical palette state without importing GLFW, renderer, config, or mux.
7. Mutations reproject logical scrollback without reparsing; truecolor remains invariant.
8. Dynamic state is ephemeral, not persisted or included in config provenance.

## Verification

- parser terminator/chunk/malformed/size tests;
- set/query/reset tests for ANSI, extended indexed, foreground, and background;
- independent pane isolation;
- reload beneath retained override and reset revealing new base;
- logical reprojection and truecolor invariance;
- bounded replies and silence for malformed/unsupported input.
