# ADR: Version configuration composition and provenance

## Status
Accepted

## Date
2026-07-17

## Context

CervTerm currently evaluates one selected Lua or Teal file into a persistent gopher-lua runtime, validates the resulting Go configuration, polls that source by content hash, and applies a validated reload on the GLFW main thread. It already preserves the active runtime when evaluation or validation fails.

Phase 2 adds includes, named profiles and environments, CLI and per-window overrides, migrations, dependency-aware reload, provenance, and diagnostics. These features must not make precedence implicit, copy Lua functions between states, weaken the last-known-good reload guarantee, or create a false security boundary around trusted local Lua.

The current unversioned Lua loader is permissive: absent or mistyped fields fall back to the base configuration and unknown fields are ignored. Existing v1 files must retain that behavior during the compatibility window. Strictness is introduced by explicit opt-in to v2 rather than retroactively changing v1.

## Decision

### Versioned documents and compatibility

The current unversioned shape is schema v1. Omitted `config_version` means v1. An explicit discriminator must be a finite integral Lua number and exactly one supported version (`1` or `2` initially). Strings, booleans, fractions, NaN/Inf, zero/negative values, versions below the oldest supported migration, and future versions all fail at `config_version` with an actionable diagnostic; future versions specifically say “requires a newer CervTerm.” The discriminator is the sole root key interpreted strictly even by the v1 compatibility path, a deliberate reserved-field exception to historical unknown-key permissiveness.

V1 continues through the legacy permissive decoder so existing files, callbacks, event handlers, and shorthand behave identically. Doctor may warn about unknown or mistyped v1 fields, but loading does not newly fail. V2 is strict: unknown keys, wrong types, sparse arrays, non-integral integers, invalid enum values, and functions outside declared callback/action slots reject the candidate.

Migration is an ordered in-memory pipeline (`v1 -> v2 -> ...`) over presence-aware raw documents. It never rewrites user files. Golden fixtures prove semantic equivalence. V2-only composition metadata in a v1 primary file is an error that instructs the user to add `config_version = 2`.

A v2 primary may include an unversioned/v1 partial. That source is decoded with v1 compatibility semantics, migrated in memory, and contributes only fields it actually supplied; defaults are not materialized per source.

“V1 compatibility” includes the exact current decoder quirks: one Lua evaluation per load, unknown ordinary config keys ignored, mistyped scalar fields ignored, numeric integer fields truncated with Go’s current conversion, and invalid entries in permissive legacy collections such as `shell.args`/`shell.env` filtered individually. Existing strict scripting surfaces remain strict: malformed `keys` entries/actions and malformed `events` handlers fail the load. Slice 1 must inventory both permissive and fail-fast behaviors as golden fixtures before adding the raw decoder. Until the slice that implements a v2 composition feature lands, slice 1 recognizes its reserved key but rejects it with an actionable “not available in this Phase 2 slice” error; no v2 field is accepted inertly.

### V2 document shape

Composition metadata is reserved at the root:

```lua
return {
  config_version = 2,
  includes = { "base.lua", "platform/windows.tl" },
  default_environment = "windows",
  default_profile = "work",
  environments = {
    windows = { window = { opacity = 0.92 } },
  },
  profiles = {
    work = { shell = { working_directory = "C:/work" } },
  },
  -- ordinary configuration fields remain at the root
  font = { family = "JetBrainsMono Nerd Font", size = 14 },
}
```

Named profile/environment values are strict partial configuration documents. They may contain bindings and event callbacks, but not `config_version`, `includes`, selection defaults, nested profiles, or nested environments.

### Includes, evaluation, and source identity

Includes are local declarative documents. Relative paths resolve against the declaring source; absolute local paths are allowed. URL-like paths, directories, devices, and other non-regular files are rejected. Remote includes are out of scope.

