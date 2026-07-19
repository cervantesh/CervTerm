package layoutrestore

import (
	"fmt"

	"cervterm/internal/layoutstate"
	"cervterm/internal/windowbounds"
)

// Launch is a current, sanitized process launch definition.
type Launch struct {
	Program string
	Args    []string
	CWD     string
}

// Target is a named current launch definition. Environment is intentionally not representable.
type Target struct {
	ID      string
	Program string
	Args    []string
	CWD     string
}

// BlueprintAppearance is the concrete appearance intent passed to the runtime.
type BlueprintAppearance struct {
	ColorScheme       string
	BackgroundOpacity float64
	TextOpacity       float64
	Blur              bool
	FontSize          float64
}

// Options contains the current configuration and display snapshot used by Prepare.
type Options struct {
	DefaultLaunch Launch
	Targets       []Target
	Monitors      []windowbounds.Monitor
	Policy        windowbounds.Policy
	Appearance    BlueprintAppearance
	CWDUsable     func(string) bool
}

// LaunchSource describes which descriptor supplied a resolved program and arguments.
type LaunchSource string

const (
	SourceCurrentTarget     LaunchSource = "current_target"
	SourcePersistedFallback LaunchSource = "persisted_fallback"
	SourceDefaultShell      LaunchSource = "default_shell"
)

// ResolvedLaunch is a complete, environment-free launch decision.
type ResolvedLaunch struct {
	Program string
	Args    []string
	CWD     string
	Source  LaunchSource
}

type Node struct {
	Type   string
	Launch *ResolvedLaunch
	Axis   string
	Ratio  int
	First  *Node
	Second *Node
}

type Tab struct {
	Title       string
	FocusedLeaf int
	Root        Node
}

type Window struct {
	Title      string
	Bounds     windowbounds.Plan
	ActiveTab  int
	Tabs       []Tab
	Appearance BlueprintAppearance
}

type Workspace struct {
	Name         string
	ActiveWindow int
	Windows      []Window
}

// Snapshot is a detached view of a prepared blueprint.
type Snapshot struct {
	ActiveWorkspace int
	Workspaces      []Workspace
}

// Blueprint is immutable; Snapshot returns a deep clone of its private data.
type Blueprint struct{ snapshot Snapshot }

func (b Blueprint) Snapshot() Snapshot { return cloneSnapshot(b.snapshot) }

