# ADR: Use a typed action model

## Status
Accepted

## Date
2026-07-16

## Context
Bindings currently combine Lua callbacks (`internal/script.Runtime`), GLFW key normalization, and fixed frontend handlers for reload, stats, zoom, scrolling, mux, clipboard, and terminal encoding. Tabs, windows, mouse bindings, command palette, quick select, and launch menu require one discoverable dispatch seam without moving toolkit types into terminal or mux layers.

The current precedence is valuable and must remain explicit: search captures first, reload is reserved, Lua keys override built-ins, and unhandled input reaches the terminal encoder. Lua callbacks execute on the main thread under a one-second watchdog and receive a host bound to the focused or event-origin pane.

## Decision
CervTerm will add a toolkit-neutral `internal/action` package and route every built-in key command through it before adding new command surfaces.

### Action representation
- `Action` is a closed interface implemented by immutable concrete Go value types. Each type exposes a stable lowercase identifier and validates its own typed arguments.
- Initial concrete actions cover copy selection, paste clipboard, open/toggle search, toggle stats, scroll, zoom, reload config, split pane, focus pane, close pane, and ordered action sequences.
- Direction, split axis, scroll unit, and zoom mode are action-package enums, never GLFW or mux enums.
- An `Envelope` pairs an action with a semantic target selector. Initial selectors are `focused` and `origin`; later tab/window selectors may be added without changing action identity.
- Dispatch context carries input source plus opaque target references (`kind` and numeric identity). The frontend resolves them to current mux objects at execution time; serialized config never embeds ephemeral pane IDs.

### Registry and metadata
A registry maps each stable identifier to its descriptor, label, category, argument decoder/encoder, target requirements, and discoverability flags. Duplicate identifiers fail construction. Registry enumeration is deterministic and is the future command-palette source.

Built-in action serialization uses this versionable shape:

```json
{
  "type": "scroll",
  "target": "focused",
  "args": {"unit": "page", "amount": -1}
}
```

Unknown action names, arguments, enum values, or fields are errors with source context. Ordered sequences serialize as an action whose arguments contain validated child envelopes. Sequence execution stops at the first error; successful earlier actions are not rolled back.

### Execution boundary
`internal/action` defines values, validation, metadata, codec contracts, context, and result/error types. It does not import GLFW, OpenGL, the script runtime, or platform clipboard APIs. It does not mutate mux state.

The GLFW application owns an executor that translates action enums to existing app/mux/clipboard/search operations on the main thread. A recognized action consumes its binding even if execution returns an error. Handlers remain responsible for their existing redraw, PTY resize-settlement, and lifecycle invariants.

Target resolution is deterministic: `focused` resolves immediately before each action executes, so a sequence may focus a pane and then act on the newly focused pane; `origin` remains fixed for the whole dispatch. A missing or stale required target returns `ErrTargetUnavailable`, consumes the matched binding, performs no fallback, and emits one notice.

Executor errors carry action ID, error class, and cause. During migration, the frontend preserves established diagnostic classes (`script error:`, `mux:`, `input:`) while guaranteeing at most one notice for one dispatched action. Standardizing user-facing prefixes is a separate compatibility change.

### Lua compatibility
The `cervterm` module exposes an `action` table containing immutable action values and typed constructors, for example `cervterm.action.CopySelection`, `cervterm.action.ScrollPage(-1)`, `cervterm.action.SplitPane("columns")`, and `cervterm.action.Multiple({...})`. Teal declarations mirror the supported constructors.

A `keys` entry has `key`, optional `mods`, `action`, and optional string `label`. `action` may be a typed action value or the existing function. A legacy function becomes a runtime-local callback action identified by binding index. Callback actions:
- remain main-thread-only and watchdog-protected;
- are intentionally not serializable across runtime reloads;
- target the focused pane for key dispatch; terminal-origin event callbacks keep their existing origin-pane host binding;
- are hidden from discovery unless `label` is a non-empty string.

This runtime-only callback exception does not weaken the serialization contract for built-in actions.

### Input precedence
The key pipeline is fixed as:
1. reserved modal activation/control plus active modal capture (Ctrl+Shift+F/search now; future palette/quick-select);
2. reserved safety/config-reload chord;
3. user typed or callback binding;
4. built-in binding table (stats, zoom, scroll, mux, clipboard);
5. terminal clipboard conveniences that depend on selection;
6. toolkit-neutral terminal encoding.

Matching a binding suppresses the corresponding character callback exactly as today. Each descriptor declares separate consume and execute policies for press and repeat. Phase 1 locks current behavior with regression tests before refactoring: legacy callbacks consume press/repeat but execute on press only; modal controls preserve current press/repeat handling; reload and stats preserve their current press-only handling; zoom, scroll, mux, and clipboard preserve their current repeat behavior. Any later normalization is a documented input-compatibility change.

## Consequences
- Keyboard, mouse, palette, scripting, and later tab/window UI can converge on one validated action vocabulary.
- Existing fixed handlers become executor operations incrementally; temporary compatibility wrappers are allowed only inside Phase 1.
- Registry/codec maintenance becomes a cross-layer contract with Lua, Teal, templates, docs, and tests.
- Concrete action types add code, but prevent untyped argument maps from spreading through application logic.
- Runtime Lua callbacks remain less discoverable and non-serializable by design.

## Alternatives Considered
1. Keep independent GLFW handlers and add palette-specific commands. Rejected because behavior, validation, labels, and targeting would drift.
2. Use one `Action` struct with a kind plus every possible argument field. Rejected because invalid field combinations become easy and grow with every feature.
3. Store arbitrary `map[string]any` arguments. Rejected because errors move to execution time and Teal cannot describe the contract precisely.
4. Import mux and GLFW identities into the action package. Rejected to preserve dependency direction and keep serialized actions stable.
5. Remove Lua callbacks in favor of typed actions. Rejected because it breaks the existing scripting API and valid custom behavior.

## Verification Requirements
- Registry tests cover duplicate IDs, deterministic enumeration, labels/categories, and unknown actions.
- Every concrete action has valid/invalid argument and JSON round-trip tests.
- Sequence tests cover ordering, nested validation, and stop-on-first-error.
- Executor tests cover focused-pane targeting, missing/stale targets, error consumption, and redraw/lifecycle behavior.
- Input tests preserve reserved search activation, reload, user, built-in, selection convenience, and PTY precedence; exact emitted bytes; character suppression; and each legacy press/repeat consume/execute policy.
- Lua/Teal tests cover typed actions, constructor errors, legacy callbacks, watchdog behavior, labels, and reload-local callback identity.
- Existing mux, zoom, scroll, clipboard, search, config reload, script, race, and GLFW test suites remain green.

## Implementation Order
1. Add `internal/action` values, registry, metadata, codec, and unit tests without changing input behavior.
2. Add the frontend executor and a built-in binding table; migrate one action family at a time behind compatibility tests.
3. Add Lua action values/constructors and adapt `keys` loading while preserving function callbacks.
4. Collapse the key callback onto the explicit precedence pipeline and remove temporary fixed-handler dispatch wrappers.
5. Update Teal declarations, default template, scripting docs, support matrix, changelog, and validation evidence.

## Reversal Signals
Revisit this decision if concrete actions cannot express required Lua behavior without leaking runtime objects, target resolution cannot remain outside the action package, or measured dispatch/serialization overhead affects the input hot path. Any replacement must retain explicit precedence, typed validation, deterministic discovery, and callback compatibility.
