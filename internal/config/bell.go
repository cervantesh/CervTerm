package config

import (
	"errors"
	"fmt"
)

type BellConfig struct {
	Mode             string
	Focus            string
	ThrottleMS       int
	VisualDurationMS int
}

func validateBell(config BellConfig) []error {
	var errs []error
	if config.Mode != "disabled" && config.Mode != "audible" && config.Mode != "visual" && config.Mode != "taskbar" {
		errs = append(errs, fmt.Errorf("bell.mode %q must be disabled, audible, visual, or taskbar", config.Mode))
	}
	if config.Focus != "always" && config.Focus != "unfocused" {
		errs = append(errs, fmt.Errorf("bell.focus %q must be always or unfocused", config.Focus))
	}
	if config.ThrottleMS < 0 || config.ThrottleMS > 60_000 {
		errs = append(errs, errors.New("bell.throttle_ms must be between 0 and 60000"))
	}
	if config.VisualDurationMS < 50 || config.VisualDurationMS > 2_000 {
		errs = append(errs, errors.New("bell.visual_duration_ms must be between 50 and 2000"))
	}
	return errs
}
