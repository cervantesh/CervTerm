package ownerthread

import "testing"

func TestAttestationRejectsZeroWrongAndStaleSources(t *testing.T) {
	var current ID
	source := SourceFunc(func() ID { return current })
	if _, ok := Capture(source); ok {
		t.Fatal("zero native thread identity was accepted")
	}
	current = 41
	attestation, ok := Capture(source)
	if !ok || attestation.ID() != 41 || !attestation.Current(source) {
		t.Fatal("stable native thread identity was not captured")
	}
	current = 42
	if attestation.Current(source) {
		t.Fatal("wrong native thread identity was accepted")
	}
	current = 0
	if attestation.Current(source) {
		t.Fatal("closed/zero native thread identity was accepted")
	}
}
