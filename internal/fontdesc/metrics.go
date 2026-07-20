package fontdesc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	MetricScaleMinimum  = 0.5
	MetricScaleMaximum  = 3.0
	MetricOffsetMinimum = -64.0
	MetricOffsetMaximum = 64.0
	metricEncodingV1    = 1
)

type MetricProjectionID [32]byte

func (id MetricProjectionID) String() string { return fmt.Sprintf("%x", id[:]) }

// MetricProjection is a fixed-grid projection over natural font metrics.
type MetricProjection struct {
	LineHeight     float64
	CellWidth      float64
	BaselineOffset float64
	GlyphOffsetX   float64
	GlyphOffsetY   float64
}

func DefaultMetricProjection() MetricProjection {
	return MetricProjection{LineHeight: 1, CellWidth: 1}
}

func NewMetricProjection(lineHeight, cellWidth, baselineOffset, glyphOffsetX, glyphOffsetY float64) (MetricProjection, error) {
	projection := MetricProjection{
		LineHeight: normalizeMetricZero(lineHeight), CellWidth: normalizeMetricZero(cellWidth),
		BaselineOffset: normalizeMetricZero(baselineOffset), GlyphOffsetX: normalizeMetricZero(glyphOffsetX), GlyphOffsetY: normalizeMetricZero(glyphOffsetY),
	}
	if err := projection.Validate(); err != nil {
		return MetricProjection{}, err
	}
	return projection, nil
}

func (m MetricProjection) Validate() error {
	for _, field := range []struct {
		name  string
		value float64
	}{{"line_height", m.LineHeight}, {"cell_width", m.CellWidth}} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value < MetricScaleMinimum || field.value > MetricScaleMaximum {
			return fmt.Errorf("%s must be finite and between %.1f and %.1f", field.name, MetricScaleMinimum, MetricScaleMaximum)
		}
	}
	for _, field := range []struct {
		name  string
		value float64
	}{{"baseline_offset", m.BaselineOffset}, {"glyph_offset_x", m.GlyphOffsetX}, {"glyph_offset_y", m.GlyphOffsetY}} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value < MetricOffsetMinimum || field.value > MetricOffsetMaximum {
			return fmt.Errorf("%s must be finite and between %.0f and %.0f", field.name, MetricOffsetMinimum, MetricOffsetMaximum)
		}
	}
	return nil
}

func (m MetricProjection) CanonicalBytes() []byte {
	if err := m.Validate(); err != nil {
		return nil
	}
	var out bytes.Buffer
	out.WriteByte(metricEncodingV1)
	for _, value := range []float64{m.LineHeight, m.CellWidth, m.BaselineOffset, m.GlyphOffsetX, m.GlyphOffsetY} {
		_ = binary.Write(&out, binary.BigEndian, math.Float64bits(normalizeMetricZero(value)))
	}
	return out.Bytes()
}

func (m MetricProjection) ID() (MetricProjectionID, error) {
	if err := m.Validate(); err != nil {
		return MetricProjectionID{}, err
	}
	return MetricProjectionID(sha256.Sum256(m.CanonicalBytes())), nil
}

// ProjectCellMetrics returns deterministic integer fixed-grid geometry. The
// reported baseline is clipped to the cell; ink offsets are applied separately.
func (m MetricProjection) ProjectCellMetrics(naturalW, naturalH, naturalBaseline int) (cellW, cellH, baseline int) {
	cellW = max(1, int(math.Round(float64(naturalW)*m.CellWidth)))
	cellH = max(1, int(math.Round(float64(naturalH)*m.LineHeight)))
	baseline = naturalBaseline + (cellH-naturalH)/2 + int(math.Round(m.BaselineOffset))
	baseline = max(0, min(cellH, baseline))
	return cellW, cellH, baseline
}

func normalizeMetricZero(value float64) float64 {
	if value == 0 {
		return 0
	}
	return value
}
