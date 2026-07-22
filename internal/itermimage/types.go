// Package itermimage implements the dormant bounded iTerm inline-image protocol leaf.
// It has no dependency on VT, mux, core, rendering, configuration, or the frontend.
package itermimage

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

type OSCEvent struct {
	Data      []byte
	Final     bool
	Cancelled bool
	Overflow  bool
}

type SizingAxis uint8

const (
	SizingIntrinsic SizingAxis = iota
	SizingWidth
	SizingHeight
)

type Metadata struct {
	Size                uint64
	Axis                SizingAxis
	Cells               uint16
	PreserveAspectRatio bool
}

type Command struct {
	Image     termimage.ImageID
	Placement termimage.PlacementID
	Metadata  Metadata
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