// Prepare resolves every saved pane and window before returning a blueprint.
func Prepare(plan layoutstate.Plan, options Options) (Blueprint, error) {
	document := plan.Snapshot()
	if _, err := layoutstate.NewPlan(document); err != nil {
		return Blueprint{}, fmt.Errorf("layoutrestore: plan: %w", err)
	}
	if document.ActiveWorkspace < 0 || document.ActiveWorkspace >= len(document.Workspaces) || len(document.Workspaces[document.ActiveWorkspace].Windows) == 0 {
		return Blueprint{}, fmt.Errorf("layoutrestore: active workspace has no usable window")
	}
	options = cloneOptions(options)
	if err := validateTargets(options.Targets); err != nil {
		return Blueprint{}, err
	}

	out := Snapshot{ActiveWorkspace: document.ActiveWorkspace, Workspaces: make([]Workspace, len(document.Workspaces))}
	for wi, savedWorkspace := range document.Workspaces {
		workspacePath := fmt.Sprintf("workspaces[%d]", wi)
		if savedWorkspace.ActiveWindow < -1 || savedWorkspace.ActiveWindow >= len(savedWorkspace.Windows) {
			return Blueprint{}, fmt.Errorf("%s.active_window: index out of range", workspacePath)
		}
		out.Workspaces[wi] = Workspace{Name: savedWorkspace.Name, ActiveWindow: savedWorkspace.ActiveWindow, Windows: make([]Window, len(savedWorkspace.Windows))}
		for wini, savedWindow := range savedWorkspace.Windows {
			windowPath := fmt.Sprintf("%s.windows[%d]", workspacePath, wini)
			bounds, err := windowbounds.Recover(savedWindow.Bounds, options.Monitors, options.Policy)
			if err != nil {
				return Blueprint{}, fmt.Errorf("%s.bounds: %w", windowPath, err)
			}
			if savedWindow.ActiveTab < 0 || savedWindow.ActiveTab >= len(savedWindow.Tabs) {
				return Blueprint{}, fmt.Errorf("%s.active_tab: index out of range", windowPath)
			}
			if savedWindow.Appearance.ColorScheme != "" && savedWindow.Appearance.ColorScheme != options.Appearance.ColorScheme {
				return Blueprint{}, fmt.Errorf("%s.appearance.color_scheme: %q is not the currently resolved scheme", windowPath, savedWindow.Appearance.ColorScheme)
			}
			window := Window{Title: savedWindow.Title, Bounds: bounds, ActiveTab: savedWindow.ActiveTab, Appearance: mergeAppearance(options.Appearance, savedWindow.Appearance), Tabs: make([]Tab, len(savedWindow.Tabs))}
			for ti, savedTab := range savedWindow.Tabs {
				tabPath := fmt.Sprintf("%s.tabs[%d]", windowPath, ti)
				root, leaves, err := resolveNode(savedTab.Root, tabPath+".root", 1, options)
				if err != nil {
					return Blueprint{}, err
				}
				if savedTab.FocusedLeaf < 0 || savedTab.FocusedLeaf >= leaves {
					return Blueprint{}, fmt.Errorf("%s.focused_leaf: index out of range", tabPath)
				}
				window.Tabs[ti] = Tab{Title: savedTab.Title, FocusedLeaf: savedTab.FocusedLeaf, Root: root}
			}
			out.Workspaces[wi].Windows[wini] = window
		}
	}
	return Blueprint{snapshot: out}, nil
}

func validateTargets(targets []Target) error {
	seen := make(map[string]struct{}, len(targets))
	for i, target := range targets {
		path := fmt.Sprintf("options.targets[%d].id", i)
		if target.ID == "" {
			return fmt.Errorf("%s: must be non-empty", path)
		}
		if _, exists := seen[target.ID]; exists {
			return fmt.Errorf("%s: duplicate target ID", path)
		}
		seen[target.ID] = struct{}{}
	}
	return nil
}

func resolveNode(saved layoutstate.Node, path string, depth int, options Options) (Node, int, error) {
	if depth > layoutstate.MaxTreeDepth {
		return Node{}, 0, fmt.Errorf("%s: depth exceeds %d", path, layoutstate.MaxTreeDepth)
	}
	switch saved.Type {
	case "pane":
		if saved.Launch == nil || saved.First != nil || saved.Second != nil || saved.Axis != "" || saved.Ratio != 0 {
			return Node{}, 0, fmt.Errorf("%s: invalid pane node union", path)
		}
		launch, err := resolveLaunch(*saved.Launch, options)
		if err != nil {
			return Node{}, 0, fmt.Errorf("%s.launch: %w", path, err)
		}
		return Node{Type: saved.Type, Launch: &launch}, 1, nil
	case "split":
		if saved.Launch != nil || saved.First == nil || saved.Second == nil || (saved.Axis != "columns" && saved.Axis != "rows") || saved.Ratio < 1 || saved.Ratio > 9999 {
			return Node{}, 0, fmt.Errorf("%s: invalid split node union", path)
		}
		first, firstLeaves, err := resolveNode(*saved.First, path+".first", depth+1, options)
		if err != nil {
			return Node{}, 0, err
		}
		second, secondLeaves, err := resolveNode(*saved.Second, path+".second", depth+1, options)
		if err != nil {
			return Node{}, 0, err
		}
		return Node{Type: saved.Type, Axis: saved.Axis, Ratio: saved.Ratio, First: &first, Second: &second}, firstLeaves + secondLeaves, nil
	default:
		return Node{}, 0, fmt.Errorf("%s.type: unknown node type", path)
	}
}

