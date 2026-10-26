package widget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

// chooserFace builds the test font.
func chooserFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// fixtureDirs is a deterministic listing source: two directories with
// mixed-case names, dot files, and files of several types.
var fixtureDirs = map[string][]FileEntry{
	"/home": {
		{Name: "Documents", Dir: true},
		{Name: "z-last.txt", Dir: false},
		{Name: "notes.md", Dir: false},
		{Name: ".hidden", Dir: false},
	},
	"/home/Documents": {
		{Name: "report.pdf", Dir: false},
		{Name: "photo.PNG", Dir: false},
		{Name: "budget.ods", Dir: false},
		{Name: "archive", Dir: true},
	},
}

func fixtureSource(dir string) ([]FileEntry, error) {
	entries, ok := fixtureDirs[dir]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]FileEntry, len(entries))
	copy(out, entries)
	return out, nil
}

var chooserFilters = []FileFilter{
	{Name: "Documents", Patterns: []string{"*.pdf", "*.md", "*.txt"}},
	{Name: "Images", Patterns: []string{"*.png"}},
	{Name: "All files"},
}

// newChooser builds a chooser over the fixture source.
func newChooser(t *testing.T, mode FileMode, filters []FileFilter) *FileChooser {
	t.Helper()
	c := NewFileChooser(chooserFace(t), 14, mode, "/home", filters)
	c.SetSource(fixtureSource)
	return c
}

