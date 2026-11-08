# System appearance

Every `app.Application` reads the desktop's colour scheme, accent and
contrast preference from xdg-desktop-portal's
`org.freedesktop.portal.Settings` (`org.freedesktop.appearance`) and
reports them with the types in package `appearance`. The toolkit never
changes the palette by itself; apps decide whether to follow.

## Following the preference

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

- `ColorScheme`, `Accent` and `Contrast` are valid once
  `NewApplication` returns.
- `On*Change` hooks run on the loop and fire only on real changes, not
  for the startup value. Each returns an `off` function; a change queued
  before `off` is dropped.
- `appearance.Unknown`, `ContrastUnknown` and an accent with `Known`
  false mean no preference, no portal or an unreadable value.

## Reduced motion

Reduced motion is applied automatically. The portal's `reduced-motion`
key, or `gtk-enable-animations = false` in GTK's `settings.ini` when the
key is absent, sets `widget.SetReducedMotion`:

- springs, smooth scrolling, keyframes, spinners and caret blink stop;
- surface enter and exit become a short fade;
- `Theme.Moves` is false, the motion tokens are zero, and
  `prefers-reduced-motion` matches.

`Application.ReducedMotion` and `OnReducedMotionChange` report it.
`LYS_UI_NO_ANIM=1` also removes the fades.

## Failure behaviour

| Situation | Behaviour |
| --- | --- |
| no session bus | every value reads unknown; no events, no goroutines |
| no portal, or an unreadable key | the value reads unknown; a portal that appears later is read then |
| bus or portal restart | reconnects with backoff from 250 ms to 8 s, re-reads, reports what changed; values keep their last state meanwhile |
| stalled portal | calls time out (3 s probe, 5 s read) and read unknown |

The same monitor follows the icon theme ([Icons](icons.md)).
Programs without an `app.Application` can use `portalsettings`
directly.
