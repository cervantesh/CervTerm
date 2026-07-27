package raster

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/image/font/sfnt"
)

const (
	fuzzKindSFNT = "sfnt-color-font"
	fuzzKindCOLR = "colr-table"
	fuzzKindCPAL = "cpal-table"
	fuzzKindSVG  = "svg-table"
)

type portableRasterSeed struct {
	name string
	kind string
}

var portableRasterSeeds = []portableRasterSeed{
	{name: "noto-color-emoji-smoke.ttf", kind: fuzzKindSFNT},
	{name: "colr-var-scale-table.bin", kind: fuzzKindCOLR},
	{name: "colr-composite-multiply-table.bin", kind: fuzzKindCOLR},
	{name: "cpal-red-green.bin", kind: fuzzKindCPAL},
	{name: "svg-gradient-table.bin", kind: fuzzKindSVG},
	{name: "svg-text-table.bin", kind: fuzzKindSVG},
}

func FuzzPortableRasterInputs(f *testing.F) {
	fixtures := loadPortableRasterSeedFixtures(f)
	for _, seed := range portableRasterSeeds {
		f.Add(seed.kind, fixtures[seed.name])
	}
	f.Add(fuzzKindSFNT, []byte{})
	f.Add(fuzzKindCOLR, []byte("\x00"))
	f.Add(fuzzKindCPAL, []byte("\x00"))
	f.Add(fuzzKindSVG, []byte("\x00"))

	cpal := fixtures["cpal-red-green.bin"]
	colr := fixtures["colr-var-scale-table.bin"]
	f.Fuzz(func(t *testing.T, kind string, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		exercisePortableRasterInput(kind, data, cpal, colr)
	})
}

func TestPortableRasterFuzzSeedInventory(t *testing.T) {
	want := []portableRasterSeed{
		{name: "noto-color-emoji-smoke.ttf", kind: fuzzKindSFNT},
		{name: "colr-var-scale-table.bin", kind: fuzzKindCOLR},
		{name: "colr-composite-multiply-table.bin", kind: fuzzKindCOLR},
		{name: "cpal-red-green.bin", kind: fuzzKindCPAL},
		{name: "svg-gradient-table.bin", kind: fuzzKindSVG},
		{name: "svg-text-table.bin", kind: fuzzKindSVG},
	}
	if !reflect.DeepEqual(portableRasterSeeds, want) {
		t.Fatalf("portable raster seed inventory = %#v, want %#v", portableRasterSeeds, want)
	}
	fixtures := loadPortableRasterSeedFixtures(t)
	cpal := fixtures["cpal-red-green.bin"]
	colr := fixtures["colr-var-scale-table.bin"]
	for _, seed := range portableRasterSeeds {
		routed, ok := exercisePortableRasterInput(seed.kind, fixtures[seed.name], cpal, colr)
		if !ok || routed != seed.kind {
			t.Fatalf("seed %s routed to %q ok=%t, want %q", seed.name, routed, ok, seed.kind)
		}
	}
	fontData := fixtures["noto-color-emoji-smoke.ttf"]
	tables, err := DetectColorTables(fontData)
	if err != nil || !tables.HasBitmapColor() {
		t.Fatalf("bitmap color TTF seed tables = %#v, err=%v", tables, err)
	}
}

func loadPortableRasterSeedFixtures(tb testing.TB) map[string][]byte {
	tb.Helper()
	fixtures := make(map[string][]byte, len(portableRasterSeeds))
	for _, seed := range portableRasterSeeds {
		if _, exists := fixtures[seed.name]; exists {
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", "testdata", seed.name))
		if err != nil {
			tb.Fatalf("read seed %s: %v", seed.name, err)
		}
		fixtures[seed.name] = data
	}
	return fixtures
}

func exercisePortableRasterInput(kind string, data, cpalFixture, colrFixture []byte) (string, bool) {
	switch kind {
	case fuzzKindSFNT:
		tables, err := DetectColorTables(data)
		if err != nil {
			return kind, false
		}
		parsed, err := sfnt.Parse(data)
		if err != nil {
			return kind, false
		}
		face, err := NewColorFace(data, parsed)
		return kind, err == nil && face != nil && tables.HasAnyColor()
	case fuzzKindCOLR:
		parser, err := newCOLRParser(data, cpalFixture)
		return kind, err == nil && parser != nil
	case fuzzKindCPAL:
		parser, err := newCOLRParser(colrFixture, data)
		return kind, err == nil && parser != nil
	case fuzzKindSVG:
		extractor, err := newSVGExtractor(data)
		return kind, err == nil && extractor != nil
	default:
		return "", false
	}
}
