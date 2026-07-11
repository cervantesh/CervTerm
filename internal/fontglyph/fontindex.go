package fontglyph

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font/sfnt"
)

type faceInfo struct {
	path      string
	index     int
	family    string
	subfamily string
}

type FontIndex struct {
	families map[string][]faceInfo
}

type FontResolution struct {
	Configured string
	Found      bool
	Regular    string
	Bold       string
	Italic     string
	BoldItalic string
	FaceIndex  int
}

func BuildFontIndex(dirs []string) *FontIndex {
	index := &FontIndex{families: make(map[string][]faceInfo)}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !isFontFile(path) {
				return nil
			}
			for _, info := range fontFaces(path) {
				key := normalizeFamily(info.family)
				if key != "" {
					index.families[key] = append(index.families[key], info)
				}
			}
			return nil
		})
	}
	return index
}

func (index *FontIndex) Lookup(family string) (regular, bold, italic, boldItalic *faceInfo) {
	faces := index.families[normalizeFamily(family)]
	for i := range faces {
		face := &faces[i]
		isBold, isItalic := classifySubfamily(face.subfamily)
		switch {
		case isBold && isItalic && boldItalic == nil:
			boldItalic = face
		case isBold && bold == nil:
			bold = face
		case isItalic && italic == nil:
			italic = face
		case !isBold && !isItalic && regular == nil:
			regular = face
		}
	}
	if regular == nil && len(faces) > 0 {
		regular = &faces[0]
	}
	return regular, bold, italic, boldItalic
}

func normalizeFamily(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func classifySubfamily(value string) (bold, italic bool) {
	normalized := normalizeFamily(value)
	return strings.Contains(normalized, "bold"), strings.Contains(normalized, "italic") || strings.Contains(normalized, "oblique")
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc":
		return true
	default:
		return false
	}
}

func fontFaces(path string) []faceInfo {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	collection, err := sfnt.ParseCollectionReaderAt(file)
	if err != nil {
		return nil
	}
	faces := make([]faceInfo, 0, collection.NumFonts())
	for i := 0; i < collection.NumFonts(); i++ {
		font, err := collection.Font(i)
		if err != nil {
			continue
		}
		family := fontName(font, sfnt.NameIDTypographicFamily, sfnt.NameIDFamily)
		if family == "" {
			continue
		}
		faces = append(faces, faceInfo{
			path: path, index: i, family: family,
			subfamily: fontName(font, sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily),
		})
	}
	return faces
}

func fontName(font *sfnt.Font, preferred, fallback sfnt.NameID) string {
	var buffer sfnt.Buffer
	if name, err := font.Name(&buffer, preferred); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if name, err := font.Name(&buffer, fallback); err == nil {
		return strings.TrimSpace(name)
	}
	return ""
}

func systemFontDirs() []string {
	if runtime.GOOS == "windows" {
		dirs := []string{filepath.Join(os.Getenv("SystemRoot"), "Fonts")}
		if dirs[0] == "Fonts" {
			dirs[0] = `C:\Windows\Fonts`
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
		return dirs
	}
	home, _ := os.UserHomeDir()
	dirs := []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".fonts"))
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		dirs = append(dirs, filepath.Join(dataHome, "fonts"))
	} else if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"))
	}
	return dirs
}

var (
	systemIndexOnce sync.Once
	systemIndex     *FontIndex
)

func loadSystemFontIndex() *FontIndex {
	systemIndexOnce.Do(func() {
		started := time.Now()
		systemIndex = BuildFontIndex(systemFontDirs())
		log.Printf("system font index scan completed in %s", time.Since(started).Round(time.Millisecond))
	})
	return systemIndex
}

func ResolveSystemFont(family string) FontResolution {
	resolution := FontResolution{Configured: family}
	if isEmbeddedFamily(family) {
		resolution.Found = true
		resolution.Regular = "embedded Go Mono"
		return resolution
	}
	regular, bold, italic, boldItalic := loadSystemFontIndex().Lookup(family)
	if regular == nil {
		return resolution
	}
	resolution.Found = true
	resolution.Regular, resolution.FaceIndex = regular.path, regular.index
	if bold != nil {
		resolution.Bold = bold.path
	}
	if italic != nil {
		resolution.Italic = italic.path
	}
	if boldItalic != nil {
		resolution.BoldItalic = boldItalic.path
	}
	return resolution
}

func isEmbeddedFamily(family string) bool {
	normalized := normalizeFamily(family)
	return normalized == "" || normalized == "go mono"
}
