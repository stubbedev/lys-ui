package wlsession

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// Regression: the client never set a pointer cursor, leaving the
// compositor free to show a text caret across the whole window. The
// requested shape must resolve to a real xcursor name, never empty.
func TestEffectiveCursor(t *testing.T) {
	t.Run("empty means the default arrow", func(t *testing.T) {
		if got := effectiveCursor(""); got != defaultCursor {
			t.Errorf("effectiveCursor(\"\") = %q, want %q", got, defaultCursor)
		}
	})

	t.Run("a requested shape passes through", func(t *testing.T) {
		if got := effectiveCursor("xterm"); got != "xterm" {
			t.Errorf("effectiveCursor(xterm) = %q", got)
		}
	})
}

// The resize_* spellings edge interactions ask for must resolve into
// real xcursor shapes, and hand/grab shapes pass through by either
// name.
func TestCursorCandidates(t *testing.T) {
	// Names every xcursor theme family is expected to know: the core
	// corner and side shapes, the double arrows, and the hand set.
	core := map[string]bool{
		defaultCursor: true, "xterm": true, "watch": true,
		"grabbing": true, "hand1": true, "grab": true, "move": true,
		"top_side": true, "bottom_side": true, "left_side": true, "right_side": true,
		"top_left_corner": true, "top_right_corner": true,
		"bottom_left_corner": true, "bottom_right_corner": true,
		"sb_h_double_arrow": true, "sb_v_double_arrow": true,
		"fd_double_arrow": true, "bd_double_arrow": true,
		"fleur": true,
	}

	t.Run("every resize spelling has real xcursor candidates", func(t *testing.T) {
		for _, shape := range []string{
			"resize_n", "resize_s", "resize_e", "resize_w",
			"resize_nw", "resize_se", "resize_ne", "resize_sw",
			"resize_ns", "resize_ew", "resize_nwse", "resize_nesw",
		} {
			cands := cursorCandidates(shape)
			if len(cands) == 0 {
				t.Errorf("%s has no candidates", shape)
				continue
			}
			coreFallback := false
			for _, c := range cands {
				if c == "" {
					t.Errorf("%s carries an empty candidate", shape)
				}
				coreFallback = coreFallback || core[c]
			}
			if !coreFallback {
				t.Errorf("%s has no core xcursor shape among %v", shape, cands)
			}
		}
	})

	t.Run("edge names point at their own side arrows", func(t *testing.T) {
		for shape, want := range map[string]string{
			"resize_n": "top_side", "resize_s": "bottom_side",
			"resize_e": "right_side", "resize_w": "left_side",
			"resize_nw": "top_left_corner", "resize_se": "bottom_right_corner",
			"resize_ne": "top_right_corner", "resize_sw": "bottom_left_corner",
		} {
			if got := cursorCandidates(shape)[0]; got != want {
				t.Errorf("%s leads with %q, want %q", shape, got, want)
			}
		}
	})

	t.Run("grabbing leads with itself and falls back to the closed hand", func(t *testing.T) {
		if got := cursorCandidates("grabbing"); !slices.Equal(got, []string{"grabbing", "closedhand", "hand2"}) {
			t.Errorf("candidates(\"grabbing\") = %v", got)
		}
	})

	t.Run("core names pass through", func(t *testing.T) {
		for _, shape := range []string{"hand1", "xterm", "left_ptr", "alias", "copy"} {
			if got := cursorCandidates(shape); len(got) != 1 || got[0] != shape {
				t.Errorf("candidates(%q) = %v, want pass-through", shape, got)
			}
		}
	})

	t.Run("empty stays the default", func(t *testing.T) {
		if got := cursorCandidates("")[0]; got != defaultCursor {
			t.Errorf("candidates(\"\")[0] = %q, want %q", got, defaultCursor)
		}
	})
}

