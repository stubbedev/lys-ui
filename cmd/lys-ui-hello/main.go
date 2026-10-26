// Command lys-ui-hello is the showcase demo, built the way an app outside
// this module builds one: a component whose view is declared with the
// typed ui builders, hosted in one toplevel window. It exercises the
// core widgets - labels, button, slider, progress bar, switch,
// checkbox, entry, text area, a scrollable list - plus tooltips, an
// animated tween, a right-click context menu bound to typed actions,
// Tab focus traversal, drag-to-move on the chrome, and Escape to close.
// Below the fold (the root scrolls) sits the sixth-pass section: a
// ListBox with filter/sort toggles, a Fixed with positioned children,
// a DrawSurface painting incrementally off a timer, a recycling
// TypedList, an OpenButton, and a stylesheet whose @media rule and
// ::before/::after content run the CSS at-rule and pseudo-element
// paths under the compositor.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

const width, height = 640, 470

// showcaseCSS is the showcase's application stylesheet: the @media
// rule keys off the live window width (app reports every configure to
// the stylesheet engine) and the ::before/::after rules give the
// sixth-pass heading its content, so both CSS paths execute on a real
// session. The window is 640 wide, so the min-width branch matches.
const showcaseCSS = `
@media (min-width: 600px) {
	.sixth-heading::before { content: "sixth "; }
}
.sixth-heading::after { content: " pass"; }
`

// surfaceTickEvery is how often the DrawSurface demo draws its next
// incremental patch: fast enough for the headless cadence check to
// see several ticks, slow enough not to load a software-rendered gate.
const surfaceTickEvery = 700 * time.Millisecond

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
	face, err := app.UIFont(15)
	if err != nil {
		return err
	}

	application := app.NewApplication(sess)
	application.SetClipboard(app.NewClipboard(sess))
	application.SetInspect(true)

	show := &showcase{env: ui.Env{Face: app.FontFallback(face), Size: 14}}
	widget.LoadStylesheet(showcaseCSS)
	theme := widget.NewStateAction("theme", "dark", func(name string) {
		if name == "light" {
			widget.SetTheme(widget.LightTheme())
		} else {
			widget.SetTheme(widget.DarkTheme())
		}
		app.Trace("demo", "theme %s", name)
	})

	var win *app.Window
	var ctrl *component.Controller[showMsg, struct{}]
	pointer := render.Rect{W: 1, H: 1}
	menu := func() []widget.MenuItem {
		return []widget.MenuItem{
			widget.ActionItem("Say hello", widget.NewAction("hello", func() { ctrl.Send(bump{}) })),
			widget.RadioItem("Light theme", theme.Target("light")),
			widget.RadioItem("Dark theme", theme.Target("dark")),
			widget.MenuSeparator(),
			widget.ActionItem("Close window", widget.NewAction("close", func() { win.Close() })),
		}
	}
	cfg := app.WindowConfig{
		Title: "lys-ui showcase", AppID: "dev.stubbe.lys.ui.hello",
		Width: width, Height: height,
		Background:    widget.Current().Bg,
		OnPointerMove: func(x, y float64) { pointer.X, pointer.Y = int(x), int(y) },
		OnResize:      func(w, h int) { show.mapped(w, h) },
		OnPress: func(button uint32, over widget.Widget) {
			switch button {
			case widget.BTNLeft:
				if show.chromeDrag(over) {
					win.BeginMove()
				}
			case widget.BTNRight:
				app.Trace("demo", "menu open at (%d,%d)", pointer.X, pointer.Y)
				if _, err := application.OpenMenuPopover(win, app.MenuPopoverConfig{
					Anchor: anchor(pointer), Face: show.env.Face, SizePx: 13,
					Items: menu(),
				}); err != nil {
					log.Printf("lys-ui-hello: menu: %v", err)
				}
			}
		},
		OnKey: func(_ *widget.Router, code uint32, _ app.Mods) {
			app.Trace("demo", "app key code=%d sym=%v", code, application.KeySym(code))
		},
	}
	open, err := application.NewOpenButton(nil, app.OpenButtonConfig{
		Label: "open a file", StartDir: os.TempDir(), Recents: 3,
		OnOpen: func(path string) { app.Trace("demo", "opened %s", filepath.Base(path)) },
	})
	if err != nil {
		return err
	}
	show.open = open
	ctrl, win, err = component.Window(application, cfg, show)
	if err != nil {
		return err
	}
	application.Every(surfaceTickEvery, show.surfaceTick)
	if err := application.AddScopedAccel(ctrl.Widget(), "Escape", widget.NewAction("close", win.Close)); err != nil {
		return err
	}
	if ms, err := strconv.Atoi(os.Getenv("LYS_UI_DEMO_IDLE_MS")); err == nil && ms > 0 {
		idle := func() { app.Trace("demo", "idle") }
		resume := func() { app.Trace("demo", "resumed") }
		if _, err := application.OnIdle(time.Duration(ms)*time.Millisecond, idle, resume); err != nil {
			app.Trace("demo", "idle notify unavailable: %v", err)
		}
	}
	return application.Run()
}

