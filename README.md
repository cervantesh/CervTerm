# CervTerm

CervTerm is an experimental GPU terminal emulator written in Go.

## Current status

CervTerm is not a finished daily-driver terminal yet, but it already includes:

- Windows ConPTY backend.
- GLFW/OpenGL frontend.
- Scrollback and alternate screen support.
- VT parsing for common cursor, erase, color, scroll-region, insert/delete, and mode sequences.
- Selection, copy, paste, and bracketed paste.
- Lua config loading and Teal check/gen support.
- OpenType-rendered embedded Go Mono atlas.

See:

- `docs/product-roadmap.md`
- `docs/config-roadmap.md`

## Build

```sh
go test ./...
go build -tags glfw -o cervterm.exe ./cmd/cervterm
```

## Run

```sh
./cervterm.exe
./cervterm.exe --version
./cervterm.exe --config path/to/cervterm.lua
```

## Lua config example

```lua
return {
  window = { width = 1100, height = 720, padding_x = 18, padding_y = 44 },
  font = { family = "Go Mono", size = 14 },
  shell = { program = "powershell.exe", args = {} },
}
```

## Teal config

`cervterm.tl` is checked and generated through the external `tl` command before CervTerm loads the generated Lua file.

## Known limitations

- Font selection is currently wired through the config model, but the renderer still uses embedded Go Mono until system font discovery lands.
- Manual TUI compatibility testing is still pending.
- Packaging is currently a CI-produced Windows artifact, not an installer.