Traversal is deterministic depth-first post-order. For `primary -> [A, B]` and `A -> [C]`, low-to-high source precedence is `C < A < B < primary`. A canonical source is evaluated and merged once at first encounter; later diamond edges are recorded but do not reapply it. Encountering a canonical source already on the active traversal stack is a cycle, and the diagnostic prints the requested and canonical path chain.

All declarative sources execute in one candidate Lua state so retained functions never cross runtimes. While an include is being evaluated, an include guard remains active through every nested `require`, `dofile`, and `loadfile` execution. Under that guard, imperative CervTerm registrations such as timers, status entries, and overlays are rejected; included code and its modules may only compute and return values. Explicit local Lua modules loaded while evaluating the primary remain the supported imperative reuse mechanism and may register against the candidate runtime. Primary evaluation necessarily occurs first to discover its include list; those candidate-only module side effects therefore precede include traversal and are explicitly outside merge precedence. Resolved local source files become dependencies, but module return values are not merged unless a declarative include returns them.

Source identity uses absolute cleaned paths, resolved symlinks, platform-correct case/volume normalization, and filesystem identity where available. Symlinks may escape the primary directory because configuration Lua is already trusted local code with filesystem/process capabilities; diagnostics always show requested and canonical paths. Identity is rechecked before commit so a symlink retarget or concurrent edit queues another candidate.

Operational limits prevent accidental explosions, not hostile trusted code:

- include depth: 16;
- distinct declarative sources: 64;
- one declarative source: 1 MiB;
- aggregate declarative bytes: 8 MiB;
- merged nodes/list entries: 100,000.

### Validation and deterministic merge

Each source receives presence-aware structural validation before merge. V2 validates recognized keys and exact value shapes; v1 records compatible known fields and diagnostics without changing legacy load behavior. Cross-field and range validation runs on the final composed candidate.

Merge is schema-driven:

- records recursively merge by recognized field;
- maps such as `shell.env` merge by key;
- lists such as `shell.args`, `keys`, and future font fallback lists replace entirely;
- scalars, typed actions, and functions replace;
- `nil` means “not supplied”;
- immutable v2-only `cervterm.config.unset` creates a tombstone: for a record field it suppresses lower user layers and restores the built-in default when one exists (otherwise the field is absent and final validation decides); for a map key it suppresses the lower key and removes it; any higher layer may set the path again and clear the tombstone.

There is no implicit list concatenation or callback chaining. Users who want combined lists construct them explicitly in Lua or a required local module. Profile and environment declarations with the same name merge by the same rules across sources; selection defaults are ordinary composition metadata and later traversal sources win, so the primary wins over included defaults.

### Selection and precedence

Environment and profile selection are independent and deterministic.

Environment selection is: `--environment`, then `CERVTERM_ENV`, then `default_environment`, then `runtime.GOOS` only when that exact environment exists, otherwise none. Profile selection is: `--profile`, then `CERVTERM_PROFILE`, then `default_profile`, otherwise none. An explicitly selected missing name is an error; automatic `runtime.GOOS` fallback is skipped silently when absent.

Exact low-to-high precedence is:

1. built-in defaults;
2. transitive includes in traversal order;
3. primary ordinary fields;
4. selected environment;
5. selected profile;
6. typed CLI overrides;
7. scoped runtime overrides (the current app scope, mapped per window only after ADR-0004).

Environment describes host context; profile is the user’s more specific intent and therefore wins. No layer depends on Lua map iteration or process-environment enumeration order. Repeated CLI overrides apply left-to-right with the last occurrence winning. Runtime setters apply in successful commit order with the last successful transaction for a field winning.

### Typed CLI and runtime overrides

CLI adds repeatable typed overrides, for example:

```text
--config-override window.opacity=0.85
--config-override scrolling.history=4000
--config-override shell.args='["pwsh","-NoLogo"]'
```