type anchor render.Rect

func (a anchor) Bounds() render.Rect { return render.Rect(a) }

type showMsg interface{ showMsg() }

type bump struct{}

type noted struct{ text string }

func (bump) showMsg()  {}
func (noted) showMsg() {}

type showcase struct {
	env    ui.Env
	count  int
	status string
	root   widget.Widget
	shown  bool

	button   *widget.Button
	progress *widget.ProgressBar
	slider   *widget.Slider
	sw       *widget.Switch
	check    *widget.CheckButton
	entry    *widget.Entry
	area     *widget.TextArea
	scrolled *widget.Scroll

	rootScroll   *widget.Scroll
	header       widget.Widget
	sixthHead    *widget.Label
	listbox      *widget.ListBox
	listNames    []string
	listSorted   bool
	listFiltered bool
	listFilter   *widget.Button
	listSort     *widget.Button
	typedSort    *widget.Button
	fixed        *widget.Fixed
	fixedLabel   *widget.Label
	fixedBtn     *widget.Button
	fixedX       int
	surface      *widget.DrawSurface
	surfaceN     int
	typed        *widget.TypedList[string]
	typedOn      bool
	open         *app.OpenButton
}

func (s *showcase) Init(cx *component.Context[showMsg, struct{}]) widget.Widget {
	s.status = "events land here"
	note := func(format string, args ...any) func() {
		return func() { cx.Input(noted{text: fmt.Sprintf(format, args...)}) }
	}
	muted := widget.Current().TextMuted
	caption := func(text string) ui.Node { return ui.Label(text).Font(nil, 11).Ink(muted) }

	servers := make([]string, 48)
	for i := range servers {
		servers[i] = fmt.Sprintf("server-%02d.example   up   41ms", i+1)
	}

	header := ui.Column(
		ui.Label("lys-ui showcase").Font(nil, 18).Ink(widget.Current().Accent),
		caption("every widget in one window; drag the header to move, esc closes"),
		ui.Label("fallback check: 你好 world 😀 Привет").Font(nil, 13),
	).Spacing(2).Watch(func(w *widget.Box) { s.header = w })

	controls := ui.Column(
		ui.Button(ui.Row(ui.Label("click me").Font(nil, 15)).Spacing(8), 10, 8).Ref(&s.button).
			OnClick(func() { cx.Input(bump{}) }).
			Tooltip("increments the counter and animates the bar").DebugName("demo:increment"),
		ui.Label("").Font(nil, 15).WatchText(func() string { return fmt.Sprintf("clicked %d times", s.count) }).
			Tooltip("your click total").DebugName("demo:count"),
		ui.ProgressBar(0).Ref(&s.progress),
		ui.Slider(0, 1, 0.05, 0).Ref(&s.slider).
			OnChanged(func(v float64) {
				s.progress.SetValue(v)
				note("slider at %.0f%%", v*100)()
			}).
			Tooltip("drag, or Tab here and use the arrows").DebugName("demo:slider"),
		ui.Row(
			ui.Label("notifications"),
			ui.Switch(true).Ref(&s.sw).OnChanged(func(on bool) { note("switch %v", on)() }).Tooltip("toggles a boolean"),
		).Spacing(8),
		ui.Row(
			ui.CheckButton(false).Ref(&s.check).OnChanged(func(c bool) { note("checkbox %v", c)() }).Tooltip("checkbox state"),
			ui.Label("remember me"),
		).Spacing(8),
	).Spacing(10)

	texts := ui.Column(
		caption("entry"),
		ui.Entry().Ref(&s.entry).Placeholder("type here; ctrl+c/x/v work").
			OnChanged(func(text string) { note("entry: %q", text)() }).
			Tooltip("single-line entry; double-click selects a word").DebugName("demo:entry"),
		caption("text area"),
		ui.TextArea().Font(nil, 13).Ref(&s.area).
			Text("multi-line text area:\nenter splits, backspace joins,\nselection spans lines.").
			Tooltip("multi-line editing"),
		caption("list"),
		ui.Scroll(ui.Column(ui.Each(slices.Values(servers), func(line string) ui.Node {
			return ui.Label(line).Font(nil, 12)
		})...).Spacing(4)).Ref(&s.scrolled).MaxContentHeight(170).ShowBars(true).
			OnScrolled(func(x, y int) { note("list scrolled to %d,%d", x, y)() }).
			Tooltip("scrollable list").DebugName("demo:list"),
	).Spacing(6)

	s.listNames = []string{"delta", "alpha", "echo", "bravo", "foxtrot", "charlie"}
	s.root = ui.Mount(cx, s.env, ui.Scroll(ui.Column(
		header,
		ui.Expand(ui.Row(ui.Expand(controls), ui.Expand(texts)).Spacing(24)),
		ui.Label("").Font(nil, 12).Ink(muted).WatchText(func() string { return s.status }),
		s.sixth(cx, caption),
	).Spacing(12).Padding(render.UniformInsets(16))).
		VerticalOnly(true).ShowBars(true).Ref(&s.rootScroll).
		OnScrolled(func(x, y int) { app.Trace("demo", "root scrolled to %d,%d", x, y) }))
	for _, name := range s.listNames {
		s.listbox.Append(widget.NewLabel(s.env.Face, 12, name, widget.Current().Text))
	}
	s.listbox.SetMinHeight(150)
	s.fixedX = 96
	return s.root
}

