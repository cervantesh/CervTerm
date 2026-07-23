# Architecture Preflight — Phase 14 Bounded Sixel and iTerm Inline Images

Date: 2026-07-23
Decision: **PROCEED ONLY THROUGH ADR 0016, DESIGN, PLAN AND SEQUENTIAL GATES**
Risk: **High** (untrusted raster/base64/PNG input, parser recovery, CPU/memory bounds, asynchronous ownership)

## Smallest safe scope

- Static Sixel DCS `q`: exact raster declaration, RGB palette, repeats, carriage/new-band commands; 256 KiB wire frame.
- iTerm `OSC 1337;File=`: inline=1 direct strict-base64 PNG, exact size, at most one cell dimension; no external I/O.
- Cursor-neutral placement and no Phase 14 replies.
- Phase 13 owner/store/budgets/lifecycle/snapshots/GL cache only; no `core.Cell` or renderer change.
- Independent restart-scoped default-off flags.

## Required architecture changes

1. Canonical owner anchor and collision-proof internal ID namespace.
2. Atomic ephemeral final-placement resource retirement.
3. Exact selected DCS and streaming selected OSC parser seams with recovery tests.
4. Protocol-neutral scheduler with complete Kitty migration before new jobs.
5. Pure Sixel and iTerm leaf packages plus one shared bounded PNG codec.
6. Test-only mux routing before public config, and dormant config before atomic production activation.

## Risks and controls

- Raster/PNG bombs: checked dimensions, pixels, bytes, repeats and operation count before growth/allocation.
- CPU denial: two shared workers, one outstanding job/pane, bounded queue and late-commit rejection; no preemption claim.
- Parser leakage: discard through BEL/ST/CAN/SUB/EOF/reset with fuzz at every split.
- Resource leak: ephemeral resource retires atomically with its final placement.
- ID collision: Kitty low-half and internal high-half partition with exhaustion tests.
- External effects: import/static gate forbids filesystem, process, network and unsafe leaves.
- GL/model safety: workers return candidates only; owner commits; GL remains context-local.

## Stop conditions

External file/path/URL/write access; unchecked arithmetic; off-owner model/GL mutation; delayed cursor effects; payload logging/leakage; internal-ID replacement; resource/reservation leak; multiplied/widened hard caps; `core.Cell` widening; renderer selection; default-on or implicit advertisement.

## Required gates

ADR-0016, feature design, implementation plan, independent design/plan verification, portable baseline, parser/adapter/decoder fuzz, lifecycle and mixed-scheduler race tests, all-disabled nil benchmarks, activation rollback review, final security/drift review and honest manual qualification.