// Name-based resolution walks the candidates until the theme has one;
// a theme missing every candidate degrades to the arrow instead of
// failing.
func TestResolveCursorName(t *testing.T) {
	theme := func(present ...string) *cursorTheme {
		c := &cursorTheme{cursors: map[string]*themedCursor{}, missing: map[string]error{}}
		for _, name := range present {
			c.cursors[name] = &themedCursor{}
		}
		for _, names := range cursorAliases {
			for _, name := range names {
				if _, ok := c.cursors[name]; !ok {
					c.missing[name] = errors.New("not in the theme")
				}
			}
		}
		for _, name := range []string{defaultCursor, "unknown-shape"} {
			if _, ok := c.cursors[name]; !ok {
				c.missing[name] = errors.New("not in the theme")
			}
		}
		return c
	}
	for _, tc := range []struct {
		name    string
		present []string
		desired string
		want    string
	}{
		{"the first candidate the theme carries wins", []string{"ew-resize", "sb_h_double_arrow", defaultCursor}, "resize_w", "ew-resize"},
		{"missing shapes fall through the candidate list", []string{"sb_h_double_arrow"}, "resize_w", "sb_h_double_arrow"},
		{"a theme with none of them degrades to the arrow", nil, "resize_nwse", defaultCursor},
		{"unknown names degrade too", nil, "unknown-shape", defaultCursor},
		{"named shapes the theme lacks degrade", nil, "grabbing", defaultCursor},
		{"empty is the arrow", nil, "", defaultCursor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, settled := theme(tc.present...).resolve(tc.desired)
			if got != tc.want || !settled {
				t.Errorf("%q resolved to %q, settled %v; want %q", tc.desired, got, settled, tc.want)
			}
		})
	}

	t.Run("a candidate not yet read holds the answer back", func(t *testing.T) {
		c := &cursorTheme{cursors: map[string]*themedCursor{"sb_h_double_arrow": {}}, missing: map[string]error{}}
		if got, settled := c.resolve("resize_w"); settled {
			t.Errorf("resize_w settled on %q with left_side unread", got)
		}
		if got := c.unread("resize_w"); !slices.Equal(got, []string{"left_side", "ew-resize", "size_hor", defaultCursor}) {
			t.Errorf("unread = %v", got)
		}
	})
}

// cursorFrame is the injected-clock half of frame advancement: the
// frame due at an elapsed offset and the time it has left.
func TestCursorFrame(t *testing.T) {
	delays := []uint32{30, 40, 30} // total 100ms

	t.Run("frames boundary exactly on their delays", func(t *testing.T) {
		for _, tc := range []struct {
			elapsed uint32
			index   int
			rest    time.Duration
		}{
			{0, 0, 30 * time.Millisecond},
			{29, 0, 1 * time.Millisecond},
			{30, 1, 40 * time.Millisecond},
			{69, 1, 1 * time.Millisecond},
			{70, 2, 30 * time.Millisecond},
			{99, 2, 1 * time.Millisecond},
		} {
			idx, rest := cursorFrame(100, delays, tc.elapsed)
			if idx != tc.index || rest != tc.rest {
				t.Errorf("cursorFrame(elapsed=%d) = (%d, %v), want (%d, %v)",
					tc.elapsed, idx, rest, tc.index, tc.rest)
			}
		}
	})

	t.Run("elapsed wraps around the loop", func(t *testing.T) {
		if idx, _ := cursorFrame(100, delays, 100); idx != 0 {
			t.Errorf("cursorFrame(elapsed=100) = frame %d, want the loop restarted at 0", idx)
		}
		if idx, _ := cursorFrame(100, delays, 145); idx != 1 {
			t.Errorf("cursorFrame(elapsed=145) = frame %d, want 1", idx)
		}
	})

	t.Run("static cursors never come due", func(t *testing.T) {
		if idx, rest := cursorFrame(0, delays, 50); idx != 0 || rest != 0 {
			t.Errorf("zero total = (%d, %v), want (0, 0)", idx, rest)
		}
		if idx, rest := cursorFrame(100, nil, 50); idx != 0 || rest != 0 {
			t.Errorf("no delays = (%d, %v), want (0, 0)", idx, rest)
		}
		if idx, rest := cursorFrame(100, []uint32{50}, 50); idx != 0 || rest != 0 {
			t.Errorf("single frame = (%d, %v), want (0, 0)", idx, rest)
		}
	})
}

