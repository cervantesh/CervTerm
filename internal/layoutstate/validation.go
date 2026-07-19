package layoutstate

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

type budget struct {
	windows, tabs, panes, nodes int
	args, stringBytes           int
}

func (b *budget) text(path, value string, max int) error {
	if err := validText(path, value, max); err != nil {
		return err
	}
	b.stringBytes += len(value)
	if b.stringBytes > MaxJSONBytes {
		return fmt.Errorf("%s: aggregate string bytes exceed %d", path, MaxJSONBytes)
	}
	return nil
}

func (b *budget) name(path, value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s: must be non-empty and trimmed", path)
	}
	return b.text(path, value, MaxNameBytes)
}

func validateDocument(document Document) error {
	if document.Version != Version1 {
		return fmt.Errorf("version: expected %d", Version1)
	}
	if len(document.Workspaces) == 0 || len(document.Workspaces) > MaxWorkspaces {
		return fmt.Errorf("workspaces: count must be 1..%d", MaxWorkspaces)
	}
	if document.ActiveWorkspace < 0 || document.ActiveWorkspace >= len(document.Workspaces) {
		return fmt.Errorf("active_workspace: index out of range")
	}
	seen := make(map[string]struct{}, len(document.Workspaces))
	b := budget{}
	for i, workspace := range document.Workspaces {
		path := fmt.Sprintf("workspaces[%d]", i)
		if err := b.name(path+".name", workspace.Name); err != nil {
			return err
		}
		if _, ok := seen[workspace.Name]; ok {
			return fmt.Errorf("%s.name: duplicate workspace name", path)
		}
		seen[workspace.Name] = struct{}{}
		if len(workspace.Windows) == 0 {
			if workspace.ActiveWindow != -1 {
				return fmt.Errorf("%s.active_window: must be -1 for empty workspace", path)
			}
		} else if workspace.ActiveWindow < 0 || workspace.ActiveWindow >= len(workspace.Windows) {
			return fmt.Errorf("%s.active_window: index out of range", path)
		}
		b.windows += len(workspace.Windows)
		if len(workspace.Windows) > MaxWindows || b.windows > MaxWindows {
			return fmt.Errorf("%s.windows: total exceeds %d", path, MaxWindows)
		}
		for j := range workspace.Windows {
			if err := validateWindow(&workspace.Windows[j], fmt.Sprintf("%s.windows[%d]", path, j), &b); err != nil {
				return err
			}
		}
	}
	if b.windows == 0 {
		return fmt.Errorf("workspaces: at least one window is required")
	}
	return nil
}

func validateWindow(window *Window, path string, b *budget) error {
	if err := b.text(path+".title", window.Title, MaxValueBytes); err != nil {
		return err
	}
	bounds := window.Bounds
	if bounds.X < -MaxLogicalSize || bounds.X > MaxLogicalSize || bounds.Y < -MaxLogicalSize || bounds.Y > MaxLogicalSize || bounds.Width < 1 || bounds.Width > MaxLogicalSize || bounds.Height < 1 || bounds.Height > MaxLogicalSize {
		return fmt.Errorf("%s.bounds: logical bounds out of range", path)
	}
	if err := b.text(path+".bounds.monitor_hint", bounds.MonitorHint, MaxValueBytes); err != nil {
		return err
	}
	if len(window.Tabs) == 0 || len(window.Tabs) > MaxTabsWindow {
		return fmt.Errorf("%s.tabs: count must be 1..%d", path, MaxTabsWindow)
	}
	if window.ActiveTab < 0 || window.ActiveTab >= len(window.Tabs) {
		return fmt.Errorf("%s.active_tab: index out of range", path)
	}
	b.tabs += len(window.Tabs)
	if b.tabs > MaxTabsTotal {
		return fmt.Errorf("%s.tabs: total exceeds %d", path, MaxTabsTotal)
	}
	if err := b.text(path+".appearance.color_scheme", window.Appearance.ColorScheme, MaxValueBytes); err != nil {
		return err
	}
	if value := window.Appearance.BackgroundOpacity; value != nil && (!finite(*value) || *value < 0 || *value > 1) {
		return fmt.Errorf("%s.appearance.background_opacity: must be finite and in 0..1", path)
	}
	if value := window.Appearance.TextOpacity; value != nil && (!finite(*value) || *value < 0 || *value > 1) {
		return fmt.Errorf("%s.appearance.text_opacity: must be finite and in 0..1", path)
	}
	if value := window.Appearance.FontSize; value != nil && (!finite(*value) || *value < 4 || *value > 512) {
		return fmt.Errorf("%s.appearance.font_size: must be finite and in 4..512", path)
	}
	for i := range window.Tabs {
		tabPath := fmt.Sprintf("%s.tabs[%d]", path, i)
		if err := b.text(tabPath+".title", window.Tabs[i].Title, MaxValueBytes); err != nil {
			return err
		}
		before := b.panes
		if err := validateNode(window.Tabs[i].Root, tabPath+".root", 1, b); err != nil {
			return err
		}
		leaves := b.panes - before
		if window.Tabs[i].FocusedLeaf < 0 || window.Tabs[i].FocusedLeaf >= leaves {
			return fmt.Errorf("%s.focused_leaf: index out of range", tabPath)
		}
	}
	return nil
}

