// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

// Package conpty provides functions for creating a process attached to a
// ConPTY pseudo-terminal.  This allows the process to call console specific
// API functions without an actual terminal being present.
//
// The concept is best explained in this blog post:
// https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/
//
// This is a local fork of github.com/ActiveState/termtest/conpty v0.5.0,
// trimmed to the parts CervTerm uses, with CREATE_NO_WINDOW added to the
// child process creation flags so the ConPTY-hosted conhost never surfaces
// as its own visible window (see conpty_windows.go Spawn).
package conpty
