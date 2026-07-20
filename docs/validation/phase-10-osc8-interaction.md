# Phase 10.2 Validation — OSC 8 Interaction Boundary

This slice accepts the terminal-originated external-effect ADR and connects OSC 8 metadata to hover/click behavior without permitting terminal output to launch a URI.

## Contract

- Explicit OSC 8 regions take precedence over plaintext URL detection and may span separate visual-row regions while retaining one URI identity.
- URI opening has one production adapter. Both pointer activation and quick select pass through the same pure policy before that adapter.
- Policy requires a fresh explicit user activation, a parseable absolute hierarchical URI, an ASCII authority without credentials, and an `http` or `https` scheme. Scheme and host are canonicalized before launch.
- Pointer press captures pane, URI, bounds, explicitness and OSC identity; release re-resolves the current focused-pane snapshot and must match that exact target before policy evaluation. Changed, removed, replaced, malformed and disallowed links are consumed and denied without launching.
- Quick select derives a one-shot activation only from its key/label handlers, consumes it beside the freshness check, and cannot open through direct/programmatic modal acceptance.
- Diagnostics expose only a scheme/origin label and denial class; URI query, fragment, credentials and payload are not included.
- Parser/core/render/mux remain free of OS calls. The OS launcher remains on the GLFW-owned thread behind a fakeable interface.

## Evidence

Pure policy tests cover activation freshness, canonicalization, bounded payload-redacted labels, scheme denial, credentials, missing/invalid authority, IPv6 zones and malformed values. GLFW tests cover OSC 8 region precedence, zero automatic activation, successful fake-adapter click, unsafe-scheme denial, press/release mutation and same-URI identity replacement, one-shot quick-select activation, launcher-error redaction, and existing quick-select compatibility.