func validateNode(node Node, path string, depth int, b *budget) error {
	if depth > MaxTreeDepth {
		return fmt.Errorf("%s: depth exceeds %d", path, MaxTreeDepth)
	}
	b.nodes++
	if b.nodes > MaxTreeNodes {
		return fmt.Errorf("%s: total nodes exceeds %d", path, MaxTreeNodes)
	}
	switch node.Type {
	case "pane":
		if node.Launch == nil || node.Axis != "" || node.Ratio != 0 || node.First != nil || node.Second != nil {
			return fmt.Errorf("%s: invalid pane node union", path)
		}
		b.panes++
		if b.panes > MaxPanes {
			return fmt.Errorf("%s: total panes exceeds %d", path, MaxPanes)
		}
		return validateLaunch(*node.Launch, path+".launch", b)
	case "split":
		if node.Launch != nil || (node.Axis != "columns" && node.Axis != "rows") || node.Ratio < 1 || node.Ratio > 9999 || node.First == nil || node.Second == nil {
			return fmt.Errorf("%s: invalid split node union", path)
		}
		if err := validateNode(*node.First, path+".first", depth+1, b); err != nil {
			return err
		}
		return validateNode(*node.Second, path+".second", depth+1, b)
	default:
		return fmt.Errorf("%s.type: unknown node type", path)
	}
}

func validateLaunch(launch Launch, path string, b *budget) error {
	if err := b.text(path+".target_id", launch.TargetID, MaxValueBytes); err != nil {
		return err
	}
	if err := b.text(path+".program", launch.Program, MaxValueBytes); err != nil {
		return err
	}
	if err := b.text(path+".cwd", launch.CWD, MaxValueBytes); err != nil {
		return err
	}
	if len(launch.Args) > MaxArgs {
		return fmt.Errorf("%s.args: count exceeds %d", path, MaxArgs)
	}
	b.args += len(launch.Args)
	if b.args > MaxArgsTotal {
		return fmt.Errorf("%s.args: total count exceeds %d", path, MaxArgsTotal)
	}
	if len(launch.Args) > 0 && launch.Program == "" {
		return fmt.Errorf("%s.args: program is required", path)
	}
	for i, value := range launch.Args {
		if err := b.text(fmt.Sprintf("%s.args[%d]", path, i), value, MaxValueBytes); err != nil {
			return err
		}
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validText(path, value string, max int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s: invalid UTF-8", path)
	}
	if len(value) > max {
		return fmt.Errorf("%s: exceeds %d bytes", path, max)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s: contains control characters", path)
		}
	}
	return nil
}
