package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/xdg"

	"github.com/stubbedev/lys-ui/internal/recentfiles"
	"github.com/stubbedev/lys-ui/widget"
)

// FileDialogConfig declares an open dialog: where it starts, what it
// accepts, and where the choice lands. The OpenFileDialog family
// (#82) wraps widget.FileChooser - the pure-Go picker - in the
// standard Dialog, with the recent-files list (XDG recently-used.xbel,
// shared with the desktop) and a validating ok button that keeps the
// dialog open until the choice is valid; the picker is portal-free by
// design, a portal backend can later ride the same DirSource seam, and
// the decision is recorded in docs/application-model.md.
type FileDialogConfig struct {
	// Title names the dialog window; empty gets a mode default.
	Title string
	// StartDir opens the picker here (home when empty); StartName
	// pre-fills the save name row.
	StartDir  string
	StartName string
	// Filters offers file name patterns; nil accepts everything.
	Filters []widget.FileFilter
	// Recents adds the Recent place and records chosen files in the
	// desktop's recently-used list.
	Recents bool
	// OnOpen receives the chosen absolute paths exactly once, on the
	// loop goroutine, after the dialog closed.
	OnOpen func(paths []string)
}

// SaveDialogConfig is FileDialogConfig for naming a file to write.
type SaveDialogConfig struct {
	Title     string
	StartDir  string
	StartName string
	Filters   []widget.FileFilter
	Recents   bool
	// SkipOverwriteConfirm returns a path that already exists without
	// asking. By default the picker asks, and declining returns to it.
	SkipOverwriteConfirm bool
	// OnSave receives the chosen absolute path exactly once, on the
	// loop goroutine, after any replace question was accepted and the
	// dialog closed.
	OnSave func(path string)
}

// OpenFileDialog opens the file picker for one existing file.
func (a *Application) OpenFileDialog(parent *Window, cfg FileDialogConfig) (*Dialog, error) {
	return a.fileDialog(parent, widget.FileModeOpen, "Open File", "Open", cfg, nil)
}

// OpenFilesDialog opens the file picker for any set of existing files.
func (a *Application) OpenFilesDialog(parent *Window, cfg FileDialogConfig) (*Dialog, error) {
	return a.fileDialog(parent, widget.FileModeOpenMultiple, "Open Files", "Open", cfg, nil)
}

// OpenFolderDialog opens the picker for one directory.
func (a *Application) OpenFolderDialog(parent *Window, cfg FileDialogConfig) (*Dialog, error) {
	return a.fileDialog(parent, widget.FileModeOpenFolder, "Select Folder", "Select", cfg, nil)
}

// SaveFileDialog opens the picker for naming a file to write, with
// overwrite confirmation unless turned off.
func (a *Application) SaveFileDialog(parent *Window, cfg SaveDialogConfig) (*Dialog, error) {
	fc := FileDialogConfig{
		Title: cfg.Title, StartDir: cfg.StartDir, StartName: cfg.StartName,
		Filters: cfg.Filters, Recents: cfg.Recents,
	}
	return a.fileDialog(parent, widget.FileModeSave, "Save File", "Save", fc, &cfg)
}

