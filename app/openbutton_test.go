package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/xdg"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/recentfiles"
	"github.com/stubbedev/lys-ui/widget"
)

func openButtonApp(t *testing.T) *Application {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a := &Application{}
	a.SetTooltipFace(face)
	t.Cleanup(func() {
		a.watchers.shutdown()
		if a.recentFiles != nil && !a.recentFiles.Stop(5*time.Second) {
			t.Error("the recents worker outlived the test")
		}
	})
	return a
}

func sharedRecents(t *testing.T) *recentfiles.Manager {
	t.Helper()
	path, err := recentfiles.DefaultPath(xdg.OS)
	if err != nil {
		t.Fatal(err)
	}
	return recentfiles.New(path)
}

func openButtonFiles(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

// TestOpenButtonChoose covers the choice path end to end: the output
// fires once with the path, the shared recents record it, and a fresh
// manager over the same store reads it back.
func TestOpenButtonChoose(t *testing.T) {
	a := openButtonApp(t)
	path := openButtonFiles(t, "one.txt")[0]
	var got []string
	ob, err := a.NewOpenButton(nil, OpenButtonConfig{OnOpen: func(p string) { got = append(got, p) }})
	if err != nil {
		t.Fatal(err)
	}
	ob.choose(path)
	if !slices.Equal(got, []string{path}) {
		t.Fatalf("output = %v, want [%s]", got, path)
	}
	pumpUntil(t, a, func() bool { return len(ob.Recents()) == 1 })
	if rec := ob.Recents(); rec[0] != path {
		t.Fatalf("recents = %v, want [%s]", rec, path)
	}
	m := sharedRecents(t)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	entries := m.List()
	if len(entries) != 1 || entries[0].Path != path || entries[0].App != "lys-ui" {
		t.Fatalf("round trip = %+v, want the path under lys-ui", entries)
	}
}

// TestOpenButtonRecentsDropdown pins the dropdown itself: base-name
// rows, most recent first, bounded to the configured size (and the
// default when unconfigured), missing files dropped.
func TestOpenButtonRecentsDropdown(t *testing.T) {
	a := openButtonApp(t)
	paths := openButtonFiles(t, "a.txt", "b.txt", "c.txt")
	m := sharedRecents(t)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if err := m.Add("lys-ui", p); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(t.TempDir(), "gone.txt")
	if err := m.Add("lys-ui", missing); err != nil {
		t.Fatal(err)
	}
	a.recentFiles = m

	ob, err := a.NewOpenButton(nil, OpenButtonConfig{Recents: 2})
	if err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, a, func() bool { return len(ob.Recents()) > 0 })
	want := []string{filepath.Base(paths[2]), filepath.Base(paths[1])}
	if items := ob.recents.Items(); !slices.Equal(items, want) {
		t.Fatalf("dropdown = %v, want %v", items, want)
	}
	if rec := ob.Recents(); !slices.Equal(rec, []string{paths[2], paths[1]}) {
		t.Fatalf("recents = %v, want %v", rec, paths[2:])
	}
	if slices.Contains(ob.recents.Items(), filepath.Base(missing)) {
		t.Error("a missing file stayed listed")
	}

	unbounded, err := a.NewOpenButton(nil, OpenButtonConfig{})
	if err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, a, func() bool { return len(unbounded.Recents()) > 0 })
	if got := len(unbounded.recents.Items()); got > openButtonDefaultRecents {
		t.Fatalf("default size listed %d, want at most %d", got, openButtonDefaultRecents)
	}
}

// TestOpenButtonDropdownPick covers choosing from the dropdown: the
// row's path is delivered and moves to the front.
func TestOpenButtonDropdownPick(t *testing.T) {
	a := openButtonApp(t)
	paths := openButtonFiles(t, "a.txt", "b.txt", "c.txt")
	m := sharedRecents(t)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if err := m.Add("lys-ui", p); err != nil {
			t.Fatal(err)
		}
	}
	a.recentFiles = m
	var got []string
	ob, err := a.NewOpenButton(nil, OpenButtonConfig{OnOpen: func(p string) { got = append(got, p) }})
	if err != nil {
		t.Fatal(err)
	}
	pumpUntil(t, a, func() bool { return len(ob.Recents()) > 0 })
	ob.recents.SetSelected(0)
	if !slices.Equal(got, []string{paths[2]}) {
		t.Fatalf("output = %v, want the newest [%s]", got, paths[2])
	}
	if rec := ob.Recents(); rec[0] != paths[2] {
		t.Fatalf("pick did not move to front: %v", rec)
	}
}

