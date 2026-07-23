# Phase 14 Implementation Plan — Bounded Sixel and iTerm Inline Images

Date: 2026-07-23
Status: **Approved after internal plan challenge**
Base: local `dev` at `be30c58`

## Execution contract

Use a clean dedicated worktree per slice. Validate, independently review, commit and local-merge each slice before starting the next. Never touch the user's dirty `fix/windows-version-resource-from-tag` worktree.

Common gate: focused tests; full untagged/tagged tests and vet; full race plus tagged focused race; maturity/import checks; `git diff --check`; touched fuzz >=60 s once available; relevant portable ten-run baseline with no new disabled allocations/wakes and <=3% unexplained median regression.

## Sequence

| Slice | Deliverable | Depends on |
|---|---|---|
| 14.0 | Authority/import/baseline | Phase 13 |
| 14.1 | Canonical owner anchor only | 14.0 |
| 14.2 | Internal ID partition only | 14.1 |
| 14.3 | Ephemeral lifecycle | 14.2 |
| 14.4 | Sixel DCS transport | 14.0 |
| 14.5 | Selected OSC transport | 14.0 |
| 14.6 | Neutral scheduler + complete Kitty migration | 14.2 |
| 14.7 | Dormant Sixel adapter | 14.4 |
| 14.8 | Dormant Sixel decoder | 14.7,14.6 |
| 14.9 | Shared bounded PNG codec + Kitty qualification | Phase 13 |
| 14.10 | Dormant iTerm adapter | 14.5 |
| 14.11 | Dormant iTerm decoder | 14.9,14.10,14.6 |
| 14.12 | Test-only Sixel mux/runtime | 14.3,14.8 |
| 14.13 | Test-only iTerm + mixed runtime | 14.12,14.11 |
| 14.14 | Dormant public config | 14.13 |
| 14.15 | Transactional production activation | 14.14 |
| 14.16 | Qualification/docs/drift | 14.15 |

## Slice contracts

### 14.0 Authority and baseline
Sync active-project/index/context plus ADR-0016/design/plan/guardrails; add a positive-allowlist Phase 14 import/invariant check; capture checkout-portable text/control/store/GL-disabled ten-run baselines and compare them to Phase 13/fixed frame budgets. Production behavior unchanged.

### 14.1 Canonical anchor
Add one owner-thread image anchor API and route Kitty through it. Test primary history/reflow/resize/alternate and later PTY movement. No ID change.

### 14.2 ID partition
Reserve Kitty low half and internal high half for image/placement IDs; monotonic no-wrap/no-reuse allocation; reject cross-namespace addressing. Test boundary/exhaustion/transfer/restore/close.

### 14.3 Ephemeral lifecycle
Add durable/ephemeral resource retention and atomic final-placement retirement across every core lifecycle path. Fault inject old-or-new publication; Kitty durable behavior unchanged.

### 14.4 Sixel DCS transport
Add exact 7-bit selected DCS preamble/final framing with 16 KiB chunks, 256 KiB frame, ST/CAN/SUB/reset/EOF recovery. Preserve APC/Kitty and nil behavior. Parser fuzz/security review.

### 14.5 Selected OSC transport
Stream exact 7-bit OSC 1337 with BEL/ST and borrowed chunks. Tests assert every selected chunk is <=16 KiB across arbitrary fragmentation and both terminators. Parser retains no payload; adapter/store-owned aggregate limits apply, and existing nonselected 64 KiB OSC stays unchanged. Parser fuzz/security review.

### 14.6 Neutral scheduler and Kitty migration
Generalize job/result/owner contract and move all Kitty scheduling/completion/reply behavior onto it. Keep two workers, one outstanding/pane, queue storage 32, hard pending 8/32 inclusive and owner-driven expiry. Deterministic clock/barrier tests cover queue-inclusive completions immediately before/after 250 ms, late-commit rejection, and prove the pane slot/transfer stays owned until worker return plus owner cleanup. Mixed fake saturation/race tests and complete Kitty requalification.

