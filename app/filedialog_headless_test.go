package app_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

// A save over an existing file asks by default, over the picker: Cancel
// returns to the picker, Replace delivers the path once, and a new name
// or SkipOverwriteConfirm never asks.
func TestHeadlessSaveDialogAsksBeforeReplacing(t *testing.T) {
	wltest.Require(t)
	dir := t.TempDir()
	const existing = "existing.txt"
	if err := os.WriteFile(filepath.Join(dir, existing), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	apps := make(chan *app.Application, 1)
	var parent *app.Window
	inproc.Show(t, func(a *app.Application, _ *app.Output) error {
		var err error
		parent, err = a.NewWindow(app.WindowConfig{Title: "parent", Width: 200, Height: 200, Root: widget.NewSpacer(200, 200)})
		apps <- a
		return err
	})
	a := <-apps
	onLoop := func(fn func() error) {
		t.Helper()
		done := make(chan error, 1)
		a.Invoke(func() { done <- fn() })
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	saved := make(chan string, 4)
	open := func(name string, skip bool) (d *app.Dialog) {
		t.Helper()
		onLoop(func() error {
			var err error
			d, err = a.SaveFileDialog(parent, app.SaveDialogConfig{
				StartDir: dir, StartName: name, SkipOverwriteConfirm: skip,
				OnSave: func(path string) { saved <- path },
			})
			return err
		})
		return d
	}
	respond := func(d *app.Dialog, resp string) {
		t.Helper()
		onLoop(func() error { d.Respond(resp); return nil })
	}
	question := func(picker *app.Dialog) (q *app.Dialog) {
		t.Helper()
		onLoop(func() error {
			open := slices.DeleteFunc(slices.Clone(a.OpenDialogs()), func(d *app.Dialog) bool { return d == picker || d.Closed() })
			if len(open) != 1 {
				return fmt.Errorf("%d open dialogs beside the picker, want the one question", len(open))
			}
			q = open[0]
			return nil
		})
		return q
	}
	noSave := func(when string) {
		t.Helper()
		onLoop(func() error { return nil })
		if len(saved) != 0 {
			t.Fatalf("%s: delivered %q", when, <-saved)
		}
	}
	wantSave := func(want string) {
		t.Helper()
		wltest.Eventually(t, "the save delivered", func() error {
			if len(saved) == 0 {
				return errors.New("nothing delivered")
			}
			return nil
		})
		if got := <-saved; got != filepath.Join(dir, want) {
			t.Fatalf("delivered %q, want %s", got, want)
		}
		noSave("after the one delivery")
	}

	d := open(existing, false)
	respond(d, "ok")
	noSave("before an answer")
	if d.Closed() {
		t.Fatal("the picker closed before the question was answered")
	}
	q := question(d)
	respond(d, "ok")
	if again := question(d); again != q {
		t.Fatal("a second ok asked a second question")
	}
	respond(q, "cancel")
	noSave("after Cancel")
	if d.Closed() {
		t.Fatal("Cancel closed the picker instead of returning to it")
	}
	respond(d, "ok")
	q = question(d)
	respond(q, "replace")
	wantSave(existing)
	if !d.Closed() || !q.Closed() {
		t.Fatalf("after Replace: picker closed %v, question closed %v", d.Closed(), q.Closed())
	}

	d = open(existing, false)
	respond(d, "ok")
	q = question(d)
	respond(d, "cancel")
	wltest.Eventually(t, "the question closes with its picker", func() error {
		var open bool
		onLoop(func() error { open = !q.Closed(); return nil })
		if open {
			return errors.New("the question outlives the picker")
		}
		return nil
	})
	noSave("after cancelling the picker")

	d = open("new.txt", false)
	respond(d, "ok")
	wantSave("new.txt")
	if !d.Closed() {
		t.Fatal("a new name left the picker open")
	}

	d = open(existing, true)
	respond(d, "ok")
	wantSave(existing)
	if !d.Closed() {
		t.Fatal("SkipOverwriteConfirm left the picker open")
	}
}
