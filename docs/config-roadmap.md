# CervTerm Configuration Roadmap

## Decision

CervTerm will support two configuration authoring modes:

1. **Lua** for simple direct configuration.
2. **Teal** for typed, AI-friendly configuration that is checked/compiled before use.

Lua is the runtime target. Teal is the safer authoring layer for users and AI agents that benefit from a typed schema.

## Current status (2026-07-16)

The original v1 sequence below is retained as design history. Go/Lua/Teal configuration, discovery, validation, generated templates, scripting callbacks, safe atomic single-file reload, opacity/blur, scrollbar, pane bindings, font selection, cursor controls, and the native pane mux are implemented. The canonical forward plan is [`wezterm-parity-roadmap.md`](wezterm-parity-roadmap.md), and support state is tracked in [`parity-support-matrix.json`](parity-support-matrix.json).

The next configuration work is a typed action registry followed by versioned composition, includes, provenance, CLI/profile overrides, dependency watching, and migration. Local/SSH/WSL domains and renderer selection are excluded.

## Why both Lua and Teal?

- Lua is simple, embeddable, and familiar in terminal configuration ecosystems.
- Teal adds static types, record schemas, and earlier validation while preserving Lua's style.
- AI-generated configuration benefits from Teal because typos and type mismatches can be caught before runtime.
- Advanced users can still choose Lua directly when they prefer flexibility and no compile step.

Recommended loading order:

1. Explicit `--config <path>` if provided.
2. `cervterm.tl` if present and Teal validation is available.
3. `cervterm.lua` if present.
4. Built-in defaults.
5. `cervterm --print-default-config` to generate an editable Lua template.

Planned default config locations:

- Windows portable mode: alongside `cervterm.exe`.
- Windows user config: `%APPDATA%/cervterm/cervterm.lua` or `%APPDATA%/cervterm/cervterm.tl`.
- Unix user config: `$XDG_CONFIG_HOME/cervterm/cervterm.lua` or `.tl`.
- Fallback Unix path: `$HOME/.config/cervterm/cervterm.lua` or `.tl`.

## Inspiration from other terminals

### WezTerm

WezTerm uses Lua as its configuration language. Config files return a Lua table, often built with `wezterm.config_builder()`. It supports config reload, CLI overrides, per-window overrides, Lua modules, and a broad API surface exposed through a `wezterm` module.

Ideas to adopt:

- Lua as the direct config language.
- Config file returns a table.
- Optional helper module exposed by CervTerm.
- Extend the existing atomic reload to composed config dependencies.
- Add typed CLI/profile/per-window overrides after precedence and provenance are accepted in ADR-0002.
- Add local includes/modules with canonical paths and cycle detection.

Ideas to exclude or defer:

- Local, SSH, WSL, serial, and remote domains are excluded.
- Plugin marketplaces and unrestricted remote code are excluded.
- Continue expanding the typed helper/action API incrementally.

### Alacritty

Alacritty uses TOML with a strongly structured surface. Its configuration is especially useful as a checklist for what a GPU terminal should expose: window dimensions, padding, opacity, font faces, font offsets, colors, indexed colors, cursor style, selection behavior, mouse bindings, keyboard bindings, shell, environment variables, and live reload.

Ideas to adopt:

- Keep the schema explicit and sectioned even though CervTerm uses Lua/Teal.
- Provide imports/includes.
- Separate `window`, `font`, `colors`, `scrolling`, `cursor`, `selection`, `mouse`, `keyboard`, `shell`, and `env` sections.
- Expose scrollback history and wheel multiplier early.
- Expose cursor shape/blink/thickness early.
- Expose normal/bright ANSI palette and indexed colors.

Ideas to defer:

- Vi mode.
- Hints.
- IPC.
- Deep platform-specific window class/decorations until packaging matures.

### Kitty

Kitty uses a custom `kitty.conf` directive format with a very broad configuration surface. It has particularly strong font controls, symbol mapping, ligatures, font features, cursor customization, scrollback controls, scrollbars, generated includes, and dynamic config reloading.

Ideas to adopt:

