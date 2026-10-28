package gtksettings

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/xdg"
)

func env(vars map[string]string) xdg.Env {
	return func(name string) string { return vars[name] }
}

func TestParseReadsTheSettingsGroup(t *testing.T) {
	got := Parse(`# a comment
[Settings]
gtk-application-prefer-dark-theme=1
gtk-theme-name = Adwaita-dark
gtk-font-name = "Adwaita Sans 11"
; another comment
gtk-xft-dpi=122880
gtk-icon-theme-name="Papirus"
gtk-cursor-theme-name = Vimix
gtk-cursor-theme-size=22
[Other]
gtk-font-name=Ignored 9
`)
	want := Settings{
		ThemeName:       "Adwaita-dark",
		IconThemeName:   "Papirus",
		CursorThemeName: "Vimix",
		CursorThemeSize: 22,
		FontName:        "Adwaita Sans 11",
		XftDPI:          122880,
	}
	if got != want {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
	if dpi := got.XftDPI.DotsPerInch(); dpi != 120 {
		t.Errorf("DotsPerInch = %v, want 120", dpi)
	}
}

func TestParseStripsOnlyBalancedQuotes(t *testing.T) {
	for data, want := range map[string]string{
		`gtk-font-name="Sans 12"`: "Sans 12",
		`gtk-font-name="Sans 12`:  `"Sans 12`,
		`gtk-font-name=Sans "12"`: `Sans "12"`,
	} {
		if got := Parse("[Settings]\n" + data + "\n").FontName; got != want {
			t.Errorf("%s: FontName = %q, want %q", data, got, want)
		}
	}
}

func TestParseIgnoresWhatGTKCannotUse(t *testing.T) {
	for name, data := range map[string]string{
		"outside [Settings]":        "gtk-font-name=Sans 12\n",
		"GTK's default DPI":         "[Settings]\ngtk-xft-dpi=-1\n",
		"a non-numeric DPI":         "[Settings]\ngtk-xft-dpi=lots\n",
		"a DPI past int32":          "[Settings]\ngtk-xft-dpi=4294967296\n",
		"a negative cursor size":    "[Settings]\ngtk-cursor-theme-size=-24\n",
		"a non-numeric cursor size": "[Settings]\ngtk-cursor-theme-size=big\n",
		"no value":                  "[Settings]\ngtk-font-name\n",
	} {
		if got := Parse(data); got != (Settings{}) {
			t.Errorf("%s: Parse = %+v, want nothing", name, got)
		}
	}
}

func TestPathsLoadSystemDirsLeastImportantFirstThenTheUser(t *testing.T) {
	got := paths(env(map[string]string{"XDG_CONFIG_DIRS": "/a:relative:/b", "XDG_CONFIG_HOME": "/home/u/cfg", "HOME": "/home/u"}))
	want := []string{"/b/gtk-4.0/settings.ini", "/a/gtk-4.0/settings.ini", "/home/u/cfg/gtk-4.0/settings.ini"}
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
	got = paths(env(map[string]string{"XDG_CONFIG_DIRS": "relative", "XDG_CONFIG_HOME": "relative", "HOME": "/home/u"}))
	want = []string{"/etc/xdg/gtk-4.0/settings.ini", "/home/u/.config/gtk-4.0/settings.ini"}
	if !slices.Equal(got, want) {
		t.Errorf("default paths = %q, want %q", got, want)
	}
	got = paths(env(nil))
	want = []string{"/etc/xdg/gtk-4.0/settings.ini"}
	if !slices.Equal(got, want) {
		t.Errorf("paths without a home = %q, want %q", got, want)
	}
}

func writeSettings(t *testing.T, dir, version, data string) {
	t.Helper()
	path := filepath.Join(dir, version, "settings.ini")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, vars map[string]string) Settings {
	t.Helper()
	s, err := Load(env(vars))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestLoadLetsLaterFilesOverrideKeyByKey(t *testing.T) {
	low, high, user := t.TempDir(), t.TempDir(), t.TempDir()
	writeSettings(t, low, "gtk-4.0", "[Settings]\ngtk-font-name=Low Sans 9\ngtk-theme-name=Low\ngtk-cursor-theme-size=48\n")
	writeSettings(t, high, "gtk-4.0", "[Settings]\ngtk-font-name=System Sans 10\ngtk-xft-dpi=98304\n")
	writeSettings(t, user, "gtk-4.0", "[Settings]\ngtk-font-name=User Sans 13\ngtk-xft-dpi=-1\n")
	vars := map[string]string{"XDG_CONFIG_DIRS": high + ":" + low, "XDG_CONFIG_HOME": user}
	want := Settings{ThemeName: "Low", CursorThemeSize: 48, FontName: "User Sans 13", XftDPI: 98304}
	if got := load(t, vars); got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
	vars["XDG_CONFIG_HOME"] = t.TempDir()
	want.FontName = "System Sans 10"
	if got := load(t, vars); got != want {
		t.Errorf("Load without a user file = %+v, want %+v", got, want)
	}
}

func TestLoadNeverReadsGTK3Settings(t *testing.T) {
	user := t.TempDir()
	writeSettings(t, user, "gtk-3.0", "[Settings]\ngtk-icon-theme-name=Old\n")
	if got := load(t, map[string]string{"XDG_CONFIG_DIRS": t.TempDir(), "XDG_CONFIG_HOME": user}); got != (Settings{}) {
		t.Errorf("Load = %+v, want nothing from gtk-3.0", got)
	}
}

func TestLoadReportsAnUnreadableFileAndKeepsTheRest(t *testing.T) {
	system, user := t.TempDir(), t.TempDir()
	writeSettings(t, system, "gtk-4.0", "[Settings]\ngtk-theme-name=System\n")
	if err := os.MkdirAll(filepath.Join(user, "gtk-4.0", "settings.ini"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := Load(env(map[string]string{"XDG_CONFIG_DIRS": system, "XDG_CONFIG_HOME": user}))
	if err == nil || !strings.Contains(err.Error(), filepath.Join(user, "gtk-4.0", "settings.ini")) {
		t.Errorf("Load error = %v, want one naming the user's settings.ini", err)
	}
	if want := (Settings{ThemeName: "System"}); got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}
