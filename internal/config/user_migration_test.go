package config

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type userMigrationManifest struct {
	ID                   string   `json:"id"`
	SourceKind           string   `json:"source_kind"`
	Provenance           string   `json:"provenance"`
	EffectiveEquivalence bool     `json:"effective_equivalence"`
	Unsupported          []string `json:"unsupported"`
}

type weztermTranslationExpected struct {
	Window struct {
		InitialCols       int     `json:"initial_cols"`
		InitialRows       int     `json:"initial_rows"`
		Decorations       string  `json:"decorations"`
		Titlebar          string  `json:"titlebar"`
		Padding           [4]int  `json:"padding"`
		Opacity           float64 `json:"opacity"`
		TextOpacity       float64 `json:"text_opacity"`
		BackgroundOpacity float64 `json:"background_opacity"`
	} `json:"window"`
	Font struct {
		Family     string   `json:"family"`
		Fallback   []string `json:"fallback"`
		Size       float64  `json:"size"`
		LineHeight float64  `json:"line_height"`
	} `json:"font"`
	Colors struct {
		Foreground          string `json:"foreground"`
		Background          string `json:"background"`
		Cursor              string `json:"cursor"`
		SelectionBackground string `json:"selection_background"`
	} `json:"colors"`
	ScrollbackLines int `json:"scrollback_lines"`
	Scrollbar       struct {
		Mode         string `json:"mode"`
		StableGutter bool   `json:"stable_gutter"`
		MinThumbPX   int    `json:"min_thumb_px"`
		ThumbColor   string `json:"thumb_color"`
	} `json:"scrollbar"`
	TabBar struct {
		Mode       string `json:"mode"`
		Position   string `json:"position"`
		MaxWidthPX int    `json:"max_width_px"`
	} `json:"tab_bar"`
	Cursor struct {
		Shape string `json:"shape"`
		Blink bool   `json:"blink"`
	} `json:"cursor"`
	BellMode string `json:"bell_mode"`
	MaxFPS   int    `json:"max_fps"`
}

type migrationFileSnapshot struct {
	Mode os.FileMode
	Data []byte
}

func TestUserMigrationCorpus(t *testing.T) {
	root := filepath.Join("testdata", "user-migration")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]string{
		"cervterm-v1-daily-driver": "cervterm-v1",
		"wezterm-daily-driver":     "wezterm",
	}
	if len(entries) != len(required) {
		t.Fatalf("real-user migration corpus has %d examples, want exactly %d", len(entries), len(required))
	}
	seen := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("unexpected corpus file %q", entry.Name())
		}
		t.Run(entry.Name(), func(t *testing.T) {
			directory := filepath.Join(root, entry.Name())
			assertSanitizedMigrationCorpus(t, directory)
			manifest := readUserMigrationManifest(t, filepath.Join(directory, "manifest.json"))
			wantKind, requiredCase := required[manifest.ID]
			if !requiredCase || manifest.ID != entry.Name() || manifest.SourceKind != wantKind || strings.TrimSpace(manifest.Provenance) == "" {
				t.Fatalf("manifest identity/provenance=%#v", manifest)
			}
			if _, duplicate := seen[manifest.ID]; duplicate {
				t.Fatalf("duplicate migration id %q", manifest.ID)
			}
			seen[manifest.ID] = manifest.SourceKind

			afterPath := filepath.Join(directory, "after.lua")
			migrated, err := LoadLua(afterPath, Defaults())
			if err != nil {
				t.Fatalf("v2 after template: %v", err)
			}
			switch manifest.SourceKind {
			case "cervterm-v1":
				if !manifest.EffectiveEquivalence || len(manifest.Unsupported) != 0 {
					t.Fatalf("CervTerm v1 manifest must claim exact supported equivalence: %#v", manifest)
				}
				assertReadOnlyEquivalentMigration(t, filepath.Join(directory, "before.lua"), afterPath)
			case "wezterm":
				if manifest.EffectiveEquivalence || len(manifest.Unsupported) == 0 {
					t.Fatalf("cross-terminal translation must list non-equivalent surfaces: %#v", manifest)
				}
				assertWeztermTranslation(t, migrated, readWeztermExpected(t, filepath.Join(directory, "expected.json")))
			}
		})
	}
	if !reflect.DeepEqual(seen, required) {
		t.Fatalf("corpus cases=%#v want %#v", seen, required)
	}
}