- Rich font configuration is implemented for ordered descriptors, real/synthetic styles, lazy fallback/rules, OpenType features, line height, cell width, baseline and glyph offsets.
- Scrollbar settings eventually: visible mode, width, opacity, colors.
- Scrollback pager concept later.
- Include/import support.
- Separate wheel and touchpad scrolling multipliers.
- Cursor shape, blink interval, and unfocused cursor style.

Ideas to defer:

- Embedded dynamic config generation.
- Font feature-level HarfBuzz tuning until font rendering is upgraded.
- Remote control and large plugin ecosystems.

## Proposed CervTerm schema v1

The first config version should be intentionally small and map only to features CervTerm already has or is about to implement.

### `window`

```lua
window = {
   width = 1100,
   height = 720,
   padding_x = 18,
   padding_y = 44,
   dynamic_title = true,
}
```

Near-term fields:

- `width`
- `height`
- `padding_x`
- `padding_y`
- `dynamic_title`
- `opacity` (implemented)
- `decorations` later

### `font`

```lua
font = {
   family = "JetBrains Mono",
   size = 14.0,
   cell_width = nil,
   cell_height = nil,
}
```

Near-term fields:

- `family`
- `size`
- `cell_width`
- `cell_height`
- `line_height` later
- `glyph_offset_x` later
- `glyph_offset_y` later
- `fallback` later

### `colors`

```lua
colors = {
   foreground = "#E6E1D8",
   background = "#080B12",
   cursor = "#60E8F0",
   selection_background = "#2A6377",
   ansi = {
      "#1B2232", "#FF5C8A", "#8BF59A", "#F8D866",
      "#7AA2FF", "#D88CFF", "#60E8F0", "#D8DEEA",
      "#57627A", "#FF7AA8", "#A6FFB5", "#FFE68A",
      "#9BB8FF", "#E5A7FF", "#90F4FF", "#FFFFFF",
   },
   indexed_colors = { [16] = "#102030", [196] = "#FF1010" },
}
```

Implemented: `colors.ansi` is a live dense list of exactly 16 `#RRGGBB` entries. `colors.indexed_colors` is a live sparse numeric map for indices 16–255; missing or unset entries retain the xterm cube/grayscale fallback.

Near-term fields:

- `foreground`
- `background`
- `surface`
- `chrome`
- `accent`
- `cursor`
- `selection_background`
- `ansi`
- `indexed_colors`

### `scrolling`

```lua
scrolling = {
   history = 2000,
   wheel_multiplier = 3,
   touch_multiplier = 1,
   hide_cursor_when_scrolled = true,
}
```

Near-term fields:

- `history`
- `wheel_multiplier`
- `touch_multiplier`
- `hide_cursor_when_scrolled`
- `scrollbar` (basic interactive scrollbar implemented; visibility policies remain planned)

### `cursor`

```lua
cursor = {
   shape = "underline",
   blink = true,
   blink_interval_ms = 1000,
   thickness = 0.15,
}
```

Near-term fields:

- `shape`: `block`, `underline`, `beam`
- `blink`
- `blink_interval_ms`
- `thickness`

### `shell`

```lua
shell = {
   program = "cmd.exe",
   args = {},
   working_directory = nil,
   env = {},
}
```

Near-term fields:

- `program`
- `args`
- `working_directory`
- `env`

### `keyboard`

```lua
keyboard = {
   bindings = {
      { key = "C", mods = "CTRL|SHIFT", action = "Copy" },
      { key = "V", mods = "CTRL|SHIFT", action = "Paste" },
   },
}
```

Near-term fields:

- `bindings`
- actions: `Copy`, `Paste`, `ScrollLineUp`, `ScrollLineDown`, `ScrollPageUp`, `ScrollPageDown`, `ResetFontSize`, `IncreaseFontSize`, `DecreaseFontSize`.

### `mouse`

```lua
mouse = {
   hide_when_typing = false,
   bypass_reporting_mods = "SHIFT",
}
```

Near-term fields:

- `hide_when_typing`
- `bypass_reporting_mods`
- mouse bindings later

## Example Lua config

```lua
return {
   window = {
      width = 1100,
      height = 720,
      padding_x = 18,
      padding_y = 44,
      dynamic_title = true,
   },

   font = {
      family = "JetBrains Mono",
      size = 14.0,
   },

   colors = {
      foreground = "#E6E1D8",
      background = "#080B12",
      cursor = "#60E8F0",
      selection_background = "#2A6377",
   },

   scrolling = {
      history = 2000,
      wheel_multiplier = 3,
      hide_cursor_when_scrolled = true,
   },

   shell = {
      program = "cmd.exe",
      args = {},
   },
}
```