Paths resolve against schema metadata. Values use JSON scalar/array/object syntax; an unquoted value is accepted only for a schema-known string. The same decoder performs coercion, unknown-path checks, and validation for Lua, CLI, and runtime patches. CLI values for fields marked sensitive, including `shell.env`, are rejected because process argument lists are observable.

Runtime setters create a typed patch owned by an opaque process-local `ConfigScopeID`; they do not mutate the composed base. The current single-window frontend allocates one scope at startup and destroys it with `App`. After ADR-0004 defines native-window identity, its controller maps each live window to a config scope without changing the patch API. Callback dispatch uses the focused scope for key/timer actions and the originating scope for origin-bound events; stale or closed scopes reject mutation and are removed. Existing Lua setters become adapters to this patch API. Runtime patches survive successful reload and are revalidated against the candidate. If one becomes invalid, the entire reload fails and the old bundle remains active. Schema metadata explicitly controls which fields permit scoped overrides; callbacks, bindings, composition metadata, and selections do not.

### Runtime setter scheduling

Each setter call is one synchronous patch transaction on the main thread, preserving current Lua semantics: it validates and prepares fallible resources, commits the scoped desired patch before returning, and a getter later in the same callback observes the new value. Failure preserves the prior patch and raises one bounded script error. Independent setter calls are not rolled back if a later call or later callback statement fails, matching today’s behavior.

Scheduling is effect-specific rather than one global debounce. Cheap live effects apply immediately. Repeated calls may coalesce redundant wake/damage requests within the loop turn, but never skip an intermediate value visible to a same-callback getter. Atlas/font resources reuse the existing prepare/install path and shared texture pool. PTY grid resize retains its existing bounded settlement debounce so visual zoom is responsive without interleaving ConPTY repaint. Native/window recreation is never performed implicitly by a rapid setter; it is recorded as pending. Every successful call requests at most one wake/damage transition for its effect.

The existing pane-local `term:set_font_size`/zoom path is an action-state mutation, not a composed configuration override, and Phase 2 does not change its public void setter contract. Font fields added to a future config patch must use the fallible prepare/commit path before becoming scope state.

### Provenance, explanation, and redaction

Provenance is stored for each schema leaf, map key, list as a whole, and function slot. Each winning record contains layer kind/name, requested and canonical source, source schema version before/after migration, dotted field path, CLI argument index or `ConfigScopeID` when relevant, and the overwritten-source chain. A future native `WindowID` may be supplemental metadata only after ADR-0004 is accepted.

Gopher-lua return tables do not retain reliable field coordinates. CervTerm will not fabricate line numbers. Syntax/runtime/Teal diagnostics retain their native file and line information; structural field diagnostics report canonical source plus dotted path; include errors also report the declaring source and `includes[N]`.

`--explain-config` prints the resolved configuration with provenance and exits; repeatable `--explain-config-field PATH` filters it. Doctor reports schema, selected environment/profile, dependency graph, pending non-live fields, and the last reload failure.

Source text and raw CLI values are never logged. Values under schema-sensitive fields and `shell.env` are redacted in doctor, explain output, errors, and rendered provenance; keys and sources may still be named. Common secret/token/password/API/private-key/credential key patterns are redacted defensively.

### Dependency graph and reload recovery

The existing 250 ms SHA-256 polling and 200 ms stable debounce are extended from one file to the source graph. Source-of-truth dependencies are the primary file, declarative includes, local Lua files resolved through CervTerm’s instrumented standard `require`, `dofile`, or `loadfile` wrappers, and Teal sources. Nested calls and cache-miss resolutions are recorded canonically. In v2, replacing those wrappers or installing custom package searchers/loaders is rejected because reload completeness could no longer be claimed; `package.path` may change and is interpreted by the instrumented filesystem searcher, while native/C loaders are not source dependencies. CervTerm-generated Lua is not watched as an independent source, preventing self-trigger loops.

Creation, deletion, rename, content change with unchanged metadata, symlink retarget, and dependency-edge change mark the graph dirty. The candidate is built only after the whole observed graph is stable for the debounce window.

