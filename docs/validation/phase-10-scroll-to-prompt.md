# Phase 10.4b1 Validation — Scroll to Prompt

This vertical slice exposes typed, pane-targeted prompt navigation over the bounded semantic-history seam.

## Contract

- `ScrollToPrompt(-1|1)` is a closed serializable typed action with registry press/repeat policy, canonical JSON codec, Lua constructor and Teal declaration.
- Direction is restricted to exactly `-1` or `1` during config/action validation.
- Navigation follows WezTerm viewport semantics: the current semantic zone is determined relative to the top of the viewport. Previous selects the newest prompt strictly above that row; next selects the oldest prompt strictly below it.
- The executor resolves the requested pane target, obtains a detached semantic snapshot, selects a prompt, revalidates pane/focus/content/reflow/viewport generations immediately before scrolling, and then uses the canonical global-row viewport API.
- Missing prompts, stale snapshots and missing panes fail without changing the viewport or falling back to another pane.
- Prompt text remains data only. The action neither copies nor executes shell-provided content and introduces no external effect.

## Evidence

Action tests cover valid/invalid values, identity, registry ordering and JSON round trips. Lua tests cover construction and invalid arguments. Core/mux tests cover absolute physical-row scrolling and generation updates. GLFW executor tests cover previous/repeated previous/next movement, exact viewport-top target selection and missing-metadata errors.
