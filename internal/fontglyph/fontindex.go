package fontglyph

import (
	"container/heap"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/sfnt"
)

type faceInfo struct {
	path      string
	index     int
	family    string
	subfamily string
}

// FontIndexDiagnostics summarizes bounded discovery without changing the
// legacy BuildFontIndex and ResolveSystemFont APIs. DuplicateFiles is a bounded
// diagnostic: it counts duplicates of identities currently retained by top-K
// selection and deliberately does not require an unbounded global seen set.
type FontIndexDiagnostics struct {
	Roots                     int
	CandidateFiles            int
	SelectedFiles             int
	FilesTruncated            int
	FacesExamined             int
	FacesIndexed              int
	FacesTruncated            int
	FilesSkipped              int
	DuplicateFiles            int
	SymlinkDirectoriesSkipped int
	SymlinkFilesSkipped       int
}

type FontIndex struct {
	families    map[string][]faceInfo
	diagnostics FontIndexDiagnostics
}

type FontResolution struct {
	Configured          string
	Found               bool
	Regular             string
	Bold                string
	Italic              string
	BoldItalic          string
	FaceIndex           int // legacy alias for RegularFaceIndex
	RegularFaceIndex    int
	BoldFaceIndex       int
	ItalicFaceIndex     int
	BoldItalicFaceIndex int
}

func BuildFontIndex(dirs []string) *FontIndex {
	index := &FontIndex{families: make(map[string][]faceInfo)}
	roots := canonicalDiscoveryRoots(dirs, &index.diagnostics)
	index.diagnostics.Roots = len(roots)
	selector := newTopKPathSelector(fontdesc.MaxDiscoveryFiles)
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				index.diagnostics.FilesSkipped++
				return nil
			}
			if path == root {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				targetInfo, err := os.Stat(path)
				if err != nil {
					index.diagnostics.SymlinkFilesSkipped++
					return nil
				}
				if targetInfo.IsDir() {
					index.diagnostics.SymlinkDirectoriesSkipped++
					return nil
				}
				target, err := filepath.EvalSymlinks(path)
				if err != nil || !pathWithinRoots(target, roots) || !isFontFile(target) {
					index.diagnostics.SymlinkFilesSkipped++
					return nil
				}
				addDiscoveryCandidate(target, selector, &index.diagnostics)
				return nil
			}
			if entry.IsDir() || !isFontFile(path) {
				return nil
			}
			addDiscoveryCandidate(path, selector, &index.diagnostics)
			return nil
		})
	}
	paths := selector.sorted()
	index.diagnostics.SelectedFiles = len(paths)
	index.diagnostics.FilesTruncated = max(0, index.diagnostics.CandidateFiles-len(paths))
	for _, path := range paths {
		remaining := fontdesc.MaxDiscoveryFaces - index.diagnostics.FacesExamined
		if remaining <= 0 {
			break
		}
		faces, examined, truncated, skipped := fontFacesBounded(path, min(fontdesc.MaxFacesPerFile, remaining))
		index.diagnostics.FacesExamined += examined
		index.diagnostics.FacesTruncated += truncated
		if skipped {
			index.diagnostics.FilesSkipped++
			continue
		}
		for _, info := range faces {
			key := normalizeFamily(info.family)
			if key != "" {
				index.families[key] = append(index.families[key], info)
				index.diagnostics.FacesIndexed++
			}
		}
	}
	return index
}

func (index *FontIndex) Diagnostics() FontIndexDiagnostics {
	if index == nil {
		return FontIndexDiagnostics{}
	}
	return index.diagnostics
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

func canonicalDiscoveryRoots(dirs []string, diagnostics *FontIndexDiagnostics) []string {
	seen := make(map[string]struct{})
	roots := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			diagnostics.FilesSkipped++
			continue
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			diagnostics.FilesSkipped++
			continue
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			diagnostics.FilesSkipped++
			continue
		}
		canonical = filepath.Clean(canonical)
		key := discoveryPathKey(canonical)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		roots = append(roots, canonical)
	}
	sort.Slice(roots, func(i, j int) bool { return compareDiscoveryPaths(roots[i], roots[j]) < 0 })
	return roots
}

func discoveryPathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func compareDiscoveryPaths(a, b string) int {
	aKey, bKey := discoveryPathKey(a), discoveryPathKey(b)
	if aKey < bKey {
		return -1
	}
	if aKey > bKey {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func pathWithinRoots(path string, roots []string) bool {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		relative, err := filepath.Rel(root, canonical)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func addDiscoveryCandidate(path string, selector *topKPathSelector, diagnostics *FontIndexDiagnostics) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		diagnostics.FilesSkipped++
		return
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		diagnostics.FilesSkipped++
		return
	}
	canonical = filepath.Clean(canonical)
	if selector.add(canonical) {
		diagnostics.DuplicateFiles++
		return
	}
	diagnostics.CandidateFiles++
}

type selectedPath struct {
	key  string
	path string
}

type maxPathHeap []selectedPath

func (paths maxPathHeap) Len() int { return len(paths) }
func (paths maxPathHeap) Less(i, j int) bool {
	return compareDiscoveryPaths(paths[i].path, paths[j].path) > 0
}
func (paths maxPathHeap) Swap(i, j int)   { paths[i], paths[j] = paths[j], paths[i] }
func (paths *maxPathHeap) Push(value any) { *paths = append(*paths, value.(selectedPath)) }
func (paths *maxPathHeap) Pop() any {
	old := *paths
	last := old[len(old)-1]
	*paths = old[:len(old)-1]
	return last
}

type topKPathSelector struct {
	limit int
	paths maxPathHeap
	keys  map[string]struct{}
}

func newTopKPathSelector(limit int) *topKPathSelector {
	selector := &topKPathSelector{limit: max(0, limit), keys: make(map[string]struct{}, max(0, limit))}
	heap.Init(&selector.paths)
	return selector
}

// add reports whether path duplicates one of the at-most-K selected identities.
func (selector *topKPathSelector) add(path string) bool {
	if selector.limit == 0 {
		return false
	}
	key := discoveryPathKey(path)
	if _, exists := selector.keys[key]; exists {
		return true
	}
	candidate := selectedPath{key: key, path: path}
	if selector.paths.Len() < selector.limit {
		heap.Push(&selector.paths, candidate)
		selector.keys[key] = struct{}{}
		return false
	}
	if compareDiscoveryPaths(path, selector.paths[0].path) < 0 {
		delete(selector.keys, selector.paths[0].key)
		selector.paths[0] = candidate
		selector.keys[key] = struct{}{}
		heap.Fix(&selector.paths, 0)
	}
	return false
}

func (selector *topKPathSelector) sorted() []string {
	selected := append([]selectedPath(nil), selector.paths...)
	sort.Slice(selected, func(i, j int) bool { return compareDiscoveryPaths(selected[i].path, selected[j].path) < 0 })
	paths := make([]string, len(selected))
	for i := range selected {
		paths[i] = selected[i].path
	}
	return paths
}

func selectTopKPaths(paths []string, limit int) []string {
	selector := newTopKPathSelector(limit)
	for _, path := range paths {
		selector.add(path)
	}
	return selector.sorted()
}

func fontFaces(path string) []faceInfo {
	faces, _, _, _ := fontFacesBounded(path, fontdesc.MaxFacesPerFile)
	return faces
}

// fontFacesBounded examines at most limit collection faces. examined counts
// every attempted face, including parse failures and faces without usable names.
func fontFacesBounded(path string, limit int) (faces []faceInfo, examined, truncated int, skipped bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, true
	}
	defer file.Close()
	collection, err := sfnt.ParseCollectionReaderAt(file)
	if err != nil {
		return nil, 0, 0, true
	}
	count := min(collection.NumFonts(), max(0, limit))
	faces = make([]faceInfo, 0, count)
	for i := 0; i < count; i++ {
		examined++
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
	return faces, examined, max(0, collection.NumFonts()-count), false
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
	resolution.Regular, resolution.FaceIndex, resolution.RegularFaceIndex = regular.path, regular.index, regular.index
	if bold != nil {
		resolution.Bold, resolution.BoldFaceIndex = bold.path, bold.index
	}
	if italic != nil {
		resolution.Italic, resolution.ItalicFaceIndex = italic.path, italic.index
	}
	if boldItalic != nil {
		resolution.BoldItalic, resolution.BoldItalicFaceIndex = boldItalic.path, boldItalic.index
	}
	return resolution
}

func isEmbeddedFamily(family string) bool {
	normalized := normalizeFamily(family)
	return normalized == "" || normalized == "go mono"
}