// cursorHarness drives the cursor path with the theme, wire push,
// clock, and timer replaced: cursors come from a fixed set, pushed
// frames are recorded, and time only moves when the test advances it.
type cursorHarness struct {
	s      *Session
	now    time.Time
	arms   []time.Duration // durations the frame timer was armed with
	frames []*cursorImage  // frames pushed to the wire, in order
	fire   func()          // fires the most recent timer, if any
}

func newCursorHarness(t *testing.T, cursors map[string]*themedCursor) *cursorHarness {
	t.Helper()
	h := &cursorHarness{s: &Session{}}
	s := h.s
	s.pointer = wirePointer{p: &wl.Pointer{}}
	s.pointerEnterSerial = 1
	s.crs.surface = &wl.Surface{} // the wire push is faked; the surface is never used
	s.crs.now = func() time.Time { return h.now }
	s.crs.after = func(d time.Duration, f func()) {
		h.arms = append(h.arms, d)
		h.fire = f
	}
	s.crs.theme = func(desired string) (*themedCursor, string, error) {
		name := defaultCursor
		if i := slices.IndexFunc(cursorCandidates(desired), func(n string) bool { _, ok := cursors[n]; return ok }); i >= 0 {
			name = cursorCandidates(desired)[i]
		}
		cur, ok := cursors[name]
		if !ok {
			return nil, "", fmt.Errorf("theme lacks %q", name)
		}
		return cur, name, nil
	}
	s.crs.push = func(img *cursorImage) error {
		h.frames = append(h.frames, img)
		return nil
	}
	return h
}

// advance moves the clock and fires the pending frame timer, as the
// real timer goroutine would.
func (h *cursorHarness) advance(d time.Duration) {
	h.now = h.now.Add(d)
	if h.fire == nil {
		return
	}
	f := h.fire
	h.fire = nil
	f()
}

// armed reports whether a frame timer is pending.
func (h *cursorHarness) armed() bool { return h.s.crs.armed }

func fakeCursor(delays ...uint32) *themedCursor {
	images := make([]*cursorImage, len(delays))
	for i, d := range delays {
		images[i] = &cursorImage{delay: d}
	}
	return &themedCursor{images: images}
}

// The watch cursor must show the right frame at the right time and
// stop advancing entirely once it is no longer shown.
func TestCursorFrameAdvancement(t *testing.T) {
	watch := fakeCursor(30, 40, 30)
	h := newCursorHarness(t, map[string]*themedCursor{
		"watch":       watch,
		defaultCursor: fakeCursor(1),
	})

	t.Run("the first frame arms the timer for its delay", func(t *testing.T) {
		if err := h.s.SetCursor("watch"); err != nil {
			t.Fatalf("SetCursor: %v", err)
		}
		if len(h.frames) != 1 || h.frames[0] != watch.images[0] {
			t.Fatal("first apply did not push frame 0")
		}
		if !h.armed() || h.arms[len(h.arms)-1] != 30*time.Millisecond {
			t.Errorf("timer armed with %v, want 30ms", h.arms[len(h.arms)-1])
		}
	})

	t.Run("each fire shows the frame due at the injected clock", func(t *testing.T) {
		h.advance(35 * time.Millisecond) // 35ms in: frame 1 runs
		if len(h.frames) != 2 || h.frames[1] != watch.images[1] {
			t.Fatalf("after 35ms the pushed frame is not frame 1")
		}
		if h.arms[len(h.arms)-1] != 35*time.Millisecond {
			t.Errorf("re-armed with %v, want 35ms (frame 1 started 5ms in)", h.arms[len(h.arms)-1])
		}
		h.advance(39 * time.Millisecond) // 74ms in: frame 2
		if len(h.frames) != 3 || h.frames[2] != watch.images[2] {
			t.Fatalf("after 74ms the pushed frame is not frame 2")
		}
		h.advance(31 * time.Millisecond) // 105ms in: wrapped to frame 0
		if len(h.frames) != 4 || h.frames[3] != watch.images[0] {
			t.Fatalf("after 105ms the pushed frame is not the wrapped frame 0")
		}
	})

	t.Run("hiding the cursor stops the chain", func(t *testing.T) {
		if err := h.s.SetCursor(""); err != nil {
			t.Fatalf("SetCursor(default): %v", err)
		}
		if len(h.frames) != 5 || h.frames[4] != h.s.crs.cur.images[0] {
			t.Fatalf("the static cursor did not push its frame")
		}
		if h.armed() {
			t.Error("a static cursor left the frame timer armed")
		}
		before := len(h.frames)
		h.advance(1 * time.Second)
		if len(h.frames) != before {
			t.Error("frames kept advancing after the animated cursor was hidden")
		}
	})

	t.Run("a stale generation drops its timer", func(t *testing.T) {
		if err := h.s.SetCursor("watch"); err != nil {
			t.Fatalf("SetCursor: %v", err)
		}
		h.advance(10 * time.Millisecond)
		gen := h.s.crs.gen
		h.s.SetCursor("") // shape changes under the pending timer
		before := len(h.frames)
		h.s.advanceCursor(gen)
		if len(h.frames) != before {
			t.Error("a stale timer pushed a frame")
		}
	})
}

