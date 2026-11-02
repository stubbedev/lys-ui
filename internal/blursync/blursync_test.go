package blursync

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

type fakeSurface struct {
	offered bool
	fail    error
	sends   [][]render.Rect
}

func (f *fakeSurface) BlurAvailable() bool { return f.offered }

func (f *fakeSurface) SetBlurRegion(rects []render.Rect) error {
	if f.fail != nil {
		return f.fail
	}
	f.sends = append(f.sends, rects)
	return nil
}

func TestAShapeGoesOutOnlyWhenItChanges(t *testing.T) {
	var s Sync
	f := &fakeSurface{offered: true}
	root := widget.NewBox(widget.Column, 0, 0)
	shape := []render.Rect{{W: 10, H: 10}}
	get := func() []render.Rect { return shape }

	s.Update(f, false, root, get)
	if len(f.sends) != 0 {
		t.Fatalf("a surface that never asked sent %v", f.sends)
	}
	s.Update(f, true, root, get)
	s.Update(f, true, root, get)
	if len(f.sends) != 1 || !s.Active() {
		t.Fatalf("sends %v, active %v; want one send, active", f.sends, s.Active())
	}
	shape = []render.Rect{{W: 20, H: 10}}
	s.Update(f, true, root, get)
	if len(f.sends) != 2 || !slices.Equal(f.sends[1], shape) {
		t.Fatalf("after the change: %v", f.sends)
	}
	s.Update(f, false, root, get)
	s.Update(f, false, root, get)
	if len(f.sends) != 3 || f.sends[2] != nil || s.Active() {
		t.Fatalf("turning off sent %v, active %v; want one removal", f.sends, s.Active())
	}
}

func TestTheFallbackClassFollowsTheOffer(t *testing.T) {
	var s Sync
	f := &fakeSurface{}
	root := widget.NewBox(widget.Column, 0, 0)
	get := func() []render.Rect { return []render.Rect{{W: 10, H: 10}} }

	s.Update(f, true, root, get)
	if len(f.sends) != 0 || !widget.HasClass(root, css.BlurUnavailable) {
		t.Fatalf("unoffered: sends %v, class %v", f.sends, widget.HasClass(root, css.BlurUnavailable))
	}
	f.offered = true
	s.Update(f, true, root, get)
	if len(f.sends) != 1 || widget.HasClass(root, css.BlurUnavailable) {
		t.Fatalf("offered: sends %v, class %v", f.sends, widget.HasClass(root, css.BlurUnavailable))
	}
	f.offered = false
	s.Update(f, true, root, get)
	if !widget.HasClass(root, css.BlurUnavailable) || s.Active() {
		t.Fatalf("withdrawn: class %v, active %v", widget.HasClass(root, css.BlurUnavailable), s.Active())
	}
}

func TestAFailedSendRetriesNextFrame(t *testing.T) {
	var s Sync
	f := &fakeSurface{offered: true, fail: errors.New("no")}
	root := widget.NewBox(widget.Column, 0, 0)
	get := func() []render.Rect { return []render.Rect{{W: 10, H: 10}} }
	s.Update(f, true, root, get)
	if s.Active() {
		t.Fatal("a failed send reports the blur active")
	}
	f.fail = nil
	s.Update(f, true, root, get)
	if len(f.sends) != 1 || !s.Active() {
		t.Fatalf("after the retry: sends %v, active %v", f.sends, s.Active())
	}
}
