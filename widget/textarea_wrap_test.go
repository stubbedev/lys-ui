package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// newWrapArea builds a wrapping area arranged into a fixed-width box.
func newWrapArea(t *testing.T, s string, width int) *TextArea {
	t.Helper()
	ta := newTextArea(t, s)
	ta.Arrange(render.Rect{X: 0, Y: 0, W: width, H: 300})
	ta.Measure(Constraints{Min: Size{}, Max: Size{W: width, H: 1000}})
	return ta
}

func TestSoftWrap(t *testing.T) {
	t.Run("a long line wraps into several visual rows", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60), 120)
		ta.ensureRows(ta.wrapWidth())
		if len(ta.rows) < 4 {
			t.Fatalf("rows = %d, want several for a 120-rune line", len(ta.rows))
		}
		// The rows must tile the logical line without gaps or overlap.
		col := 0
		for _, r := range ta.rows {
			if r.startCol != col {
				t.Fatalf("row starts at %d, want %d", r.startCol, col)
			}
			col = r.endCol
		}
		if col != len(ta.lines[0]) {
			t.Errorf("rows end at %d, want %d", col, len(ta.lines[0]))
		}
	})

	t.Run("wrap width zero keeps one rune per row", func(t *testing.T) {
		ta := newWrapArea(t, "abc", 16) // 16 - 16 padding = 0 usable width
		ta.ensureRows(ta.wrapWidth())
		if len(ta.rows) != 3 {
			t.Fatalf("rows = %d, want one per rune at zero width", len(ta.rows))
		}
		for i, r := range ta.rows {
			if r.endCol-r.startCol != 1 {
				t.Errorf("row %d holds %d runes, want exactly 1", i, r.endCol-r.startCol)
			}
		}
	})

	t.Run("an unbreakable token wider than the viewport wraps rune by rune", func(t *testing.T) {
		token := strings.Repeat("W", 80)
		ta := newWrapArea(t, token, 100)
		ta.ensureRows(ta.wrapWidth())
		if len(ta.rows) < 2 {
			t.Fatal("wide token did not wrap")
		}
		for _, r := range ta.rows {
			if r.endCol <= r.startCol {
				t.Error("empty visual row inside a token")
			}
		}
		if got := ta.Text(); got != token {
			t.Error("wrapping changed the logical text")
		}
	})

	t.Run("click on the second visual row maps into the logical line", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60), 120)
		ta.ensureRows(ta.wrapWidth())
		if len(ta.rows) < 2 {
			t.Fatal("expected at least two visual rows")
		}
		second := ta.rows[1]
		lineH := ta.lineHeight()
		p := Point{X: textPad.Left + 4, Y: textPad.Top + lineH + lineH/2}
		got := ta.posAt(p)
		if got.line != second.line {
			t.Errorf("clicked line = %d, want %d", got.line, second.line)
		}
		if got.col < second.startCol || got.col > second.endCol {
			t.Errorf("clicked col %d outside row range [%d,%d]", got.col, second.startCol, second.endCol)
		}
		if got.col <= ta.rows[0].endCol-ta.rows[0].startCol {
			t.Errorf("click on row 2 resolved into row 1's columns")
		}
	})

	t.Run("vertical motion walks wrapped rows and keeps logical text", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("x", 200)+"\nend", 100)
		ta.ensureRows(ta.wrapWidth())
		ta.SetCursor(0, 0)
		ta.KeyAction(KeyDown, 0)
		ta.KeyAction(KeyDown, 0)
		line, col := ta.CursorPos()
		if line != 0 || col <= 0 {
			t.Errorf("two downs from origin = %d:%d, want later in line 0", line, col)
		}
		steps := 0
		for line < 1 && steps < 500 {
			ta.KeyAction(KeyDown, 0)
			line, col = ta.CursorPos()
			steps++
		}
		if line != 1 {
			t.Fatalf("walk never reached line 1")
		}
		if col != 0 {
			t.Errorf("end of walk = %d:%d, want 1:0 (sticky left edge)", line, col)
		}
	})

	t.Run("home and end move within the visual row", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60), 120)
		ta.ensureRows(ta.wrapWidth())
		// Put the caret on the second visual row.
		ta.SetCursor(0, ta.rows[1].startCol+3)
		ta.KeyAction(KeyHome, 0)
		line, col := ta.CursorPos()
		if line != 0 || col != ta.rows[1].startCol {
			t.Errorf("home = %d:%d, want row start 0:%d", line, col, ta.rows[1].startCol)
		}
		ta.KeyAction(KeyEnd, 0)
		line, col = ta.CursorPos()
		if line != 0 || col != ta.rows[1].endCol {
			t.Errorf("end = %d:%d, want row end 0:%d", line, col, ta.rows[1].endCol)
		}
	})

	t.Run("selection bands follow the logical range across rows", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60), 120)
		ta.ensureRows(ta.wrapWidth())
		start := pos{0, 10}
		end := pos{0, 30}
		covered := 0
		for _, r := range ta.rows {
			from := max(r.startCol, start.col)
			to := min(r.endCol, end.col)
			if from < to {
				covered += to - from
			}
		}
		if covered != end.col-start.col {
			t.Errorf("rows covered %d columns of the selection, want %d", covered, end.col-start.col)
		}
	})

	t.Run("wrap off clips instead", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60), 120)
		ta.SetWrap(false)
		ta.ensureRows(ta.wrapWidth())
		if len(ta.rows) != 1 {
			t.Errorf("wrap off produced %d rows, want one per logical line", len(ta.rows))
		}
	})

	t.Run("editing keeps logical positions canonical", func(t *testing.T) {
		ta := newWrapArea(t, strings.Repeat("ab", 60)+"\nkeep", 120)
		ta.ensureRows(ta.wrapWidth())
		ta.SetCursor(1, 2)
		ta.InsertRune('!')
		if got := ta.Text(); got != strings.Repeat("ab", 60)+"\nke!ep" {
			t.Errorf("text = %q", got)
		}
	})
}

func TestTabTrap(t *testing.T) {
	ta := newTextArea(t, "")
	ta.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})

	t.Run("a focused area traps plain tab as indentation", func(t *testing.T) {
		if !ta.TrapTab(false) {
			t.Fatal("TrapTab reported false")
		}
		if got := ta.Text(); got != "\t" {
			t.Errorf("text = %q, want a tab", got)
		}
	})

	t.Run("indent setting inserts spaces", func(t *testing.T) {
		ta.SetIndent(4)
		ta.TrapTab(false)
		if got := ta.Text(); got != "\t    " {
			t.Errorf("text = %q, want tab plus four spaces", got)
		}
	})
}
