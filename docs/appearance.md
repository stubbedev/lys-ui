# System appearance

How an app follows the desktop's color scheme, accent color and
contrast preference, and why lys-ui never applies them on its own.

The preferences live in xdg-desktop-portal's
`org.freedesktop.portal.Settings` (`org.freedesktop.appearance`
namespace), which Hyprland, GNOME, KDE and others all publish. Every
`app.Application` watches them over the session bus in pure Go and
reports them with the public types in package `appearance`.

## Following the preference

Theming is explicit (`widget.SetTheme`, [architecture.md](architecture.md#theming)):
the toolkit never switches palettes by itself, so an app with a fixed
brand simply never follows. Following is a few lines:

```go
follow := func(s appearance.ColorScheme) {
	switch s {
	case appearance.Dark:
		widget.SetTheme(widget.DarkTheme())
	case appearance.Light:
		widget.SetTheme(widget.LightTheme())
	}
}
follow(application.ColorScheme())
application.OnColorSchemeChange(follow)

application.OnAccentChange(func(a appearance.Accent) {
	if c, ok := a.Color(); ok {
		widget.SetTheme(widget.Current().WithAccent(c))
	}
})

application.OnContrastChange(func(c appearance.Contrast) {
	if c == appearance.ContrastHigh {
		widget.SetTheme(widget.HighContrastTheme())
	}
})
```

- The getters (`ColorScheme`, `Accent`, `Contrast`) are valid as soon
  as `NewApplication` returns: the startup read is synchronous.
- The `On*Change` hooks run on the loop goroutine, so they may touch
  widgets directly. Each returns an `off` function, and a change queued
  before `off` ran is dropped.
- The startup read is a baseline, never an event.
- Duplicate announcements are deduplicated, so hooks hear real changes
  only.

`appearance.Unknown`, `ContrastUnknown` and an accent with `Known`
false mean no preference, no portal, or an unreadable value. They are a
reason to keep the current theme.

## Reduced motion

The portal's `reduced-motion` key, and `gtk-enable-animations = false` in
`settings.ini` where it is not set, is the one preference lys-ui applies on its
own, because an app has no reason to overrule it and should not need to know:
`NewApplication` calls `widget.SetReducedMotion` and follows changes. Springs,
smooth scrolling, keyframes, spinners and the caret's blink stop; a surface's
enter and exit become a short fade; `Theme.Moves` answers false, so the motion
tokens are zero and `prefers-reduced-motion` matches. `Application.ReducedMotion`
and `OnReducedMotionChange` report it. `LYS_UI_NO_ANIM=1` goes further and
collapses fades as well.

## Failure model

- **No session bus.** The monitor is inert: everything reads unknown
  forever, no events fire, and no goroutines run.
- **No portal, or the key is unreadable.** The values read unknown. The
  name-ownership probe is a fast bus call, so lys-ui never waits on dbus
  activation at startup. A portal that appears later is picked up on
  the next start.
- **The bus or portal restarts mid-session.** The monitor reconnects
  with exponential backoff (250ms doubling to 8s), re-reads every key,
  and reports any preference that changed while it was disconnected.
- **Bounded calls.** Every bus round trip is bounded (3s probe, 5s
  read), so a stalled portal costs an unknown, never a hung startup.

The same monitor follows the icon-theme setting for the icon lookup
([icons.md](icons.md)). That is a lookup refresh, never a palette swap.

## Tests

`internal/portalsettings` runs against a real private `dbus-daemon`
with a scripted mock portal. The tests cover:

- the startup read and every unknown fallback
- ordered, deduplicated delivery of signal bursts
- no-portal and no-bus inertia
- reconnecting across a bus restart
- `Close` draining goroutines

They fail when no `dbus-daemon` binary exists. `app/appearance_test.go`
pins loop delivery: hooks run on the pump, in order, and nothing is
delivered after `off`.