func assertReadOnlyEquivalentMigration(t *testing.T, beforePath, afterPath string) {
	t.Helper()
	beforeBytes, err := os.ReadFile(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "cervterm.lua")
	if err := os.WriteFile(targetPath, beforeBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "adjacent.keep"), []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeSnapshot := snapshotMigrationDirectory(t, targetDir)
	legacy, err := LoadLua(targetPath, Defaults())
	if err != nil {
		t.Fatalf("load v1 copy: %v", err)
	}
	migrated, err := LoadLua(afterPath, Defaults())
	if err != nil {
		t.Fatalf("load v2 template: %v", err)
	}
	if !reflect.DeepEqual(legacy, migrated) {
		t.Fatalf("effective configuration changed\nv1=%#v\nv2=%#v", legacy, migrated)
	}
	if afterSnapshot := snapshotMigrationDirectory(t, targetDir); !reflect.DeepEqual(beforeSnapshot, afterSnapshot) {
		t.Fatalf("migration loader changed source directory\nbefore=%#v\nafter=%#v", beforeSnapshot, afterSnapshot)
	}
}

func snapshotMigrationDirectory(t *testing.T, directory string) map[string]migrationFileSnapshot {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make(map[string]migrationFileSnapshot, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		snapshot[entry.Name()] = migrationFileSnapshot{Mode: info.Mode(), Data: data}
	}
	return snapshot
}

func assertSanitizedMigrationCorpus(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("nested corpus directory %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		lower := bytes.ToLower(data)
		for _, private := range []string{"userprofile", "powershell", "\\.pi\\", "c___h", "api_token", "secret_value", "c:\\users\\", "/home/"} {
			if bytes.Contains(lower, []byte(private)) {
				t.Fatalf("sanitized corpus file %s contains private token %q", entry.Name(), private)
			}
		}
	}
}

func assertWeztermTranslation(t *testing.T, cfg Config, want weztermTranslationExpected) {
	t.Helper()
	if cfg.Window.InitialCols != want.Window.InitialCols || cfg.Window.InitialRows != want.Window.InitialRows || cfg.Window.Decorations != want.Window.Decorations || cfg.Window.Titlebar != want.Window.Titlebar || cfg.Window.Opacity != want.Window.Opacity || cfg.Window.TextOpacity != want.Window.TextOpacity || cfg.Window.BackgroundOpacity != want.Window.BackgroundOpacity {
		t.Fatalf("window translation=%#v want %#v", cfg.Window, want.Window)
	}
	padding := [4]int{cfg.Window.PaddingLeft, cfg.Window.PaddingRight, cfg.Window.PaddingTop, cfg.Window.PaddingBottom}
	if padding != want.Window.Padding {
		t.Fatalf("padding=%v want %v", padding, want.Window.Padding)
	}
	fallback := make([]string, len(cfg.Font.Fallback))
	for index, descriptor := range cfg.Font.Fallback {
		fallback[index] = descriptor.Family
	}
	if cfg.Font.Family != want.Font.Family || !reflect.DeepEqual(fallback, want.Font.Fallback) || cfg.Font.Size != want.Font.Size || cfg.Font.LineHeight != want.Font.LineHeight {
		t.Fatalf("font translation=%#v fallback=%#v want %#v", cfg.Font, fallback, want.Font)
	}
	if cfg.Colors.Foreground != want.Colors.Foreground || cfg.Colors.Background != want.Colors.Background || cfg.Colors.Cursor != want.Colors.Cursor || cfg.Colors.SelectionBackground != want.Colors.SelectionBackground {
		t.Fatalf("color translation=%#v want %#v", cfg.Colors, want.Colors)
	}
	if cfg.Scrolling.History != want.ScrollbackLines || cfg.Scrollbar.Mode != want.Scrollbar.Mode || cfg.Scrollbar.StableGutter != want.Scrollbar.StableGutter || cfg.Scrollbar.MinThumbPX != want.Scrollbar.MinThumbPX || cfg.Scrollbar.ThumbColor != want.Scrollbar.ThumbColor {
		t.Fatalf("scroll translation scrolling=%#v scrollbar=%#v", cfg.Scrolling, cfg.Scrollbar)
	}
	if cfg.TabBar.Mode != want.TabBar.Mode || cfg.TabBar.Position != want.TabBar.Position || cfg.TabBar.MaxWidthPX != want.TabBar.MaxWidthPX || cfg.Cursor.Shape != want.Cursor.Shape || cfg.Cursor.Blink != want.Cursor.Blink || cfg.Bell.Mode != want.BellMode || cfg.Render.MaxFPS != want.MaxFPS {
		t.Fatalf("UX translation tab=%#v cursor=%#v bell=%#v render=%#v", cfg.TabBar, cfg.Cursor, cfg.Bell, cfg.Render)
	}
}

func readUserMigrationManifest(t *testing.T, path string) userMigrationManifest {
	t.Helper()
	var manifest userMigrationManifest
	readStrictMigrationJSON(t, path, &manifest)
	return manifest
}

func readWeztermExpected(t *testing.T, path string) weztermTranslationExpected {
	t.Helper()
	var expected weztermTranslationExpected
	readStrictMigrationJSON(t, path, &expected)
	return expected
}

func readStrictMigrationJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("JSON must contain exactly one document")
	}
}