After failure, polling covers the union of the last successful graph and all paths discovered by the failed attempt, including missing include targets. This allows creating or repairing a dependency to recover automatically. A later attempt replaces the failed-attempt set; success installs the new graph. Identical repeated failures are notification-rate-limited without disabling retries.

Teal keeps the external `tl` authoring dependency and source-adjacent generated Lua compatibility contract, but generation is transactional. Every discovered `.tl` source generates first into candidate-owned staging and reserves its eventual canonical `.lua` output path; evaluation uses the staged output. The complete graph must reject any explicit include/module that collides with a reserved generated path before user-adjacent files change. After complete graph and candidate validation, owned generated outputs publish by atomic replacement as a journaled preparation step before active bundle transfer; a multi-output failure restores prior bytes/removes newly created outputs before reporting failure. An absent target may be created; a target previously marked as CervTerm-generated for the same Teal source may be replaced. For compatibility with outputs created before ownership markers, an unmarked target may be adopted only when its bytes exactly equal the staged generation for that source. Any other unowned pre-existing file is never overwritten and rejects the candidate. Only the `.tl` source is authoritative for polling; diagnostics name both source and staged/published output when relevant.

### Atomic candidate bundle

A candidate bundle contains the migrated/composed desired configuration, selected layers, typed overrides, effective scoped configurations, provenance, dependency graph, candidate Lua runtime/bindings/events/timers/overlays, and a classified application plan.

Candidate construction does not mutate active app or mux state. On the OS/main thread, every fallible live resource is prepared first. One main-thread commit routine owns the transfer. Candidate resources remain candidate-owned until the transfer point; failures before transfer close only candidate resources. The routine then installs bundle/runtime/binding/graph references and prepared live resources through mechanically infallible assignments. Any unavoidable fallible live mutation is journaled and fault-injected after each step, then reversed in exact reverse order on failure. The old runtime closes only after ownership transfer succeeds. A partial config/runtime/mux swap is a failed implementation.

If source identity changes during evaluation, a valid candidate may commit, but the newer edit is immediately queued exactly as today. Any evaluation, migration, validation, override, preparation, or commit error leaves the previous config, bindings, runtime, graph, and effective UI state intact.

### Desired versus effective diff semantics

Every public field has one schema classification:

- **live** — existing objects update immediately;
- **new pane** — existing panes retain effective values; new panes use desired values;
- **new window** — existing windows retain effective values; new windows use desired values;
- **window recreate** — desired value is accepted but requires explicit recreation;
- **restart** — desired value waits for process restart.

Current examples: colors, opacity, blur, cursor/scrolling/scrollbar policy, bindings, and events are live; shell program/args/cwd/env are new-pane; initial width/height are new-window; future native decoration/context fields may require recreation. Renderer selection is not introduced.

Desired and effective scoped snapshots are stored separately. Successful reload may therefore accept non-live values without claiming they changed existing objects. Notice, doctor, and explain output enumerate exact pending paths rather than a single restart boolean.

## Consequences

- V1 remains a reversible single-file compatibility path for at least one checkpoint release; composition requires explicit v2 opt-in.
- Deterministic schema merge prevents list duplication and callback-order ambiguity, at the cost of explicit list construction.
- Provenance and graph watching add memory and implementation complexity but make every winner and reload trigger explainable.
- Trusted local Lua remains trusted; path canonicalization and limits are correctness controls, not a sandbox.
- Runtime state becomes a bundle rather than independent config/runtime pointers, enabling atomic reload and later window-scoped configuration.
- Every implementation slice that changes a public field or source behavior must update all applicable Go/Lua/Teal/template/validation/reload/test/documentation surfaces in that same PR; the final slice is closeout evidence, not deferred contract repair.
- Source field diagnostics are path-precise but not line-precise unless the underlying Lua/Teal parser supplies coordinates.