// "none" hides the pointer without consulting the theme (no theme
// carries a shape by that name, so a lookup would show the arrow), and
// any later shape brings a visible cursor back.
func TestCursorHidden(t *testing.T) {
	h := newCursorHarness(t, map[string]*themedCursor{
		defaultCursor: fakeCursor(1),
	})
	if err := h.s.SetCursor(CursorHidden); err != nil {
		t.Fatalf("SetCursor(none): %v", err)
	}
	if len(h.frames) != 1 || h.frames[0] != nil {
		t.Fatalf("hiding pushed %v, want one null frame", h.frames)
	}
	if h.s.crs.shown != CursorHidden || h.armed() {
		t.Fatalf("shown %q armed %v after hiding", h.s.crs.shown, h.armed())
	}
	if err := h.s.SetCursor(""); err != nil {
		t.Fatalf("SetCursor(default): %v", err)
	}
	if len(h.frames) != 2 || h.frames[1] == nil {
		t.Fatal("the default shape did not come back after hiding")
	}
	if h.s.crs.shown != defaultCursor {
		t.Errorf("shown %q, want the arrow", h.s.crs.shown)
	}
}

// A surface that set a special shape must fall back to the arrow on
// pointer leave - xcursor shapes stick across surfaces of one client,
// so the next surface would otherwise inherit the old shape.
func TestCursorRestoreOnLeave(t *testing.T) {
	surf := &wl.Surface{}
	h := newCursorHarness(t, map[string]*themedCursor{
		"xterm":       fakeCursor(1),
		defaultCursor: fakeCursor(1),
	})

	t.Run("leave restores the default", func(t *testing.T) {
		if err := h.s.SetCursor("xterm"); err != nil {
			t.Fatalf("SetCursor: %v", err)
		}
		if got := h.s.crs.shown; got != "xterm" {
			t.Fatalf("shape = %q, want xterm", got)
		}
		h.s.HandlePointerLeave(wl.PointerLeaveEvent{Surface: surf})
		if got := h.s.crs.shown; got != defaultCursor {
			t.Errorf("after leave the shape is %q, want %q", got, defaultCursor)
		}
		if got := h.s.crs.desired; got != defaultCursor {
			t.Errorf("after leave the desired shape is %q, want %q", got, defaultCursor)
		}
		if h.armed() {
			t.Error("leave left the frame timer armed")
		}
	})

	t.Run("a leave under grab keeps the shape", func(t *testing.T) {
		if err := h.s.SetCursor("xterm"); err != nil {
			t.Fatalf("SetCursor: %v", err)
		}
		h.s.grabSurface = surf
		h.s.HandlePointerLeave(wl.PointerLeaveEvent{Surface: surf})
		if got := h.s.crs.shown; got != "xterm" {
			t.Errorf("drag leave flipped the shape to %q", got)
		}
	})

	t.Run("capability loss stops the timer and keeps the choice", func(t *testing.T) {
		if err := h.s.SetCursor("xterm"); err != nil {
			t.Fatalf("SetCursor: %v", err)
		}
		h.s.pointerLost()
		if h.armed() {
			t.Error("pointer loss left the frame timer armed")
		}
		if got := h.s.crs.desired; got != "xterm" {
			t.Errorf("pointer loss forgot the shape: %q", got)
		}
	})
}
