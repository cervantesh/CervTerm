package config

import "testing"

func TestMaxFPSV2AndV1Isolation(t *testing.T) {
	v2 := paddingDocument(t, `return {config_version=2,render={max_fps=120}}`)
	if got := FromDocument(Defaults(), v2).Render.MaxFPS; got != 120 {
		t.Fatalf("v2 max_fps=%d", got)
	}
	v1 := paddingDocument(t, `return {render={max_fps=120}}`)
	if got := FromDocument(Defaults(), v1).Render.MaxFPS; got != 0 {
		t.Fatalf("v1 max_fps=%d", got)
	}
}

func TestMaxFPSValidation(t *testing.T) {
	for _, value := range []int{-1, 1001} {
		cfg := Defaults()
		cfg.Render.MaxFPS = value
		if err := cfg.Validate(); err == nil {
			t.Fatalf("max_fps %d accepted", value)
		}
	}
}
