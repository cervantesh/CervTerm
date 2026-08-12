# macOS re-verification evidence — 2026-08-12

Raw evidence from re-running this branch's full test/build/launch sequence
on real macOS hardware, captured after the Codex adversarial-review and
Opus-adjudication rounds described in
[`../macos-platform-validation-2026-08-12.md`](../macos-platform-validation-2026-08-12.md).
This directory holds the actual command output rather than a prose summary
of it, so a reviewer can check the claims directly instead of trusting a
paraphrase.

**Environment** ([`00-environment.txt`](00-environment.txt)): macOS 15.7.7
(24G720), Intel x86_64, Go 1.26.5, commit `81793cb`.

| File | Command | Result |
|---|---|---|
| [`01-go-test-full.txt`](01-go-test-full.txt) | `go test ./... -count=1 -v` | **PASS** — 44/44 packages, 1414 subtests, 0 failures |
| [`02-go-vet.txt`](02-go-vet.txt) | `go vet -unsafeptr=false ./...` | clean, exit 0 |
| [`03-fuzz-smoke.txt`](03-fuzz-smoke.txt) | `go test ./internal/vt -fuzz=FuzzParserAdvanceDoesNotPanic -fuzztime=5s` | **PASS**, 75k+ execs, no panics |
| [`04-glfw-tests.txt`](04-glfw-tests.txt) | `go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1 -v` | **PASS** |
| [`05-mux-zoom-tests.txt`](05-mux-zoom-tests.txt) | `go test -tags glfw ./internal/mux ./internal/frontend/glfwgl -run 'Zoom\|Divider\|Layout\|Mux\|Atlas' -count=1 -v` | **PASS** |
| [`06-mux-race.txt`](06-mux-race.txt) | `go test -race ./internal/mux -count=1 -v` | **PASS** |
| [`07-blur-tests.txt`](07-blur-tests.txt) | `go test -tags glfw ./internal/frontend/glfwgl -run 'Blur' -count=1 -v` | **PASS**, including the real darwin cgo/Objective-C `NSVisualEffectView` provider |
| [`08-build-doctor-capture.txt`](08-build-doctor-capture.txt) | headless build, `--doctor`, zsh PTY capture, GLFW app build | all succeed; `mac-pty-ok` round-trips byte-exact |

## GUI launch — real screenshot, this time with an actual desktop session

The previous validation pass in this same environment could not observe any
GUI window: `screencapture` showed only the desktop picture and menu bar
with no Dock and no window, even with the app running, because that
automated session had no interactive logged-in desktop. This time an
interactive session was present, and the launch is directly observable:

[`screenshots/cervterm-window-only.png`](screenshots/cervterm-window-only.png)

Shows the native `CervTerm` window (standard macOS traffic-light controls,
system title bar — consistent with the `window.titlebar="dark" is
unsupported on this platform; using system titlebar` fallback logged at
startup) with a live `zsh` shell prompt (`ricardo@Silverbox CervTerm %`),
confirming window creation, GLFW/OpenGL rendering, PTY shell spawn, and text
rendering all work end-to-end. The original full-screen capture also showed
unrelated personal windows/desktop content and was not committed — only the
cropped window is included here.

This is still not the interactive/visual checklist verification described
as outstanding in the companion validation document (font/glyph quality,
zoom, blur appearance, multi-pane layout) — that needs deliberate,
sustained interaction this pass didn't attempt — but it does newly confirm
the app launches and renders correctly on real macOS, which was previously
only inferred from automated tests and process logs.
