package portalsettings

import (
	"testing"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/appearance"
)

// TestAccentValue pins the accent mapping: a (ddd) triple inside a
// variant maps with Known; wrong arity, wrong types, out-of-range
// components, and bare values without the variant map to no
// preference rather than a guessed color.
func TestAccentValue(t *testing.T) {
	got := accentValue(dbus.MakeVariant([]float64{0.25, 0.5, 0.75}))
	if !got.Known || got.R != 0.25 || got.G != 0.5 || got.B != 0.75 {
		t.Errorf("triple mapped to %+v", got)
	}
	loose := accentValue(dbus.MakeVariant([]any{float64(0.1), float64(0.2), float64(0.3)}))
	if !loose.Known || loose.B != 0.3 {
		t.Errorf("loose triple mapped to %+v", loose)
	}
	for _, bad := range []any{
		dbus.MakeVariant([]float64{0.5}),
		dbus.MakeVariant([]float64{0.5, 0.5, 1.5}),
		dbus.MakeVariant([]any{float64(0.5), float64(0.5), "blue"}),
		dbus.MakeVariant("purple"),
	} {
		if v := accentValue(bad); v.Known {
			t.Errorf("%v mapped to %+v, want unknown", bad, v)
		}
	}
}

// TestContrastValue pins the contrast mapping: 1 is high, 0 and
// anything else are no preference.
func TestContrastValue(t *testing.T) {
	if got := contrastValue(dbus.MakeVariant(uint32(1))); got != appearance.ContrastHigh {
		t.Errorf("1 mapped to %v", got)
	}
	for _, v := range []any{dbus.MakeVariant(uint32(0)), dbus.MakeVariant(uint32(7)), dbus.MakeVariant("high")} {
		if got := contrastValue(v); got != appearance.ContrastUnknown {
			t.Errorf("%v mapped to %v, want unknown", v, got)
		}
	}
}

// TestMonitorSignalRouting pins the dispatch for all four settings,
// fed signals directly: the right key routes to the right setting,
// foreign keys and namespaces change nothing, and duplicates do not
// re-fire listeners.
func TestMonitorSignalRouting(t *testing.T) {
	m := &Monitor{}
	var scheme, icons, accents, contrasts int
	m.OnColorSchemeChange(func(appearance.ColorScheme) { scheme++ })
	m.OnIconThemeChange(func(string) { icons++ })
	m.OnAccentChange(func(appearance.Accent) { accents++ })
	m.OnContrastChange(func(appearance.Contrast) { contrasts++ })

	settingSignal := func(ns, key string, value any) *dbus.Signal {
		return &dbus.Signal{Name: changedSig, Path: portalPath, Body: []any{ns, key, value}}
	}
	m.signal(settingSignal(schemeNamespace, schemeKey, dbus.MakeVariant(uint32(1))))
	m.signal(settingSignal(schemeNamespace, schemeKey, dbus.MakeVariant(uint32(1)))) // duplicate drops
	m.signal(settingSignal(interfaceNamespace, iconThemeKey, dbus.MakeVariant("Papirus")))
	m.signal(settingSignal(schemeNamespace, accentKey, dbus.MakeVariant([]float64{0.2, 0.4, 0.9})))
	m.signal(settingSignal(schemeNamespace, contrastKey, dbus.MakeVariant(uint32(1))))
	m.signal(settingSignal(schemeNamespace, "gtk-theme", dbus.MakeVariant("Adwaita")))
	m.signal(settingSignal("org.other", schemeKey, dbus.MakeVariant(uint32(2))))
	m.signal(&dbus.Signal{Name: changedSig, Path: "/somewhere/else", Body: []any{schemeNamespace, schemeKey, dbus.MakeVariant(uint32(2))}})

	if scheme != 1 || icons != 1 || accents != 1 || contrasts != 1 {
		t.Errorf("fired scheme=%d icons=%d accents=%d contrasts=%d, want 1 each",
			scheme, icons, accents, contrasts)
	}
	if m.ColorScheme() != appearance.Dark {
		t.Errorf("appearance = %v", m.ColorScheme())
	}
	if m.IconTheme() != "Papirus" {
		t.Errorf("icon theme = %q", m.IconTheme())
	}
	if a := m.Accent(); !a.Known || a.B != 0.9 {
		t.Errorf("accent = %+v", a)
	}
	if m.Contrast() != appearance.ContrastHigh {
		t.Errorf("contrast = %v", m.Contrast())
	}
}

// TestSettingUnregisterTombstones pins the off functions: an
// unregistered listener stops firing without disturbing its neighbors.
func TestSettingUnregisterTombstones(t *testing.T) {
	m := &Monitor{}
	var first, second int
	off := m.OnAccentChange(func(appearance.Accent) { first++ })
	m.OnAccentChange(func(appearance.Accent) { second++ })
	off()
	m.signal(&dbus.Signal{Name: changedSig, Path: portalPath, Body: []any{
		schemeNamespace, accentKey, dbus.MakeVariant([]float64{0.9, 0.1, 0.1}),
	}})
	if first != 0 || second != 1 {
		t.Errorf("after unregister: first=%d second=%d, want 0 and 1", first, second)
	}
}
