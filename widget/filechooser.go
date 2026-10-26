package widget

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// FileChooser is the pure-Go file picker behind app.OpenFileDialog and
// app.SaveFileDialog (#82): a places row, the current directory's
// listing, filters, a name row in save mode, and Apply validation. It
// is the whole picker lys-ui needs - List, Entry, Dropdown, and Button
// are the parts - so it works on any compositor with no portal in
// sight; the directory listing comes from an injected source, the seam
// tests and any future portal backend plug into.
//
// Navigation: Up climbs one directory, the place buttons jump home, to
// the filesystem root, or into the recents view, and activating a row
// enters a directory or applies a file. Apply validates: an empty
// selection, an empty name, or a name carrying a path separator sets
// the status line and applies nothing.
type FileChooser struct {
	composite
	face   render.Font
	sizePx float64

	mode    FileMode
	filters []FileFilter
	filter  int
	source  DirSource
	recents func() []string

	dir       string
	listing   []FileEntry
	paths     []string
	inRecents bool

	column     *Box
	places     *Box
	path       *Label
	list       *List
	model      *chooserModel
	name       *Entry
	filterDrop *Dropdown
	status     *Label

	// OnApply, when set, receives every validated apply: the chosen
	// absolute paths (one in save and single-open mode, the selection
	// in multiple mode, a directory in folder mode).
	OnApply func(paths []string)
}

// FileMode is what a FileChooser is picking.
type FileMode uint8

const (
	// FileModeOpen picks one existing file.
	FileModeOpen FileMode = iota
	// FileModeOpenMultiple picks any set of existing files.
	FileModeOpenMultiple
	// FileModeOpenFolder picks one directory (the current one when no
	// row is selected).
	FileModeOpenFolder
	// FileModeSave names a file in the current directory; overwrite
	// confirmation is the caller's policy on the absolute path Apply
	// returned.
	FileModeSave
)

// FileFilter names one set of acceptable files: Patterns are shell
// globs matched against the base name, case-insensitively ("*.png",
// "Makefile"). Directories always pass; an empty pattern list accepts
// every file.
type FileFilter struct {
	Name     string
	Patterns []string
}

// FileEntry is one row of a listing: its name and whether it is a
// directory.
type FileEntry struct {
	Name string
	Dir  bool
}

// DirSource lists one directory; the default walks the filesystem,
// hiding dot files. Tests and portal backends substitute their own.
type DirSource func(dir string) ([]FileEntry, error)

