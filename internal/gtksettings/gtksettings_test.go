package gtksettings

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseReadsTheSettingsGroup(t *testing.T) {
	got := Parse(`# a comment
[Settings]
gtk-application-prefer-dark-theme=1
gtk-font-name = "Adwaita Sans 11"
; another comment
gtk-xft-dpi=122880
gtk-icon-theme-name=Papirus
[Other]
gtk-font-name=Ignored 9
`)
	if want := (Settings{FontName: "Adwaita Sans 11", DPI: 120, IconThemeName: "Papirus"}); got != want {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func TestParseIgnoresWhatGTKCannotUse(t *testing.T) {
	for name, data := range map[string]string{
		"outside [Settings]": "gtk-font-name=Sans 12\n",
		"GTK's default DPI":  "[Settings]\ngtk-xft-dpi=-1\n",
		"a non-numeric DPI":  "[Settings]\ngtk-xft-dpi=lots\n",
		"no value":           "[Settings]\ngtk-font-name\n",
	} {
		if got := Parse(data); got != (Settings{}) {
			t.Errorf("%s: Parse = %+v, want nothing", name, got)
		}
	}
}

func TestPathsLoadSystemDirsLeastImportantFirstThenTheUser(t *testing.T) {
	got := paths("/a:/b", "/home/u/.config", "/home/u")
	want := []string{"/b/gtk-4.0/settings.ini", "/a/gtk-4.0/settings.ini", "/home/u/.config/gtk-4.0/settings.ini"}
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
	got = paths("", "", "/home/u")
	want = []string{"/etc/xdg/gtk-4.0/settings.ini", "/home/u/.config/gtk-4.0/settings.ini"}
	if !slices.Equal(got, want) {
		t.Errorf("default paths = %q, want %q", got, want)
	}
}

func TestLoadLetsTheUserFileOverrideKeyByKey(t *testing.T) {
	system, user := t.TempDir(), t.TempDir()
	write := func(dir, data string) {
		t.Helper()
		path := filepath.Join(dir, "gtk-4.0", "settings.ini")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(system, "[Settings]\ngtk-font-name=System Sans 10\ngtk-xft-dpi=98304\n")
	write(user, "[Settings]\ngtk-font-name=User Sans 13\n")
	t.Setenv("XDG_CONFIG_DIRS", system)
	t.Setenv("XDG_CONFIG_HOME", user)
	if got, want := Load(), (Settings{FontName: "User Sans 13", DPI: 96}); got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got, want := Load(), (Settings{FontName: "System Sans 10", DPI: 96}); got != want {
		t.Errorf("Load without a user file = %+v, want %+v", got, want)
	}
}
