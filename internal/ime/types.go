package ime

import "errors"

const (
	MaxPreeditBytes      = 16 * 1024
	MaxPreeditRunes      = 4096
	MaxPreeditUTF16Units = MaxPreeditBytes
	MaxCommitBytes       = 64 * 1024
	MaxCommitRunes       = MaxCommitBytes
	MaxCommitUTF16Units  = MaxCommitBytes
)

var (
	ErrAlreadyActive       = errors.New("composition is already active")
	ErrInactive            = errors.New("composition is inactive")
	ErrInvalidTarget       = errors.New("invalid composition target")
	ErrInvalidGeneration   = errors.New("stale composition generation")
	ErrInvalidUTF16        = errors.New("invalid UTF-16 composition text")
	ErrInvalidCursor       = errors.New("invalid UTF-16 composition cursor")
	ErrInvalidAttributes   = errors.New("invalid composition attributes")
	ErrPreeditLimit        = errors.New("preedit limit exceeded")
	ErrCommitLimit         = errors.New("commit limit exceeded")
	ErrEmptyCommit         = errors.New("composition commit is empty")
	ErrInvalidCancelReason = errors.New("invalid composition cancellation reason")
	ErrCounterExhausted    = errors.New("composition counter exhausted")
)

type TargetKind uint8

const (
	TargetNone TargetKind = iota
	TargetPane
	TargetSearch
	TargetModal
)

func (kind TargetKind) Valid() bool {
	return kind >= TargetPane && kind <= TargetModal
}

type Target struct {
	Kind       TargetKind
	ID         uint64
	Activation uint64
}

func (target Target) Valid() bool {
	return target.Kind.Valid() && target.ID != 0 && target.Activation != 0
}

type CancelReason uint8

const (
	CancelNone CancelReason = iota
	CancelExplicit
	CancelMalformed
	CancelFocusLost
	CancelTargetChanged
	CancelModalChanged
	CancelWindowHidden
	CancelDisabled
	CancelTeardown
)

func (reason CancelReason) Valid() bool {
	return reason >= CancelExplicit && reason <= CancelTeardown
}

// Windows composition attribute values. They are kept toolkit-neutral here so
// strict normalization can be tested without importing Win32 packages.
const (
	AttributeInput              byte = 0
	AttributeTargetConverted    byte = 1
	AttributeConverted          byte = 2
	AttributeTargetNotConverted byte = 3
	AttributeInputError         byte = 4
	AttributeFixedConverted     byte = 5
)

type NativeUpdate struct {
	UTF16       []uint16
	CursorUTF16 int
	Attributes  []byte
}

type Span struct {
	Start int
	End   int
}

type Snapshot struct {
	Active         bool
	Target         Target
	Generation     uint64
	Revision       uint64
	Text           string
	Runes          []rune
	CursorRune     int
	TargetRuneSpan Span
	LastCancel     CancelReason
}

type Commit struct {
	Target     Target
	Generation uint64
	Text       string
	Runes      []rune
}
