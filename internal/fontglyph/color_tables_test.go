package fontglyph

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestDetectColorTables(t *testing.T) {
	font := fakeSFNT("head", "CBDT", "CBLC", "COLR", "CPAL", "SVG ")
	tables, err := DetectColorTables(font)
	if err != nil {
		t.Fatalf("DetectColorTables: %v", err)
	}
	if !tables.HasCBDT || !tables.HasCBLC || !tables.HasCOLR || !tables.HasCPAL || !tables.HasSVG {
		t.Fatalf("missing detected tables: %#v", tables)
	}
	if !tables.HasBitmapColor() || !tables.HasLayerColor() || !tables.HasAnyColor() {
		t.Fatalf("unexpected color support flags: %#v", tables)
	}
	if got := tables.PreferredFormat(); got != "CBDT/CBLC" {
		t.Fatalf("preferred format = %q, want CBDT/CBLC", got)
	}
}

func TestDetectColorTablesCOLRVersion(t *testing.T) {
	tables, err := DetectColorTables(fakeSFNTWithTableData(map[string][]byte{
		"COLR": {0, 1},
		"CPAL": {0, 0},
	}))
	if err != nil {
		t.Fatalf("DetectColorTables: %v", err)
	}
	if !tables.HasCOLRVersion || tables.COLRVersion != 1 {
		t.Fatalf("COLR version = (%v,%d), want (true,1)", tables.HasCOLRVersion, tables.COLRVersion)
	}
	if !tables.HasRenderableLayerColor() {
		t.Fatalf("COLRv1+CPAL should enter the subset render path")
	}

	tables, err = DetectColorTables(fakeSFNTWithTableData(map[string][]byte{
		"COLR": {0, 0},
		"CPAL": {0, 0},
	}))
	if err != nil {
		t.Fatalf("DetectColorTables: %v", err)
	}
	if !tables.HasRenderableLayerColor() {
		t.Fatalf("COLRv0+CPAL should be marked renderable")
	}
}

func TestDetectColorTablesPreferredFormatFallbacks(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{name: "sbix", tags: []string{"sbix"}, want: "sbix"},
		{name: "colr", tags: []string{"COLR", "CPAL"}, want: "COLR/CPAL"},
		{name: "svg", tags: []string{"SVG "}, want: "SVG"},
		{name: "none", tags: []string{"head"}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tables, err := DetectColorTables(fakeSFNT(tc.tags...))
			if err != nil {
				t.Fatalf("DetectColorTables: %v", err)
			}
			if got := tables.PreferredFormat(); got != tc.want {
				t.Fatalf("preferred format = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectColorTablesInvalidData(t *testing.T) {
	_, err := DetectColorTables([]byte{0, 1, 2})
	if !errors.Is(err, ErrInvalidFontData) {
		t.Fatalf("expected ErrInvalidFontData, got %v", err)
	}
}

func fakeSFNT(tags ...string) []byte {
	data := make([]byte, 12+len(tags)*16)
	copy(data[0:4], []byte{0x00, 0x01, 0x00, 0x00})
	binary.BigEndian.PutUint16(data[4:6], uint16(len(tags)))
	for i, tag := range tags {
		offset := 12 + i*16
		copy(data[offset:offset+4], []byte(tag))
	}
	return data
}

func fakeSFNTWithTableData(tables map[string][]byte) []byte {
	data := make([]byte, 12+len(tables)*16)
	copy(data[0:4], []byte{0x00, 0x01, 0x00, 0x00})
	binary.BigEndian.PutUint16(data[4:6], uint16(len(tables)))
	payloadOffset := 12 + len(tables)*16
	i := 0
	for tag, payload := range tables {
		dirOffset := 12 + i*16
		copy(data[dirOffset:dirOffset+4], []byte(tag))
		binary.BigEndian.PutUint32(data[dirOffset+8:dirOffset+12], uint32(payloadOffset))
		binary.BigEndian.PutUint32(data[dirOffset+12:dirOffset+16], uint32(len(payload)))
		data = append(data, payload...)
		payloadOffset += len(payload)
		i++
	}
	return data
}
