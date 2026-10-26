// Command lys-ui-settings is the M5 settings app: typed persisted keys
// (app/settings.go) behind PreferencesView rows - the adaptive shell
// of pages, view switcher, and built-in search - so every edit writes
// through to <config>/dev.stubbe.lys.ui.settings/settings.json and every
// restart comes back exactly where it left off. Change something,
// quit, run it again.
package main

import (
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

var (
	nameKey    = app.Key[string]{Name: "name", Default: "someone"}
	volumeKey  = app.Key[float64]{Name: "volume", Default: 0.5}
	notifyKey  = app.Key[bool]{Name: "notifications", Default: true}
	accentKey  = app.Key[string]{Name: "accent", Default: "blue"}
	searchKey  = app.Key[bool]{Name: "searchable-rows", Default: true}
	panelKey   = app.Key[bool]{Name: "panel-clock", Default: true}
	denseKey   = app.Key[bool]{Name: "dense", Default: false}
	accentList = []string{"blue", "teal", "green", "yellow", "pink"}
)

type prefs struct {
	env      ui.Env
	settings *app.Settings
}

func (p *prefs) Init(cx *component.Context[struct{}, struct{}]) widget.Widget {
	path := "settings.json"
	if dir, err := os.UserConfigDir(); err == nil {
		path = filepath.Join(dir, "dev.stubbe.lys.ui.settings", "settings.json")
	}
	face := p.env.Face
	root := widget.NewBox(widget.Column, 12, 12)
	root.Append(p.buildPages(face), true)
	root.Append(widget.NewLabel(face, 12, "edits persist to "+path+"; restart to see them; Escape quits", widget.Current().TextMuted), false)
	return ui.Mount(cx, p.env, ui.Use(root))
}

func (p *prefs) Update(*component.Context[struct{}, struct{}], struct{}) {}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "lys-ui settings", AppID: "dev.stubbe.lys.ui.settings",
		Width: 520, Height: 520, Background: widget.Current().Bg,
	}, func(a *app.Application) (*prefs, error) {
		face, err := app.UIFont(14)
		if err != nil {
			return nil, err
		}
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		return &prefs{env: ui.Env{Face: face, Size: 14}, settings: app.NewSettings(a, "dev.stubbe.lys.ui.settings")}, nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func (p *prefs) buildPages(face render.Font) widget.Widget {
	general := widget.NewPreferencesPage()
	general.SetTitle("General")

	identity := widget.NewPreferencesGroup(face, 14, "Identity")
	name := widget.NewEntryRow(face, 14, "Name", nameKey.Get(p.settings))
	name.Entry().OnChanged = func(s string) { nameKey.Set(p.settings, s) }
	identity.Add(name)

	sound := widget.NewPreferencesGroup(face, 14, "Sound & Notifications")
	volume := widget.NewSpinRow(face, 14, "Volume", "", 0, 100, 5, 0, func(v float64) {
		volumeKey.Set(p.settings, v/100)
	})
	volume.SetValue(volumeKey.Get(p.settings) * 100)
	sound.Add(volume)
	notify := widget.NewSwitchRow(face, 14, "Notifications", "", func(on bool) {
		notifyKey.Set(p.settings, on)
	})
	notify.Set(notifyKey.Get(p.settings))
	sound.Add(notify)
	general.Add(identity)
	general.Add(sound)

	appearance := widget.NewPreferencesPage()
	appearance.SetTitle("Appearance")
	colors := widget.NewPreferencesGroup(face, 14, "Accent")
	accent := widget.NewComboRow(face, 14, "Accent color", "", accentList,
		max(slices.Index(accentList, accentKey.Get(p.settings)), 0), func(i int) {
			accentKey.Set(p.settings, accentList[i])
		})
	colors.Add(accent)
	rows := widget.NewPreferencesGroup(face, 14, "Rows")
	toggles := []struct {
		key   app.Key[bool]
		title string
	}{
		{searchKey, "Searchable rows"},
		{panelKey, "Panel clock"},
		{denseKey, "Dense lists"},
	}
	for _, t := range toggles {
		row := widget.NewSwitchRow(face, 14, t.title, "", func(on bool) { t.key.Set(p.settings, on) })
		row.Set(t.key.Get(p.settings))
		rows.Add(row)
	}
	appearance.Add(colors)
	appearance.Add(rows)

	view := widget.NewPreferencesView(face, 14)
	view.Add(general)
	view.Add(appearance)
	return view
}
