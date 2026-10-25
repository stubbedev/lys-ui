package icons

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound reports that no theme in the chain, nor any unthemed
// fallback directory, holds the requested icon.
var ErrNotFound = errors.New("icons: icon not found")

// extensions are the file suffixes tried for one icon name, in spec
// order: a directory shipping both prefers the raster PNG for that
// fixed size.
var extensions = []string{".png", ".svg", ".svgz"}

// Lookup resolves name at logical size and 120-based device scale
// frac120 and returns the matched file's path. It walks the default
// cache's theme chain; on a Cache use its Lookup method.
func Lookup(name string, size int, frac120 uint32) (string, error) {
	return Default().Lookup(name, size, frac120)
}

// Lookup resolves name like the package function, against this cache's
// theme and search paths, per the icon theme spec's lookup algorithm:
// for each theme of the inheritance chain (hicolor last), first any
// directory whose size rule accepts the request, then the nearest
// existing one, then the unthemed fallback across the base directories,
// and last the bundled set - whose matches return a "builtin:" path
// rather than a file.
func (c *Cache) Lookup(name string, size int, frac120 uint32) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path, err := c.resolveLocked(name, size, frac120)
	if err != nil {
		return "", err
	}
	return path, nil
}

// resolveLocked is Lookup without the lock; the error wraps ErrNotFound.
func (c *Cache) resolveLocked(name string, size int, frac120 uint32) (string, error) {
	paths := c.searchPathsLocked()
	scale := dirScale(frac120)
	if c.files == nil {
		c.chain = themeChain(c.theme, paths)
		c.files = dirIndex{}
	}
	for _, t := range c.chain {
		if path := lookupTheme(c.files, t, paths, name, size, scale); path != "" {
			return path, nil
		}
	}
	if path := lookupUnthemed(c.files, paths, name); path != "" {
		return path, nil
	}
	if path := lookupBuiltin(name); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("%w: %q at size %d scale %d", ErrNotFound, name, size, scale)
}

// lookupTheme runs both spec passes for one theme. A theme that has no
// directory at the requested scale falls back to its scale-1
// directories (the raster then upscales into the larger device box),
// matching how GTK serves scale-1-only themes on hidpi.
func lookupTheme(ix dirIndex, t *Theme, baseDirs []string, name string, size, scale int) string {
	if path := lookupThemeScale(ix, t, baseDirs, name, size, scale); path != "" {
		return path
	}
	if scale != 1 {
		return lookupThemeScale(ix, t, baseDirs, name, size, 1)
	}
	return ""
}

// lookupThemeScale runs the exact pass then the nearest pass at one
// integer directory scale.
func lookupThemeScale(ix dirIndex, t *Theme, baseDirs []string, name string, size, scale int) string {
	// Exact pass: the first directory whose size rule accepts the
	// request, in declaration order, wins.
	for _, base := range baseDirs {
		for _, d := range t.Dirs {
			if !d.matches(size, scale) {
				continue
			}
			if path := findIconFile(ix, base, t.DirName, d.Path, name); path != "" {
				return path
			}
		}
	}
	// Nearest pass: the smallest size distance among the directories
	// that actually hold the name. Ties keep the earlier directory.
	best := ""
	bestDist := 0
	for _, base := range baseDirs {
		for _, d := range t.Dirs {
			if d.Scale != scale {
				continue
			}
			dist := d.distance(size)
			if best != "" && dist >= bestDist {
				continue
			}
			if path := findIconFile(ix, base, t.DirName, d.Path, name); path != "" {
				best, bestDist = path, dist
			}
		}
	}
	return best
}

// lookupUnthemed is the spec's fallback: base directories are searched
// for the bare name, no theme or size rule involved.
func lookupUnthemed(ix dirIndex, baseDirs []string, name string) string {
	for _, base := range baseDirs {
		if path := findIconFile(ix, base, "", "", name); path != "" {
			return path
		}
	}
	return ""
}

// findIconFile returns the first existing file for the icon name under
// base/theme/dir with a known extension. A "-symbolic" name also tries
// its unsuffixed base: themes often ship only the plain glyph.
func findIconFile(ix dirIndex, base, theme, dir, name string) string {
	d := filepath.Join(base, theme, dir)
	for _, nm := range nameCandidates(name) {
		for _, ext := range extensions {
			if ix.isFile(d, nm+ext) {
				return filepath.Join(d, nm+ext)
			}
		}
	}
	return ""
}

// nameCandidates lists the file stems tried for an icon name.
func nameCandidates(name string) []string {
	if base, ok := strings.CutSuffix(name, symbolicSuffix); ok {
		return []string{name, base}
	}
	return []string{name}
}

// fileKind is what a directory entry is, as far as a lookup needs.
type fileKind uint8

const (
	// fileLink is a symlink not yet followed: themes link their sizes
	// and aliases, and a lookup stats one only when it asks for it.
	fileLink fileKind = iota + 1
	fileRegular
	fileOther
)

// dirIndex holds the entries of the theme directories lookups have
// read, each read once: a lookup answers from memory instead of
// statting every candidate path, as GTK's icon-theme.cache does. A
// cache reset drops it, so icons installed since are found.
type dirIndex map[string]map[string]fileKind

// isFile reports whether dir/name exists and is (or links to) a
// regular file.
func (ix dirIndex) isFile(dir, name string) bool {
	entries, ok := ix[dir]
	if !ok {
		entries = readEntries(dir)
		ix[dir] = entries
	}
	switch entries[name] {
	case fileRegular:
		return true
	case fileLink:
		kind := fileOther
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			kind = fileRegular
		}
		entries[name] = kind
		return kind == fileRegular
	}
	return false
}

// readEntries lists dir's files and links; a missing or unreadable
// directory holds none.
func readEntries(dir string) map[string]fileKind {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	entries := make(map[string]fileKind, len(des))
	for _, de := range des {
		switch t := de.Type(); {
		case t.IsRegular():
			entries[de.Name()] = fileRegular
		case t&os.ModeSymlink != 0:
			entries[de.Name()] = fileLink
		}
	}
	return entries
}

// symbolicSuffix marks a symbolic icon name.
const symbolicSuffix = "-symbolic"

// FallbackName is the first of name and its dash-shortened forms that
// resolves, GThemedIcon's default fallbacks: a missing
// preferences-system-network tries preferences-system, then
// preferences. A -symbolic suffix stays on every form. It returns name
// itself when none resolves.
func (c *Cache) FallbackName(name string) string {
	base, suffix := strings.CutSuffix(name, symbolicSuffix)
	tail := ""
	if suffix {
		tail = symbolicSuffix
	}
	for n := base; n != ""; {
		if _, err := c.Lookup(n+tail, 16, 120); err == nil {
			return n + tail
		}
		i := strings.LastIndexByte(n, '-')
		if i < 0 {
			break
		}
		n = n[:i]
	}
	return name
}
