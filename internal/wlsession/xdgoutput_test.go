package wlsession

import (
	"errors"
	"strings"
	"testing"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// fakeXdgOutput records one xdg_output's requests and keeps the event
// listeners the session registered.
type fakeXdgOutput struct {
	destroyed int
	posH      wlr.ZxdgOutputV1LogicalPositionHandler
	sizeH     wlr.ZxdgOutputV1LogicalSizeHandler
	doneH     wlr.ZxdgOutputV1DoneHandler
	nameH     wlr.ZxdgOutputV1NameHandler
	descH     wlr.ZxdgOutputV1DescriptionHandler
}

func (f *fakeXdgOutput) Destroy() error { f.destroyed++; return nil }

func (f *fakeXdgOutput) AddLogicalPositionHandler(h wlr.ZxdgOutputV1LogicalPositionHandler) {
	f.posH = h
}
func (f *fakeXdgOutput) AddLogicalSizeHandler(h wlr.ZxdgOutputV1LogicalSizeHandler) { f.sizeH = h }
func (f *fakeXdgOutput) AddDoneHandler(h wlr.ZxdgOutputV1DoneHandler)               { f.doneH = h }
func (f *fakeXdgOutput) AddNameHandler(h wlr.ZxdgOutputV1NameHandler)               { f.nameH = h }
func (f *fakeXdgOutput) AddDescriptionHandler(h wlr.ZxdgOutputV1DescriptionHandler) { f.descH = h }

// fakeXdgOutputMaker records the get_xdg_output requests and hands
// out fake xdg_output objects.
type fakeXdgOutputMaker struct {
	wlOutputs []*wl.Output
	outs      []*fakeXdgOutput
	err       error
}

func (f *fakeXdgOutputMaker) GetOutput(out *wl.Output) (xdgOutputAPI, error) {
	if f.err != nil {
		return nil, f.err
	}
	o := &fakeXdgOutput{}
	f.wlOutputs = append(f.wlOutputs, out)
	f.outs = append(f.outs, o)
	return o, nil
}

// TestXdgOutputIsOptional pins the bind-or-skip contract: the manager
// global must never gate Connect, and a session without it keeps the
// outputs registry-ordered with empty names.
func TestXdgOutputIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "zxdg_output_manager_v1" {
			t.Errorf("%q is required; xdg-output must stay feature-detected", g)
		}
	}
	s := &Session{}
	if s.XdgOutputAvailable() {
		t.Error("XdgOutputAvailable = true without the manager global")
	}
	s.ensureXdgOutputs() // no-op
}

