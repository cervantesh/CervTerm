package layoutstate

const (
	Version1       = 1
	MaxJSONBytes   = 1 << 20
	MaxWorkspaces  = 64
	MaxWindows     = 32
	MaxTabsWindow  = 256
	MaxTabsTotal   = 8192
	MaxTreeDepth   = 32
	MaxPanes       = 8192
	MaxTreeNodes   = 16384
	MaxArgs        = 128
	MaxArgsTotal   = 16384
	MaxNameBytes   = 128
	MaxValueBytes  = 4 << 10
	MaxLogicalSize = 1_000_000
)

type Document struct {
	Version         int         `json:"version"`
	ActiveWorkspace int         `json:"active_workspace"`
	Workspaces      []Workspace `json:"workspaces"`
}

type Workspace struct {
	Name         string   `json:"name"`
	ActiveWindow int      `json:"active_window"`
	Windows      []Window `json:"windows"`
}

type Window struct {
	Title      string     `json:"title"`
	Bounds     Bounds     `json:"bounds"`
	ActiveTab  int        `json:"active_tab"`
	Tabs       []Tab      `json:"tabs"`
	Appearance Appearance `json:"appearance"`
}

type Bounds struct {
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	MonitorHint string `json:"monitor_hint"`
}

type Appearance struct {
	ColorScheme       string   `json:"color_scheme"`
	BackgroundOpacity *float64 `json:"background_opacity,omitempty"`
	TextOpacity       *float64 `json:"text_opacity,omitempty"`
	Blur              *bool    `json:"blur,omitempty"`
	FontSize          *float64 `json:"font_size,omitempty"`
}

type Tab struct {
	Title       string `json:"title"`
	FocusedLeaf int    `json:"focused_leaf"`
	Root        Node   `json:"root"`
}

type Node struct {
	Type   string  `json:"type"`
	Launch *Launch `json:"launch,omitempty"`
	Axis   string  `json:"axis,omitempty"`
	Ratio  int     `json:"ratio,omitempty"`
	First  *Node   `json:"first,omitempty"`
	Second *Node   `json:"second,omitempty"`
}

// Launch is an allowlisted fresh-process descriptor. Args and paths must come from
// an explicitly sanitized launch definition, never from a live process command line.
// Environment values and credentials have no representation in this schema.
// Restore resolves a non-empty TargetID against current configuration first. If it is
// unavailable, non-empty Program/Args/CWD form the sanitized local fallback; without a
// Program, the current default shell is used with CWD. With no TargetID, Program is the
// explicit local command, or an empty descriptor selects the current default shell.
type Launch struct {
	TargetID string   `json:"target_id"`
	Program  string   `json:"program"`
	Args     []string `json:"args"`
	CWD      string   `json:"cwd"`
}

// Plan is an immutable, validated layout. Its fields are intentionally private.
type Plan struct{ document Document }

func NewPlan(document Document) (Plan, error) {
	if err := validateDocument(document); err != nil {
		return Plan{}, err
	}
	copy := cloneDocument(document)
	if _, err := marshalDocument(copy); err != nil {
		return Plan{}, err
	}
	return Plan{document: copy}, nil
}

func (p Plan) Snapshot() Document { return cloneDocument(p.document) }

func cloneDocument(d Document) Document {
	out := d
	out.Workspaces = make([]Workspace, len(d.Workspaces))
	for i, w := range d.Workspaces {
		out.Workspaces[i] = w
		out.Workspaces[i].Windows = make([]Window, len(w.Windows))
		for j, win := range w.Windows {
			out.Workspaces[i].Windows[j] = win
			out.Workspaces[i].Windows[j].Appearance = cloneAppearance(win.Appearance)
			out.Workspaces[i].Windows[j].Tabs = make([]Tab, len(win.Tabs))
			for k, tab := range win.Tabs {
				out.Workspaces[i].Windows[j].Tabs[k] = tab
				out.Workspaces[i].Windows[j].Tabs[k].Root = cloneNode(tab.Root)
			}
		}
	}
	return out
}

func cloneAppearance(a Appearance) Appearance {
	out := a
	if a.BackgroundOpacity != nil {
		v := *a.BackgroundOpacity
		out.BackgroundOpacity = &v
	}
	if a.TextOpacity != nil {
		v := *a.TextOpacity
		out.TextOpacity = &v
	}
	if a.Blur != nil {
		v := *a.Blur
		out.Blur = &v
	}
	if a.FontSize != nil {
		v := *a.FontSize
		out.FontSize = &v
	}
	return out
}

func cloneNode(n Node) Node {
	out := n
	if n.Launch != nil {
		v := *n.Launch
		v.Args = make([]string, len(n.Launch.Args))
		copy(v.Args, n.Launch.Args)
		out.Launch = &v
	}
	if n.First != nil {
		v := cloneNode(*n.First)
		out.First = &v
	}
	if n.Second != nil {
		v := cloneNode(*n.Second)
		out.Second = &v
	}
	return out
}
