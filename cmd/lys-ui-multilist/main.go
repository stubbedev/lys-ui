// Command lys-ui-multilist is the multi-select list client the headless
// input suite (internal/headlesstest) drives: one window holding a
// multiple-selection widget.List whose selection and activation land
// in the trace log, so synthetic keyboard and pointer gestures can be
// asserted end to end through the compositor.
package main

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/widget"
)

// rows and rowH fix the list geometry; once mapped at the pinned size
// the poller below traces every row's center, so the harness clicks
// real rows instead of hardcoded coordinates.
const (
	rows  = 10
	rowH  = 26
	title = "lys-ui multi-select list"
)

func main() {
	err := run()
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
		// Normal close: the window ended the loop.
	case errors.Is(err, app.ErrDisconnected):
		// The compositor went away; exit with the supervisor code.
		os.Exit(app.DisconnectExitCode)
	default:
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
	application.SetTooltipFace(tf)

	list := widget.NewList(&rowModel{font: tf}, rowH)
	list.SetSelectionMode(widget.SelectionMultiple)

	// fmtRows renders the selection for the trace: "[2,4,5]" — the
	// closing bracket keeps substring waits unambiguous.
	fmtRows := func(sel []int) string {
		if len(sel) == 0 {
			return "[none]"
		}
		parts := make([]string, len(sel))
		for i, r := range sel {
			parts[i] = strconv.Itoa(r)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	list.OnSelectionChanged = func(sel []int) {
		app.Trace("demo", "selection %s", fmtRows(sel))
	}
	list.OnActivate = func(i int) {
		app.Trace("demo", "activated %d", i)
	}

	root := widget.NewBox(widget.Column, 8, 10)
	root.Append(widget.NewLabel(tf, 12, title, widget.Current().TextMuted), false)
	root.Append(list, true)

	var dialog *app.Dialog
	dialogs := 0
	var w *app.Window
	w, err = application.NewWindow(app.WindowConfig{
		Title:      title,
		AppID:      "dev.stubbe.lys.ui.multilist",
		Width:      300,
		Height:     300,
		Root:       root,
		Background: widget.Current().Bg,
		OnKey: func(_ *widget.Router, code uint32, mods app.Mods) {
			// d opens a modal dialog parented to this window, so the
			// headless suite can drive the dialog-modality lifecycle
			// (open, block, respond, restore) end to end.
			if mods&app.ModCtrl != 0 || sess.KeySym(code) != xkb.Keysym('d') {
				return
			}
			if dialog != nil && !dialog.Closed() {
				return
			}
			_, d, err := component.Dialog(application, w, app.DialogConfig{
				Title:           "modal dialog",
				Width:           320,
				Height:          110,
				Modal:           true,
				Buttons:         []app.DialogButton{{Label: "OK", Response: "ok"}},
				DefaultResponse: "ok",
				CancelResponse:  "ok",
			}, &notice{face: tf})
			if err != nil {
				app.Trace("demo", "dialog error %v", err)
				return
			}
			dialog = d
			dialogs++
			app.Trace("demo", "dialog open %d", dialogs)
		},
		OnClosed: func() { app.Trace("demo", "closed") },
	})
	if err != nil {
		return err
	}

	// Deterministic geometry for the headless input tests: once the
	// first configure lands, lay the tree out at the mapped size and
	// trace every row's center. app.Run re-arranges at the same size on
	// every draw, so this arrange changes nothing.
	traced := false
	application.Every(50*time.Millisecond, func() {
		if traced || w.EnsureUsable() != nil {
			return
		}
		traced = true
		ww, wh := w.Size()
		app.Trace("demo", "mapped %dx%d", ww, wh)
		root.Measure(widget.Constraints{Max: widget.Size{W: ww, H: wh}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: ww, H: wh})
		b := list.Bounds()
		for i := range rows {
			app.Trace("demo", "row %d center (%d,%d)", i, b.X+b.W/2, b.Y+i*rowH+rowH/2)
		}
	})

	return application.Run()
}

// rowModel serves ten plain label rows.
type rowModel struct {
	font render.Font
}

func (m *rowModel) Len() int { return rows }

func (m *rowModel) Row(i int) widget.Widget {
	return widget.NewLabel(m.font, 13, "row "+strconv.Itoa(i), widget.Current().Text)
}

// notice is the modal dialog's content as a component: its response
// and its shutdown with the dialog are traced for the harness.
type notice struct{ face render.Font }

func (n *notice) Init(*component.Context[struct{}, struct{}]) widget.Widget {
	return widget.NewLabel(n.face, 13, "input to the parent window is blocked while I am open", widget.Current().Text)
}

func (n *notice) Update(*component.Context[struct{}, struct{}], struct{}) {}

func (n *notice) OnResponse(_ *component.Context[struct{}, struct{}], response string) {
	app.Trace("demo", "dialog response %s", response)
}

func (n *notice) Shutdown(*component.Context[struct{}, struct{}]) {
	app.Trace("demo", "dialog component shut down")
}
