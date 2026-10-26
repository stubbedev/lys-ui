package wlsession

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// fakeToplevelHandle records the requests a toplevel receives and
// keeps the event listeners the session registered, so tests can
// drive the events back.
type fakeToplevelHandle struct {
	requests []string
	seat     *wl.Seat

	titleH  wlr.ZwlrForeignToplevelHandleV1TitleHandler
	appIDH  wlr.ZwlrForeignToplevelHandleV1AppIdHandler
	stateH  wlr.ZwlrForeignToplevelHandleV1StateHandler
	doneH   wlr.ZwlrForeignToplevelHandleV1DoneHandler
	closedH wlr.ZwlrForeignToplevelHandleV1ClosedHandler
}

func (f *fakeToplevelHandle) do(name string) { f.requests = append(f.requests, name) }

func (f *fakeToplevelHandle) SetMaximized() error   { f.do("set_maximized"); return nil }
func (f *fakeToplevelHandle) UnsetMaximized() error { f.do("unset_maximized"); return nil }
func (f *fakeToplevelHandle) SetMinimized() error   { f.do("set_minimized"); return nil }
func (f *fakeToplevelHandle) UnsetMinimized() error { f.do("unset_minimized"); return nil }
func (f *fakeToplevelHandle) Activate(seat *wl.Seat) error {
	f.seat = seat
	f.do("activate")
	return nil
}
func (f *fakeToplevelHandle) Close() error   { f.do("close"); return nil }
func (f *fakeToplevelHandle) Destroy() error { f.do("destroy"); return nil }

func (f *fakeToplevelHandle) AddTitleHandler(h wlr.ZwlrForeignToplevelHandleV1TitleHandler) {
	f.titleH = h
}

func (f *fakeToplevelHandle) AddAppIdHandler(h wlr.ZwlrForeignToplevelHandleV1AppIdHandler) {
	f.appIDH = h
}

func (f *fakeToplevelHandle) AddStateHandler(h wlr.ZwlrForeignToplevelHandleV1StateHandler) {
	f.stateH = h
}

func (f *fakeToplevelHandle) AddDoneHandler(h wlr.ZwlrForeignToplevelHandleV1DoneHandler) {
	f.doneH = h
}

func (f *fakeToplevelHandle) AddClosedHandler(h wlr.ZwlrForeignToplevelHandleV1ClosedHandler) {
	f.closedH = h
}

// TestForeignToplevelIsOptional pins the bind-or-skip contract: the
// manager global must never gate Connect, and a session that never
// saw it exposes no toplevels and survives every request.
func TestForeignToplevelIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "zwlr_foreign_toplevel_manager_v1" {
			t.Errorf("%q is required; foreign toplevels must stay feature-detected", g)
		}
	}
	s := &Session{}
	if s.ForeignToplevelAvailable() {
		t.Error("ForeignToplevelAvailable = true without the manager global")
	}
	if len(s.Toplevels()) != 0 {
		t.Errorf("Toplevels = %v, want none", s.Toplevels())
	}
	// Every request path is nil-safe: a bare session and a toplevel
	// without a wire handle must not panic.
	s.HandleZwlrForeignToplevelManagerV1Finished(wlr.ZwlrForeignToplevelManagerV1FinishedEvent{})
	var nilTL *Toplevel
	nilTL.Activate()
	nilTL.Close()
	seatless := &Toplevel{sess: s}
	seatless.Activate()
	seatless.SetMaximized()
	seatless.SetMinimized()
	seatless.UnsetMaximized()
	seatless.UnsetMinimized()
	seatless.Close()
}

