// Command lys-ui-launcher is the M5 launcher app: a keyboard-exclusive
// layer-shell overlay holding a search entry that filters a grid of
// apps, and live thumbnails of the compositor's open toplevels
// captured through the capture package on a private connection.
// Escape dismisses; a click or Enter launches the app detached.
package main

import (
	"errors"
	"image"
	"log"
	"os/exec"
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/draw"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

type appEntry struct {
	name string
	icon string
	exec string
}

var apps = []appEntry{
	{name: "Files", icon: "system-file-manager", exec: "nautilus"},
	{name: "Terminal", icon: "utilities-terminal", exec: "foot"},
	{name: "Browser", icon: "applications-internet", exec: "firefox"},
	{name: "Text Editor", icon: "accessories-text-editor", exec: "gedit"},
	{name: "Image Viewer", icon: "applications-graphics", exec: "eog"},
	{name: "Media Player", icon: "applications-multimedia", exec: "mpv"},
	{name: "Calculator", icon: "accessories-calculator", exec: "gnome-calculator"},
	{name: "Settings", icon: "preferences-system", exec: "gnome-control-center"},
	{name: "Mail", icon: "internet-mail", exec: "thunderbird"},
	{name: "Calendar", icon: "office-calendar", exec: "gnome-calendar"},
	{name: "Software", icon: "system-software-install", exec: "gnome-software"},
	{name: "Monitor", icon: "utilities-system-monitor", exec: "htop"},
}

func filterApps(entries []appEntry, query string) []appEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return entries
	}
	return slices.DeleteFunc(slices.Clone(entries), func(a appEntry) bool {
		return !strings.Contains(strings.ToLower(a.name), q)
	})
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
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	application := app.NewApplication(sess)
	theme := widget.Current()

	grid := widget.NewFlowBox(12, 12)
	grid.SetSingleClickActivate(true)
	fillGrid(grid, face, theme, apps, launch)
	search := widget.NewSearchEntry(face, 14, "type to filter")
	search.OnSearchChanged = func(q string) { fillGrid(grid, face, theme, filterApps(apps, q), launch) }

	windowsLabel := widget.NewLabel(face, 12, "open windows", theme.TextMuted)
	windows := widget.NewFlowBox(8, 8)
	body := widget.NewBox(widget.Column, 12, 16)
	body.Append(search, false)
	body.Append(widget.NewScroll(grid), true)
	body.Append(windowsLabel, false)
	body.Append(windows, false)

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewLayer(app.LayerConfig{
		Layer:      app.LayerOverlay,
		Anchor:     app.AnchorTop | app.AnchorLeft | app.AnchorRight,
		Margin:     app.Margins{Top: 80},
		Height:     480,
		Keyboard:   app.KeyboardExclusive,
		Namespace:  "lys-ui-launcher",
		Root:       body,
		Background: render.RGBA(0x1E, 0x1E, 0x2E, 235),
	}); err != nil {
		return err
	}
	loadThumbnails(application, face, theme, windows)
	return application.Run()
}

type launcher func(appEntry)

func launch(a appEntry) {
	cmd := exec.Command(a.exec) //nolint:gosec // the launcher's whole job: run the picked app
	if err := cmd.Start(); err != nil {
		log.Printf("lys-ui-launcher: %v", err)
	}
}

func fillGrid(grid *widget.FlowBox, face *render.Typeface, theme *widget.Theme, entries []appEntry, launch launcher) {
	grid.Clear()
	for _, a := range entries {
		tile := widget.NewBox(widget.Column, 6, 4)
		tile.Append(widget.NewThemeIcon(a.icon, 32), false)
		tile.Append(widget.NewLabel(face, 12, a.name, theme.Text), false)
		grid.Append(tile)
	}
	grid.OnActivate = func(i int) {
		if i < len(entries) {
			launch(entries[i])
		}
	}
	grid.InvalidateLayout()
}

func loadThumbnails(application *app.Application, face *render.Typeface, theme *widget.Theme, strip *widget.FlowBox) {
	go func() {
		client, err := capture.Connect()
		if err != nil {
			return
		}
		defer func() { _ = client.Close() }()
		if !client.HasToplevelCapture() {
			return
		}
		toplevels, err := client.Toplevels()
		if err != nil || len(toplevels) == 0 {
			return
		}
		for _, t := range toplevels {
			frame, err := client.CaptureToplevel(t, capture.Options{})
			if err != nil {
				continue
			}
			img, err := frame.Image()
			if err != nil {
				continue
			}
			thumb := scaleThumb(img)
			title := t.Title
			application.Invoke(func() {
				tile := widget.NewBox(widget.Column, 4, 0)
				tile.Append(widget.NewImage(thumb), false)
				tile.Append(widget.NewLabel(face, 11, title, theme.TextMuted), false)
				strip.Append(tile)
			})
		}
	}()
}

func scaleThumb(img image.Image) image.Image {
	b := img.Bounds()
	const h = 72
	w := max(b.Dx()*h/max(b.Dy(), 1), 1)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
