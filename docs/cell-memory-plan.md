# Cell Memory + Cache Plan (shrink `core.Cell` 48 → 32 bytes)

## Goal

`core.Cell` is the atom of the grid — held by the live screen, the scrollback
ring, the render snapshot, and every reflow/search temporary, and scanned in
full every frame by `HashRows`/`CopyView`/`drawRow`. Today it is **48 bytes**,
of which **24 (50%) is the `Combining []rune` slice header** — `nil` in ~99.9%
of cells. Shrinking it wins on two axes at once:

- **Memory:** −33% on everything that stores cells (scrollback at 200 cols ×
  2000 rows: 19 MB → 12.7 MB).
- **Cache:** more cells per cache line on the per-frame scan.

## Measurements (premise validated, not assumed)

Synthetic scan of 2000×120 cells (mirrors a full-grid `HashRows` pass):

| Cell size | ns/scan | vs 48 B |
|---|---|---|
| 48 B (current) | 155.3 µs | — |
| 32 B (pointer) | 143.4 µs | −8% |
| 24 B (index)   | 121.1 µs | −22% |

The 32-byte pointer form is the **safe** target (−8% scan, −33% memory) with no
lifecycle machinery. The 24-byte index form is a possible later stretch (§Stretch).

## Representation

```go
type Cell struct {
    noCompare [0]func() // keeps Cell non-comparable (see Trap 5); zero-size fields go FIRST
    combining *[]rune   // nil when the cell has no combining marks (the common case)
    Rune      rune
    Attr      Attr
    WideContinuation bool
}
```

Fields ordered so the zero-size guard and the 8-byte pointer sit first (a
zero-size field at the END of a struct forces padding); total data 26 B →
aligned to **32 B**. `combining` is **unexported** and reached only through
methods (the seam — see Phase A). Value copies of `Cell` copy the pointer
(sharing the backing `[]rune`), so **every mutation copies-on-write** (§Trap 1).

## Two-phase plan (the seam de-risks the representation change)

### Phase A — add the accessor seam (pure refactor, no behavior change)

Introduce methods, **unexport the field in this same phase** (keeping its
`[]rune` type), and replace the **14** direct `.Combining` accesses:

| Method | Replaces |
|---|---|
| `Combining() []rune` | `range cell.Combining`, reads |
| `HasCombining() bool` | `len(cell.Combining) != 0/> 0` |
| `AppendCombining(r rune)` | `t.cells[idx].Combining = append(...)` (parser) |
| `CloneCombining() []rune` | the deep copy in `snapshotScreen` |
| `NewCellWithCombining(r rune, attr Attr, marks ...rune) Cell` | cross-package `Cell{Combining: []rune{...}}` literals |

Unexporting in Phase A (not B) makes the seam **compiler-enforced**: a straggler
or future direct access outside `core` fails to build instead of silently
bypassing the seam. Code inside `core` (the parser hot path) may still touch the
field directly. Scope note: three tests in other packages build
`Cell{Combining: ...}` literals (`cluster_test.go`, `damage_test.go`) — they
move to the constructor; ~4 more test reads move to the accessors.

Behavior is identical; the full suite stays green. Phase A ships on its own and
is independently valuable: after it, the representation lives behind 5 methods,
so **any** future change to it touches those, not 14 call sites. This is the
payoff that turns "risky to change" into "cheap to change."

### Phase B — swap the representation behind the seam

Change the field to `combining *[]rune`, implement copy-on-write in
`AppendCombining`, and adjust the 4 accessors. Only `Cell` + its methods change;
the 14 call sites from Phase A are untouched. Verify with the memory + cache
benchmark and the emoji/accent E2E.

## Review traps

