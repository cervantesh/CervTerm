package shape

import "cervterm/internal/fontdesc"

// SourceFace is a detached discovery projection. Source must already be the
// canonical cache source selected by the root facade.
type SourceFace struct {
	Source   string
	Index    int
	Metadata fontdesc.FaceMetadata
}

// Lookup returns detached source faces for one normalized family.
type Lookup func(family string) []SourceFace

// Candidate is one ranked concrete face.
type Candidate struct {
	Source   string
	Index    int
	Metadata fontdesc.FaceMetadata
	Rank     fontdesc.RankingTuple
}

// Plan is one pure ordered load attempt. It contains stable identities but no
// source bytes, cache lease, parsed pointer, raster resource, or native handle.
type Plan struct {
	Descriptor      fontdesc.Descriptor
	Target          fontdesc.FaceTarget
	Selected        Candidate
	Tier            fontdesc.SourceTier
	AuthoredIndex   uint32
	Synthetic       fontdesc.SyntheticMode
	CanonicalFaceID fontdesc.CanonicalFaceID
	ResolvedKey     fontdesc.ResolvedFaceKey
}

// Resolver performs deterministic pure selection over a detached lookup.
type Resolver struct {
	lookup Lookup
}

// NewResolver constructs a resolver over a detached discovery projection.
func NewResolver(lookup Lookup) Resolver { return Resolver{lookup: lookup} }
