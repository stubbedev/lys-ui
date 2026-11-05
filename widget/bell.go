package widget

import "sync/atomic"

// errorBell is the installed bell (app rings the widget's window
// through xdg-system-bell); nil makes ErrorBell silent.
var errorBell atomic.Pointer[func(Widget)]

var surfaceFade atomic.Pointer[func(Widget, float64) bool]

// SetSurfaceFade installs fn, which sets the opacity the compositor
// applies to the surface whose root is root and reports whether it
// did: false when root is not a surface's root or the compositor
// cannot fade surfaces. A Revealer at its surface's root fades through
// it instead of repainting its child every frame. nil removes it.
func SetSurfaceFade(fn func(root Widget, alpha float64) bool) {
	if fn == nil {
		surfaceFade.Store(nil)
		return
	}
	surfaceFade.Store(&fn)
}

func fadeSurface(root Widget, alpha float64) bool {
	fn := surfaceFade.Load()
	return fn != nil && (*fn)(root, alpha)
}

// SetErrorBell installs the bell ErrorBell rings; nil silences it.
func SetErrorBell(fn func(w Widget)) {
	if fn == nil {
		errorBell.Store(nil)
		return
	}
	errorBell.Store(&fn)
}

// ErrorBell signals an input w rejected (gtk_widget_error_bell):
// typing into a read-only field, a value that does not parse. The
// application rings the system bell for w's window; the compositor may
// flash it instead.
func ErrorBell(w Widget) {
	if fn := errorBell.Load(); fn != nil {
		(*fn)(w)
	}
}

// RootOf is the top of w's tree: the widget with no parent above it.
func RootOf(w Widget) Widget {
	for {
		p := parentOf(w)
		if p == nil {
			return w
		}
		w = p
	}
}