1. **Copy-on-write is mandatory *in Phase B*.** In Phase B `combining` is
   `*[]rune`, so a value copy of a `Cell` shares the *pointer* — a live
   `AppendCombining` via `*ptr = append(*ptr, r)` would mutate an already-taken
   snapshot. `AppendCombining` must allocate a fresh slice+pointer per append.
   The only in-place mutator is the parser (verified: single write site).
   *Phase A note:* while the field is a value `[]rune`, this is **not
   observable** — value copies get independent headers, and appends only write
   the suffix that a shorter snapshot view never sees. So COW cannot be
   distinguished from in-place append by any test until Phase B. The pair review
   (gpt-5.6-sol) confirmed a naïve Phase-A "COW test" is vacuous; the real
   distinguishing test (shared-pointer corruption / `unsafe.SliceData` identity)
   is a **Phase B deliverable**.
2. **Shallow snapshots.** `CopyView`/`render.Capture` copy cells by value every
   frame *without* deep-copying combining. Phase A pins the observable invariant
   (`TestCombiningSnapshotFrozenAfterAppend`: capture a view, add a mark to the
   live cell, assert the snapshot is unchanged — passes today, and **fails in
   Phase B if COW is forgotten**). This test is the Phase B tripwire.
3. **`snapshotScreen` deep copy** (`append([]rune(nil), ...)`) must go through
   `CloneCombining` so the alt-screen snapshot never aliases live slices.
4. **Damage hashing** (`render/damage.go`) must hash the same bytes as before
   (length + runes) via the accessor, or partial redraws desync.
5. **Keep `Cell` non-comparable.** Today slices make it non-comparable, so no
   `==` on cells can exist (it wouldn't compile). A pointer field would make it
   comparable for the first time — enabling future `==` that compares combining
   *pointer identity* instead of content. The zero-size `noCompare [0]func()`
   field preserves today's semantics at zero bytes.
6. **Blank-cell detection** (`screen.go` trim/erase) uses `HasCombining()`.
7. **Wide/cluster paths** (`cluster.go`, `runs.go`) read combining to decide
   shaping — route through `HasCombining()`/`Combining()`.
8. **`sizeof(Cell)` guard:** an `unsafe.Sizeof` test pins the size at ≤32 so a
   future field-reorder (or misplacing the zero-size field last, which forces
   padding) can't silently re-bloat it.

## Tests

- **Phase A (shipped):** `Combining()/HasCombining()/CloneCombining()` +
  constructor round-trip; `TestCombiningSnapshotFrozenAfterAppend` (real
  CopyView path — the Phase B COW tripwire).
- **Phase B:** `unsafe.Sizeof(Cell) == 32`; the **distinguishing** COW test —
  after a value copy, `AppendCombining` twice and assert the snapshot is frozen
  AND the two cells share no backing (`unsafe.SliceData` identity); the same via
  the real CopyView path. These are only observable once `combining` is a
  pointer.
- **Existing suites** (must stay green): core screen/terminal/reflow/search,
  fontglyph color/cluster, render damage, selection.
- **Benchmarks:** `HashRows` and `CopyView` over a real grid, before/after, to
  confirm the ~8% scan win materializes on the actual code (not just the
  synthetic scan).
- **E2E:** render an accented word (`áéíñü`) and a ZWJ/flag emoji sequence; both
  draw identically to today. Heap after fill drops ~33% on the cell-held
  structures.

## Stretch (separate, later): 24-byte index form

Replace the pointer with a `uint32` index into a small combining-run pool
(`Cell` → 24 B, −50% memory, −22% scan). Deferred because the pool needs
lifecycle care (indices must survive scroll into scrollback; a naive
append-only pool leaks over a session). Revisit only if Phase B lands cleanly
and the extra 14% scan win is wanted.

## Flow

- **Phase A** (`feat/cell-accessor-seam` off main): mechanical seam refactor —
  implementer **Codex (gpt-5.6-luna)**; review **Opus** + Fable; full gates.
  Ships alone.
- **Phase B** (`perf/cell-shrink` stacked on A): representation + copy-on-write —
  implementer **Codex (gpt-5.6-sol)** or **Opus** (correctness-sensitive);
  review the *other* engine + Fable traps checklist + E2E + benchmark.
- Release after B. Stretch (24-byte) only on explicit follow-up.