// osDirSource is the default listing: os.ReadDir with dot files
// hidden, directories first, names compared case-insensitively.
func osDirSource(dir string) ([]FileEntry, error) {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]FileEntry, 0, len(dirents))
	for _, d := range dirents {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		entries = append(entries, FileEntry{Name: d.Name(), Dir: d.IsDir()})
	}
	slices.SortStableFunc(entries, func(a, b FileEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return entries, nil
}

// NewFileChooser returns a chooser in mode starting at startDir (the
// home directory when empty), filtered by filters (a nil or empty list
// accepts everything; more than one shows a dropdown).
func NewFileChooser(face render.Font, sizePx float64, mode FileMode, startDir string, filters []FileFilter) *FileChooser {
	face = requireFace("widget.NewFileChooser", face)
	if startDir == "" {
		startDir, _ = os.UserHomeDir()
	}
	if len(filters) == 0 {
		filters = []FileFilter{{Name: Tr("All files")}}
	}
	c := &FileChooser{
		face: face, sizePx: sizePx, mode: mode,
		filters: filters, source: osDirSource,
		dir: startDir,
	}
	th := Current()
	c.model = &chooserModel{face: face, sizePx: sizePx}
	c.list = NewList(c.model, int(sizePx)+14)
	switch mode {
	case FileModeOpenMultiple:
		c.list.SetSelectionMode(SelectionMultiple)
	case FileModeOpenFolder:
		c.list.SetSelectionMode(SelectionBrowse)
	}
	c.list.OnSelect = func(int) { c.syncFromSelection() }
	c.list.OnActivate = func(i int) { c.activate(i) }
	c.path = NewLabel(face, sizePx, "", 0)
	c.path.AddClass(css.DimLabel)
	c.status = NewLabel(face, sizePx-2, "", 0)
	c.status.AddClass(css.Accent)
	c.buildPlaces()
	c.column = NewBox(Column, 10, 0)
	c.column.Append(c.places, false)
	c.column.Append(c.path, false)
	c.column.Append(c.list, true)
	if mode == FileModeSave {
		c.name = NewEntry(face, sizePx, th.Text)
		nameRow := NewBox(Row, 8, 0)
		nameRow.Append(NewLabel(face, sizePx, Tr("Name:"), th.Text), false)
		nameRow.Append(c.name, true)
		c.column.Append(nameRow, true)
	}
	if len(filters) > 1 {
		names := make([]string, len(filters))
		for i, f := range filters {
			names[i] = f.Name
		}
		c.filterDrop = NewDropdown(face, sizePx, names, 0)
		c.filterDrop.OnSelect = func(i int) { c.setFilter(i) }
		filterRow := NewBox(Row, 8, 0)
		filterRow.Append(NewLabel(face, sizePx, Tr("Filter:"), th.Text), false)
		filterRow.Append(c.filterDrop, false)
		c.column.Append(filterRow, false)
	}
	c.column.Append(c.status, false)
	c.initComposite(c, c.column)
	c.navigate(startDir)
	return c
}

// SetSaveName pre-fills the save name row (save mode only).
func (c *FileChooser) SetSaveName(name string) {
	if c.name != nil {
		c.name.SetText(name)
	}
}

// SetSource replaces the directory listing source (tests, portals).
func (c *FileChooser) SetSource(src DirSource) {
	if src == nil {
		src = osDirSource
	}
	c.source = src
	c.refresh()
}

// SetRecentsSource wires the recents view: paths lists the recent
// files (most recent first) and the Recent place appears; without it
// the picker is a plain browser.
func (c *FileChooser) SetRecentsSource(paths func() []string) {
	c.recents = paths
	c.buildPlaces()
	c.InvalidateLayout()
}

// Dir reports the directory being browsed ("" in the recents view).
func (c *FileChooser) Dir() string { return c.dir }

// Apply validates and applies the current selection: in open modes the
// selected files, in folder mode the selected or current directory, in
// save mode the name row joined onto the directory. It returns the
// absolute paths and reports whether the apply was valid; an invalid
// apply sets the status line instead.
func (c *FileChooser) Apply() ([]string, bool) {
	switch c.mode {
	case FileModeOpen:
		if i := c.list.Selected(); i >= 0 && !c.listing[i].Dir {
			if paths, ok := c.applyOne(i); ok {
				return paths, true
			}
			return nil, false
		}
		c.setStatus(Tr("choose a file"))
	case FileModeOpenMultiple:
		picked := c.pickedFiles()
		if len(picked) > 0 {
			return picked, true
		}
		c.setStatus(Tr("choose at least one file"))
	case FileModeOpenFolder:
		if i := c.list.Selected(); i >= 0 && c.listing[i].Dir {
			return []string{c.paths[i]}, true
		}
		if c.dir != "" {
			return []string{c.dir}, true
		}
		c.setStatus(Tr("choose a folder"))
	case FileModeSave:
		if c.dir == "" {
			c.setStatus(Tr("save needs a directory"))
			break
		}
		name := strings.TrimSpace(c.name.Text())
		if name == "" {
			c.setStatus(Tr("enter a file name"))
			break
		}
		if strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) {
			c.setStatus(Tr("the name cannot contain a path separator"))
			break
		}
		return []string{filepath.Join(c.dir, name)}, true
	}
	return nil, false
}

// pickedFiles collects the selected file rows of a multiple chooser.
func (c *FileChooser) pickedFiles() []string {
	var picked []string
	for _, i := range c.list.Selection() {
		if i >= 0 && i < len(c.listing) && !c.listing[i].Dir {
			picked = append(picked, c.paths[i])
		}
	}
	return picked
}

// climb moves one directory up, staying put at the filesystem root.
func (c *FileChooser) climb() {
	if c.dir == "" {
		return
	}
	c.navigate(filepath.Dir(c.dir))
}

// activate enters a directory row or applies a file row.
func (c *FileChooser) activate(i int) {
	if i < 0 || i >= len(c.listing) {
		return
	}
	if c.listing[i].Dir {
		if !c.inRecents {
			c.navigate(c.paths[i])
		}
		return
	}
	if c.mode == FileModeSave {
		c.name.SetText(c.listing[i].Name)
		c.list.Select(i)
		return
	}
	if c.mode == FileModeOpenFolder {
		return
	}
	if paths, ok := c.applyOne(i); ok {
		c.emitApply(paths)
	}
}

// applyOne validates a single file row against the active filter.
func (c *FileChooser) applyOne(i int) ([]string, bool) {
	if !c.passes(c.listing[i].Name) {
		c.setStatus("\"" + c.listing[i].Name + "\" " + Tr("does not match") + " " + c.filters[c.filter].Name)
		return nil, false
	}
	return []string{c.paths[i]}, true
}

// emitApply hands a valid result to the OnApply hook.
func (c *FileChooser) emitApply(paths []string) {
	c.setStatus("")
	if c.OnApply != nil {
		c.OnApply(paths)
	}
}

