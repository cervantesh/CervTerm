//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
)

func TestNativeBlurProviderApply(t *testing.T) {
	tests := []struct {
		name      string
		request   BlurRequest
		want      BlurStatus
		wantCalls []bool
		wantErr   error
	}{
		{
			name:      "disabled removes material",
			request:   BlurRequest{},
			want:      BlurDisabled,
			wantCalls: []bool{false},
		},
		{
			name: "disabled ignores translucent background",
			request: BlurRequest{
				TranslucentBackground: true,
			},
			want:      BlurDisabled,
			wantCalls: []bool{false},
		},
		{
			name: "opaque background enables material",
			request: BlurRequest{
				Enabled: true,
			},
			want:      BlurActive,
			wantCalls: []bool{true},
		},
		{
			name: "translucent background preserves alpha",
			request: BlurRequest{
				Enabled:               true,
				TranslucentBackground: true,
			},
			want:      BlurIncompatible,
			wantCalls: []bool{false},
			wantErr:   errBlurIncompatible,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []bool
			provider := &nativeBlurProvider{
				name: "test-native",
				set: func(enabled bool) error {
					calls = append(calls, enabled)
					return nil
				},
			}

			result := provider.Apply(tt.request)
			if result.Status != tt.want {
				t.Fatalf("status = %s, want %s", result.Status, tt.want)
			}
			if !errors.Is(result.Err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", result.Err, tt.wantErr)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Fatalf("set calls = %v, want %v", calls, tt.wantCalls)
			}
		})
	}
}

func TestNativeBlurProviderFailureAndClose(t *testing.T) {
	setErr := errors.New("set failed")
	tests := []struct {
		name    string
		request BlurRequest
		err     error
		want    BlurStatus
	}{
		{name: "enable failure", request: BlurRequest{Enabled: true}, err: setErr, want: BlurFailed},
		{name: "disable failure", request: BlurRequest{}, err: setErr, want: BlurFailed},
		{
			name: "incompatible material removal failure",
			request: BlurRequest{
				Enabled:               true,
				TranslucentBackground: true,
			},
			err:  setErr,
			want: BlurFailed,
		},
		{
			name:    "unsupported native attribute",
			request: BlurRequest{Enabled: true},
			err:     errors.Join(setErr, errBlurUnsupported),
			want:    BlurUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &nativeBlurProvider{
				name: "test-native",
				set:  func(bool) error { return tt.err },
			}
			result := provider.Apply(tt.request)
			if result.Status != tt.want || !errors.Is(result.Err, tt.err) {
				t.Fatalf("Apply() = {%s, %v}, want {%s, %v}", result.Status, result.Err, tt.want, tt.err)
			}
		})
	}

	provider := &nativeBlurProvider{
		name: "test-native",
		set:  func(bool) error { return setErr },
	}
	if err := provider.Close(); !errors.Is(err, setErr) {
		t.Fatalf("Close() error = %v, want %v", err, setErr)
	}
}

func TestUnsupportedBlurProvider(t *testing.T) {
	provider := unsupportedBlurProvider{name: "test-unsupported"}
	if provider.Name() != "test-unsupported" {
		t.Fatalf("Name() = %q", provider.Name())
	}

	disabled := provider.Apply(BlurRequest{})
	if disabled.Status != BlurDisabled || disabled.Err != nil {
		t.Fatalf("disabled result = {%s, %v}", disabled.Status, disabled.Err)
	}

	enabled := provider.Apply(BlurRequest{Enabled: true})
	if enabled.Status != BlurUnsupported || !errors.Is(enabled.Err, errBlurUnsupported) {
		t.Fatalf("enabled result = {%s, %v}", enabled.Status, enabled.Err)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTransparentCompositorBlurProvider(t *testing.T) {
	tests := []struct {
		name      string
		request   BlurRequest
		want      BlurStatus
		wantCalls []bool
		wantErr   error
	}{
		{
			name:      "disabled removes compositor effect",
			request:   BlurRequest{},
			want:      BlurDisabled,
			wantCalls: []bool{false},
		},
		{
			name: "opaque background is incompatible",
			request: BlurRequest{
				Enabled:                         true,
				TransparentFramebufferAvailable: true,
			},
			want:      BlurIncompatible,
			wantCalls: []bool{false},
			wantErr:   errBlurRequiresTranslucentBackground,
		},
		{
			name: "missing transparent framebuffer is incompatible",
			request: BlurRequest{
				Enabled:               true,
				TranslucentBackground: true,
			},
			want:      BlurIncompatible,
			wantCalls: []bool{false},
			wantErr:   errBlurTransparentFramebufferMissing,
		},
		{
			name: "translucent background activates compositor effect",
			request: BlurRequest{
				Enabled:                         true,
				TranslucentBackground:           true,
				TransparentFramebufferAvailable: true,
			},
			want:      BlurActive,
			wantCalls: []bool{true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []bool
			provider := &nativeBlurProvider{
				name:          "test-compositor",
				compatibility: transparentCompositorCompatibility,
				set: func(enabled bool) error {
					calls = append(calls, enabled)
					return nil
				},
			}
			result := provider.Apply(tt.request)
			if result.Status != tt.want {
				t.Fatalf("status = %s, want %s", result.Status, tt.want)
			}
			if !errors.Is(result.Err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", result.Err, tt.wantErr)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Fatalf("set calls = %v, want %v", calls, tt.wantCalls)
			}
		})
	}
}

func TestFailedBlurProvider(t *testing.T) {
	initErr := errors.New("native initialization failed")
	provider := failedBlurProvider{name: "failed-test", err: initErr}

	disabled := provider.Apply(BlurRequest{})
	if disabled.Status != BlurDisabled || disabled.Err != nil {
		t.Fatalf("disabled result = %+v, want disabled without error", disabled)
	}

	enabled := provider.Apply(BlurRequest{Enabled: true})
	if enabled.Status != BlurFailed || !errors.Is(enabled.Err, initErr) {
		t.Fatalf("enabled result = %+v, want failed with initialization error", enabled)
	}
}

func TestBlurStatusString(t *testing.T) {
	tests := map[BlurStatus]string{
		BlurDisabled:     "disabled",
		BlurActive:       "active",
		BlurUnsupported:  "unsupported",
		BlurIncompatible: "incompatible",
		BlurFailed:       "failed",
		BlurStatus(255):  "unknown",
	}
	for status, want := range tests {
		if got := status.String(); got != want {
			t.Errorf("BlurStatus(%d).String() = %q, want %q", status, got, want)
		}
	}
}