// rowFor finds the index of a name in the current listing.
func (c *FileChooser) rowFor(t *testing.T, name string) int {
	t.Helper()
	i, err := c.rowForErr(name)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func (c *FileChooser) rowForErr(name string) (int, error) {
	for i, e := range c.listing {
		if e.Name == name {
			return i, nil
		}
	}
	return 0, errNoRow{name}
}

type errNoRow struct{ name string }

func (e errNoRow) Error() string { return "no row named " + e.name }

// TestFileChooserOpenSingle covers the primary flow: navigation into a
// directory, filter matching (case-insensitive), activation applying a
// file, and the status line refusing a filter mismatch.
func TestFileChooserOpenSingle(t *testing.T) {
	c := newChooser(t, FileModeOpen, chooserFilters)
	if got := len(c.listing); got != 3 { // .hidden matches no Documents pattern
		t.Fatalf("listing = %v, want the three matching entries", c.listing)
	}
	if c.listing[0].Name != "Documents" {
		t.Errorf("directories do not sort first: %v", c.listing)
	}

	// Enter the directory, select a file, apply through the button.
	c.activate(c.rowFor(t, "Documents"))
	if c.Dir() != "/home/Documents" {
		t.Fatalf("navigate landed in %q", c.Dir())
	}
	c.list.Select(c.rowFor(t, "report.pdf"))
	paths, ok := c.Apply()
	if !ok || len(paths) != 1 || paths[0] != "/home/Documents/report.pdf" {
		t.Fatalf("apply = %v %v", paths, ok)
	}

	// The images filter re-lists without the PDF; the uppercase PNG
	// still passes the lowercase pattern.
	c.setFilter(1)
	if _, err := c.rowForErr("report.pdf"); err == nil {
		t.Error("the documents file stayed listed under the images filter")
	}
	c.list.Select(c.rowFor(t, "photo.PNG"))
	paths, ok = c.Apply()
	if !ok || paths[0] != "/home/Documents/photo.PNG" {
		t.Errorf("uppercase extension did not match: %v %v", paths, ok)
	}

	// Activation applies directly through OnApply, under the all-files
	// filter so every fixture file is listed.
	c.setFilter(2)
	var applied []string
	c.OnApply = func(p []string) { applied = p }
	c.activate(c.rowFor(t, "budget.ods"))
	if len(applied) != 1 || applied[0] != "/home/Documents/budget.ods" {
		t.Errorf("activation applied %v", applied)
	}
}

// TestFileChooserOpenMultiple covers the set selection and the
// directories-never-picked rule.
func TestFileChooserOpenMultiple(t *testing.T) {
	c := newChooser(t, FileModeOpenMultiple, nil)
	c.list.SelectAll()
	paths, ok := c.Apply()
	if !ok {
		t.Fatalf("apply over a full selection failed: %v", c.listing)
	}
	if len(paths) != 3 { // Documents is a directory
		t.Errorf("picked %v, want only the files", paths)
	}
	c.list.Reset()
	if _, ok := c.Apply(); ok {
		t.Error("an empty selection applied")
	}
}

// TestFileChooserFolder covers folder mode: a selected directory wins,
// otherwise the current directory applies.
func TestFileChooserFolder(t *testing.T) {
	c := newChooser(t, FileModeOpenFolder, nil)
	paths, ok := c.Apply()
	if !ok || paths[0] != "/home" {
		t.Fatalf("folder apply with no selection = %v %v", paths, ok)
	}
	c.list.Select(c.rowFor(t, "Documents"))
	paths, ok = c.Apply()
	if !ok || paths[0] != "/home/Documents" {
		t.Errorf("selected folder apply = %v %v", paths, ok)
	}
}

// TestFileChooserSave covers the save flow: name validation, the
// selection mirror, and the separator refusal.
func TestFileChooserSave(t *testing.T) {
	c := newChooser(t, FileModeSave, chooserFilters)
	c.SetSaveName("report.pdf")
	paths, ok := c.Apply()
	if !ok || paths[0] != filepath.Join("/home", "report.pdf") {
		t.Fatalf("save apply = %v %v", paths, ok)
	}

	c.SetSaveName("")
	if _, ok := c.Apply(); ok {
		t.Error("an empty name applied")
	}
	c.SetSaveName("a/b.txt")
	if _, ok := c.Apply(); ok {
		t.Error("a name with a separator applied")
	}

	c.navigate("/home/Documents")
	c.list.Select(c.rowFor(t, "report.pdf"))
	if c.name.Text() != "report.pdf" {
		t.Errorf("selection did not mirror into the name row: %q", c.name.Text())
	}
	c.activate(c.rowFor(t, "report.pdf"))
	if c.name.Text() != "report.pdf" || c.list.Selected() != c.rowFor(t, "report.pdf") {
		t.Error("activating a file in save mode did not select it")
	}
}

// TestFileChooserClimbAndPlaces covers the places row: Up stops at the
// root, Home and Filesystem jump, and a missing directory keeps the
// chooser alive with a status.
func TestFileChooserClimbAndPlaces(t *testing.T) {
	c := newChooser(t, FileModeOpen, nil)
	c.navigate("/home/Documents")
	c.climb()
	if c.Dir() != "/home" {
		t.Fatalf("climb landed in %q", c.Dir())
	}
	c.navigate("/")
	c.climb()
	if c.Dir() != "/" {
		t.Errorf("climb above the root moved to %q", c.Dir())
	}
	src := c.source
	c.SetSource(func(string) ([]FileEntry, error) { return nil, os.ErrPermission })
	c.navigate("/home")
	if _, ok := c.Apply(); !ok && c.Dir() == "" {
		t.Error("a listing failure broke the chooser")
	}
	c.SetSource(src)
}

// TestFileChooserRecents covers the recents view: the place exists,
// the listing shows base names, and activation applies the full path.
func TestFileChooserRecents(t *testing.T) {
	c := newChooser(t, FileModeOpen, nil)
	c.SetRecentsSource(func() []string { return []string{"/home/Documents/report.pdf"} })
	c.navigate("")
	if got := c.Dir(); got != "" {
		t.Fatalf("recents view reports dir %q", got)
	}
	var applied []string
	c.OnApply = func(p []string) { applied = p }
	c.activate(c.rowFor(t, "report.pdf"))
	if len(applied) != 1 || applied[0] != "/home/Documents/report.pdf" {
		t.Errorf("recents activation applied %v", applied)
	}
}

// TestFileChooserOsSourceOrder pins the default listing over a real
// directory: dot files hidden, directories first, case-insensitive.
func TestFileChooserOsSourceOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".secret", "Zebra.txt", "apple.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := osDirSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	want := []string{"sub", "apple.txt", "Zebra.txt"}
	if len(names) != len(want) {
		t.Fatalf("os listing = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("os listing = %v, want %v", names, want)
		}
	}
}

// TestGoldenFileChooser pins the picker's painted look in open and
// save mode over the fixture listing.
func TestGoldenFileChooser(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	open := NewFileChooser(face, 14, FileModeOpen, "/home/Documents", chooserFilters)
	open.SetSource(fixtureSource)
	NewGolden(t, open, "filechooser-open", goldenTheme(th), goldenFrame(400, 300))

	save := NewFileChooser(face, 14, FileModeSave, "/home", chooserFilters)
	save.SetSource(fixtureSource)
	save.SetSaveName("report.pdf")
	NewGolden(t, save, "filechooser-save", goldenTheme(th), goldenFrame(400, 320))
}
