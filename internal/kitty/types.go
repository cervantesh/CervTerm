package kitty

import "cervterm/internal/termimage"

type APCEvent struct {
	Data                       []byte
	Final, Cancelled, Overflow bool
}
type Action byte

const (
	ActionTransmit         Action = 't'
	ActionTransmitAndPlace Action = 'T'
	ActionPlace            Action = 'p'
	ActionDelete           Action = 'd'
	ActionQuery            Action = 'q'
)

type PixelFormat uint8

const (
	FormatRGB24  PixelFormat = 24
	FormatRGBA32 PixelFormat = 32
	FormatPNG    PixelFormat = 100
)

type Compression uint8

const (
	CompressionNone Compression = iota
	CompressionZlib
)

type Quiet uint8

const (
	QuietNormal Quiet = iota
	QuietErrorsOnly
	QuietAll
)

type DecodeSpec struct {
	Format        PixelFormat
	Compression   Compression
	Width, Height uint32
}
type PlacementRequest struct {
	ID         termimage.PlacementID
	Crop       *termimage.PixelRect
	Cols, Rows uint16
	Z          int16
	MoveCursor bool
}
type Command struct {
	Action    Action
	Image     termimage.ImageID
	Transfer  *termimage.CandidateTransfer
	Decode    DecodeSpec
	Placement *PlacementRequest
	Delete    *termimage.DeleteSelector
}
type ReplyCode uint8

const (
	ReplyNone ReplyCode = iota
	ReplyOK
	ReplyInvalid
	ReplyUnsupported
	ReplyLimit
	ReplyTimeout
	ReplyCancelled
	ReplyNotFound
	ReplyFailed
)

type Outcome struct {
	Command *Command
	Reply   ReplyPlan
	Failure ReplyCode
}