// sixth builds the sixth-pass section: the widgets that landed in the
// sixth parity pass, each wired to a demo trace so the headless suite
// can drive it through a live compositor.
func (s *showcase) sixth(cx *component.Context[showMsg, struct{}], caption func(string) ui.Node) ui.Node {
	note := func(format string, args ...any) func() {
		return func() { cx.Input(noted{text: fmt.Sprintf(format, args...)}) }
	}
	muted := widget.Current().TextMuted
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("item-%02d", i)
	}
	factory := widget.TypedFactory[string]{
		Setup: func() widget.Widget {
			return widget.NewLabel(s.env.Face, 12, "", widget.Current().Text)
		},
		Bind: func(item string, row widget.Widget) {
			if l, ok := row.(*widget.Label); ok {
				l.SetText(item)
			}
		},
	}
	name := func(rows []int) string {
		picked := make([]string, len(rows))
		for i, r := range rows {
			picked[i] = s.listNames[r]
		}
		return strings.Join(picked, ",")
	}
	filter := func(r *widget.ListBoxRow) bool { return len(s.listNames[r.Index()]) <= 5 }
	sortRows := func(a, b *widget.ListBoxRow) int {
		return strings.Compare(s.listNames[a.Index()], s.listNames[b.Index()])
	}
	openNode := ui.Node(ui.Label("(open button needs a session)").Font(nil, 11).Ink(muted))
	if s.open != nil {
		openNode = ui.Use(s.open.Widget)
	}
	left := ui.Column(
		ui.Label("").Font(nil, 12).Ink(muted).Ref(&s.sixthHead).Classes("sixth-heading"),
		caption("fixed"),
		ui.Fixed().Ref(&s.fixed).
			Put(ui.Button(ui.Label("move").Font(nil, 11), 4, 8).Ref(&s.fixedBtn).
				OnClick(func() {
					s.fixedX -= 20
					s.fixed.Move(s.fixedLabel, s.fixedX, 6)
					note("fixed moved to (%d,6)", s.fixedX)()
				}).Tooltip("moves the fixed label 20px left").DebugName("demo:fixedbtn"), 4, 2).
			Put(ui.Label("positioned").Font(nil, 11).Ink(muted).Ref(&s.fixedLabel), 96, 6),
		caption("listbox"),
		ui.ListBox().Ref(&s.listbox).
			OnSelectionChanged(func(rows []int) { note("listbox select %s", name(rows))() }).
			OnActivate(func(i int) { note("listbox activate %s", s.listNames[i])() }).
			HeaderFunc(func(r *widget.ListBoxRow) widget.Widget {
				if r.Index() != 0 {
					return nil
				}
				return widget.NewLabel(s.env.Face, 11, "servers", muted)
			}).
			Tooltip("non-virtualized list with filter and sort").DebugName("demo:listbox"),
		ui.Row(
			ui.Button(ui.Label("filter").Font(nil, 11), 4, 8).Ref(&s.listFilter).
				OnClick(func() {
					s.listFiltered = !s.listFiltered
					if s.listFiltered {
						s.listbox.SetFilterFunc(filter)
					} else {
						s.listbox.SetFilterFunc(nil)
					}
					note("listbox filter %v", s.listFiltered)()
				}).Tooltip("hides the rows longer than five letters").DebugName("demo:listfilter"),
			ui.Button(ui.Label("sort").Font(nil, 11), 4, 8).Ref(&s.listSort).
				OnClick(func() {
					s.listSorted = !s.listSorted
					if s.listSorted {
						s.listbox.SetSortFunc(sortRows)
					} else {
						s.listbox.SetSortFunc(nil)
					}
					note("listbox sort %v", s.listSorted)()
				}).Tooltip("sorts the rows alphabetically").DebugName("demo:listsort"),
			ui.Button(ui.Label("typed sort").Font(nil, 11), 4, 8).Ref(&s.typedSort).
				OnClick(func() {
					s.typedOn = !s.typedOn
					if s.typedOn {
						s.typed.SetSort(strings.Compare)
					} else {
						s.typed.SetSort(nil)
					}
					note("typed sort %v", s.typedOn)()
				}).Tooltip("sorts the typed list").DebugName("demo:typedsort"),
		).Spacing(6),
	).Spacing(6)
	right := ui.Column(
		caption("draw surface"),
		ui.DrawSurface(240, 64).Ref(&s.surface).
			OnResize(func(cv *render.Canvas) {
				cv.FillRectDevice(cv.Rect(), widget.Current().Bg)
			}).
			Tooltip("a retained canvas painted incrementally").DebugName("demo:surface"),
		caption("typed list"),
		ui.TypedList(widget.NewTypedItems(items...), factory, 22).MaxHeight(110).Ref(&s.typed).
			OnSelectionChanged(func(picked []string) {
				if len(picked) > 0 {
					note("typed select %s", picked[0])()
				}
			}).
			OnActivate(func(item string) { note("typed activate %s", item)() }).
			Tooltip("100 rows through a recycling viewport").DebugName("demo:typedlist"),
		caption("open button"),
		openNode,
	).Spacing(6)
	return ui.Row(ui.Expand(left), ui.Expand(right)).Spacing(24)
}

