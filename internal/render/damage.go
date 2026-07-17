package render

import "cervterm/internal/core"

const (
	fnvOffset64 = uint64(14695981039346656037)
	fnvPrime64  = uint64(1099511628211)
)

// HashRows writes one FNV-1a content hash per complete row into dst.
func HashRows(dst []uint64, cells []core.Cell, cols int) {
	if cols <= 0 {
		return
	}
	rows := len(cells) / cols
	if len(dst) < rows {
		panic("render.HashRows: destination too short")
	}
	for row := 0; row < rows; row++ {
		hash := fnvOffset64
		for _, cell := range cells[row*cols : (row+1)*cols] {
			hash = hashUint32(hash, uint32(cell.Rune))
			marks := cell.Combining()
			hash = hashUint32(hash, uint32(len(marks)))
			for _, combining := range marks {
				hash = hashUint32(hash, uint32(combining))
			}
			hash = hashLogicalColor(hash, cell.Attr.FG)
			hash = hashLogicalColor(hash, cell.Attr.BG)
			hash = hashBool(hash, cell.Attr.Bold)
			hash = hashBool(hash, cell.Attr.Dim)
			hash = hashBool(hash, cell.Attr.Italic)
			hash = hashBool(hash, cell.Attr.Underline)
			hash = hashBool(hash, cell.Attr.Inverse)
			hash = hashBool(hash, cell.Attr.Strikethrough)
			hash = hashBool(hash, cell.Attr.Blink)
			hash = hashBool(hash, cell.WideContinuation)
		}
		dst[row] = hash
	}
}

func hashUint32(hash uint64, value uint32) uint64 {
	for shift := uint(0); shift < 32; shift += 8 {
		hash ^= uint64(byte(value >> shift))
		hash *= fnvPrime64
	}
	return hash
}

func hashLogicalColor(hash uint64, value core.LogicalColor) uint64 {
	return hashUint32(hash, uint32(value))
}

func hashBool(hash uint64, value bool) uint64 {
	if value {
		hash ^= 1
	}
	return hash * fnvPrime64
}
