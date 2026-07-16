package mux

const (
	// RatioScale is the denominator used by SplitRatio values.
	RatioScale = 10_000
	// DefaultSplitRatio gives each side one half of the available pixels.
	DefaultSplitRatio SplitRatio = 5_000
	// DividerPixels is reserved between the children of every split.
	DividerPixels = 1
	// MinPaneCols and MinPaneRows apply when adding a split. Existing trees are
	// still laid out best-effort when their framebuffer becomes smaller.
	MinPaneCols = 10
	MinPaneRows = 3
)

// SplitRatio is the first child's share in ten-thousandths of the pixels left
// after reserving the divider.
type SplitRatio int

// PixelRect is a half-open framebuffer rectangle: [X, Right) x [Y, Bottom).
type PixelRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func (r PixelRect) Right() int  { return r.X + r.Width }
func (r PixelRect) Bottom() int { return r.Y + r.Height }
func (r PixelRect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// CellMetrics describes a pane's fixed cell size and symmetric inner padding.
type CellMetrics struct {
	CellWidth  int
	CellHeight int
	PaddingX   int
	PaddingY   int
}

// CellMetricsResolver returns the cell metrics for one active pane.
type CellMetricsResolver func(PaneID) (CellMetrics, bool)

// UniformCellMetrics adapts one metric value to the per-pane layout API.
func UniformCellMetrics(metrics CellMetrics) CellMetricsResolver {
	return func(PaneID) (CellMetrics, bool) { return metrics, true }
}

// PaneGeometry carries both framebuffer and terminal-grid geometry for a leaf.
type PaneGeometry struct {
	Pane   PaneID
	Pixels PixelRect
	Cols   int
	Rows   int
}

// Divider is the one-pixel framebuffer region between a split's children.
type Divider struct {
	Split     SplitID
	Axis      SplitAxis
	Pixels    PixelRect
	Container PixelRect
}

// Layout is a deterministic value projection of the current split tree.
type Layout struct {
	Panes      []PaneGeometry
	Dividers   []Divider
	Compressed bool
}

// Layout projects the current topology using one metric value for every pane.
// It is retained as the compatibility wrapper for uniform callers.
func (m *Model) Layout(bounds PixelRect, metrics CellMetrics) (Layout, error) {
	if err := validateGeometry(bounds, metrics); err != nil {
		return Layout{}, err
	}
	return m.LayoutWithMetrics(bounds, UniformCellMetrics(metrics))
}

// LayoutWithMetrics projects the current topology using metrics resolved per pane.
func (m *Model) LayoutWithMetrics(bounds PixelRect, resolve CellMetricsResolver) (Layout, error) {
	if err := validateBounds(bounds); err != nil || resolve == nil {
		return Layout{}, ErrInvalidGeometry
	}

	result := Layout{}
	if m.root == nil {
		return result, nil
	}
	if err := layoutNode(m.root, bounds, resolve, &result); err != nil {
		return Layout{}, err
	}
	return result, nil
}

func layoutNode(n *node, rect PixelRect, resolve CellMetricsResolver, result *Layout) error {
	if n.isLeaf() {
		metrics, ok := resolve(n.pane)
		if !ok {
			return ErrPaneNotFound
		}
		if err := validateCellMetrics(metrics); err != nil {
			return err
		}
		cols, rows := cellGeometry(rect, metrics)
		result.Panes = append(result.Panes, PaneGeometry{
			Pane:   n.pane,
			Pixels: rect,
			Cols:   cols,
			Rows:   rows,
		})
		if cols < MinPaneCols || rows < MinPaneRows {
			result.Compressed = true
		}
		return nil
	}

	first, divider, second := splitPixelRect(rect, n.axis, n.ratio)
	if err := layoutNode(n.first, first, resolve, result); err != nil {
		return err
	}
	result.Dividers = append(result.Dividers, Divider{Split: n.split, Axis: n.axis, Pixels: divider, Container: rect})
	return layoutNode(n.second, second, resolve, result)
}

func splitPixelRect(rect PixelRect, axis SplitAxis, ratio SplitRatio) (PixelRect, PixelRect, PixelRect) {
	if axis == SplitColumns {
		firstWidth, dividerWidth, secondWidth := splitExtent(rect.Width, ratio)
		first := PixelRect{X: rect.X, Y: rect.Y, Width: firstWidth, Height: rect.Height}
		divider := PixelRect{X: first.Right(), Y: rect.Y, Width: dividerWidth, Height: rect.Height}
		second := PixelRect{X: divider.Right(), Y: rect.Y, Width: secondWidth, Height: rect.Height}
		return first, divider, second
	}

	firstHeight, dividerHeight, secondHeight := splitExtent(rect.Height, ratio)
	first := PixelRect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: firstHeight}
	divider := PixelRect{X: rect.X, Y: first.Bottom(), Width: rect.Width, Height: dividerHeight}
	second := PixelRect{X: rect.X, Y: divider.Bottom(), Width: rect.Width, Height: secondHeight}
	return first, divider, second
}

func splitExtent(total int, ratio SplitRatio) (first, divider, second int) {
	if total <= 0 {
		return 0, 0, 0
	}
	divider = DividerPixels
	available := total - divider
	// Divide before multiplying so the calculation cannot overflow for a valid
	// platform int. The first child is deliberately floored.
	whole := available / RatioScale
	remainder := available % RatioScale
	first = whole*int(ratio) + remainder*int(ratio)/RatioScale
	second = available - first
	return first, divider, second
}

func cellGeometry(rect PixelRect, metrics CellMetrics) (cols, rows int) {
	usableWidth := rect.Width - 2*metrics.PaddingX
	usableHeight := rect.Height - 2*metrics.PaddingY
	if usableWidth > 0 {
		cols = usableWidth / metrics.CellWidth
	}
	if usableHeight > 0 {
		rows = usableHeight / metrics.CellHeight
	}
	return cols, rows
}

func validateGeometry(bounds PixelRect, metrics CellMetrics) error {
	if err := validateBounds(bounds); err != nil {
		return err
	}
	return validateCellMetrics(metrics)
}

func validateBounds(bounds PixelRect) error {
	if bounds.Width < 0 || bounds.Height < 0 {
		return ErrInvalidGeometry
	}
	return nil
}

func validateCellMetrics(metrics CellMetrics) error {
	if metrics.CellWidth <= 0 || metrics.CellHeight <= 0 || metrics.PaddingX < 0 || metrics.PaddingY < 0 {
		return ErrInvalidGeometry
	}
	return nil
}
