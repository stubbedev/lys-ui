// Package gtksettings reads GTK 4's settings.ini: the [Settings] group
// GTK falls back to for every setting the desktop does not publish
// through its portal. The system files (each $XDG_CONFIG_DIRS entry,
// /etc/xdg by default) load first and the user's
// ($XDG_CONFIG_HOME/gtk-4.0/settings.ini) overrides them key by key,
// as gtksettings.c loads them. GTK 4 never reads gtk-3.0's file, so
// neither does Load.
package gtksettings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/xdg"
)

// Settings are the settings.ini keys a desktop's GTK apps take their
// look from. A zero field is one no file sets.
type Settings struct {
	// ThemeName is gtk-theme-name.
	ThemeName string
	// IconThemeName is gtk-icon-theme-name.
	IconThemeName string
	// CursorThemeName is gtk-cursor-theme-name.
	CursorThemeName string
	// CursorThemeSize is gtk-cursor-theme-size, in logical pixels.
	CursorThemeSize uint32
	// FontName is gtk-font-name, a Pango font description.
	FontName string
	// XftDPI is gtk-xft-dpi.
	XftDPI XftDPI
}

// XftDPI is gtk-xft-dpi: the text resolution in 1024ths of a dot per
// inch, 0 when unset (GTK's -1, "default", reads as unset).
type XftDPI int32

// DotsPerInch is the resolution in dots per inch, 0 when unset.
func (d XftDPI) DotsPerInch() float64 { return float64(d) / 1024 }

// Load reads every settings.ini GTK 4 would under env, later files
// overriding earlier ones key by key. A missing file sets nothing. A
// file that exists but cannot be read is skipped and reported in the
// error, which joins one such failure per file; the Settings returned
// alongside hold what the readable files set.
func Load(env xdg.Env) (Settings, error) {
	var s Settings
	var errs []error
	for _, path := range paths(env) {
		data, err := os.ReadFile(path) //nolint:gosec // the XDG settings.ini locations GTK itself reads
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("gtksettings: read %s: %w", path, err))
			continue
		}
		s = s.merge(Parse(string(data)))
	}
	return s, errors.Join(errs...)
}

// paths lists the settings.ini files in load order: the system config
// dirs from least to most important ($XDG_CONFIG_DIRS lists the most
// important first), then the user's.
func paths(env xdg.Env) []string {
	dirs := env.ConfigDirs()
	slices.Reverse(dirs)
	if home, ok := env.ConfigHome(); ok {
		dirs = append(dirs, home)
	}
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = filepath.Join(d, "gtk-4.0", "settings.ini")
	}
	return out
}

// merge overlays the keys next sets.
func (s Settings) merge(next Settings) Settings {
	overlay(&s.ThemeName, next.ThemeName)
	overlay(&s.IconThemeName, next.IconThemeName)
	overlay(&s.CursorThemeName, next.CursorThemeName)
	overlay(&s.CursorThemeSize, next.CursorThemeSize)
	overlay(&s.FontName, next.FontName)
	overlay(&s.XftDPI, next.XftDPI)
	return s
}

func overlay[T comparable](dst *T, next T) {
	var zero T
	if next != zero {
		*dst = next
	}
}

// Parse reads the [Settings] group of one settings.ini: key = value
// lines, # and ; comments, values optionally double-quoted. Keys in
// other groups, malformed lines and malformed numbers are ignored, as
// GTK's permissive load ignores what it cannot use.
func Parse(data string) Settings {
	var s Settings
	group := ""
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			group = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || group != "Settings" {
			continue
		}
		s.set(strings.TrimSpace(key), unquote(strings.TrimSpace(value)))
	}
	return s
}

func (s *Settings) set(key, value string) {
	switch key {
	case "gtk-theme-name":
		s.ThemeName = value
	case "gtk-icon-theme-name":
		s.IconThemeName = value
	case "gtk-cursor-theme-name":
		s.CursorThemeName = value
	case "gtk-cursor-theme-size":
		if n, err := strconv.ParseUint(value, 10, 32); err == nil {
			s.CursorThemeSize = uint32(n)
		}
	case "gtk-font-name":
		s.FontName = value
	case "gtk-xft-dpi":
		if n, err := strconv.ParseInt(value, 10, 32); err == nil && n > 0 {
			s.XftDPI = XftDPI(n)
		}
	}
}

func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	return value
}
