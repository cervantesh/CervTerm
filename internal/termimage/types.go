package termimage

import "errors"

type ImageID uint32
type PlacementID uint32
type TransferID uint32
type ResourceGeneration uint64
type StoreEpoch uint64

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
)
