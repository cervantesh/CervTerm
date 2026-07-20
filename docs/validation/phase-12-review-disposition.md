# Phase 12 independent review disposition

Date: 2026-07-20

## Activation review

Independent implementation, coverage and adversarial reviews examined the Phase 12.9 diff before `b73d02d`/`5794064`. Findings and resolutions:

| Finding | Resolution evidence |
| --- | --- |
| Hidden workspace/native-window provider could retain text | Workspace transitions synchronously publish root-only documents; native visibility/iconification participates in capture; privacy tests cover both. |
| Refresh failure retained a stale registered provider | Runtime capture/publication/event failures now close/disconnect the per-window lifecycle and clear runtime activation. |
| Viewport, selection, search, modal, IME and geometry changes could leave stale documents | Every mutation path now issues semantic invalidation; action/Lua scrolling and semantic selection received dedicated fixes. |
| `PaneDirty` caused false events and dormant scheduler was bypassed | Production dispatch uses the Phase 12 intent classifier/scheduler; repaint-only events have a focused zero-invalidation test. |
| Native events lacked listener gating and event-specific APIs | `UiaClientsAreListening` gates native calls; text, selection/caret, focus, structure and fixed metadata-only notification APIs are distinct. |
| Bell/notification events bypassed coalescing or leaked while hidden | Announcements queue into the projection cycle, coalesce by kind, carry fixed labels only, and are suppressed for root-only hidden documents. |
| Config evaluation/state mutation proof and 386 evidence absent | Filesystem no-mutation test added. Windows 386 link was attempted and recorded SKIP due x64-only MinGW; no 386 claim is made. |

All automated gates and focused repairs passed before merge. Real assistive-technology rows remain SKIP, so review did not authorize a support/default-on claim.

## Close-out review

Independent code and coverage review then challenged the first Phase 12.10 draft. Findings and final disposition:

| Finding | Resolution evidence |
| --- | --- |
| Candidate tree was not immutable | Focused benchmarks are committed at `1dfded5`, the synchronized fingerprinted process-evidence harness at `e091a84`, and production activation remains `5794064`. |
| Accessibility benchmarks lacked baseline comparison | Identical fixtures ran three times at `820c389` and `1dfded5`; raw outputs, medians and deltas are tracked. |
| Existing parser/core/render reference appeared regressed | The baseline tool was rerun from clean detached worktrees at both revisions on the same host/toolchain. All median deltas were 1.47–6.92%, below the 15% threshold; allocations were unchanged. |
| No production heap/frame/wake evidence | The `accessibilitymetrics` build-tag probe starts from a script signal after native-window readiness/warmup and records an exact three-second interval plus post-GC heap. Three-run medians are reported for baseline, disabled and enabled. |
| Process measurement was not reproducible | `scripts/measure-accessibility-process.ps1` records executable/config paths and SHA-256 fingerprints in tracked raw CSV; the report includes exact build/run commands and configs. |
| Unicode fixture was not production-shaped | Fixture now uses attached combining/ZWJ clusters and wide-cell continuations. |
| Event benchmark measured one transition, not coalescing | Each cycle now queues a two-transition burst before one drain. |
| Only allocation counts were bounded | Tests enforce both bytes/op and allocations/op ceilings. |
| Provider benchmark bypassed COM ownership | Primary result now calls the native COM callback path and clears its BSTR VARIANT; helper result remains diagnostic only. |
| Manual matrix omitted DPI/reflow/event-rate/shutdown | Explicit Narrator/NVDA SKIP rows were added; SKIP remains a non-claim. |
| ADR/review evidence was not auditable | Accepted ADR 0013 and its close-out evidence are tracked at `.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0013-expose-bounded-accessibility-snapshots-through-a-projection-owned-native-capability.md`, mirrored in active T50 state, and indexed with this repository disposition. |

## Final constraint

Phase 12 may close only as **partial, Windows-only, experimental and default-off**. A supported/default-on decision requires a new approved slice and real Narrator/NVDA PASS evidence.
