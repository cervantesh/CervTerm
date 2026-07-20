package accessibility

import (
	"errors"
	"math"
)

const (
	MaxRows          = 512
	MaxGraphemes     = 16_384
	MaxUTF8Bytes     = 1 << 20
	MaxNodes         = 256
	MaxNodeNameBytes = 4 << 10
)

var (
	ErrInvalidDocument  = errors.New("invalid accessibility document")
	ErrInvalidIdentity  = errors.New("invalid accessibility identity")
	ErrInvalidText      = errors.New("invalid accessibility text")
	ErrInvalidBounds    = errors.New("invalid accessibility bounds")
	ErrInvalidRange     = errors.New("invalid accessibility range")
	ErrStaleRange       = errors.New("stale accessibility range")
	ErrCounterExhausted = errors.New("accessibility document counter exhausted")
)

type NodeKind uint8

const (
	NodeKindNone NodeKind = iota
	NodeKindWindow
	NodeKindTab
	NodeKindPane
	NodeKindInput
	NodeKindItem
)

func (kind NodeKind) Valid() bool { return kind >= NodeKindWindow && kind <= NodeKindItem }

type NodeID struct {
	Kind       NodeKind
	Projection uint64
	Object     uint64
	Activation uint64
}

func (id NodeID) Valid() bool {
	return id.Kind.Valid() && id.Projection != 0 && id.Object != 0
}

type Role uint8

const (
	RoleNone Role = iota
	RoleWindow
	RoleTab
	RoleTerminal
	RoleDialog
	RoleTextField
	RoleList
	RoleListItem
	RoleStatus
)

func (role Role) Valid() bool { return role >= RoleWindow && role <= RoleStatus }

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func (rect Rect) Valid() bool {
	return finite(rect.X) && finite(rect.Y) && finite(rect.Width) && finite(rect.Height) && rect.Width >= 0 && rect.Height >= 0
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

type Span struct {
	Start int
	End   int
}

func (span Span) Valid(limit int) bool {
	return span.Start >= 0 && span.End >= span.Start && span.End <= limit
}

type RowDraft struct {
	Text        string
	Bounds      []Rect
	SoftWrapped bool
}

type NodeDraft struct {
	ID        NodeID
	Parent    NodeID
	Role      Role
	Name      string
	Rows      []RowDraft
	Caret     *int
	Selection *Span
}

type DocumentDraft struct {
	ProviderID uint64
	Generation uint64
	Nodes      []NodeDraft
	Focus      NodeID
}

type RowSnapshot struct {
	Text          string
	Bounds        []Rect
	StartGrapheme int
	EndGrapheme   int
	SoftWrapped   bool
}

type NodeSnapshot struct {
	ID        NodeID
	Parent    NodeID
	Role      Role
	Name      string
	Text      string
	Rows      []RowSnapshot
	Caret     int
	HasCaret  bool
	Selection Span
	HasSelect bool
}
