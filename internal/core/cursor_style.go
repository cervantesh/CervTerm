package core

// CursorStyle is a DECSCUSR (CSI Ps SP q) cursor style. Style 0 means "no
// explicit style": renderers fall back to the configured cursor. In each
// non-zero pair the odd value blinks and the even value is steady, and the pair
// selects the shape (block / underline / bar). Keeping the raw DECSCUSR code as
// a named type keeps the "which style blinks / what shape" rule in the domain
// instead of as magic numbers 1..6 scattered across the renderer.
type CursorStyle int

const (
	CursorStyleDefault           CursorStyle = 0
	CursorStyleBlinkingBlock     CursorStyle = 1
	CursorStyleSteadyBlock       CursorStyle = 2
	CursorStyleBlinkingUnderline CursorStyle = 3
	CursorStyleSteadyUnderline   CursorStyle = 4
	CursorStyleBlinkingBar       CursorStyle = 5
	CursorStyleSteadyBar         CursorStyle = 6
)

// CursorShape is the visual form a cursor style selects.
type CursorShape int

const (
	// CursorShapeDefault means the style carries no shape (style 0 or an
	// unrecognized value): the configured cursor shape should be used.
	CursorShapeDefault CursorShape = iota
	CursorShapeBlock
	CursorShapeUnderline
	CursorShapeBar
)

// Shape reports the shape this style selects, or CursorShapeDefault when the
// style is 0 or unrecognized (renderers then use the configured shape).
func (c CursorStyle) Shape() CursorShape {
	switch c {
	case CursorStyleBlinkingBlock, CursorStyleSteadyBlock:
		return CursorShapeBlock
	case CursorStyleBlinkingUnderline, CursorStyleSteadyUnderline:
		return CursorShapeUnderline
	case CursorStyleBlinkingBar, CursorStyleSteadyBar:
		return CursorShapeBar
	}
	return CursorShapeDefault
}

// Blink reports whether this style forces the cursor to blink. ok is false for
// style 0 or an unrecognized value, meaning the style imposes no blink and the
// configured blink setting should be honored.
func (c CursorStyle) Blink() (blink, ok bool) {
	switch c {
	case CursorStyleBlinkingBlock, CursorStyleBlinkingUnderline, CursorStyleBlinkingBar:
		return true, true
	case CursorStyleSteadyBlock, CursorStyleSteadyUnderline, CursorStyleSteadyBar:
		return false, true
	}
	return false, false
}