// syncFromSelection mirrors a single-selection file pick into the save
// name row.
func (c *FileChooser) syncFromSelection() {
	if c.mode != FileModeSave {
		return
	}
	if i := c.list.Selected(); i >= 0 && i < len(c.listing) && !c.listing[i].Dir {
		c.name.SetText(c.listing[i].Name)
	}
}

// setFilter switches the active filter and re-lists the directory.
func (c *FileChooser) setFilter(i int) {
	if i < 0 || i >= len(c.filters) {
		return
	}
	c.filter = i
	c.refresh()
}

// passes reports whether a file name matches the active filter.
func (c *FileChooser) passes(name string) bool {
	patterns := c.filters[c.filter].Patterns
	if len(patterns) == 0 {
		return true
	}
	folded := strings.ToLower(name)
	for _, p := range patterns {
		if ok, _ := filepath.Match(strings.ToLower(p), folded); ok {
			return true
		}
	}
	return false
}

// navigate browses dir ("" jumps to the recents view when a source is
// wired).
func (c *FileChooser) navigate(dir string) {
	c.dir = dir
	c.refresh()
}

// refresh re-lists the current view into the model.
func (c *FileChooser) refresh() {
	c.inRecents = c.dir == ""
	c.listing = c.listing[:0]
	c.paths = c.paths[:0]
	c.path.SetText(c.dirLabel())
	if c.inRecents {
		if c.recents == nil {
			c.setStatus(Tr("no recent files"))
		} else {
			for _, p := range c.recents() {
				c.listing = append(c.listing, FileEntry{Name: filepath.Base(p)})
				c.paths = append(c.paths, p)
			}
		}
	} else {
		entries, err := c.source(c.dir)
		if err != nil {
			c.setStatus(err.Error())
		} else {
			for _, e := range entries {
				if e.Dir || c.passes(e.Name) {
					c.listing = append(c.listing, e)
					c.paths = append(c.paths, filepath.Join(c.dir, e.Name))
				}
			}
		}
	}
	c.model.set(c.listing)
	c.list.Reset()
	c.setStatus("")
	c.InvalidateLayout()
}

// dirLabel titles the current view.
func (c *FileChooser) dirLabel() string {
	if c.inRecents {
		return Tr("Recent files")
	}
	return c.dir
}

// setStatus shows or clears the status line.
func (c *FileChooser) setStatus(text string) {
	c.status.SetText(text)
}

// buildPlaces rebuilds the place buttons for the wired sources into
// the places row, keeping the row's slot in the layout.
func (c *FileChooser) buildPlaces() {
	th := Current()
	if c.places == nil {
		c.places = NewBox(Row, 6, 0)
	} else {
		c.places.Clear()
	}
	climb := NewButton(NewLabel(c.face, c.sizePx, Tr("Up"), th.OnAccent), 8, 4)
	climb.OnClick = c.climb
	home := NewButton(NewLabel(c.face, c.sizePx, Tr("Home"), th.OnAccent), 8, 4)
	home.OnClick = func() {
		dir, _ := os.UserHomeDir()
		c.navigate(dir)
	}
	root := NewButton(NewLabel(c.face, c.sizePx, Tr("Filesystem"), th.OnAccent), 8, 4)
	root.OnClick = func() { c.navigate("/") }
	c.places.Append(climb, false)
	c.places.Append(home, false)
	c.places.Append(root, false)
	if c.recents != nil {
		recent := NewButton(NewLabel(c.face, c.sizePx, Tr("Recent"), th.OnAccent), 8, 4)
		recent.OnClick = func() { c.navigate("") }
		c.places.Append(recent, false)
	}
}

// chooserModel is the List model over the current listing; rows are
// labels built once per listing, directories marked with a trailing
// separator and dimmed.
type chooserModel struct {
	face    render.Font
	sizePx  float64
	entries []FileEntry
	rows    []Widget
}

func (m *chooserModel) set(entries []FileEntry) {
	m.entries = entries
	m.rows = m.rows[:0]
}

// Len implements ListModel.
func (m *chooserModel) Len() int { return len(m.entries) }

// Row implements ListModel.
func (m *chooserModel) Row(i int) Widget {
	if i < 0 || i >= len(m.entries) {
		return nil
	}
	for len(m.rows) < i+1 {
		m.rows = append(m.rows, nil)
	}
	if m.rows[i] == nil {
		th := Current()
		name := m.entries[i].Name
		color := th.Text
		if m.entries[i].Dir {
			name += string(filepath.Separator)
			color = th.TextMuted
		}
		m.rows[i] = NewLabel(m.face, m.sizePx, name, color)
	}
	return m.rows[i]
}
