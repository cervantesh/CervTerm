package fontglyph

import (
	"strings"

	"cervterm/internal/fontdesc"
)

// faceCandidate is the concrete root-facade projection of one shape candidate.
// It preserves the historical unexported concrete identity for same-package
// callers while shape owns ranking and resolution behavior.
type faceCandidate struct {
	path     string
	index    int
	metadata fontdesc.FaceMetadata
	rank     fontdesc.RankingTuple
}

// resolvedFacePlan preserves the historical concrete root projection. It owns
// no source bytes, parsed value, cache lease, raster object, or native handle.
type resolvedFacePlan struct {
	descriptor      fontdesc.Descriptor
	target          fontdesc.FaceTarget
	selected        faceCandidate
	tier            fontdesc.SourceTier
	authoredIndex   uint32
	synthetic       fontdesc.SyntheticMode
	canonicalFaceID fontdesc.CanonicalFaceID
	resolvedKey     fontdesc.ResolvedFaceKey
}

// normalizeFamily remains the 5.5a root discovery compatibility helper.
func normalizeFamily(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}