// surfaceTick paints one incremental patch on the DrawSurface: the
// retained canvas means only the new rect is drawn and damaged, never
// the surface's history. It runs on the loop from the application's
// timer wheel.
func (s *showcase) surfaceTick() {
	if s.surface == nil {
		return
	}
	b := s.surface.Bounds()
	if b.W < 40 {
		return
	}
	s.surfaceN++
	r := render.Rect{X: (s.surfaceN * 14) % (b.W - 14), Y: 10, W: 12, H: 12}
	s.surface.Draw(r, func(cv *render.Canvas) {
		cv.FillRect(r, widget.Current().Accent)
	})
	app.Trace("demo", "surface drew %d", s.surfaceN)
}

func (s *showcase) Update(_ *component.Context[showMsg, struct{}], msg showMsg) {
	switch m := msg.(type) {
	case bump:
		s.count++
		s.progress.SetValue(0)
		anim.Start(600*time.Millisecond, s.progress.SetValue)
		s.note(fmt.Sprintf("button clicked %d times", s.count))
	case noted:
		s.note(m.text)
	}
}

func (s *showcase) note(text string) {
	s.status = text
	app.Trace("demo", "%s", text)
}

type control struct {
	name string
	w    interface{ Bounds() render.Rect }
}

func (s *showcase) controls() []control {
	out := []control{
		{"button", s.button},
		{"slider", s.slider},
		{"switch", s.sw},
		{"checkbox", s.check},
		{"entry", s.entry},
		{"textarea", s.area},
		{"scroll", s.scrolled},
		{"listbox", s.listbox},
		{"listfilter", s.listFilter},
		{"listsort", s.listSort},
		{"typedsort", s.typedSort},
		{"fixedbtn", s.fixedBtn},
		{"typedlist", s.typed},
		{"surface", s.surface},
	}
	if s.open != nil {
		if box, ok := s.open.Widget.(interface{ Children() []widget.Widget }); ok && len(box.Children()) == 2 {
			kids := box.Children()
			btn, bok := kids[0].(interface{ Bounds() render.Rect })
			dd, dok := kids[1].(interface{ Bounds() render.Rect })
			if bok && dok {
				out = append(out, control{"openfile", btn}, control{"openrecents", dd})
			}
		}
	}
	return out
}

