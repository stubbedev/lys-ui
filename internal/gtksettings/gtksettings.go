// Package gtksettings reads GTK 4's settings.ini: the [Settings] group
// GTK falls back to for every setting the desktop does not publish
// through its portal. The system files (each $XDG_CONFIG_DIRS entry,
// /etc/xdg by default) load first and the user's
// ($XDG_CONFIG_HOME/gtk-4.0/settings.ini) overrides them, as
// gtksettings.c loads them.
package gtksettings

import (
	"bufio"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

// Settings are the settings.ini keys lys-ui honors.
type Settings struct {
	// FontName is gtk-font-name, "" when no file sets it.
	FontName string
	// IconThemeName is gtk-icon-theme-name, "" when no file sets it.
	IconThemeName string
	// DPI is gtk-xft-dpi in dots per inch, 0 when no file sets it (or
	// sets -1, GTK's "default").
	DPI float64
}

// Load reads every settings.ini GTK would, later files overriding
// earlier ones key by key. A missing file is skipped; an unreadable
// one is reported at Debug and skipped.
func Load() Settings {
	var s Settings
	for _, path := range paths(os.Getenv("XDG_CONFIG_DIRS"), os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME")) {
		data, err := os.ReadFile(path) //nolint:gosec // the XDG settings.ini locations GTK itself reads
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				logutil.L().Debug("gtksettings: cannot read settings.ini", slog.String("path", path), slog.Any("err", err))
			}
			continue
		}
		s = s.merge(Parse(string(data)))
	}
	return s
}

// paths lists the settings.ini files in load order: the system config
// dirs from least to most important ($XDG_CONFIG_DIRS lists the most
// important first), then the user's.
func paths(configDirs, configHome, home string) []string {
	dirs := filepath.SplitList(configDirs)
	if configDirs == "" {
		dirs = []string{"/etc/xdg"}
	}
	slices.Reverse(dirs)
	if configHome == "" && home != "" {
		configHome = filepath.Join(home, ".config")
	}
	if configHome != "" {
		dirs = append(dirs, configHome)
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d != "" {
			out = append(out, filepath.Join(d, "gtk-4.0", "settings.ini"))
		}
	}
	return out
}

// merge overlays the keys next sets.
func (s Settings) merge(next Settings) Settings {
	if next.FontName != "" {
		s.FontName = next.FontName
	}
	if next.DPI != 0 {
		s.DPI = next.DPI
	}
	if next.IconThemeName != "" {
		s.IconThemeName = next.IconThemeName
	}
	return s
}

// Parse reads the [Settings] group of one settings.ini: key = value
// lines, # and ; comments, values optionally double-quoted. Keys in
// other groups and malformed lines are ignored, as GKeyFile's
// permissive load in GTK ignores what it cannot use.
func Parse(data string) Settings {
	var s Settings
	group := ""
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
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
		value = strings.TrimSpace(value)
		if unq, err := strconv.Unquote(value); err == nil {
			value = unq
		}
		switch strings.TrimSpace(key) {
		case "gtk-font-name":
			s.FontName = value
		case "gtk-icon-theme-name":
			s.IconThemeName = value
		case "gtk-xft-dpi":
			if dpi, err := strconv.Atoi(value); err == nil && dpi > 0 {
				s.DPI = float64(dpi) / 1024
			}
		}
	}
	return s
}
