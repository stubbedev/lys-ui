// Command lys-ui-i18n localizes one app with a real gettext catalog: the
// Norwegian .po embedded under locale/ is loaded through package i18n,
// and every string - lys-ui's built-in chrome (dialog buttons, the file
// picker's places) and the demo's own, a plural included - flows
// through widget.Tr and TrN. It starts in the environment's language
// when a catalog exists for it; the button toggles. Escape quits.
package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/i18n"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

//go:embed locale
var locales embed.FS

type msg int

const (
	toggleLanguage msg = iota
	click
)

type greeter struct {
	app       *app.Application
	env       ui.Env
	norsk     *i18n.Catalog
	norwegian bool
	clicks    int
}

func (g *greeter) Init(cx *component.Context[msg, struct{}]) widget.Widget {
	onAccent := widget.Current().OnAccent
	return ui.Mount(cx, g.env, ui.Column(
		ui.Label("").Font(nil, 16).WatchText(func() string { return widget.Tr("greeting") }),
		ui.Button(ui.Label("norsk / english").Ink(onAccent), 12, 6).
			OnClick(func() { cx.Input(toggleLanguage) }),
		ui.Button(ui.Label("").Ink(onAccent).WatchText(func() string {
			return fmt.Sprintf(widget.TrN("%d click", "%d clicks", g.clicks), g.clicks)
		}), 12, 6).OnClick(func() { cx.Input(click) }),
		ui.Button(ui.Label("").Ink(onAccent).WatchText(func() string { return widget.Tr("question") }), 12, 6).
			OnClick(g.ask),
		ui.Label("").Font(nil, 12).Ink(widget.Current().TextMuted).
			WatchText(func() string { return widget.Tr("the buttons and dialogs follow the catalog; Escape quits") }),
	).Spacing(12).Padding(render.UniformInsets(24)))
}

func (g *greeter) Update(_ *component.Context[msg, struct{}], m msg) {
	switch m {
	case toggleLanguage:
		g.norwegian = !g.norwegian
		g.apply()
	case click:
		g.clicks++
	}
}

func (g *greeter) apply() {
	if g.norwegian {
		widget.SetMessageCatalog(g.norsk)
	} else {
		widget.SetMessageCatalog(nil)
	}
}

func (g *greeter) ask() {
	_, _ = g.app.MessageBox(nil, widget.Tr("question"), widget.Tr("greeting"),
		[]app.DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: app.ButtonRoleCancel},
			{Label: widget.Tr("OK"), Response: "ok", Role: app.ButtonRoleDefault},
		})
}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "lys-ui i18n", AppID: "dev.stubbe.lys.ui.i18n",
		Width: 420, Height: 260, Background: widget.Current().Bg,
	}, func(a *app.Application) (*greeter, error) {
		face, err := app.UIFont(14)
		if err != nil {
			return nil, err
		}
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		norsk, err := i18n.Load(locales, "lys-ui-i18n", []string{"nb"})
		if err != nil {
			return nil, err
		}
		g := &greeter{app: a, env: ui.Env{Face: face, Size: 14}, norsk: norsk}
		for _, l := range i18n.Locales() {
			if l == "nb" || l == "no" {
				g.norwegian = true
			}
		}
		g.apply()
		return g, nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
