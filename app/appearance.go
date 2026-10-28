package app

import (
	"sync"
	"sync/atomic"

	"github.com/stubbedev/lys-lib/xdg"

	"github.com/stubbedev/lys-ui/appearance"
	"github.com/stubbedev/lys-ui/gtksettings"
	"github.com/stubbedev/lys-ui/internal/logutil"
)

// ColorScheme returns the desktop's current dark/light preference,
// appearance.Unknown without a portal. It is valid from NewApplication
// on, so the first frame can be themed from it.
func (a *Application) ColorScheme() appearance.ColorScheme { return a.portal.ColorScheme() }

// OnColorSchemeChange registers fn for every color-scheme change. fn
// runs on the loop goroutine, so it may call widget.SetTheme directly.
// The returned function unregisters it; a change already queued when it
// runs is dropped.
func (a *Application) OnColorSchemeChange(fn func(appearance.ColorScheme)) (off func()) {
	return onLoop(a, a.portal.OnColorSchemeChange, fn)
}

// Accent returns the desktop's accent-color preference.
func (a *Application) Accent() appearance.Accent { return a.portal.Accent() }

// OnAccentChange registers fn for every accent-color change, on the
// loop goroutine like OnColorSchemeChange.
func (a *Application) OnAccentChange(fn func(appearance.Accent)) (off func()) {
	return onLoop(a, a.portal.OnAccentChange, fn)
}

// Contrast returns the desktop's contrast preference.
func (a *Application) Contrast() appearance.Contrast { return a.portal.Contrast() }

// OnContrastChange registers fn for every contrast change, on the loop
// goroutine like OnColorSchemeChange.
func (a *Application) OnContrastChange(fn func(appearance.Contrast)) (off func()) {
	return onLoop(a, a.portal.OnContrastChange, fn)
}

// DesktopFontSize is the default font size, in logical pixels, a GTK
// app on this desktop uses - what 1rem is there: the desktop font's
// size (the portal's font-name, else settings.ini's gtk-font-name, else
// GTK's "Sans 10") at its text DPI (the portal's text-scaling-factor,
// else gtk-xft-dpi, else 96). lys-ui keeps its own 16px rem unless the
// app applies this with widget.SetRootFontSize.
func (a *Application) DesktopFontSize() float64 {
	ini := a.gtkSettings()
	return appearance.ResolveFontSettings(a.portal.FontName(), a.portal.TextScale(), ini.FontName, ini.XftDPI.DotsPerInch()).DefaultFontPx()
}

// OnDesktopFontSizeChange registers fn for every change of
// DesktopFontSize (a portal font-name or text-scaling-factor change
// that moves it), on the loop goroutine like OnColorSchemeChange.
func (a *Application) OnDesktopFontSizeChange(fn func(px float64)) (off func()) {
	last := a.DesktopFontSize()
	report := func() {
		if px := a.DesktopFontSize(); px != last {
			last = px
			fn(px)
		}
	}
	offFont := onLoop(a, a.portal.OnFontNameChange, func(string) { report() })
	offScale := onLoop(a, a.portal.OnTextScaleChange, func(float64) { report() })
	return sync.OnceFunc(func() {
		offFont()
		offScale()
	})
}

// gtkSettings reads settings.ini once per application. A file that
// cannot be read is reported and the readable ones still apply, as GTK
// warns and carries on.
func (a *Application) gtkSettings() gtksettings.Settings {
	a.gtkIniOnce.Do(func() {
		ini, err := gtksettings.Load(xdg.OS)
		if err != nil {
			logutil.L().Warn("app: settings.ini", "err", err)
		}
		a.gtkIni = ini
	})
	return a.gtkIni
}

func onLoop[T any](a *Application, register func(func(T)) func(), fn func(T)) (off func()) {
	var stopped atomic.Bool
	unregister := register(func(v T) {
		a.Invoke(func() {
			if !stopped.Load() {
				fn(v)
			}
		})
	})
	return sync.OnceFunc(func() {
		stopped.Store(true)
		unregister()
	})
}