func resolveLaunch(saved layoutstate.Launch, options Options) (ResolvedLaunch, error) {
	chosen := options.DefaultLaunch
	source := SourceDefaultShell
	fallbackCWD := options.DefaultLaunch.CWD

	if saved.TargetID != "" {
		if target, ok := findTarget(options.Targets, saved.TargetID); ok {
			chosen = Launch{Program: target.Program, Args: cloneStrings(target.Args), CWD: target.CWD}
			source = SourceCurrentTarget
			fallbackCWD = target.CWD
		} else if saved.Program != "" {
			chosen = Launch{Program: saved.Program, Args: cloneStrings(saved.Args), CWD: saved.CWD}
			source = SourcePersistedFallback
		} else {
			chosen.CWD = saved.CWD
		}
	} else if saved.Program != "" {
		chosen = Launch{Program: saved.Program, Args: cloneStrings(saved.Args), CWD: saved.CWD}
		source = SourcePersistedFallback
	} else {
		chosen.CWD = saved.CWD
	}

	if source == SourceCurrentTarget && saved.CWD != "" && usable(options.CWDUsable, saved.CWD) {
		chosen.CWD = saved.CWD
	}
	if !usable(options.CWDUsable, chosen.CWD) {
		if usable(options.CWDUsable, fallbackCWD) {
			chosen.CWD = fallbackCWD
		} else {
			chosen.CWD = ""
		}
	}
	if chosen.Program == "" {
		return ResolvedLaunch{}, fmt.Errorf("program: final program is empty")
	}
	return ResolvedLaunch{Program: chosen.Program, Args: cloneStrings(chosen.Args), CWD: chosen.CWD, Source: source}, nil
}

func findTarget(targets []Target, id string) (Target, bool) {
	for _, target := range targets {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

func usable(check func(string) bool, cwd string) bool {
	if cwd == "" {
		return false
	}
	return check == nil || check(cwd)
}

func mergeAppearance(current BlueprintAppearance, saved layoutstate.Appearance) BlueprintAppearance {
	if saved.ColorScheme != "" {
		current.ColorScheme = saved.ColorScheme
	}
	if saved.BackgroundOpacity != nil {
		current.BackgroundOpacity = *saved.BackgroundOpacity
	}
	if saved.TextOpacity != nil {
		current.TextOpacity = *saved.TextOpacity
	}
	if saved.Blur != nil {
		current.Blur = *saved.Blur
	}
	if saved.FontSize != nil {
		current.FontSize = *saved.FontSize
	}
	return current
}

func cloneOptions(in Options) Options {
	out := in
	out.DefaultLaunch.Args = cloneStrings(in.DefaultLaunch.Args)
	out.Targets = make([]Target, len(in.Targets))
	for i, target := range in.Targets {
		out.Targets[i] = target
		out.Targets[i].Args = cloneStrings(target.Args)
	}
	out.Monitors = append([]windowbounds.Monitor(nil), in.Monitors...)
	return out
}

func cloneStrings(in []string) []string { return append([]string(nil), in...) }

func cloneSnapshot(in Snapshot) Snapshot {
	out := in
	out.Workspaces = make([]Workspace, len(in.Workspaces))
	for wi, workspace := range in.Workspaces {
		out.Workspaces[wi] = workspace
		out.Workspaces[wi].Windows = make([]Window, len(workspace.Windows))
		for wini, window := range workspace.Windows {
			out.Workspaces[wi].Windows[wini] = window
			out.Workspaces[wi].Windows[wini].Tabs = make([]Tab, len(window.Tabs))
			for ti, tab := range window.Tabs {
				out.Workspaces[wi].Windows[wini].Tabs[ti] = tab
				out.Workspaces[wi].Windows[wini].Tabs[ti].Root = cloneNode(tab.Root)
			}
		}
	}
	return out
}

func cloneNode(in Node) Node {
	out := in
	if in.Launch != nil {
		launch := *in.Launch
		launch.Args = cloneStrings(in.Launch.Args)
		out.Launch = &launch
	}
	if in.First != nil {
		first := cloneNode(*in.First)
		out.First = &first
	}
	if in.Second != nil {
		second := cloneNode(*in.Second)
		out.Second = &second
	}
	return out
}