// TestXdgOutputNameLandsOnOutput drives the name and geometry events
// through a fake maker and asserts they land on the Output struct —
// the DP-1/HDMI-A-1 identities wayle config matches on.
func TestXdgOutputNameLandsOnOutput(t *testing.T) {
	s := &Session{}
	fake := &fakeXdgOutputMaker{}
	s.xdgOutputMgr = fake

	var identities []*Output
	s.OnOutputIdentity = func(o *Output) { identities = append(identities, o) }

	out := &Output{WL: &wl.Output{}, Scale: 1, name: 40}
	s.trackOutput(out)
	s.ensureXdgOutputs()
	if len(fake.outs) != 1 || fake.wlOutputs[0] != out.WL {
		t.Fatalf("get_xdg_output calls = %d, want 1 for the tracked output", len(fake.outs))
	}
	xdg := fake.outs[0]
	if xdg.nameH == nil || xdg.descH == nil || xdg.posH == nil || xdg.sizeH == nil || xdg.doneH == nil {
		t.Fatal("xdg_output events left unwired")
	}

	xdg.nameH.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-1"})
	if out.Name != "DP-1" {
		t.Errorf("output name = %q, want DP-1", out.Name)
	}
	if len(identities) != 1 || identities[0] != out {
		t.Fatalf("identity hooks = %d, want 1", len(identities))
	}
	xdg.descH.HandleZxdgOutputV1Description(wlr.ZxdgOutputV1DescriptionEvent{Description: "Dell U2723QE"})
	if out.Description != "Dell U2723QE" {
		t.Errorf("description = %q, want the xdg_output one", out.Description)
	}
	xdg.posH.HandleZxdgOutputV1LogicalPosition(wlr.ZxdgOutputV1LogicalPositionEvent{X: 1920, Y: 0})
	xdg.sizeH.HandleZxdgOutputV1LogicalSize(wlr.ZxdgOutputV1LogicalSizeEvent{Width: 1920, Height: 1080})
	if out.LogicalX != 1920 || out.LogicalY != 0 || out.LogicalW != 1920 || out.LogicalH != 1080 {
		t.Errorf("logical geometry = %d,%d %dx%d, want 1920,0 1920x1080", out.LogicalX, out.LogicalY, out.LogicalW, out.LogicalH)
	}
	xdg.doneH.HandleZxdgOutputV1Done(wlr.ZxdgOutputV1DoneEvent{}) // deprecated: accepted, ignored

	// The identity hook fires on a name change, not on a repeat.
	xdg.nameH.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-1"})
	if len(identities) != 1 {
		t.Errorf("identity hooks after repeat = %d, want 1", len(identities))
	}
	xdg.nameH.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-2"}) // compositor renames (rare, but pinned)
	if out.Name != "DP-2" || len(identities) != 2 {
		t.Errorf("rename: name=%q hooks=%d, want DP-2/2", out.Name, len(identities))
	}

	// Hotplug after the manager bound: the new output gets its
	// xdg_output on the next ensure, existing ones are not re-created.
	second := &Output{WL: &wl.Output{}, Scale: 1, name: 41}
	s.trackOutput(second)
	s.ensureXdgOutputs()
	if len(fake.outs) != 2 || fake.wlOutputs[1] != second.WL {
		t.Fatalf("after hotplug: get_xdg_output calls = %d, want 2 (one per output)", len(fake.outs))
	}

	// A wire failure leaves the output discoverable but unnamed, and
	// the next ensure retries it.
	fake.err = errXdgFail
	third := &Output{WL: &wl.Output{}, Scale: 1, name: 42}
	s.trackOutput(third)
	s.ensureXdgOutputs()
	if len(fake.outs) != 2 || third.xdg != nil {
		t.Fatalf("failed get_xdg_output must not attach: outs=%d attached=%v", len(fake.outs), third.xdg != nil)
	}
}

var errXdgFail = errors.New("get_xdg_output failed")

// Every identity watcher hears a name after the hook; a stopped one
// hears nothing more, and an unchanged name notifies no one.
func TestWatchOutputIdentityFansOut(t *testing.T) {
	s := &Session{}
	fake := &fakeXdgOutputMaker{}
	s.xdgOutputMgr = fake
	var order []string
	s.OnOutputIdentity = func(*Output) { order = append(order, "hook") }
	stopA := s.WatchOutputIdentity(func(o *Output) { order = append(order, "a:"+o.Name) })
	s.WatchOutputIdentity(func(o *Output) { order = append(order, "b:"+o.Name) })

	out := &Output{WL: &wl.Output{}, Scale: 1, name: 41}
	s.trackOutput(out)
	s.ensureXdgOutputs()
	name := fake.outs[0].nameH
	name.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-1"})
	if got := strings.Join(order, ","); got != "hook,a:DP-1,b:DP-1" {
		t.Fatalf("notify order = %s", got)
	}
	order = nil
	name.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-1"})
	if len(order) != 0 {
		t.Fatalf("an unchanged name notified %v", order)
	}
	stopA()
	stopA()
	name.HandleZxdgOutputV1Name(wlr.ZxdgOutputV1NameEvent{Name: "DP-2"})
	if got := strings.Join(order, ","); got != "hook,b:DP-2" {
		t.Fatalf("after stop = %s", got)
	}
}
