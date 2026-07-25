package discovery

import (
	"testing"

	"cervterm/internal/fontdesc"
)

func TestFaceContractKeepsStableSourceIndexAndMetadata(t *testing.T) {
	metadata := fontdesc.FaceMetadata{Family: "Example Mono", Subfamily: "Regular", Weight: 400, Stretch: 100}.Normalized()
	face := NewFace("example.ttc", 3, metadata.Family, metadata.Subfamily, metadata)
	if face.Path() != "example.ttc" || face.Index() != 3 || face.Family() != metadata.Family || face.Subfamily() != metadata.Subfamily || face.Metadata() != metadata {
		t.Fatalf("face contract = %#v", face)
	}
}
