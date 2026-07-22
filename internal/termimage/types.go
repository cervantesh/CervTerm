package termimage

import "errors"

type ImageID uint32
type PlacementID uint32
type TransferID uint32
type ResourceGeneration uint64
type StoreEpoch uint64
type ResourceRetention uint8

const (
	ResourceDurable ResourceRetention = iota
	ResourceEphemeral
)

func (r ResourceRetention) Valid() bool { return r == ResourceDurable || r == ResourceEphemeral }

const (
	MaxWireImageID         ImageID     = 0x7fffffff
	MinInternalImageID     ImageID     = 0x80000000
	MaxWirePlacementID     PlacementID = 0x7fffffff
	MinInternalPlacementID PlacementID = 0x80000000
)

func IsWireImageID(id ImageID) bool         { return id > 0 && id <= MaxWireImageID }
func IsWirePlacementID(id PlacementID) bool { return id > 0 && id <= MaxWirePlacementID }

type ResourceRef struct {
	Image      ImageID
	Generation ResourceGeneration
}

type Header struct {
	Transfer TransferID
	Image    ImageID
}

type DetachedResource struct {
	Ref           ResourceRef
	Width, Height uint32
	Stride        uint32
	RGBA          []byte
}

type PixelRect struct {
	X, Y          uint32
	Width, Height uint32
}

type CellAnchor struct {
	Row int64
	Col uint32
}

type Placement struct {
	ID       PlacementID
	Resource ResourceRef
	Anchor   CellAnchor
	Cols     uint16
	Rows     uint16
	Crop     *PixelRect
	Z        int16
	Opacity  uint8
}

type Projection struct {
	Placements []Placement
	Generation uint64
}

type PlacementSpec struct {
	ID      PlacementID
	Anchor  CellAnchor
	Cols    uint16
	Rows    uint16
	Crop    *PixelRect
	Z       int16
	Opacity uint8
}

type DeleteSelector struct {
	Placement      *PlacementID
	Image          *ImageID
	All            bool
	CurrentScreen  bool
	UnderCursor    bool
	DeleteResource bool
	WireIDsOnly    bool
}

type Usage struct {
	EncodedBytes     uint64
	DecodedBytes     uint64
	Images           uint64
	Placements       uint64
	PendingTransfers uint64
}

var (
	ErrClosed              = errors.New("termimage store is closed")
	ErrInvalidID           = errors.New("termimage identity must be nonzero")
	ErrInvalidLimits       = errors.New("termimage limits are invalid")
	ErrLimitExceeded       = errors.New("termimage resource limit exceeded")
	ErrDuplicateTransfer   = errors.New("termimage transfer already exists")
	ErrTransferClosed      = errors.New("termimage transfer is closed")
	ErrTransferExpired     = errors.New("termimage transfer expired")
	ErrTooManyChunks       = errors.New("termimage transfer chunk limit exceeded")
	ErrInvalidChunk        = errors.New("termimage transfer chunk is invalid")
	ErrGenerationExhausted = errors.New("termimage resource generation exhausted")
	ErrInvalidPlacement    = errors.New("termimage placement is invalid")
	ErrInvalidCrop         = errors.New("termimage crop is invalid")
	ErrInvalidSelector     = errors.New("termimage delete selector is invalid")
	ErrCandidateInvalid    = errors.New("termimage decoded candidate is invalid")
	ErrPreparedState       = errors.New("termimage prepared state is invalid")
	ErrInternalIDExhausted = errors.New("termimage internal identity namespace exhausted")
	ErrInvalidRetention    = errors.New("termimage resource retention is invalid")
)