// fileDialog is the shared core of all four file dialogs: chooser in a
// Dialog whose ok button validates through the chooser, recents wired
// when asked, and the save-side replace question asked over the picker,
// which stays open until the question is answered.
func (a *Application) fileDialog(parent *Window, mode widget.FileMode, defTitle, okLabel string, cfg FileDialogConfig, save *SaveDialogConfig) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	title := cfg.Title
	if title == "" {
		title = widget.Tr(defTitle)
	}
	chooser := widget.NewFileChooser(face, 14, mode, cfg.StartDir, cfg.Filters)
	if cfg.StartName != "" && mode == widget.FileModeSave {
		chooser.SetSaveName(cfg.StartName)
	}
	if cfg.Recents {
		if m, ok := a.recentsManager(); ok {
			chooser.SetRecentsSource(func() []string { return existingRecents(m) })
		}
	}
	var chosen []string
	var d *Dialog
	deliver := func() {
		if save != nil {
			if save.OnSave != nil {
				save.OnSave(chosen[0])
			}
		} else if cfg.OnOpen != nil {
			cfg.OnOpen(chosen)
		}
	}
	record := func() {
		if cfg.Recents {
			a.addRecents(chosen)
		}
	}
	var replaceQuestion *Dialog
	replaceAccepted := false
	askingDisk := false
	askReplace := func() error {
		var err error
		replaceQuestion, err = a.NewDialog(d.win, DialogConfig{
			Title:   widget.Tr("Replace file?"),
			Content: a.confirmBody(filepath.Base(chosen[0])),
			Modal:   true,
			Buttons: []DialogButton{
				{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel},
				{Label: widget.Tr("Replace"), Response: "replace", Role: ButtonRoleDefault},
			},
			DefaultResponse: "replace",
			CancelResponse:  "cancel",
			Width:           340, Height: 140,
			OnResponse: func(resp string) {
				if resp != "replace" {
					return
				}
				replaceAccepted = true
				d.Respond("ok")
			},
		})
		return err
	}
	checkOverwrite := func(path string) {
		askingDisk = true
		chooser.ProbeExists(path, func(exists bool) {
			askingDisk = false
			if d.Closed() {
				return
			}
			if !exists {
				replaceAccepted = true
				d.Respond("ok")
				return
			}
			if err := askReplace(); err != nil {
				chooser.SetStatus(strings.ReplaceAll(widget.Tr("could not ask to replace the file: {error}"), "{error}", err.Error()))
			}
		})
	}
	chooser.OnApply = func(paths []string) {
		chosen = paths
		d.Respond("ok")
	}
	d, err := a.NewDialog(parent, DialogConfig{
		Title: title,
		Width: 560, Height: 440,
		Content: chooser,
		Modal:   true,
		Buttons: []DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel},
			{Label: widget.Tr(okLabel), Response: "ok", Role: ButtonRoleDefault},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		ValidateResponse: func(resp string) bool {
			if resp != "ok" || replaceAccepted {
				return true
			}
			if askingDisk || replaceQuestion != nil && !replaceQuestion.Closed() {
				return false
			}
			paths, ok := chooser.Apply()
			if !ok {
				return false
			}
			chosen = paths
			if save == nil || save.SkipOverwriteConfirm {
				return true
			}
			checkOverwrite(chosen[0])
			return false
		},
		OnResponse: func(resp string) {
			if resp != "ok" {
				return
			}
			record()
			deliver()
		},
		OnClosed: func() {
			if replaceQuestion != nil {
				replaceQuestion.Close()
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// confirmBody builds the replace-question card body.
func (a *Application) confirmBody(name string) widget.Widget {
	face := a.resolveFace(nil)
	th := widget.Current()
	named := strings.ReplaceAll(widget.Tr("A file named \"{name}\" already exists."), "{name}", name)
	return widget.NewBox(widget.Column, 8, 16).
		Append(widget.NewLabel(face, 14, named, 0), false).
		Append(widget.NewLabel(face, 13, widget.Tr("Replacing it overwrites its contents."), th.TextMuted), false)
}

const recentsStopTimeout = 3 * time.Second

// recentsManager lazily owns the desktop shared recents list. The list
// lives on the manager's worker, its load first in line; touch it only
// through Post. It is false, and logged, when the environment names no
// place to keep the list.
func (a *Application) recentsManager() (*recentfiles.Manager, bool) {
	if a.recentFiles != nil {
		return a.recentFiles, true
	}
	path, err := recentfiles.DefaultPath(xdg.OS)
	if err != nil {
		debug.Log("config", "recents: %v", err)
		return nil, false
	}
	m := recentfiles.New(path)
	a.recentFiles = m
	a.OnStop(func() {
		if !m.Stop(recentsStopTimeout) {
			debug.Log("config", "recents: the worker outlived the stop")
		}
	})
	postRecents(m, func() {
		if err := m.Load(); err != nil {
			debug.Log("config", "recents: %v", err)
		}
	})
	return m, true
}

func postRecents(m *recentfiles.Manager, work func()) bool {
	if m.Post(work) {
		return true
	}
	debug.Log("config", "recents: dropped work, the worker is stuck or stopped")
	return false
}

func (a *Application) addRecents(paths []string) {
	m, ok := a.recentsManager()
	if !ok {
		return
	}
	postRecents(m, func() {
		if err := m.Add("lys-ui", paths...); err != nil {
			debug.Log("config", "recents add %v: %v", paths, err)
		}
	})
}

func existingRecents(m *recentfiles.Manager) []string {
	answer := make(chan []string, 1)
	if !postRecents(m, func() { answer <- existingPaths(m.List(), 0) }) {
		return nil
	}
	return <-answer
}

func existingPaths(entries []recentfiles.Entry, limit int) []string {
	var paths []string
	for _, e := range entries {
		if limit > 0 && len(paths) == limit {
			break
		}
		if _, err := os.Stat(e.Path); err == nil {
			paths = append(paths, e.Path)
		}
	}
	return paths
}