## Alternatives Considered

1. **Use Lua `require` only.** Rejected because it provides no schema merge, provenance, deterministic precedence, or declarative dependency diagnostics.
2. **Concatenate lists and chain callbacks.** Rejected because duplicate keybindings and side-effect order become implicit.
3. **Apply each source directly to Go defaults.** Rejected because presence is lost and included defaults incorrectly override earlier user values.
4. **Strictly reinterpret all v1 files.** Rejected because today’s permissive loader would make that a compatibility break.
5. **Restrict includes below the primary directory.** Rejected as a false security boundary for trusted Lua; canonical identity and diagnostics still apply.
6. **Mutate the active runtime while evaluating reload.** Rejected because rollback cannot be complete.
7. **Watch generated Teal Lua.** Rejected because CervTerm’s own generation would create feedback loops.

## Verification Requirements

- V1 compatibility and v1-to-v2 semantic migration goldens, including discriminator errors, unknown ordinary keys, mistyped scalars, numeric truncation, filtered `shell.args`/`shell.env` entries, fail-fast malformed keys/events, one-evaluation behavior, and temporarily unavailable v2 metadata.
- Per-source strict v2 validation, unknown/type errors, sparse lists, integer bounds, and unset behavior.
- Full precedence matrix across defaults/includes/primary/environment/profile/CLI/window.
- Transitive, diamond, duplicate, cyclic, missing, deleted, symlink-alias, retarget, concurrent-edit, and operational-limit fixtures.
- Map merge, list replacement, function/action winning, and profile/environment merge tests.
- Provenance winner/override-chain tests, native diagnostic locations, explain output, and secret redaction.
- Typed CLI/runtime override parsing, capability checks, config-scope lifecycle/origin routing, synchronous setter visibility, effect-specific wake/damage coalescing, persistence across reload, and invalid-override rollback.
- Graph polling/debounce, failed-candidate recovery, identical-error rate limiting, Teal regeneration, generated-output collision rejection, and module dependency capture.
- Fault-injected candidate preparation/commit tests proving old config/runtime/bindings/effective state survive every failure point.
- Live/new-pane/new-window/recreate/restart desired-versus-effective tests.
- Full tests, GLFW-tagged tests, race subsets, vet with the repository unsafe-pointer policy, maturity gates, performance comparison, and installed-package smoke.

## Implementation Order

1. Add presence-aware raw v1/v2 documents, schema metadata, strict v2 validation, migrations, goldens, and their applicable Lua/Teal/template/docs updates without changing v1 loading.
2. Add canonical source graph, declarative includes, limits/cycles, candidate-state evaluation, local-module dependency capture, Teal collision rules, and user-facing include documentation.
3. Add schema merge, unset, selected environment/profile layers, provenance, redaction, explain/doctor output, and matching Teal/template contracts.
4. Add typed CLI overrides plus `ConfigScopeID` runtime patches, scope lifecycle/origin routing, setter coalescing, and installed CLI documentation/tests.
5. Replace independent config/runtime pointers with a candidate bundle and graph watcher; preserve failed-attempt recovery and atomic swap while updating reload diagnostics.
6. Add field diff classifications and desired/effective scoped application behavior with public compatibility documentation.
7. Benchmark, run full validation and installed-package smoke, mark the support matrix, and perform architecture drift review.

## Rollback

V2 is gated by `config_version = 2`. Disabling v2 returns to the untouched v1 single-file path without rewriting files. Each implementation slice must leave v1 tests green. Failed reload always retains the last successful bundle.

## Reversal Signals

Revisit this decision if presence-aware merging cannot retain Lua callbacks in one candidate state, platform path identity cannot prevent graph aliases, or atomic live preparation cannot be made rollback-safe. Any replacement must retain explicit precedence, strict v2 validation, v1 compatibility, explainable provenance, dependency-aware last-known-good reload, and sensitive-value redaction.
