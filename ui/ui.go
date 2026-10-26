// Package ui declares widget trees with typed builders, the
// counterpart of relm4's view! macro. Every widget has a builder: a
// constructor per widget.New* function, a chainable method per setter,
// adder, binder and exported field, and the props all widgets share.
// Builders are descriptions. Mount builds one inside a component, so
// its Watch and Track refreshes follow the component's updates; Build
// builds one on its own.
//
//	ui.Column(
//		ui.Label("count").Ref(&c.label),
//		ui.Button(ui.Label("+"), 8, 6).OnClick(func() { cx.Input(Increment) }),
//		ui.Label("").WatchText(func() string { return strconv.Itoa(c.n) }),
//	)
//
// Templates are ordinary functions returning a Node.
package ui

//go:generate go run ../internal/cmd/uigen ../widget widgets_gen.go

import (
	"cmp"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/widget"
)

// Node is a buildable piece of a widget tree.
type Node interface {
	Build(s *Scope) widget.Widget
}

// Of is a Node that builds a widget of a known type, so other builders
// can take it where a widget constructor needs one: a ViewSwitcher's
// Stack, a scrollbar's target.
type Of[W widget.Widget] interface {
	Node
	Widget(s *Scope) W
}

// Env is what builders read when a widget constructor needs a value the
// description leaves out: the typeface, the text size, and the text
// color. A zero Ink leaves the color to the cascade: the text inherits
// its container's color (the theme's text color at the root, the
// accent foreground on a suggested-action button) and follows theme
// switches.
type Env struct {
	Face render.Font
	Size float64
	Ink  render.Color
}

type refresher interface {
	Watch(fn func())
	Track(fn func(), deps ...component.Change)
	OnShutdown(fn func())
}

// Scope is one build of a description: its environment, where refreshes
// and cleanups register, and the widgets built so far. Building the
// same builder twice in one scope yields the same widget, so one
// builder can be placed in the tree and referenced from another.
type Scope struct {
	env     Env
	refresh refresher
	built   map[any]widget.Widget
}

func newScope(env Env, r refresher) *Scope {
	return &Scope{env: env, refresh: r, built: map[any]widget.Widget{}}
}

// Env returns the scope's environment.
func (s *Scope) Env() Env { return s.env }

func (s *Scope) face(f render.Font) render.Font {
	if f != nil {
		return f
	}
	if s.env.Face == nil {
		panic("ui: a text widget needs a typeface: set Env.Face or call Font on its builder")
	}
	return s.env.Face
}

func (s *Scope) size(px float64) float64 {
	if px > 0 {
		return px
	}
	if s.env.Size <= 0 {
		panic("ui: a text widget needs a size: set Env.Size or call Font on its builder")
	}
	return s.env.Size
}

func (s *Scope) ink(c render.Color) render.Color {
	return cmp.Or(c, s.env.Ink)
}

func (s *Scope) watch(fn func()) { s.refresh.Watch(fn) }

func (s *Scope) track(fn func(), deps []component.Change) { s.refresh.Track(fn, deps...) }

func (s *Scope) onShutdown(fn func()) { s.refresh.OnShutdown(fn) }

func build(s *Scope, n Node) widget.Widget {
	if n == nil {
		return nil
	}
	return n.Build(s)
}

func buildAll(s *Scope, ns []Node) []widget.Widget {
	ws := make([]widget.Widget, 0, len(ns))
	for _, n := range ns {
		ws = append(ws, build(s, n))
	}
	return ws
}

func buildOf[W widget.Widget](s *Scope, n Of[W]) W {
	if n == nil {
		var zero W
		return zero
	}
	return n.Widget(s)
}

// Mount builds n as the view of the component cx belongs to: Watch and
// Track refreshes run after the component's updates, and cleanups run
// when it shuts down.
func Mount[In, Out any](cx *component.Context[In, Out], env Env, n Node) widget.Widget {
	return newScope(env, cx).root(n)
}

// Build builds n on its own. View.Refresh runs its Watch and Track
// refreshes; View.Close runs its cleanups.
func Build(env Env, n Node) (widget.Widget, *View) {
	v := &View{}
	return newScope(env, v).root(n), v
}

func (s *Scope) root(n Node) widget.Widget {
	w := build(s, n)
	if w == nil {
		panic("ui: the root node built no widget")
	}
	return w
}

// View is a description built with Build: its refreshes and cleanups.
type View struct {
	refreshes []func()
	cleanups  []func()
}

// Watch registers fn to run now and on every Refresh.
func (v *View) Watch(fn func()) {
	v.refreshes = append(v.refreshes, fn)
	fn()
}

// Track registers fn to run now, and on Refresh when a dependency
// changed.
func (v *View) Track(fn func(), deps ...component.Change) {
	v.refreshes = append(v.refreshes, func() {
		for _, d := range deps {
			if d.Changed() {
				fn()
				return
			}
		}
	})
	fn()
}

// OnShutdown registers fn to run on Close.
func (v *View) OnShutdown(fn func()) { v.cleanups = append(v.cleanups, fn) }

// Refresh runs the view's refreshes in registration order.
func (v *View) Refresh() {
	for _, fn := range v.refreshes {
		fn()
	}
}

// Close runs the view's cleanups in reverse order, once.
func (v *View) Close() {
	cleanups := v.cleanups
	v.cleanups = nil
	for _, fn := range slices.Backward(cleanups) {
		fn()
	}
}
