# Daily-Driver Features Plan: cwd tracking, scrollback search, status segments

Three stacked slices, sequential (all touch the script API surface; search
and segments both touch the HUD/draw path).

---

## Slice 1 — OSC 7 cwd tracking   [Codex, small]

Shells emit `OSC 7 ; file://host/path BEL/ST` on every prompt (PowerShell via
profile, bash via PROMPT_COMMAND — document both). CervTerm ignores it today.

### Changes

- **vt parser**: handle OSC 7 — parse the `file://` URI: strip scheme+host,
  percent-decode, convert `/C:/Users/x` → `C:\Users\x` on Windows-style
  drive paths (keep forward-slash paths as-is otherwise). Empty/invalid URI
  → ignore (do not clear the last known cwd).
- **core**: `Terminal.SetCwd/Cwd` + monotonic `CwdSeq` (mirror Title's
  change-detection pattern), included in `render.Snapshot`.
- **frontend**: `processTermEvents` compares like title → fires new script
  event `events.cwd = function(term, dir)`.
- **script**: `term:cwd(): string` Host method + binding; `.d.tl` + docs
  (with the PowerShell profile snippet + bash PROMPT_COMMAND snippet).

### Traps
1. Percent-decoding (`%20` etc.) and UTF-8 paths.
2. `file://wsl$/...`/UNC hosts: if host is neither empty nor `localhost`,
   keep the raw path form (`\\host\path`) rather than guessing.
3. OSC terminator handling must accept both BEL and ST (match existing OSC
   0/2 code).
4. Snapshot/capture cost unchanged (string copy only when changed).

### Tests
Parser cases (BEL/ST, percent-encoding, drive path, UNC, invalid), core
seq/state, script event fire, term:cwd binding via fake host.

### E2E
PowerShell profile snippet → `cd` around → `events.cwd` notifies the dir;
`term:cwd()` returns it from a keybinding.

---

## Slice 2 — Scrollback search   [Opus, big]

`ctrl+shift+f` opens a search bar (bottom overlay, HUD-style); type a query;
Enter/n jumps to next match (upward), shift+n previous; Esc closes. Matches
highlighted; viewport jumps to the match row.

### Design

- **core**: `SearchBackward(query string, fromGlobalRow int) (globalRow, col
  int, ok bool)` over scrollback+screen cells (case-insensitive; plain
  substring v1 — no regex). Pure and unit-testable: operate on the same
  physical-row view Resize uses. Matches can span wrapped row boundaries
  within one logical line (v1: within a single physical row only — document
  the limitation).
- **frontend**: modal search state on App (`searching bool`, `query
  []rune`, `matches`, `current`). While searching, key/char callbacks feed
  the query instead of the PTY (Esc exits, Enter = next, Backspace edits).
  Draw: overlay bar at the bottom (reuses HUD drawing helpers) + highlight
  fill on the current match cells (selection-style fillRect under the
  glyphs) + viewport jump via existing ScrollViewport w/ offset math.
  Damage: searching forces full-frame (overlay + highlights are global
  state — add to the prepareDamage global list).
- **script**: `term:search(query): boolean` jumps to first match
  (scriptable search); the interactive UI is frontend-only.
- **config**: `keys` — search hotkey fixed `ctrl+shift+f` v1 (configurable
  later; note in docs).

### Traps
1. Input routing: while the bar is open NOTHING reaches the PTY (incl.
   ctrl+c) except the search keys; closing restores exactly.
2. Highlight coordinates are viewport-relative; matches above the viewport
   need offset math against DisplayOffset (reuse the global-row convention
   from Resize).
3. Damage: search mode in the global-fallback list (like selection).
4. Query editing is rune-based (no byte slicing of UTF-8).
5. Empty query = no-op, never a full-buffer match storm.

### Tests
Core search table tests (case, misses, from-row semantics, wrapped rows
documented-limitation, unicode). Frontend: gates only; E2E interactive.

### E2E
Fill scrollback (100 lines), ctrl+shift+f, type a unique token, Enter →
viewport jumps and highlights; Esc restores; typing reaches the shell again.

---

## Slice 3 — Scriptable status segments   [Codex]

Persistent, script-owned status line (separate from the debug HUD): scripts
register segments; CervTerm renders them in a one-row bar at the top-right
edge (over the terminal, HUD-style translucent) only when at least one
segment exists.

### Design

- **script**: `cervterm.status(id: string, text: string)` — set/replace a
  segment (empty text removes it); segments render right-aligned in
  registration order, ` · `-separated. Main-thread only (handlers/timers).
- **frontend**: segments stored on App (ordered map); `statusLine()` string
  cached like hudLines (rebuild only on change); drawn by drawHUD's helper
  as a third band, independent of showStats. Damage: status-change marks
  needsRedraw; visible status does NOT force full-frame every frame — it
  forces full-frame only on frames where it changed (add `statusChanged`
  to the damage-global list, not `statusVisible`). Rationale: unlike the
  live HUD numbers, segments only change when a script sets them.
- Combine with timers: `cervterm.every(1000, ...)` + `cervterm.status(...)`
  = live clock/git-branch/battery segments — the worked example in docs.

### Traps
1. Damage correctness: segment text changes must repaint the band's rows —
   simplest correct v1: status change → full redraw that frame (like
   notice); steady frames keep row damage.
2. The band overlays the top rows — scrolled content beneath must repaint
   when the band shrinks/disappears (band geometry in the damage state).
3. Watchdog: status called from a failing handler — no special handling
   (existing notice path covers it).
4. Cap total segments length to the window width (truncate with …).

### Tests
Script-layer: register/replace/remove ordering via fake host. Frontend:
gates; E2E visual.

### E2E
Config with `every(1000)` + `status("clock", os.date(...))` → clock segment
top-right ticking while idle; removing (empty text) clears the band.

---

## Flow

1. `feat/osc7-cwd` off main → Codex → Fable review+E2E+PR+merge.
2. `feat/scrollback-search` off updated main → Opus → Fable review+E2E+PR+merge.
3. `feat/status-segments` off updated main → Codex → Fable review+E2E+PR+merge.
4. Release `v0.4.1-beta.1` (or v0.5.0 if search lands as headline).
