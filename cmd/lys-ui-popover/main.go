// Command lys-ui-popover is the popover client the headless input suite
// (internal/headlesstest) drives: one window whose button opens a
// popover holding a focused Entry and a label a timer keeps changing,
// so typing through the popup grab and loop-driven repaints of an open
// popover can be asserted end to end through the compositor.
package main

import (
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

func main() {
	err := run()
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
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
	t := widget.Current()

	button := widget.NewButton(widget.NewLabel(tf, 13, "open", t.Text), 8, 6)
	menuButton := widget.NewButton(widget.NewLabel(tf, 13, "menu", t.Text), 8, 6)
	root := widget.NewBox(widget.Column, 8, 10)
	root.Append(button, false)
	root.Append(menuButton, false)
	dropButton := widget.NewButton(widget.NewLabel(tf, 13, "drop", t.Text), 8, 6)
	root.Append(dropButton, false)

	// Popovers parent to layer surfaces (a bar's dropdowns): a top-left
	// anchored layer keeps the traced coordinates compositor ones. It
	// names no output, so the compositor picks one: the null output
	// argument the suite exercises on every run.
	if err := sess.Roundtrip(); err != nil {
		return err
	}
	var w *app.LayerWindow
	w, err = application.NewLayer(app.LayerConfig{
		Layer:      app.LayerTop,
		Anchor:     app.AnchorTop | app.AnchorLeft,
		Width:      300,
		Height:     200,
		Namespace:  "lys-ui-popover",
		Root:       root,
		Background: t.Bg,
		OnClosed:   func() { app.Trace("demo", "closed") },
	})
	if err != nil {
		return err
	}

	// The open popover's ticker: the label changes from the loop while
	// nothing else happens, so its frames prove loop-driven repaints.
	ticks := 0
	var tick *widget.Label
	var content *widget.Box
	application.Every(100*time.Millisecond, func() {
		if tick == nil {
			return
		}
		ticks++
		tick.SetText("tick " + strconv.Itoa(ticks))
	})
	button.OnClick = func() {
		entry := widget.NewEntry(tf, 13, t.Text)
		grown := false
		entry.OnChanged = func(s string) {
			app.Trace("demo", "popover text %s", s)
			// The typed text grows the content a row: the popover must
			// grow with it (xdg_popup.reposition), not clip it. Driven
			// by the typing, not a timer, so "text, then grew" holds on
			// a compositor of any speed.
			if s == "hi" && !grown {
				grown = true
				content.Append(widget.NewLabel(tf, 12, "a row added while open", t.TextMuted), false)
				app.Trace("demo", "popover grew")
			}
		}
		ticks = 0
		tick = widget.NewLabel(tf, 12, "tick 0", t.TextMuted)
		content = widget.NewBox(widget.Column, 6, 8)
		content.Append(entry, false)
		content.Append(tick, false)
		_, err := application.OpenPopover(w, app.PopoverConfig{
			Anchor:   button,
			Content:  content,
			Focus:    entry,
			OnClosed: func() { tick = nil; app.Trace("demo", "popover closed") },
		})
		if err != nil {
			app.Trace("demo", "popover error %v", err)
			return
		}
		app.Trace("demo", "popover open")
	}

	// The dropdown: a click-away popover with an exit that takes a while,
	// so the suite sees the keyboard stay with the focused window, a
	// click into it take the keys, and every way out play the exit.
	dropEntry := widget.NewEntry(tf, 13, t.Text)
	dropEntry.OnChanged = func(s string) { app.Trace("demo", "dropdown text %s", s) }
	dropBox := widget.NewBox(widget.Column, 6, 8)
	dropBox.Append(dropEntry, false)
	dropSub := widget.NewButton(widget.NewLabel(tf, 13, "more", t.Text), 8, 6)
	dropBox.Append(dropSub, false)
	var drop *app.Popover
	dropSub.OnClick = func() {
		_, err := application.OpenMenuPopover(w, app.MenuPopoverConfig{
			Anchor:   dropSub,
			Parent:   drop,
			Face:     tf,
			SizePx:   13,
			Items:    []widget.MenuItem{{Label: "leaf", OnClick: func() { app.Trace("demo", "dropdown menu leaf") }}},
			OnClosed: func() { app.Trace("demo", "dropdown menu closed") },
		})
		if err != nil {
			app.Trace("demo", "dropdown menu error %v", err)
			return
		}
		app.Trace("demo", "dropdown menu open")
	}
	dropTraced := false
	application.Every(20*time.Millisecond, func() {
		if drop == nil || drop.Closed() || dropTraced {
			return
		}
		e, s := dropEntry.Bounds(), dropSub.Bounds()
		if s.H == 0 {
			return
		}
		dropTraced = true
		top := dropButton.Bounds().Y + dropButton.Bounds().H - widget.Current().ShadowGutter()
		app.Trace("demo", "dropdown controls entry y %d more y %d", top+e.Y+e.H/2, top+s.Y+s.H/2)
	})
	dropButton.OnClick = func() {
		var err error
		dropTraced = false
		drop, err = application.OpenPopover(w, app.PopoverConfig{
			Anchor:    dropButton,
			Content:   dropBox,
			Dismissal: app.PopoverClickAway,
			Exit: func(done func()) {
				app.Trace("demo", "dropdown exit")
				time.AfterFunc(150*time.Millisecond, func() {
					application.Invoke(func() {
						app.Trace("demo", "dropdown exit done")
						done()
					})
				})
			},
			OnClosed: func() { app.Trace("demo", "dropdown closed") },
		})
		if err != nil {
			app.Trace("demo", "dropdown error %v", err)
			return
		}
		app.Trace("demo", "dropdown open")
	}

	// The nested menu: a submenu row and a leaf inside it, so the suite
	// drives a popover nested under a popover from the keyboard.
	var menu *app.MenuPopover
	menuButton.OnClick = func() {
		m, err := application.OpenMenuPopover(w, app.MenuPopoverConfig{
			Anchor: menuButton,
			Face:   tf,
			SizePx: 13,
			Items: []widget.MenuItem{
				{Label: "Recent", Items: []widget.MenuItem{
					{Label: "a.txt", OnClick: func() { app.Trace("demo", "menu leaf a") }},
				}},
				{Label: "Quit", OnClick: func() { app.Trace("demo", "menu leaf quit") }},
			},
			OnClosed: func() { app.Trace("demo", "menu closed") },
		})
		if err != nil {
			app.Trace("demo", "menu error %v", err)
			return
		}
		menu = m
		app.Trace("demo", "menu open")
	}
	depth := 0
	application.Every(20*time.Millisecond, func() {
		d := 0
		if menu != nil {
			d = menu.Depth()
		}
		if d != depth {
			depth = d
			app.Trace("demo", "menu depth %d", d)
		}
	})

	traced := false
	application.Every(50*time.Millisecond, func() {
		if traced || w.EnsureUsable() != nil {
			return
		}
		traced = true
		ww, wh := w.Size()
		app.Trace("demo", "mapped %dx%d", ww, wh)
		root.Arrange(render.Rect{W: ww, H: wh})
		b := button.Bounds()
		app.Trace("demo", "control open center (%d,%d)", b.X+b.W/2, b.Y+b.H/2)
		mb := menuButton.Bounds()
		app.Trace("demo", "control menu center (%d,%d)", mb.X+mb.W/2, mb.Y+mb.H/2)
		db := dropButton.Bounds()
		app.Trace("demo", "control drop center (%d,%d)", db.X+db.W/2, db.Y+db.H/2)
	})
	return application.Run()
}
