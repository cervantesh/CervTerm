// Package action defines CervTerm's toolkit-neutral command vocabulary.
//
// Actions are immutable values paired with semantic target selectors. The
// package owns validation, deterministic metadata, bounded strict JSON codecs,
// trigger policy, and opaque dispatch context. It deliberately does not execute
// commands or import GLFW, OpenGL, mux, script-runtime, or platform APIs.
package action
