package app

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/widget"
)

func (f *wireFixture) answerFrames() {
	const frameOpcode, frameSize, doneEvent, deleteIDEvent = 3, 12, 0, 1
	const eventSize, displayID = 12, 1
	buf := make([]byte, 1<<16)
	var reply [2 * eventSize]byte
	have := 0
	for {
		n, err := f.peer.Read(buf[have:])
		if err != nil {
			return
		}
		have += n
		off := 0
		for have-off >= 8 {
			word := binary.NativeEndian.Uint32(buf[off+4:])
			size := int(word >> 16)
			if have-off < size {
				break
			}
			if word&0xffff == frameOpcode && size == frameSize {
				id := binary.NativeEndian.Uint32(buf[off+8:])
				binary.NativeEndian.PutUint32(reply[0:], id)
				binary.NativeEndian.PutUint32(reply[4:], eventSize<<16|doneEvent)
				binary.NativeEndian.PutUint32(reply[8:], 1)
				binary.NativeEndian.PutUint32(reply[12:], displayID)
				binary.NativeEndian.PutUint32(reply[16:], eventSize<<16|deleteIDEvent)
				binary.NativeEndian.PutUint32(reply[20:], id)
				if _, err := f.peer.Write(reply[:]); err != nil {
					return
				}
			}
			off += size
		}
		have = copy(buf, buf[off:have])
	}
}

func TestAllocBudgetWireFrame(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	f := newWireFixture(t, "wayland-budget")
	f.ctx.SetBuffered(true)
	sysfont.Warm(t.Context())
	animclock.Reset()
	a := NewApplication(f.sess)
	h := f.window(t, a)
	area := h.wnd.router.Root.(*widget.DrawingArea)
	go f.answerFrames()
	step := func() error {
		if !h.wnd.framePending {
			f.sess.WakeAfter(0)
		}
		return f.sess.Step()
	}
	frame := func() {
		h.wnd.dirty = true
		area.QueueDraw()
		for _, b := range h.bufs {
			if b.Busy() {
				b.Release()
			}
		}
		if err := a.tick(step, time.Now()); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	for range 20 {
		frame()
	}
	before := h.wnd.paintedPixels
	sends := sendmsgCalls(t, f.ctx)
	const runs = 200
	allocs := testing.AllocsPerRun(runs, frame)
	t.Logf("real-wire frame and Step: %.2f allocs, %.2f sendmsg, %d px", allocs, float64(sendmsgCalls(t, f.ctx)-sends)/(runs+1), before)
	if allocs != 0 {
		t.Errorf("a real-wire frame and Step allocate %v", allocs)
	}
	if got := sendmsgCalls(t, f.ctx) - sends; got > runs+1 {
		t.Errorf("%d frames took %d sendmsg, want one each", runs+1, got)
	}
}

func spinnerRig(t *testing.T) (*Application, *paintHarness, *time.Time) {
	t.Helper()
	clk := time.Unix(1000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return clk }))
	t.Cleanup(animclock.Reset)
	root := widget.NewBox(widget.Column, 8, 8)
	sp := widget.NewSpinner(24)
	sp.SetSpinning(true)
	root.Append(sp, false)
	h := newPaintHarness(root, 640, 470)
	a := idleBudgetApp(t)
	a.windows = []*hostWindow{h.wnd}
	return a, h, &clk
}

func TestAllocBudgetSpinnerTick(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	a, h, clk := spinnerRig(t)
	tick := func() {
		*clk = clk.Add(16 * time.Millisecond)
		for _, b := range h.bufs {
			if b.Busy() {
				b.Release()
			}
		}
		h.wnd.frame.Ready = true
		if err := a.tick(a.step, *clk); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	for range 20 {
		tick()
	}
	commits := h.surf.commits
	if allocs := testing.AllocsPerRun(200, tick); allocs != 0 {
		t.Errorf("a spinner's frame allocates %v", allocs)
	}
	if h.surf.commits == commits {
		t.Error("the spinner committed nothing")
	}
}

func TestAllocBudgetWithheldWindowTick(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	a, h, clk := spinnerRig(t)
	h.surf.withholdFrames = true
	tick := func() {
		*clk = clk.Add(animclock.FallbackInterval)
		if err := a.tick(a.step, *clk); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	tick()
	commits := h.surf.commits
	if allocs := testing.AllocsPerRun(100, tick); allocs != 0 {
		t.Errorf("a pass over a withheld spinner allocates %v", allocs)
	}
	if h.surf.commits != commits {
		t.Errorf("a window with its callback withheld committed %d frames", h.surf.commits-commits)
	}
}

func TestAllocBudgetBlurredWindowTick(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	a := idleBudgetApp(t)
	entry := widget.NewEntry(imeBudgetFace(t), 14, 0)
	entry.SetText("a field that lost the keyboard")
	widget.SizeWithin(entry, widget.Size{W: 600, H: 40})
	entry.Arrange(render.Rect{W: 600, H: 40})
	w := a.windows[0]
	w.router.Root = entry
	if err := w.router.SetFocus(entry); err != nil {
		t.Fatal(err)
	}
	if err := w.router.SetFocus(nil); err != nil {
		t.Fatal(err)
	}
	tick := func() {
		if err := a.tick(a.step, time.Now()); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	tick()
	tick()
	if animclock.Active() {
		t.Error("a blurred field kept its caret blinking")
	}
	if allocs := testing.AllocsPerRun(100, tick); allocs != 0 {
		t.Errorf("a pass over a blurred window allocates %v", allocs)
	}
}