func (s *showcase) mapped(w, h int) {
	if s.shown {
		return
	}
	s.shown = true
	log.Printf("lys-ui-hello: mapped at %dx%d", w, h)
	app.Trace("demo", "mapped %dx%d", w, h)
	s.root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	s.root.Arrange(render.Rect{W: w, H: h})
	mw, mh := widget.MediaViewportOf(s.root)
	app.Trace("demo", "media viewport %dx%d", mw, mh)
	app.Trace("demo", "css before %q after %q",
		widget.PseudoContent(s.sixthHead, "before"), widget.PseudoContent(s.sixthHead, "after"))
	for _, c := range s.controls() {
		b := c.w.Bounds()
		app.Trace("demo", "control %s center (%d,%d)", c.name, b.X+b.W/2, b.Y+b.H/2)
	}
}

// chromeDrag reports whether a left press that landed on over should
// start a window move: only the header is chrome. The showcase's
// content scrolls, and a move grab starves the pressed widget of its
// release - a ListBox row clicks on release, and a DrawSurface press
// belongs to the canvas - so no content press ever drags the frame,
// not even one on a plain label.
func (s *showcase) chromeDrag(over widget.Widget) bool {
	for w := over; w != nil; {
		if w == s.header {
			return true
		}
		if _, isScroll := w.(*widget.Scroll); isScroll {
			return false
		}
		p, ok := w.(interface{ Parent() widget.Widget })
		if !ok {
			return false
		}
		w = p.Parent()
	}
	return false
}
