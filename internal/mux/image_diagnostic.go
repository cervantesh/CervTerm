package mux

import "time"

// ImageDiagnosticProtocol identifies a Phase 14 image protocol without exposing
// pane, resource, transfer, placement, or payload identity.
type ImageDiagnosticProtocol string

const (
	ImageDiagnosticProtocolSixel ImageDiagnosticProtocol = "sixel"
	ImageDiagnosticProtocolITerm ImageDiagnosticProtocol = "iterm"
)

// ImageDiagnosticReason is the fixed, privacy-safe failure vocabulary used by
// programmatic Phase 14 diagnostics.
type ImageDiagnosticReason string

const (
	ImageDiagnosticReasonInvalid     ImageDiagnosticReason = "invalid"
	ImageDiagnosticReasonUnsupported ImageDiagnosticReason = "unsupported"
	ImageDiagnosticReasonLimit       ImageDiagnosticReason = "limit"
	ImageDiagnosticReasonTimeout     ImageDiagnosticReason = "timeout"
	ImageDiagnosticReasonCancelled   ImageDiagnosticReason = "cancelled"
	ImageDiagnosticReasonFailed      ImageDiagnosticReason = "failed"
	ImageDiagnosticReasonStale       ImageDiagnosticReason = "stale"
	ImageDiagnosticReasonBusy        ImageDiagnosticReason = "busy"
)

// ImageDiagnostic is deliberately limited to fixed classifications, a count,
// and elapsed time. It must not grow terminal-derived or image-derived fields.
type ImageDiagnostic struct {
	Protocol ImageDiagnosticProtocol
	Reason   ImageDiagnosticReason
	Count    uint64
	Duration time.Duration
}

func (m *Mux) emitImageDiagnostic(protocol ImageDiagnosticProtocol, reason ImageDiagnosticReason, startedAt, finishedAt time.Time) {
	if m == nil || m.options.ImageDiagnostic == nil {
		return
	}
	duration := time.Duration(0)
	if !startedAt.IsZero() && !finishedAt.IsZero() && !finishedAt.Before(startedAt) {
		duration = finishedAt.Sub(startedAt)
	}
	diagnostic := ImageDiagnostic{Protocol: protocol, Reason: reason, Count: 1, Duration: duration}
	callback := m.options.ImageDiagnostic
	func() {
		defer func() {
			_ = recover()
		}()
		callback(diagnostic)
	}()
}

func (m *Mux) emitImageDiagnosticNow(protocol ImageDiagnosticProtocol, reason ImageDiagnosticReason, startedAt time.Time) {
	if m == nil || m.options.ImageDiagnostic == nil {
		return
	}
	m.emitImageDiagnostic(protocol, reason, startedAt, m.options.Now())
}
