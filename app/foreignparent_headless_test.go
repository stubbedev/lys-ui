package app_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

// exportedWindow maps a toplevel in a client of its own and returns it
// with its client and the handle the compositor exported it as.
func exportedWindow(t *testing.T, title string) (*app.Application, *app.Window, string) {
	t.Helper()
	var a *app.Application
	opened := make(chan *app.Window, 1)
	inproc.Show(t, func(application *app.Application, _ *app.Output) error {
		a = application
		w, err := a.NewWindow(app.WindowConfig{Title: title, AppID: "org.example.Owner", Root: widget.NewDrawingArea(96, 96)})
		opened <- w
		return err
	})
	w := <-opened
	var handle string
	wltest.Eventually(t, "the owner's export handle", func() error {
		got := make(chan string, 1)
		a.Invoke(func() { got <- w.ExportHandle() })
		if handle = <-got; handle == "" {
			return errors.New("no handle yet")
		}
		return nil
	})
	return a, w, handle
}

// foreignDialog maps, in a client of its own, a modal dialog parented to
// the toplevel handle names; lost counts its OnParentLost calls.
func foreignDialog(t *testing.T, handle string, lost *atomic.Int32) (*app.Application, *app.Window) {
	t.Helper()
	var b *app.Application
	opened := make(chan *app.Window, 1)
	configured := make(chan struct{}, 1)
	inproc.Show(t, func(application *app.Application, _ *app.Output) error {
		b = application
		w, err := b.NewWindow(app.WindowConfig{
			Title: "dialog", AppID: "org.example.Dialog", Root: widget.NewDrawingArea(48, 48),
			ForeignParent: handle, Modal: true,
			OnParentLost: func() { lost.Add(1) },
			OnResize: func(int, int) {
				select {
				case configured <- struct{}{}:
				default:
				}
			},
		})
		opened <- w
		return err
	})
	w := <-opened
	wltest.Eventually(t, "the dialog configured", func() error {
		select {
		case <-configured:
			return nil
		default:
			return errors.New("not configured yet")
		}
	})
	return b, w
}

// stillOpen fails unless w, of application a, is open.
func stillOpen(a *app.Application, w *app.Window) error {
	closed := make(chan bool, 1)
	a.Invoke(func() { closed <- w.Closed() })
	if <-closed {
		return errors.New("the dialog closed with its parent")
	}
	return nil
}

// A dialog parented to another client's exported toplevel keeps its
// parent while the parent lives, hears OnParentLost exactly once when
// the parent closes, and stays mapped.
func TestHeadlessForeignParentLostOnce(t *testing.T) {
	wltest.Require(t)
	owner, ownerWin, handle := exportedWindow(t, "owner")
	var lost atomic.Int32
	b, dialog := foreignDialog(t, handle, &lost)
	if n := lost.Load(); n != 0 {
		t.Fatalf("OnParentLost ran %d times with the parent open", n)
	}
	owner.Invoke(ownerWin.Close)
	wltest.Eventually(t, "OnParentLost after the parent closed", func() error {
		if n := lost.Load(); n != 1 {
			return fmt.Errorf("OnParentLost ran %d times", n)
		}
		return nil
	})
	synced := make(chan struct{})
	b.Invoke(func() { close(synced) })
	<-synced
	if n := lost.Load(); n != 1 {
		t.Errorf("OnParentLost ran %d times for one parent closing, want once", n)
	}
	if err := stillOpen(b, dialog); err != nil {
		t.Error(err)
	}
}

// A handle that names no toplevel is a lost parent at once, and the
// window stays mapped without one.
func TestHeadlessUnknownForeignParent(t *testing.T) {
	wltest.Require(t)
	var lost atomic.Int32
	b, dialog := foreignDialog(t, "no-such-handle", &lost)
	wltest.Eventually(t, "OnParentLost for an unknown handle", func() error {
		if n := lost.Load(); n != 1 {
			return fmt.Errorf("OnParentLost ran %d times", n)
		}
		return nil
	})
	if err := stillOpen(b, dialog); err != nil {
		t.Error(err)
	}
}