// TestOpenButtonDialogRouting pins the dialog hand-off: folder mode
// routes to the folder picker, filters and start dir pass through,
// and a dialog choice delivers through the same choose path.
func TestOpenButtonDialogRouting(t *testing.T) {
	a := openButtonApp(t)
	paths := openButtonFiles(t, "one.txt")
	filters := []widget.FileFilter{{Name: "Text", Patterns: []string{"*.txt"}}}
	var mode widget.FileMode
	var cfg FileDialogConfig
	ob, err := a.NewOpenButton(nil, OpenButtonConfig{
		StartDir: "/tmp", Filters: filters, Folder: true,
		OnOpen: func(string) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	ob.open = func(_ *Window, m widget.FileMode, c FileDialogConfig) error {
		mode, cfg = m, c
		return nil
	}
	ob.openDialog()
	if mode != widget.FileModeOpenFolder {
		t.Fatalf("mode = %v, want the folder mode", mode)
	}
	if cfg.StartDir != "/tmp" || cfg.OnOpen == nil || len(cfg.Filters) != 1 ||
		cfg.Filters[0].Name != filters[0].Name || !slices.Equal(cfg.Filters[0].Patterns, filters[0].Patterns) {
		t.Fatalf("dialog config = %+v, want the start dir, filters and output wired", cfg)
	}
	var got []string
	ob.cfg.OnOpen = func(p string) { got = append(got, p) }
	cfg.OnOpen([]string{paths[0]})
	if !slices.Equal(got, []string{paths[0]}) {
		t.Fatalf("output = %v, want [%s]", got, paths[0])
	}
	pumpUntil(t, a, func() bool { return len(ob.Recents()) == 1 })
	if rec := ob.Recents(); rec[0] != paths[0] {
		t.Fatalf("recents = %v, want the choice recorded", rec)
	}
	ob.cfg.Folder = false
	ob.openDialog()
	if mode != widget.FileModeOpen {
		t.Fatalf("mode = %v, want the file mode", mode)
	}
}

func TestOpenButtonWaitsOnNoDisk(t *testing.T) {
	a := openButtonApp(t)
	path := openButtonFiles(t, "one.txt")[0]
	m, ok := a.recentsManager()
	if !ok {
		t.Fatal("no recents manager with a data home set")
	}
	hold := make(chan struct{})
	if !m.Post(func() { <-hold }) {
		t.Fatal("the worker refused the holding job")
	}
	var got []string
	ob, err := a.NewOpenButton(nil, OpenButtonConfig{OnOpen: func(p string) { got = append(got, p) }})
	if err != nil {
		t.Fatal(err)
	}
	ob.choose(path)
	a.pump(time.Now())
	if !slices.Equal(got, []string{path}) {
		t.Fatalf("output = %v behind a stuck disk, want the choice", got)
	}
	if len(ob.Recents()) != 0 {
		t.Fatalf("recents = %v behind a stuck disk", ob.Recents())
	}
	close(hold)
	pumpUntil(t, a, func() bool { return len(ob.Recents()) == 1 })
	if !m.Stop(5 * time.Second) {
		t.Fatal("the worker never settled")
	}
	fresh := sharedRecents(t)
	if err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if l := fresh.List(); len(l) != 1 || l[0].Path != path {
		t.Errorf("file holds %+v, want the choice", l)
	}
}

func TestRecentsReachTheFileAtRunExit(t *testing.T) {
	a := openButtonApp(t)
	path := openButtonFiles(t, "one.txt")[0]
	a.addRecents([]string{path})
	a.watchers.shutdown()
	fresh := sharedRecents(t)
	if err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if l := fresh.List(); len(l) != 1 || l[0].Path != path {
		t.Errorf("file holds %+v after the exit, want the choice", l)
	}
}

// TestWithoutADataHomeAChoiceIsDeliveredAndNoListIsKept covers an
// environment that names no user data directory: the choice still
// reaches the app, and no recents file appears in the working
// directory in the list's stead.
func TestWithoutADataHomeAChoiceIsDeliveredAndNoListIsKept(t *testing.T) {
	a := openButtonApp(t)
	path := openButtonFiles(t, "one.txt")[0]
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("XDG_DATA_HOME", "share")
	t.Setenv("HOME", "")
	var got []string
	ob, err := a.NewOpenButton(nil, OpenButtonConfig{OnOpen: func(p string) { got = append(got, p) }})
	if err != nil {
		t.Fatal(err)
	}
	ob.choose(path)
	a.pump(time.Now())
	if !slices.Equal(got, []string{path}) {
		t.Fatalf("output = %v, want [%s]", got, path)
	}
	if a.recentFiles != nil {
		t.Error("a recents manager exists with nowhere to keep its list")
	}
	if rec := ob.Recents(); len(rec) != 0 {
		t.Errorf("recents = %v with nowhere to keep them", rec)
	}
	left, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the choice left %s in the working directory", left[0].Name())
	}
}
