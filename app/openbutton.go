package app

import (
	"os"
	"path/filepath"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/widget"
)

const openButtonDefaultRecents = 5

// OpenButtonConfig declares an open button: the label, the starting
// directory, file filters, whether it picks folders, how many recent
// files to show, and the OnOpen callback for the chosen path.
type OpenButtonConfig struct {
	Label    string
	StartDir string
	Filters  []widget.FileFilter
	Folder   bool
	Recents  int
	OnOpen   func(path string)
}

// OpenButton is a button that opens a file dialog and a dropdown of
// recent files; choosing either fires OnOpen with the path. It embeds
// the composite Widget (button plus recents dropdown).
type OpenButton struct {
	widget.Widget
	app     *Application
	parent  *Window
	cfg     OpenButtonConfig
	open    func(parent *Window, mode widget.FileMode, cfg FileDialogConfig) error
	recents *widget.Dropdown
	paths   []string
}

// NewOpenButton returns an open button parented to parent. It needs a
// resolved face; when the application has none it returns ErrNoDialogFace.
func (a *Application) NewOpenButton(parent *Window, cfg OpenButtonConfig) (*OpenButton, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	ob := &OpenButton{app: a, parent: parent, cfg: cfg}
	ob.open = a.openButtonDialog
	label := cfg.Label
	if label == "" {
		label = widget.Tr("Open")
	}
	btn := widget.NewButton(widget.NewLabel(face, 14, label, 0), 8, 6)
	btn.OnClick = ob.openDialog
	ob.recents = widget.NewDropdown(face, 13, nil, -1)
	ob.recents.OnSelect = func(int) { ob.choose(ob.paths[ob.recents.Selected()]) }
	ob.Widget = widget.NewBox(widget.Row, 6, 0).Append(btn, false).Append(ob.recents, false)
	ob.RefreshRecents()
	return ob, nil
}

// Recents returns a copy of the recent file paths the dropdown shows.
func (ob *OpenButton) Recents() []string {
	out := make([]string, len(ob.paths))
	copy(out, ob.paths)
	return out
}

// RefreshRecents rebuilds the recents dropdown from the recent-files
// manager, keeping at most cfg.Recents entries that still exist on disk.
func (ob *OpenButton) RefreshRecents() {
	size := ob.cfg.recentSize()
	paths := ob.paths[:0]
	for _, e := range ob.app.recentsManager().List() {
		if len(paths) == size {
			break
		}
		if _, err := os.Stat(e.Path); err == nil {
			paths = append(paths, e.Path)
		}
	}
	ob.paths = paths
	labels := make([]string, len(paths))
	for i, p := range paths {
		labels[i] = filepath.Base(p)
	}
	ob.recents.SetItems(labels, len(labels)-1)
}

func (ob *OpenButton) openDialog() {
	cfg := FileDialogConfig{StartDir: ob.cfg.StartDir, Filters: ob.cfg.Filters}
	cfg.OnOpen = func(paths []string) {
		if len(paths) > 0 {
			ob.choose(paths[0])
		}
	}
	if err := ob.open(ob.parent, ob.cfg.fileMode(), cfg); err != nil {
		debug.Log("config", "open button: %v", err)
	}
}

func (ob *OpenButton) choose(path string) {
	if err := ob.app.recentsManager().Add(path, "lys-ui"); err != nil {
		debug.Log("config", "recents add %s: %v", path, err)
	}
	if ob.cfg.OnOpen != nil {
		ob.cfg.OnOpen(path)
	}
	ob.RefreshRecents()
}

func (cfg OpenButtonConfig) fileMode() widget.FileMode {
	if cfg.Folder {
		return widget.FileModeOpenFolder
	}
	return widget.FileModeOpen
}

func (cfg OpenButtonConfig) recentSize() int {
	if cfg.Recents > 0 {
		return cfg.Recents
	}
	return openButtonDefaultRecents
}

func (a *Application) openButtonDialog(parent *Window, mode widget.FileMode, cfg FileDialogConfig) error {
	var err error
	if mode == widget.FileModeOpenFolder {
		_, err = a.OpenFolderDialog(parent, cfg)
	} else {
		_, err = a.OpenFileDialog(parent, cfg)
	}
	return err
}
