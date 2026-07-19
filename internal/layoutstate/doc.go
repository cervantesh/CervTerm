// Package layoutstate defines the versioned, layout-only document used to
// restore fresh terminal processes. It deliberately contains no live session,
// process, PTY, renderer, parser, scrollback, environment, credential, or runtime
// identity state. Persisted launch arguments must be explicitly sanitized by callers.
package layoutstate
