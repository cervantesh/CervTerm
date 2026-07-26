// Package discovery owns bounded font-file discovery and the immutable family
// index. It depends only on the stable font descriptor vocabulary.
package discovery

import "cervterm/internal/fontdesc"

// Face is one stable discovered source#index identity and its normalized
// descriptor metadata.
type Face struct {
	path      string
	index     int
	family    string
	subfamily string
	metadata  fontdesc.FaceMetadata
}

// NewFace constructs a detached discovered face value.
func NewFace(path string, index int, family, subfamily string, metadata fontdesc.FaceMetadata) Face {
	return Face{path: path, index: index, family: family, subfamily: subfamily, metadata: metadata}
}

func (f Face) Path() string                    { return f.path }
func (f Face) Index() int                      { return f.index }
func (f Face) Family() string                  { return f.family }
func (f Face) Subfamily() string               { return f.subfamily }
func (f Face) Metadata() fontdesc.FaceMetadata { return f.metadata }

// Diagnostics summarizes bounded discovery. DuplicateFiles is intentionally
// bounded to identities retained by deterministic top-K selection.
type Diagnostics struct {
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

// Index is an immutable family index after construction.
type Index struct {
	families    map[string][]Face
	diagnostics Diagnostics
}

// Resolution preserves the fontglyph compatibility facade's path-and-index
// result without performing face loading.
type Resolution struct {
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