// TestForeignToplevelRoundTrip drives the full event flow against a
// fake handle: title, app-id, and state arrive in a batch, done
// applies them to the listed toplevel, the requests go back out, and
// closed retires the entry.
func TestForeignToplevelRoundTrip(t *testing.T) {
	s := &Session{seat: &wl.Seat{}}
	var added, updated, removed []*Toplevel
	s.OnToplevelAdded = func(tl *Toplevel) { added = append(added, tl) }
	s.OnToplevelUpdated = func(tl *Toplevel) { updated = append(updated, tl) }
	s.OnToplevelRemoved = func(tl *Toplevel) { removed = append(removed, tl) }

	fake := &fakeToplevelHandle{}
	tl := s.addToplevel(fake)
	if len(added) != 1 || added[0] != tl || len(s.Toplevels()) != 1 {
		t.Fatalf("addToplevel did not list the toplevel: added=%d listed=%d", len(added), len(s.Toplevels()))
	}

	// The listener wiring: every handle event has a listener attached.
	if fake.titleH == nil || fake.appIDH == nil || fake.stateH == nil || fake.doneH == nil || fake.closedH == nil {
		t.Fatal("addToplevel left handle events unwired")
	}

	fake.titleH.HandleZwlrForeignToplevelHandleV1Title(wlr.ZwlrForeignToplevelHandleV1TitleEvent{Title: "Terminal"})
	fake.appIDH.HandleZwlrForeignToplevelHandleV1AppId(wlr.ZwlrForeignToplevelHandleV1AppIdEvent{AppId: "foot"})
	fake.stateH.HandleZwlrForeignToplevelHandleV1State(wlr.ZwlrForeignToplevelHandleV1StateEvent{
		State: []int32{
			wlr.ZwlrForeignToplevelHandleV1StateActivated,
			wlr.ZwlrForeignToplevelHandleV1StateMaximized,
		},
	})
	fake.doneH.HandleZwlrForeignToplevelHandleV1Done(wlr.ZwlrForeignToplevelHandleV1DoneEvent{})

	if tl.Title != "Terminal" || tl.AppID != "foot" {
		t.Errorf("title = %q app-id = %q, want Terminal/foot", tl.Title, tl.AppID)
	}
	if !tl.Activated || !tl.Maximized || tl.Minimized || tl.Fullscreen {
		t.Errorf("state = %+v, want activated+maximized only", tl)
	}
	if len(updated) != 1 || updated[0] != tl {
		t.Fatalf("done events delivered = %d, want 1", len(updated))
	}

	// Requests: activate carries the session's seat; close goes out
	// verbatim.
	tl.Activate()
	tl.SetMinimized()
	tl.Close()
	want := []string{"activate", "set_minimized", "close"}
	if len(fake.requests) != len(want) {
		t.Fatalf("requests = %v, want %v", fake.requests, want)
	}
	for i, r := range want {
		if fake.requests[i] != r {
			t.Errorf("request[%d] = %q, want %q", i, fake.requests[i], r)
		}
	}
	if fake.seat != s.seat {
		t.Error("activate must carry the session's seat")
	}

	// A later batch that only refreshes state keeps the known title.
	fake.stateH.HandleZwlrForeignToplevelHandleV1State(wlr.ZwlrForeignToplevelHandleV1StateEvent{
		State: []int32{wlr.ZwlrForeignToplevelHandleV1StateMinimized},
	})
	fake.doneH.HandleZwlrForeignToplevelHandleV1Done(wlr.ZwlrForeignToplevelHandleV1DoneEvent{})
	if tl.Title != "Terminal" || tl.AppID != "foot" {
		t.Errorf("partial batch lost identity: %q/%q", tl.Title, tl.AppID)
	}
	if !tl.Minimized || tl.Activated || tl.Maximized {
		t.Errorf("state = %+v, want minimized only", tl)
	}

	// Closed retires the handle: dropped from the list, destroyed on
	// the wire, removal hook fired.
	fake.closedH.HandleZwlrForeignToplevelHandleV1Closed(wlr.ZwlrForeignToplevelHandleV1ClosedEvent{})
	if len(s.Toplevels()) != 0 || len(removed) != 1 || removed[0] != tl {
		t.Fatalf("after closed: listed=%d removed=%d, want 0/1", len(s.Toplevels()), len(removed))
	}
	if !slices.Contains(fake.requests, "destroy") {
		t.Errorf("closed handle not destroyed: %v", fake.requests)
	}
}

// TestDecodeToplevelState pins the state enum mapping.
func TestDecodeToplevelState(t *testing.T) {
	max, min, act, full := decodeToplevelState([]int32{
		wlr.ZwlrForeignToplevelHandleV1StateMaximized,
		wlr.ZwlrForeignToplevelHandleV1StateFullscreen,
		wlr.ZwlrForeignToplevelHandleV1StateActivated,
	})
	if !max || !act || min || !full {
		t.Errorf("decode = %v/%v/%v/%v, want maximized+activated+fullscreen", max, min, act, full)
	}
	max, min, act, full = decodeToplevelState(nil)
	if max || min || act || full {
		t.Errorf("empty decode = %v/%v/%v/%v, want all false", max, min, act, full)
	}
}
