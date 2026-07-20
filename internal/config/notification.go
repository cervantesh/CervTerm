package config

import (
	"errors"
	"fmt"
)

type NotificationConfig struct {
	Enabled     bool
	Focus       string
	RateLimitMS int
}

func validateNotification(config NotificationConfig) []error {
	var errs []error
	if config.Focus != "always" && config.Focus != "unfocused" {
		errs = append(errs, fmt.Errorf("notification.focus %q must be always or unfocused", config.Focus))
	}
	if config.RateLimitMS < 100 || config.RateLimitMS > 60_000 {
		errs = append(errs, errors.New("notification.rate_limit_ms must be between 100 and 60000"))
	}
	return errs
}
