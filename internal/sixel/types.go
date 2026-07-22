// Package sixel implements the bounded protocol leaf for selected Sixel DCS frames.
// It has no dependency on VT, mux, core, rendering, configuration, or the frontend.
package sixel

import "cervterm/internal/termimage"

type Failure uint8

const (
	FailureNone Failure = iota
	FailureInvalid
	FailureUnsupported
	FailureLimit
	FailureTimeout
	FailureCancelled
	FailureFailed
)

type DCSEvent struct {
	Data      []byte
	Final     bool
	Cancelled bool
	Overflow  bool
}

type Raster struct {
	Width  uint32
	Height uint32
}

type Command struct {
	Image     termimage.ImageID
	Placement termimage.PlacementID
	Raster    Raster
	Transfer  *termimage.CandidateTransfer
}

func (c *Command) Close() {
	if c == nil || c.Transfer == nil {
		return
	}
	c.Transfer.Close()
	c.Transfer = nil
}

type Outcome struct {
	Command *Command
	Failure Failure
}
