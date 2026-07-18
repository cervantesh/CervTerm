package config

import (
	"fmt"
	"strings"
)

const (
	MaxLaunchTargets    = 128
	MaxLaunchIDBytes    = 64
	MaxLaunchLabelBytes = 64
	MaxLaunchArgs       = 128
	MaxLaunchEnv        = 256
	MaxLaunchValueBytes = 4 * 1024
)

type LaunchTarget struct {
	ID      string
	Label   string
	Program string
	Args    []string
	CWD     string
	Env     map[string]string
}

func cloneLaunchTargets(source []LaunchTarget) []LaunchTarget {
	if source == nil {
		return nil
	}
	out := make([]LaunchTarget, len(source))
	for i, target := range source {
		out[i] = target
		out[i].Args = append([]string(nil), target.Args...)
		out[i].Env = cloneStringValues(target.Env)
	}
	return out
}

func cloneStringValues(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func validateLaunchMenu(targets []LaunchTarget) error {
	if len(targets) > MaxLaunchTargets {
		return fmt.Errorf("launch_menu must contain at most %d entries", MaxLaunchTargets)
	}
	seen := make(map[string]struct{}, len(targets))
	for i, target := range targets {
		path := fmt.Sprintf("launch_menu[%d]", i+1)
		if err := launchString(path+".id", target.ID, 1, MaxLaunchIDBytes); err != nil {
			return err
		}
		if _, ok := seen[target.ID]; ok {
			return fmt.Errorf("%s.id %q is duplicated", path, target.ID)
		}
		seen[target.ID] = struct{}{}
		if err := launchString(path+".label", target.Label, 1, MaxLaunchLabelBytes); err != nil {
			return err
		}
		if err := launchString(path+".program", target.Program, 1, MaxLaunchValueBytes); err != nil {
			return err
		}
		if err := launchString(path+".cwd", target.CWD, 0, MaxLaunchValueBytes); err != nil {
			return err
		}
		if len(target.Args) > MaxLaunchArgs {
			return fmt.Errorf("%s.args must contain at most %d entries", path, MaxLaunchArgs)
		}
		for j, value := range target.Args {
			if err := launchString(fmt.Sprintf("%s.args[%d]", path, j+1), value, 0, MaxLaunchValueBytes); err != nil {
				return err
			}
		}
		if len(target.Env) > MaxLaunchEnv {
			return fmt.Errorf("%s.env must contain at most %d entries", path, MaxLaunchEnv)
		}
		for key, value := range target.Env {
			if err := launchString(path+".env key", key, 1, MaxLaunchValueBytes); err != nil {
				return err
			}
			if err := launchString(path+".env["+key+"]", value, 0, MaxLaunchValueBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func launchString(path, value string, min, max int) error {
	if len(value) < min || len(value) > max {
		return fmt.Errorf("%s must contain %d to %d bytes", path, min, max)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s must not contain NUL", path)
	}
	return nil
}
