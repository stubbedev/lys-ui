package app

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/portalsettings"
)

// fontPortal answers the two interface keys DesktopFontSize reads.
type fontPortal struct{ font, scale *dbus.Variant }

func (p fontPortal) ReadOne(namespace, key string) (dbus.Variant, *dbus.Error) {
	switch {
	case namespace == "org.gnome.desktop.interface" && key == "font-name" && p.font != nil:
		return *p.font, nil
	case namespace == "org.gnome.desktop.interface" && key == "text-scaling-factor" && p.scale != nil:
		return *p.scale, nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(os.ErrNotExist)
}

// desktopFontApp is an Application whose portal is p on a private bus,
// with settings.ini holding ini.
func desktopFontApp(t *testing.T, p fontPortal, ini string) *Application {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_DIRS", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", home)
	if ini != "" {
		path := filepath.Join(home, "gtk-4.0", "settings.ini")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(ini), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bus := dbustest.Start(t)
	portalConn := bus.Conn(t)
	if err := portalConn.Export(p, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.Settings"); err != nil {
		t.Fatal(err)
	}
	if _, err := portalConn.RequestName("org.freedesktop.portal.Settings", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	m := portalsettings.NewOn(bus.Conn(t))
	t.Cleanup(m.Close)
	return &Application{portal: m}
}

func variant(v any) *dbus.Variant {
	out := dbus.MakeVariant(v)
	return &out
}

func TestDesktopFontSizeIsGTKsRem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		portal fontPortal
		ini    string
		want   float64
	}{
		{"the portal's font at its text scale", fontPortal{font: variant("Adwaita Sans 11"), scale: variant(1.5)}, "[Settings]\ngtk-font-name=Cantarell 20\n", 11 * 144.0 / 72},
		{"settings.ini when the portal has no font", fontPortal{}, "[Settings]\ngtk-font-name=Cantarell 12\ngtk-xft-dpi=122880\n", 12 * 120.0 / 72},
		{"GTK's Sans 10 when nothing sets one", fontPortal{}, "", 10 * 96.0 / 72},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := desktopFontApp(t, tc.portal, tc.ini)
			if got := a.DesktopFontSize(); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("DesktopFontSize = %v, want %v", got, tc.want)
			}
		})
	}
}
