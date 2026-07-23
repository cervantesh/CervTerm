package main

import (
	"fmt"
	"runtime"

	"cervterm/internal/config"
)

const doctorActivationNotProbed = "not-probed"

type doctorCapability struct {
	ID                  string
	Status              string
	Platform            string
	ManualQualification string
	SupportClaim        string
	ConfiguredIntent    string
	BuildAvailability   string
	DefaultEnabled      *bool
}

func printSupportDoctor(cfg config.Config) {
	fmt.Println("  capabilities:")
	for _, capability := range doctorCapabilities(cfg) {
		fmt.Printf("    %s: intent=%s build=%s activation=%s manual=%s claim=%s\n",
			capability.ID,
			capability.ConfiguredIntent,
			capability.BuildAvailability,
			doctorActivationNotProbed,
			capability.ManualQualification,
			capability.SupportClaim,
		)
	}
}

func doctorCapabilities(cfg config.Config) []doctorCapability {
	return []doctorCapability{
		{
			ID: "appearance.advanced", Status: "supported", Platform: "cross-platform-glfw",
			ManualQualification: "partial", SupportClaim: "supported-subset",
			ConfiguredIntent:  fmt.Sprintf("decorations=%s,titlebar=%s", cfg.Window.Decorations, cfg.Window.Titlebar),
			BuildAvailability: doctorGLFWAvailability("glfw-window"),
		},
		{
			ID: "clipboard.osc52", Status: "supported", Platform: "all",
			ManualQualification: "automated", SupportClaim: "supported",
			ConfiguredIntent: "osc52=" + cfg.Clipboard.OSC52, BuildAvailability: "built-in",
		},
		{
			ID: "input.ime_preedit", Status: "experimental", Platform: "windows",
			ManualQualification: "partial-skipped-jck", SupportClaim: "none", DefaultEnabled: boolPointer(false),
			ConfiguredIntent: boolIntent(cfg.IME.Enabled), BuildAvailability: doctorWindowsAvailability("windows-native"),
		},
		{
			ID: "accessibility.windows_uia", Status: "experimental", Platform: "windows",
			ManualQualification: "phase15-gui-lifecycle-only-no-at", SupportClaim: "none", DefaultEnabled: boolPointer(false),
			ConfiguredIntent: boolIntent(cfg.Accessibility.Enabled), BuildAvailability: doctorWindowsAvailability("windows-uia"),
		},
		{
			ID: "shell.windows_native_notifications", Status: "experimental", Platform: "windows",
			ManualQualification: "unrun", SupportClaim: "none", DefaultEnabled: boolPointer(false),
			ConfiguredIntent: boolIntent(cfg.Notification.Enabled), BuildAvailability: doctorWindowsAvailability("windows-native"),
		},
		{
			ID: "graphics.kitty", Status: "experimental", Platform: "linux-wslg-scoped; windows-conpty-filtered",
			ManualQualification: "phase15-linux-wslg-accepted-reply-windows-filtered", SupportClaim: "subset_only", DefaultEnabled: boolPointer(false),
			ConfiguredIntent: boolIntent(cfg.Graphics.Kitty.Enabled), BuildAvailability: doctorGLFWAvailability("glfw-opengl"),
		},
		{
			ID: "graphics.sixel_iterm", Status: "experimental", Platform: "windows-iterm-glfw-opengl; linux-wslg-sixel-scoped",
			ManualQualification: "phase15-windows-iterm-pass-linux-wslg-sixel-pass-conpty-filter-boundary", SupportClaim: "none", DefaultEnabled: boolPointer(false),
			ConfiguredIntent:  fmt.Sprintf("sixel=%t,iterm=%t", cfg.Graphics.Sixel.Enabled, cfg.Graphics.ITerm.Enabled),
			BuildAvailability: doctorGLFWAvailability("glfw-opengl"),
		},
	}
}

func doctorGLFWAvailability(available string) string {
	if doctorGLFWBuild {
		return available
	}
	return "unavailable-headless"
}

func doctorWindowsAvailability(available string) string {
	if !doctorGLFWBuild {
		return "unavailable-headless"
	}
	if runtime.GOOS == "windows" {
		return available
	}
	return "unavailable-platform"
}

func boolIntent(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func boolPointer(value bool) *bool { return &value }
