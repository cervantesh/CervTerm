# CervTerm VT Compatibility Checklist

This checklist turns the roadmap's vttest item into a repeatable manual/automated target. Mark an item only after it has either a unit/golden test or a recorded manual result.

## Baseline screen behavior

- [x] Printable ASCII and UTF-8 input.
- [x] CR, LF, BS, TAB controls.
- [x] Autowrap on/off (`CSI ?7 h/l`).
- [x] Resize reflow of wrapped logical lines.
- [x] Alternate screen enter/exit (`CSI ?1049 h/l`).

## Cursor movement and modes

- [x] CUP/HVP (`CSI H`, `CSI f`).
- [x] CUU/CUD/CUF/CUB (`CSI A/B/C/D`).
- [x] CNL/CPL/CHA/VPA/SU/SD (`CSI E/F/G/d/S/T`).
- [x] Save/restore cursor (`ESC 7/8`, `CSI s/u`).
- [x] Cursor visibility (`CSI ?25 h/l`).
- [x] Application cursor mode (`CSI ?1 h/l`).
- [x] Application keypad mode (`ESC =`, `ESC >`).

## Erase and editing

- [x] Erase in display (`CSI J`, including `3J` scrollback clear).
- [x] Erase in line (`CSI K`).
- [x] Scroll regions (`CSI t;b r`).
- [x] Insert/delete characters (`CSI @`, `CSI P`).
- [x] Insert/delete lines (`CSI L`, `CSI M`).

## Colors and attributes

- [x] SGR reset/default colors.
- [x] ANSI 8/16 colors.
- [x] 256-color foreground/background.
- [x] Truecolor foreground/background.
- [x] Additional SGR attributes beyond bold (underline, inverse, italic, strikethrough).

## Input and mouse

- [x] Navigation keys and F1-F12 encoding.
- [x] Ctrl/Alt/Shift modified navigation/function keys.
- [x] Bracketed paste.
- [x] SGR mouse press/release/wheel/drag encoding.
- [x] Mouse modifier encoding for press/wheel/drag.
- [ ] Non-SGR legacy mouse encodings if required by target apps.

## Unicode and rendering

- [x] CJK width-2 cells.
- [x] Combining mark storage and renderer cluster path.
- [x] NFC composition attempt for canonical combining clusters.
- [x] Emoji variation selector/modifier grouping in renderer clusters.
- [ ] Full GSUB/GPOS shaping for complex scripts and fully-qualified ZWJ emoji glyph substitution.

## Regression harness

- [x] Unit tests for parser/core/input behavior.
- [x] Parser fuzz smoke test.
- [x] Replay-style VT golden fixture under `internal/vt/testdata/`.
- [x] Capture/replay documentation for adding vttest-style raw fixtures.
- [ ] Capture real vttest sessions as raw fixtures with an authoritative PTY recorder.
