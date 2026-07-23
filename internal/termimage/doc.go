// Package termimage owns bounded, protocol-neutral terminal image resources.
//
// Store topology and publication are owner-thread-only. Process accounting and
// reservation release are concurrency-safe so stale worker results can return
// leases without mutating store maps. Pixel acquisition always returns a
// detached copy; this package never imports terminal core, mux, render, window,
// toolkit, or GPU packages.
package termimage
