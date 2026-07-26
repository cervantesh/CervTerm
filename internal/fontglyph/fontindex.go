package fontglyph

import (
	"sync"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/discovery"
)

// Compatibility facade: discovery/index ownership lives in discovery while the
// root package retains its concrete legacy type identity and method set.
type faceInfo struct {
	path      string
	index     int
	family    string
	subfamily string
	metadata  fontdesc.FaceMetadata
}

type FontIndex struct {
	discovery *discovery.Index
}

// FontIndexDiagnostics preserves the legacy root package identity while the
// discovery package owns the authoritative index and accounting.
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

// FontResolution preserves the legacy root result identity at the facade.
type FontResolution struct {
	Configured          string
	Found               bool
	Regular             string
	Bold                string
	Italic              string
	BoldItalic          string
	FaceIndex           int
	RegularFaceIndex    int
	BoldFaceIndex       int
	ItalicFaceIndex     int
	BoldItalicFaceIndex int
}

func BuildFontIndex(dirs []string) *FontIndex {
	return wrapDiscoveryIndex(discovery.Build(dirs))
}

func wrapDiscoveryIndex(index *discovery.Index) *FontIndex {
	if index == nil {
		return nil
	}
	return &FontIndex{discovery: index}
}

func (index *FontIndex) Diagnostics() FontIndexDiagnostics {
	if index == nil || index.discovery == nil {
		return FontIndexDiagnostics{}
	}
	return rootFontIndexDiagnostics(index.discovery.Diagnostics())
}

func rootFontIndexDiagnostics(value discovery.Diagnostics) FontIndexDiagnostics {
	return FontIndexDiagnostics{
		Roots: value.Roots, CandidateFiles: value.CandidateFiles, SelectedFiles: value.SelectedFiles, FilesTruncated: value.FilesTruncated,
		FacesExamined: value.FacesExamined, FacesIndexed: value.FacesIndexed, FacesTruncated: value.FacesTruncated, FilesSkipped: value.FilesSkipped,
		DuplicateFiles: value.DuplicateFiles, SymlinkDirectoriesSkipped: value.SymlinkDirectoriesSkipped, SymlinkFilesSkipped: value.SymlinkFilesSkipped,
	}
}

func (index *FontIndex) Lookup(family string) (regular, bold, italic, boldItalic *faceInfo) {
	if index == nil || index.discovery == nil {
		return nil, nil, nil, nil
	}
	discoveredRegular, discoveredBold, discoveredItalic, discoveredBoldItalic := index.discovery.Lookup(family)
	return rootFaceInfo(discoveredRegular), rootFaceInfo(discoveredBold), rootFaceInfo(discoveredItalic), rootFaceInfo(discoveredBoldItalic)
}

func rootFaceInfo(discovered *discovery.Face) *faceInfo {
	if discovered == nil {
		return nil
	}
	value := faceInfo{
		path: discovered.Path(), index: discovered.Index(), family: discovered.Family(),
		subfamily: discovered.Subfamily(), metadata: discovered.Metadata(),
	}
	return &value
}

func fontIndexFaces(index *FontIndex, family string) []faceInfo {
	if index == nil || index.discovery == nil {
		return nil
	}
	discovered := index.discovery.Faces(family)
	faces := make([]faceInfo, len(discovered))
	for i := range discovered {
		faces[i] = faceInfo{
			path: discovered[i].Path(), index: discovered[i].Index(), family: discovered[i].Family(),
			subfamily: discovered[i].Subfamily(), metadata: discovered[i].Metadata(),
		}
	}
	return faces
}

var (
	rootSystemIndexOnce sync.Once
	rootSystemIndex     *FontIndex
)

func loadSystemFontIndex() *FontIndex {
	rootSystemIndexOnce.Do(func() { rootSystemIndex = wrapDiscoveryIndex(discovery.SystemIndex()) })
	return rootSystemIndex
}

func ResolveSystemFont(family string) FontResolution {
	return rootFontResolution(discovery.ResolveSystemFont(family))
}
func rootFontResolution(value discovery.Resolution) FontResolution {
	return FontResolution{
		Configured: value.Configured, Found: value.Found, Regular: value.Regular, Bold: value.Bold, Italic: value.Italic, BoldItalic: value.BoldItalic,
		FaceIndex: value.FaceIndex, RegularFaceIndex: value.RegularFaceIndex, BoldFaceIndex: value.BoldFaceIndex,
		ItalicFaceIndex: value.ItalicFaceIndex, BoldItalicFaceIndex: value.BoldItalicFaceIndex,
	}
}
func systemFontDirs() []string { return discovery.SystemFontDirs() }
func isEmbeddedFamily(family string) bool {
	normalized := normalizeFamily(family)
	return normalized == "" || normalized == "go mono"
}