### 14.7 Dormant Sixel adapter
Implement exact token/state model and sealed transfer in a pure leaf. Fragmentation/truncation/grammar fuzz; no mux production wiring.

### 14.8 Sixel worker
Two-pass checked raster, operation/dimension/pixel/byte bounds, detached palette and immutable candidate. Derive Sixel span exactly as `ceil(W/cellPixelWidth)` by `ceil(H/cellPixelHeight)` with checked `1..256` rejection. Goldens include non-divisible cell sizes, bombs, cancellation, rollback and >=60 s fuzz.

### 14.9 Shared PNG codec
Extract bounded PNG decode from Kitty into `termimage`, then migrate/requalify Kitty byte-for-byte before iTerm uses it. No base64/protocol metadata in codec.

### 14.10 Dormant iTerm adapter
Implement exact metadata and strict-base64 transfer state. Reserve pane/process transfer and encoded bytes before retaining each chunk; exhaustion tests prove zero retained bytes and exact rollback. Reject every external-I/O and broad sizing form. BEL/ST fragmentation/hostile metadata fuzz; no mux production wiring.

### 14.11 iTerm worker
Strict base64, exact size, one bounded PNG+EOF and immutable candidate. Golden formulas: absent axes use intrinsic ceil-to-cell; width-only derives `ceil(Hi*C*cw/(Wi*ch))`; height-only derives `ceil(Wi*R*ch/(Hi*cw))`; all arithmetic checked and spans outside `1..256` reject. Corpus, non-divisible aspect cases, bombs, cancellation, rollback and >=60 s fuzz.

### 14.12 Test-only Sixel mux/runtime
Programmatic option only. Capture anchor/epoch/generation/metrics/palette/IDs, submit shared job, owner revalidate and atomic ephemeral commit. Test later PTY, history/reflow, hidden/transfer/restore/reset/RIS/close/stale/late/race.

### 14.13 Test-only iTerm and mixed runtime
Add selected OSC route and the same owner transaction. Prove all three protocols share pane activity/workers/FIFO/budgets and Phase 14 reserves no reply slot or changes Kitty/DSR/OSC reply order. Emit every fixed Phase 14 failure class through injected diagnostics and assert only protocol/reason/count/duration fields exist—never payload, pixels, metadata names or base64.

### 14.14 Dormant config
Add strict-v2 restart-scoped default-false Sixel/iTerm flags with composition/provenance/unset/diff/template/Lua/Teal/doctor. Frontend still ignores them.

### 14.15 Production activation
Use `imagesEnabled` to create one shared model/scheduler and one distinct cache per GL context, with only enabled adapters. Test all eight flag combinations, two simultaneous fake GL contexts with isolated cache identity/teardown, child/restore/context failures, reverse rollback and all-disabled literal nil. Flags remain default-off.

### 14.16 Qualification and drift
Record exact subset/divergences/rollback, parser/decode/lifecycle/scheduler/race/fuzz/performance/no-I/O evidence, Windows/OpenGL matrix and final ADR drift. Automated close-out replays every failure diagnostic and enforces the exact protocol/reason/count/duration allowlist plus absence of payload/pixels/metadata/base64. Keep experimental/default-off/support none for failed or unrun manual rows.

## Rollback and stop conditions

Operational rollback: disable the affected protocol and restart. Code rollback: activation -> config -> mux routes -> workers/adapters/shared codec -> scheduler -> parser transports -> ephemeral lifecycle -> IDs -> anchor.

Stop on payload leakage, external-I/O need, off-owner mutation, cursor effects, internal-ID replacement, lifecycle/reservation leak, cap multiplication/widening, autonomous timer mutation, hidden support/default-on, `core.Cell` widening, renderer changes, unexplained >3% regression or any gate failure.
