package core

import "math"

// PaletteBase is the configured palette beneath pane-local OSC overrides.
// It is a fixed-size value so it can be copied without allocation.
type PaletteBase struct {
	FG      RGB
	BG      RGB
	Indexed [256]RGB
}

// PaletteOverrides is the complete pane-local dynamic palette state.
// IndexedSet is a 256-bit presence map for Indexed.
type PaletteOverrides struct {
	FG         RGB
	BG         RGB
	FGSet      bool
	BGSet      bool
	Indexed    [256]RGB
	IndexedSet [4]uint64
	Generation uint64
}

// DefaultPaletteBase returns the core defaults plus the complete xterm palette.
func DefaultPaletteBase() PaletteBase {
	base := PaletteBase{FG: DefaultFG, BG: DefaultBG}
	for index := range base.Indexed {
		base.Indexed[index] = resolveIndexedColor(uint8(index), ansi16)
	}
	return base
}

// ColorResolver returns a resolver for this base.
func (b PaletteBase) ColorResolver() ColorResolver {
	return ColorResolver{DefaultFG: b.FG, DefaultBG: b.BG, indexed: b.Indexed}
}

// Apply overlays the pane-local values on a configured base.
func (o PaletteOverrides) Apply(base PaletteBase) PaletteBase {
	if o.FGSet {
		base.FG = o.FG
	}
	if o.BGSet {
		base.BG = o.BG
	}
	for word, bits := range o.IndexedSet {
		for bits != 0 {
			bit := uint(bitsTrailingZeros(bits))
			index := uint8(word*64 + int(bit))
			base.Indexed[index] = o.Indexed[index]
			bits &^= uint64(1) << bit
		}
	}
	return base
}

// ColorResolver returns the effective resolver for base plus these overrides.
func (o PaletteOverrides) ColorResolver(base PaletteBase) ColorResolver {
	return o.Apply(base).ColorResolver()
}

// HasIndexed reports whether index has an OSC override.
func (o PaletteOverrides) HasIndexed(index uint8) bool {
	return o.IndexedSet[index/64]&(uint64(1)<<uint(index%64)) != 0
}

func bitsTrailingZeros(value uint64) int {
	// The caller only passes non-zero values. This small loop keeps palette state
	// independent of renderer/toolkit packages and runs at most 64 iterations.
	count := 0
	for value&1 == 0 {
		value >>= 1
		count++
	}
	return count
}

func (t *Terminal) SetPaletteBase(base PaletteBase) { t.paletteBase = base }
func (t *Terminal) PaletteBase() PaletteBase        { return t.paletteBase }
func (t *Terminal) PaletteOverrides() PaletteOverrides {
	return t.paletteOverrides
}

func (t *Terminal) EffectivePaletteIndex(index uint8) RGB {
	if t.paletteOverrides.HasIndexed(index) {
		return t.paletteOverrides.Indexed[index]
	}
	return t.paletteBase.Indexed[index]
}

func (t *Terminal) SetPaletteIndex(index uint8, value RGB) {
	word, bit := index/64, uint(index%64)
	if t.paletteOverrides.HasIndexed(index) && t.paletteOverrides.Indexed[index] == value {
		return
	}
	t.paletteOverrides.Indexed[index] = value
	t.paletteOverrides.IndexedSet[word] |= uint64(1) << bit
	t.bumpPaletteGeneration()
}

func (t *Terminal) ResetPaletteIndex(index uint8) {
	word, bit := index/64, uint(index%64)
	mask := uint64(1) << bit
	if t.paletteOverrides.IndexedSet[word]&mask == 0 {
		return
	}
	t.paletteOverrides.IndexedSet[word] &^= mask
	t.bumpPaletteGeneration()
}

func (t *Terminal) ResetPaletteIndexes() {
	if t.paletteOverrides.IndexedSet == [4]uint64{} {
		return
	}
	t.paletteOverrides.IndexedSet = [4]uint64{}
	t.bumpPaletteGeneration()
}

func (t *Terminal) EffectivePaletteFG() RGB {
	if t.paletteOverrides.FGSet {
		return t.paletteOverrides.FG
	}
	return t.paletteBase.FG
}

func (t *Terminal) EffectivePaletteBG() RGB {
	if t.paletteOverrides.BGSet {
		return t.paletteOverrides.BG
	}
	return t.paletteBase.BG
}

func (t *Terminal) SetPaletteFG(value RGB) {
	if t.paletteOverrides.FGSet && t.paletteOverrides.FG == value {
		return
	}
	t.paletteOverrides.FG = value
	t.paletteOverrides.FGSet = true
	t.bumpPaletteGeneration()
}

func (t *Terminal) SetPaletteBG(value RGB) {
	if t.paletteOverrides.BGSet && t.paletteOverrides.BG == value {
		return
	}
	t.paletteOverrides.BG = value
	t.paletteOverrides.BGSet = true
	t.bumpPaletteGeneration()
}

func (t *Terminal) ResetPaletteFG() {
	if !t.paletteOverrides.FGSet {
		return
	}
	t.paletteOverrides.FGSet = false
	t.bumpPaletteGeneration()
}

func (t *Terminal) ResetPaletteBG() {
	if !t.paletteOverrides.BGSet {
		return
	}
	t.paletteOverrides.BGSet = false
	t.bumpPaletteGeneration()
}

func (t *Terminal) bumpPaletteGeneration() {
	if t.paletteOverrides.Generation < math.MaxUint64 {
		t.paletteOverrides.Generation++
	}
}
