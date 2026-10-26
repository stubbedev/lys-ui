package widget

// GridView is a virtualized grid over a model, GTK's GridView: cells
// of one size flow left to right in as many columns as the width fits
// (between the minimum and maximum column counts), only the visible
// lines exist as widgets, and selection, rubber band, keyboard motion
// in two dimensions and activation work as in List. It styles as
// `gridview` with `child` cells.
type GridView struct {
	List
}

// NewGridView returns a grid over model with cells of at least
// cellWidth by cellHeight pixels, between 1 and 7 columns (GTK's
// defaults).
func NewGridView[W Widget](model ListModel[W], cellWidth, cellHeight int) *GridView {
	g := &GridView{}
	initList(&g.List, model, cellHeight)
	g.SetElement("gridview")
	g.rowElement = "child"
	g.cellW = max(1, cellWidth)
	g.minCols, g.maxCols = 1, 7
	return g
}

// SetMinColumns sets the fewest columns the grid lays out, even when
// cells end up narrower than their width.
func (g *GridView) SetMinColumns(n int) {
	if n != g.minCols {
		g.minCols = max(1, n)
		g.InvalidateLayout()
	}
}

// SetMaxColumns sets the most columns the grid lays out; zero lifts
// the cap.
func (g *GridView) SetMaxColumns(n int) {
	if n != g.maxCols {
		g.maxCols = max(0, n)
		g.InvalidateLayout()
	}
}

// SetColumnSpacing sets the gap between columns; the stylesheet's
// horizontal border-spacing adds to it. A point in a gap hits no cell.
func (g *GridView) SetColumnSpacing(n int) {
	if n != g.colSpacing {
		g.colSpacing = max(0, n)
		g.InvalidateLayout()
	}
}

// SetRowSpacing sets the gap between lines; the stylesheet's vertical
// border-spacing adds to it.
func (g *GridView) SetRowSpacing(n int) {
	if n != g.rowSpacing {
		g.rowSpacing = max(0, n)
		g.InvalidateLayout()
	}
}

// ColumnSpacing returns the programmatic gap between columns.
func (g *GridView) ColumnSpacing() int { return g.colSpacing }

// RowSpacing returns the programmatic gap between lines.
func (g *GridView) RowSpacing() int { return g.rowSpacing }

// MinColumns returns the column floor.
func (g *GridView) MinColumns() int { return g.minCols }

// MaxColumns returns the column cap, zero for none.
func (g *GridView) MaxColumns() int { return g.maxCols }

// Columns returns the column count of the last layout.
func (g *GridView) Columns() int { return g.columns() }
