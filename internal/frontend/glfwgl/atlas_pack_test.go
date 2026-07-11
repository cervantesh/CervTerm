package glfwgl

import "testing"

func TestShelfPacker(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		inserts    [][2]int
		wantOK     []bool
		resetReuse bool
	}{
		{"fills shelf", 8, 8, [][2]int{{3, 2}, {5, 2}}, []bool{true, true}, false},
		{"exact fit", 8, 8, [][2]int{{8, 8}}, []bool{true}, false},
		{"overflow", 8, 8, [][2]int{{8, 8}, {1, 1}}, []bool{true, false}, false},
		{"reset reuse", 8, 8, [][2]int{{8, 8}}, []bool{true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newShelfPacker(tt.width, tt.height)
			for i, size := range tt.inserts {
				_, _, ok := p.Insert(size[0], size[1])
				if ok != tt.wantOK[i] {
					t.Fatalf("insert %d ok = %v, want %v", i, ok, tt.wantOK[i])
				}
			}
			if tt.resetReuse {
				p.Reset()
				if _, _, ok := p.Insert(8, 8); !ok {
					t.Fatal("Insert after Reset failed")
				}
			}
		})
	}
}

func TestEntryGenerationInvalidation(t *testing.T) {
	tests := []struct {
		name            string
		entryGeneration uint64
		atlasGeneration uint64
		want            bool
	}{
		{"current", 4, 4, true},
		{"evicted", 3, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entryGenerationValid(tt.entryGeneration, tt.atlasGeneration); got != tt.want {
				t.Fatalf("entryGenerationValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
