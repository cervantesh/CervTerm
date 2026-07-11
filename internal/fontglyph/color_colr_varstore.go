package fontglyph

import "encoding/binary"

type colrVariationStore struct {
	regions []colrVariationRegion
	data    []colrVariationData
}

type colrVariationRegion struct {
	axes []colrVariationAxis
}

type colrVariationAxis struct {
	start float64
	peak  float64
	end   float64
}

type colrVariationData struct {
	regionIndexes []uint16
	deltas        [][]int16
}

func parseCOLRVariationStore(data []byte, offset int) (*colrVariationStore, error) {
	if offset == 0 {
		return nil, nil
	}
	if offset < 0 || offset+8 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	if binary.BigEndian.Uint16(data[offset:offset+2]) != 1 {
		return nil, ErrUnsupportedCOLR
	}
	regionListOffset := offset + int(binary.BigEndian.Uint32(data[offset+2:offset+6]))
	dataCount := int(binary.BigEndian.Uint16(data[offset+6 : offset+8]))
	if offset+8+dataCount*4 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	store := &colrVariationStore{}
	regions, err := parseCOLRVariationRegions(data, regionListOffset)
	if err != nil {
		return nil, err
	}
	store.regions = regions
	store.data = make([]colrVariationData, dataCount)
	for i := 0; i < dataCount; i++ {
		dataOffset := offset + int(binary.BigEndian.Uint32(data[offset+8+i*4:offset+12+i*4]))
		variationData, err := parseCOLRVariationData(data, dataOffset)
		if err != nil {
			return nil, err
		}
		store.data[i] = variationData
	}
	return store, nil
}

func parseCOLRVariationRegions(data []byte, offset int) ([]colrVariationRegion, error) {
	if offset < 0 || offset+4 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	axisCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	regionCount := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
	recordSize := axisCount * 6
	if axisCount < 0 || regionCount < 0 || offset+4+regionCount*recordSize > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	regions := make([]colrVariationRegion, regionCount)
	for i := 0; i < regionCount; i++ {
		region := colrVariationRegion{axes: make([]colrVariationAxis, axisCount)}
		base := offset + 4 + i*recordSize
		for axis := 0; axis < axisCount; axis++ {
			o := base + axis*6
			region.axes[axis] = colrVariationAxis{
				start: f2dot14(binary.BigEndian.Uint16(data[o : o+2])),
				peak:  f2dot14(binary.BigEndian.Uint16(data[o+2 : o+4])),
				end:   f2dot14(binary.BigEndian.Uint16(data[o+4 : o+6])),
			}
		}
		regions[i] = region
	}
	return regions, nil
}

func parseCOLRVariationData(data []byte, offset int) (colrVariationData, error) {
	if offset < 0 || offset+6 > len(data) {
		return colrVariationData{}, ErrInvalidCOLRTable
	}
	itemCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	shortDeltaCount := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
	regionIndexCount := int(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
	if shortDeltaCount > regionIndexCount || offset+6+regionIndexCount*2 > len(data) {
		return colrVariationData{}, ErrInvalidCOLRTable
	}
	out := colrVariationData{regionIndexes: make([]uint16, regionIndexCount), deltas: make([][]int16, itemCount)}
	pos := offset + 6
	for i := range out.regionIndexes {
		out.regionIndexes[i] = binary.BigEndian.Uint16(data[pos : pos+2])
		pos += 2
	}
	for item := 0; item < itemCount; item++ {
		out.deltas[item] = make([]int16, regionIndexCount)
		for region := 0; region < regionIndexCount; region++ {
			if region < shortDeltaCount {
				if pos+2 > len(data) {
					return colrVariationData{}, ErrInvalidCOLRTable
				}
				out.deltas[item][region] = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
				pos += 2
			} else {
				if pos+1 > len(data) {
					return colrVariationData{}, ErrInvalidCOLRTable
				}
				out.deltas[item][region] = int16(int8(data[pos]))
				pos++
			}
		}
	}
	return out, nil
}

func (s *colrVariationStore) delta(varIndex uint32, coords []float64) (float64, bool) {
	if s == nil || varIndex == 0xFFFFFFFF || len(coords) == 0 {
		return 0, false
	}
	outer := int(varIndex >> 16)
	inner := int(varIndex & 0xFFFF)
	if outer < 0 || outer >= len(s.data) || inner < 0 || inner >= len(s.data[outer].deltas) {
		return 0, false
	}
	data := s.data[outer]
	var sum float64
	matched := false
	for i, regionIndex := range data.regionIndexes {
		if int(regionIndex) >= len(s.regions) || i >= len(data.deltas[inner]) {
			continue
		}
		scalar := s.regions[regionIndex].scalar(coords)
		if scalar == 0 {
			continue
		}
		sum += float64(data.deltas[inner][i]) * scalar
		matched = true
	}
	return sum, matched
}

func (r colrVariationRegion) scalar(coords []float64) float64 {
	scalar := 1.0
	for i, axis := range r.axes {
		coord := 0.0
		if i < len(coords) {
			coord = coords[i]
		}
		axisScalar := axis.scalar(coord)
		if axisScalar == 0 {
			return 0
		}
		scalar *= axisScalar
	}
	return scalar
}

func (a colrVariationAxis) scalar(coord float64) float64 {
	if a.peak == 0 || coord <= a.start || coord >= a.end {
		return 0
	}
	if coord == a.peak {
		return 1
	}
	if coord < a.peak {
		return (coord - a.start) / (a.peak - a.start)
	}
	return (a.end - coord) / (a.end - a.peak)
}
