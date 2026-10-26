// Command lys-ui-columns is the tabular/tree view demo (#90): ten
// thousand rows sorted and filtered through the header and the search
// entry (both instant - the view is virtualized), a tree tab with the
// expander column and indent guides, and a drag-to-reorder tab.
// Escape quits.
package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

type row struct {
	Name   string
	Kind   string
	Size   int
	Rating int
}

// tableColumns is the shared column set: name, kind, size, rating.
func tableColumns(tf *render.Typeface, theme *widget.Theme) []widget.TableColumn[row] {
	return []widget.TableColumn[row]{
		{
			Title: "Name", Width: 150, Expand: true,
			Cell: func(r row) widget.Widget {
				return widget.NewLabel(tf, 13, r.Name, theme.Text)
			},
			Sort: func(a, b row) int { return strings.Compare(a.Name, b.Name) },
		},
		{
			Title: "Kind", Width: 80,
			Cell: func(r row) widget.Widget {
				return widget.NewLabel(tf, 13, r.Kind, theme.TextMuted)
			},
			Sort: func(a, b row) int { return strings.Compare(a.Kind, b.Kind) },
		},
		{
			Title: "KiB", Width: 70,
			Cell: func(r row) widget.Widget {
				return widget.NewLabel(tf, 13, strconv.Itoa(r.Size/1024), theme.Text)
			},
			Sort: func(a, b row) int { return a.Size - b.Size },
		},
		{
			Title: "Rating", Width: 60,
			Cell: func(r row) widget.Widget {
				return widget.NewLabel(tf, 13, strings.Repeat("*", r.Rating), theme.Accent)
			},
			Sort: func(a, b row) int { return a.Rating - b.Rating },
		},
	}
}

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	tf, err := app.UIFont(14)
	if err != nil {
		return err
	}

	application := app.NewApplication(sess)
	theme := widget.Current()

	tabs := widget.NewNotebook(tf)
	tabs.AppendTab("10k rows", buildTable(tf, theme))
	tabs.AppendTab("tree", buildTree(tf, theme))
	tabs.AppendTab("reorder", buildReorder(tf, theme))

	hint := widget.NewLabel(tf, 12, "click headers to sort, type to filter, drag rows in the last tab; Escape quits", theme.TextMuted)
	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(hint, false)
	root.Append(tabs, true)

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewWindow(app.WindowConfig{
		Title:      "lys-ui columns",
		AppID:      "dev.stubbe.lys.ui.columns",
		Width:      640,
		Height:     420,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}

// buildTable is the 10k-row tab: filter entry wired to SetFilter,
// header sorting for free, virtualization doing the rest.
func buildTable(tf *render.Typeface, theme *widget.Theme) widget.Widget {
	rows := make(widget.SliceModel[row], 10_000)
	kinds := []string{"doc", "image", "sound", "video", "source"}
	rng := rand.New(rand.NewSource(7)) //nolint:gosec // demo data, not secrets
	for i := range rows {
		rows[i] = row{
			Name:   fmt.Sprintf("item-%05d.%s", i, kinds[rng.Intn(len(kinds))]),
			Kind:   kinds[rng.Intn(len(kinds))],
			Size:   1 << rng.Intn(22),
			Rating: rng.Intn(6),
		}
	}
	v := widget.NewColumnView(tf, 13, rows, tableColumns(tf, theme))
	v.SortBy(0, false)

	search := widget.NewEntry(tf, 13, theme.Text)
	search.SetPlaceholder("filter by name")
	search.OnChanged = func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		if q == "" {
			v.SetFilter(nil)
			return
		}
		v.SetFilter(func(r row) bool { return strings.Contains(strings.ToLower(r.Name), q) })
	}
	box := widget.NewBox(widget.Column, 8, 0)
	box.Append(search, false)
	box.Append(v, true)
	return box
}

// buildTree is the tree tab: a two-level tree rendered through the
// same ColumnView, FlatTree flattening underneath.
func buildTree(tf *render.Typeface, theme *widget.Theme) widget.Widget {
	kids := func(name string, n int) []widget.TreeRow[row] {
		out := make([]widget.TreeRow[row], n)
		for i := range out {
			out[i] = widget.TreeRow[row]{Value: row{Name: name + "/" + strconv.Itoa(i), Kind: "doc", Size: 2048, Rating: 3}}
		}
		return out
	}
	roots := []widget.TreeRow[row]{
		{Value: row{Name: "Documents", Kind: "folder"}, Children: kids("Documents", 4)},
		{Value: row{Name: "Pictures", Kind: "folder"}, Children: kids("Pictures", 3)},
		{Value: row{Name: "readme.md", Kind: "doc", Size: 1024, Rating: 5}},
	}
	tree := widget.NewFlatTree(roots)
	cols := []widget.TableColumn[widget.FlatRow[row]]{
		{
			Title:  "Name",
			Expand: true,
			Cell: widget.TreeCell(tree, 13, func(r row) widget.Widget {
				color := theme.Text
				if r.Kind == "folder" {
					color = theme.TextMuted
				}
				return widget.NewLabel(tf, 13, r.Name, color)
			}),
		},
		{
			Title: "Kind", Width: 80,
			Cell: func(r widget.FlatRow[row]) widget.Widget {
				return widget.NewLabel(tf, 13, r.Value.Kind, theme.TextMuted)
			},
			Sort: func(a, b widget.FlatRow[row]) int { return strings.Compare(a.Value.Kind, b.Value.Kind) },
		},
	}
	v := widget.NewColumnView(tf, 13, tree, cols)
	tree.SetRefresh(v.List().Reset)
	return v
}

// buildReorder is the drag tab: a small list with row drag enabled;
// the move lands in OnRowReorder, the model is reordered, SetModel
// refreshes.
func buildReorder(tf *render.Typeface, theme *widget.Theme) widget.Widget {
	rows := widget.SliceModel[row]{}
	for i := range 8 {
		rows = append(rows, row{Name: "step " + strconv.Itoa(i+1), Kind: "task", Size: i * 1024, Rating: i % 6})
	}
	v := widget.NewColumnView(tf, 13, rows, tableColumns(tf, theme)[:2])
	v.EnableRowDrag(true)
	v.OnRowReorder = func(from, to int) {
		data := append(widget.SliceModel[row]{}, rows...)
		moved := data[from]
		data = append(data[:from], data[from+1:]...)
		data = append(data[:to], append(widget.SliceModel[row]{moved}, data[to:]...)...)
		rows = data
		v.SetModel(rows)
	}
	status := widget.NewLabel(tf, 12, "drag a row onto another to reorder", theme.TextMuted)
	box := widget.NewBox(widget.Column, 8, 0)
	box.Append(v, true)
	box.Append(status, false)
	return box
}