## Example Teal config

```lua
local record WindowConfig
   width: integer
   height: integer
   padding_x: integer
   padding_y: integer
   dynamic_title: boolean
end

local record FontConfig
   family: string
   size: number
end

local record ColorsConfig
   foreground: string
   background: string
   cursor: string
   selection_background: string
end

local record ScrollingConfig
   history: integer
   wheel_multiplier: integer
   hide_cursor_when_scrolled: boolean
end

local record ShellConfig
   program: string
   args: {string}
end

local record Config
   window: WindowConfig
   font: FontConfig
   colors: ColorsConfig
   scrolling: ScrollingConfig
   shell: ShellConfig
end

local config: Config = {
   window = {
      width = 1100,
      height = 720,
      padding_x = 18,
      padding_y = 44,
      dynamic_title = true,
   },

   font = {
      family = "JetBrains Mono",
      size = 14.0,
   },

   colors = {
      foreground = "#E6E1D8",
      background = "#080B12",
      cursor = "#60E8F0",
      selection_background = "#2A6377",
   },

   scrolling = {
      history = 2000,
      wheel_multiplier = 3,
      hide_cursor_when_scrolled = true,
   },

   shell = {
      program = "cmd.exe",
      args = {},
   },
}

return config
```

## Implementation phases

### Phase 1: Go config model (implemented)

- Add `internal/config`.
- Define `Config` structs.
- Add defaults matching current CervTerm behavior.
- Add validation for colors, dimensions, font size, shell fields.
- Provide `--print-default-config` so users can generate a validated Lua template without opening documentation.

### Phase 2: Lua loader (implemented)

- Embed a Lua runtime.
- Load `cervterm.lua` returning a table.
- Convert Lua table to Go `Config`.
- Reject unknown fields or warn depending on strict mode.

### Phase 3: Teal support (implemented)

- Add `cervterm.tl` support.
- Run `tl check` if available.
- Run `tl gen` to Lua or use generated Lua cache.
- If `tl` is unavailable, show an actionable error and fallback only if configured.

### Phase 4: Atomic reload (single-file implemented; composition/dependency watching planned)

- Existing behavior atomically reloads the primary configuration after full candidate validation.
- Phase 2 of the parity roadmap adds dependency watching, provenance, migrations, and deterministic composition.
- Live-safe and restart-required diffs must be explicit; renderer/backend selection is not a configuration goal.

### Phase 5: Advanced configuration (appearance/window subset implemented; superseded by parity roadmap)

Implemented Phase 5.1-5.6 fields include:

- `padding.left/right/top/bottom`;
- independent text/background opacity with whole-window opacity/blur compatibility checks;
- bounded solid, linear-gradient, and image background layers with fit/alignment;
- scrollbar `always`/`hover`/`scrolling`/`never`, stable gutter, and bounded animation FPS;
- `render.max_fps` presentation limiting without changing on-demand idle policy;
- initial terminal rows/columns and capability-aware native decorations/titlebar requests.

Renderer/backend selection is deliberately not part of the schema. Phase 6 key tables and advanced mouse bindings are implemented. Phase 7 adds bounded `quick_select.rules` and ordered `launch_menu` descriptors with atomic composition, provenance, Teal declarations, retained UI actions, and argv-only spawn semantics; theme catalogs/imports remain tracked by their own parity phases.

## Original v1 non-goals and current disposition

- Broad callbacks and native pane multiplexing are now implemented.
- Visible tabs, local windows, and layout-only workspaces are planned.
- Local/SSH/WSL/remote domains remain excluded.
- Dynamic remote configuration and plugin marketplaces remain excluded.
- Rich font descriptors/features/metrics are implemented without renderer-backend selection.

## Sources

- WezTerm configuration files: https://wezterm.org/config/files.html
- WezTerm config reference: https://wezterm.org/config/lua/config/index.html
- Alacritty configuration reference: https://alacritty.org/config-alacritty.html
- Kitty configuration reference: https://sw.kovidgoyal.net/kitty/conf/
