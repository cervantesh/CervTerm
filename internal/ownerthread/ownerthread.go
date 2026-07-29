// Package ownerthread provides stable native-thread identity without cgo.
// Callers capture an ID at an ownership boundary and compare Current before
// every thread-affine mutation or native API call.
package ownerthread

// ID is a process-local native thread identity. Zero is always invalid.
type ID uint64

// Current returns the calling native thread identity, or zero when the platform
// cannot supply one.
func Current() ID { return current() }

// Source is the narrow seam used by thread-affine controllers and tests.
type Source interface {
	Current() ID
}

// SourceFunc adapts a function to Source.
type SourceFunc func() ID

// Current implements Source.
func (f SourceFunc) Current() ID {
	if f == nil {
		return 0
	}
	return f()
}

// Native is the production Source.
type Native struct{}

// Current implements Source.
func (Native) Current() ID { return Current() }

// Attestation is an immutable capture of one exact native thread.
type Attestation struct {
	id ID
}

// Capture samples source and rejects a zero identity.
func Capture(source Source) (Attestation, bool) {
	if source == nil {
		return Attestation{}, false
	}
	id := source.Current()
	return Attestation{id: id}, id != 0
}

// ID returns the captured identity.
func (a Attestation) ID() ID { return a.id }

// Current reports whether source still identifies the captured native thread.
func (a Attestation) Current(source Source) bool {
	return source != nil && a.id != 0 && source.Current() == a.id
}
