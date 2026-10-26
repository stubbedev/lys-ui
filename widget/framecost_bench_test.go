package widget

import (
	"strconv"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// settingsTree is a settings window's shape: a sidebar of rows beside a
// scrolled column of cards, each a few labelled fields, switches,
// dropdowns and buttons.
func settingsTree(tb testing.TB) Widget {
	tb.Helper()
	face := goldenFace(tb)
	side := NewBox(Column, 4, 8)
	for i := range 14 {
		b := NewButton(NewLabel(face, 14, "Section "+strconv.Itoa(i), 0), 8, 6)
		b.AddClass(css.Ghost)
		side.Append(b, false)
	}
	pages := NewBox(Column, 24, 24)
	for c := range 12 {
		card := NewCard(face, "Group "+strconv.Itoa(c))
		card.SetDescription("Settings that belong together, described in a line of muted text.")
		rows := NewBox(Column, 12, 0)
		for r := range 6 {
			row := NewBox(Row, 12, 0)
			text := NewBox(Column, 2, 0)
			text.Append(NewLabel(face, 14, "Option "+strconv.Itoa(r), 0), false)
			hint := NewLabel(face, 12, "What this option changes, in a sentence.", 0)
			hint.AddClass(css.DimLabel)
			text.Append(hint, false)
			row.Append(text, true)
			switch r % 3 {
			case 0:
				row.AppendAligned(NewSwitch(r%2 == 0), false, AlignCenter)
			case 1:
				row.AppendAligned(NewDropdown(face, 14, []string{"Automatic", "Always", "Never"}, 0), false, AlignCenter)
			default:
				e := NewEntry(face, 14, 0)
				e.SetText("value " + strconv.Itoa(r))
				row.AppendAligned(e, false, AlignCenter)
			}
			rows.Append(row, false)
		}
		card.SetContent(rows)
		save := NewButton(NewLabel(face, 14, "Apply", 0), 0, 0)
		save.AddClass(css.SuggestedAction)
		card.AppendFooter(save)
		pages.Append(card, false)
	}
	root := NewBox(Row, 0, 0)
	root.Append(side, false)
	root.Append(NewSeparator(Vertical), false)
	root.Append(NewScroll(pages), true)
	root.AddClass(css.Background)
	return root
}

// settingsFrame lays root out and paints it whole at w x h.
func settingsFrame(root Widget, cv *render.Canvas, w, h int) {
	root.Measure(Constraints{Max: Size{W: w, H: h}})
	root.Arrange(render.Rect{W: w, H: h})
	CollectDamage(root)
	root.Paint(cv)
}

func BenchmarkSettingsFrame(b *testing.B) {
	const w, h = 1605, 1391
	root := settingsTree(b)
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	settingsFrame(root, cv, w, h)
	b.Run("layout", func(b *testing.B) {
		for b.Loop() {
			root.Measure(Constraints{Max: Size{W: w, H: h}})
			root.Arrange(render.Rect{W: w, H: h})
			CollectDamage(root)
		}
	})
	b.Run("paint", func(b *testing.B) {
		for b.Loop() {
			root.Paint(cv)
		}
	})
	b.Run("sweep", func(b *testing.B) {
		i := 0
		for b.Loop() {
			i++
			settingsFrame(root, cv, w-(i*7)%600, h)
		}
	})
	b.Run("resize", func(b *testing.B) {
		i := 0
		for b.Loop() {
			i++
			settingsFrame(root, cv, w-i%2*8, h)
		}
	})
}
