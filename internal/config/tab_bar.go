package config

import (
	"errors"
	"fmt"
)

type TabBarConfig struct {
	Mode            string
	Position        string
	HeightPX        int
	MinWidthPX      int
	MaxWidthPX      int
	PaddingX        int
	ShowNewButton   bool
	ShowCloseButton bool
}

func validateTabBar(c TabBarConfig) []error {
	var errs []error
	if c.Mode != "multiple" && c.Mode != "always" && c.Mode != "hidden" {
		errs = append(errs, fmt.Errorf("tab_bar.mode %q must be multiple, always, or hidden", c.Mode))
	}
	if c.Position != "top" && c.Position != "bottom" {
		errs = append(errs, fmt.Errorf("tab_bar.position %q must be top or bottom", c.Position))
	}
	if c.HeightPX < 18 || c.HeightPX > 96 {
		errs = append(errs, errors.New("tab_bar.height_px must be between 18 and 96"))
	}
	if c.MinWidthPX < 32 || c.MinWidthPX > 512 {
		errs = append(errs, errors.New("tab_bar.min_width_px must be between 32 and 512"))
	}
	if c.MaxWidthPX < 32 || c.MaxWidthPX > 1024 {
		errs = append(errs, errors.New("tab_bar.max_width_px must be between 32 and 1024"))
	} else if c.MaxWidthPX < c.MinWidthPX {
		errs = append(errs, errors.New("tab_bar.max_width_px must be greater than or equal to tab_bar.min_width_px"))
	}
	if c.PaddingX < 0 || c.PaddingX > 64 {
		errs = append(errs, errors.New("tab_bar.padding_x must be between 0 and 64"))
	}
	return errs
}
