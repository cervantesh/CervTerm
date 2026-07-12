package render

import (
	"slices"
	"testing"
)

func TestCoverageLUTIdentity(t *testing.T) {
	lut := CoverageLUT(1, 0)
	for i, got := range lut {
		if got != uint8(i) {
			t.Fatalf("lut[%d] = %d, want %d", i, got, i)
		}
	}
}

func TestCoverageLUTEndpointsAndMonotonicity(t *testing.T) {
	tests := []struct {
		name          string
		gamma, darken float64
	}{
		{"defaults", 1.4, 0.1},
		{"full gamma", 2.2, 0},
		{"low gamma darkened", 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lut := CoverageLUT(tt.gamma, tt.darken)
			if lut[0] != 0 || lut[255] != 255 {
				t.Fatalf("endpoints = (%d, %d), want (0, 255)", lut[0], lut[255])
			}
			for i := 1; i < len(lut); i++ {
				if lut[i] < lut[i-1] {
					t.Fatalf("lut is decreasing at %d: %d < %d", i, lut[i], lut[i-1])
				}
			}
		})
	}
}

func TestCoverageLUTGammaBrightensMidtones(t *testing.T) {
	lut := CoverageLUT(2.2, 0)
	for _, coverage := range []int{64, 128} {
		if lut[coverage] <= uint8(coverage) {
			t.Errorf("lut[%d] = %d, want brighter than %d", coverage, lut[coverage], coverage)
		}
	}
}

func TestCoverageLUTDarkenRaisesLowCoverage(t *testing.T) {
	lut := CoverageLUT(1, 0.2)
	if got := int(lut[100]); got < 119 || got > 121 {
		t.Fatalf("lut[100] = %d, want 120 +/- 1", got)
	}
}

func TestApplyCoverageLUT(t *testing.T) {
	tests := []struct {
		name string
		lut  [256]uint8
		pix  []uint8
		want []uint8
	}{
		{"all channels", CoverageLUT(2, 0), []uint8{16, 64, 128, 255}, []uint8{64, 128, 181, 255}},
		{"identity", CoverageLUT(1, 0), []uint8{0, 17, 128, 255}, []uint8{0, 17, 128, 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLength := len(tt.pix)
			ApplyCoverageLUT(tt.pix, &tt.lut)
			if len(tt.pix) != originalLength {
				t.Fatalf("length = %d, want %d", len(tt.pix), originalLength)
			}
			if !slices.Equal(tt.pix, tt.want) {
				t.Fatalf("pixels = %v, want %v", tt.pix, tt.want)
			}
		})
	}
}
