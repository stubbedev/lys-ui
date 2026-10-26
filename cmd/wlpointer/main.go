// Command wlpointer drives a zwlr_virtual_pointer_v1 device against a
// Wayland compositor: synthetic pointer input for interactive testing.
// It is the client-side half of the headless test environment (`just
// test-env`): move, click, and sweep exercise real compositor input
// paths against a widget program whose debug traces (`just demo-debug`
// builds) show what the input pipeline delivered.
//
// Usage:
//
//	wlpointer move X Y
//	wlpointer click X Y [BUTTON]
//	wlpointer axis X Y DY
//	wlpointer dwell X Y MS
//	wlpointer sweep X0 Y0 X1 Y1 [STEP]
//
// Coordinates are compositor space. Extents for absolute motion come
// from the current mode of the first wl_output, so no sizes are
// hardcoded. The device is created, driven, and destroyed per run:
// every invocation is stateless, which keeps test recipes composable.
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// btnLeft is the linux-evdev code wl_pointer uses for the primary button.
const btnLeft = 0x110

type vpManager struct{ wl.BaseProxy }

func newVPManager(ctx *wl.Context) *vpManager {
	m := new(vpManager)
	ctx.Register(m)
	return m
}

type virtualPointer struct{ wl.BaseProxy }

func newVirtualPointer(ctx *wl.Context) *virtualPointer {
	p := new(virtualPointer)
	ctx.Register(p)
	return p
}

// registry records the globals a run needs while they stream past.
type registry struct {
	seatName     uint32
	vpName       uint32
	outputName   uint32
	modeW, modeH int32
	haveMode     bool
}

func (r *registry) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_seat":
		r.seatName = ev.Name
	case "zwlr_virtual_pointer_manager_v1":
		r.vpName = ev.Name
	case "wl_output":
		r.outputName = ev.Name
	}
}

func (r *registry) HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent) {}

// HandleOutputMode tracks the current mode (flag bit 0) of the bound
// output; absolute pointer motion is normalized against its size.
func (r *registry) HandleOutputMode(ev wl.OutputModeEvent) {
	if ev.Flags&1 != 0 {
		r.modeW, r.modeH = ev.Width, ev.Height
		r.haveMode = true
	}
}

func (r *registry) HandleOutputGeometry(wl.OutputGeometryEvent) {}
func (r *registry) HandleOutputDone(wl.OutputDoneEvent)         {}
func (r *registry) HandleOutputScale(wl.OutputScaleEvent)       {}

// roundtrip dispatches until the display's sync callback completes,
// retrying past proxies destroyed mid-queue the way lys-ui's session
// does.
func roundtrip(d *wl.Display) error {
	cb, err := d.Sync()
	if err != nil {
		return err
	}
	defer wlclient.CallbackDestroy(cb)
	for {
		err = d.Context().RunTill(cb)
		if err == nil {
			return nil
		}
		if err.Error() != "proxy nil" {
			return err
		}
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wlpointer: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		die("usage: wlpointer move X Y | click X Y [BUTTON] | sweep X0 Y0 X1 Y1 [STEP]")
	}
	cmd, rest := args[0], args[1:]

	d, err := wl.Connect("")
	if err != nil {
		die("connect: %v", err)
	}
	defer func() { _ = d.Context().Close() }()
	reg, _ := d.GetRegistry()
	g := &registry{}
	wlclient.RegistryAddListener(reg, g)
	if err := roundtrip(d); err != nil {
		die("registry roundtrip: %v", err)
	}
	if g.vpName == 0 || g.seatName == 0 {
		die("compositor lacks zwlr_virtual_pointer_manager_v1 or wl_seat")
	}

	// Absolute motion is normalized against the current output mode;
	// the sweep probe below binds the first output to learn it.
	var out *wl.Output
	if g.outputName != 0 {
		out = wlclient.RegistryBindOutputInterface(reg, g.outputName, 2)
		wlclient.OutputAddListener(out, g)
	}
	if err := roundtrip(d); err != nil {
		die("output roundtrip: %v", err)
	}
	if !g.haveMode {
		die("no wl_output reported a current mode")
	}

	ctx := reg.Context()
	seat := wlclient.RegistryBindSeatInterface(reg, g.seatName, 1)
	mgr := newVPManager(ctx)
	if err := reg.Bind(g.vpName, "zwlr_virtual_pointer_manager_v1", 1, mgr); err != nil {
		die("bind manager: %v", err)
	}
	vp := newVirtualPointer(ctx)
	if err := ctx.SendRequest(mgr, 0, seat, vp); err != nil {
		die("create virtual pointer: %v", err)
	}
	if err := roundtrip(d); err != nil {
		die("post-create roundtrip: %v", err)
	}

	t := func() uint32 { return uint32(time.Now().UnixMilli() & math.MaxUint32) }
	move := func(x, y int32) {
		if err := ctx.SendRequest(vp, 1, t(), intconv.Uint32(x), intconv.Uint32(y), intconv.Uint32(g.modeW), intconv.Uint32(g.modeH)); err != nil {
			die("motion_absolute: %v", err)
		}
		_ = ctx.SendRequest(vp, 4)
	}
	button := func(code, state uint32) {
		if err := ctx.SendRequest(vp, 2, t(), code, state); err != nil {
			die("button: %v", err)
		}
		_ = ctx.SendRequest(vp, 4)
	}
	settle := func() { _ = roundtrip(d); time.Sleep(80 * time.Millisecond) }

	atoi := func(s string) int32 {
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			die("bad number %q: %v", s, err)
		}
		return int32(v)
	}

	switch cmd {
	case "move":
		if len(rest) != 2 {
			die("move needs X Y")
		}
		move(atoi(rest[0]), atoi(rest[1]))
		settle()
	case "click":
		if len(rest) < 2 {
			die("click needs X Y")
		}
		code := int32(btnLeft)
		if len(rest) == 3 {
			code = atoi(rest[2])
		}
		move(atoi(rest[0]), atoi(rest[1]))
		settle()
		button(uint32(code), 1)
		_ = roundtrip(d)
		time.Sleep(60 * time.Millisecond)
		button(uint32(code), 0)
		_ = roundtrip(d)
		time.Sleep(40 * time.Millisecond)
	case "sweep":
		if len(rest) < 4 {
			die("sweep needs X0 Y0 X1 Y1 [STEP]")
		}
		x0, y0, x1, y1 := atoi(rest[0]), atoi(rest[1]), atoi(rest[2]), atoi(rest[3])
		step := int32(40)
		if len(rest) == 5 {
			step = atoi(rest[4])
		}
		if step == 0 {
			step = 1
		}
		move(x0, y0)
		settle()
		// Horizontal when the endpoints share a row, else vertical;
		// print each point so callers can align coordinates with the
		// target's own traces.
		if y0 == y1 {
			for x := x0; x <= x1; x += step {
				fmt.Printf("MARK %d %d\n", x, y0)
				move(x, y0)
				_ = roundtrip(d)
				time.Sleep(45 * time.Millisecond)
			}
		} else {
			for y := y0; y <= y1; y += step {
				fmt.Printf("MARK %d %d\n", x0, y)
				move(x0, y)
				_ = roundtrip(d)
				time.Sleep(45 * time.Millisecond)
			}
		}
		time.Sleep(60 * time.Millisecond)
	case "axis":
		// Vertical wheel tick at X Y: move first, since a fresh
		// virtual device starts without pointer focus. dy is in
		// wayland axis units, positive down (wl_pointer.axis).
		if len(rest) != 3 {
			die("axis needs X Y DY")
		}
		dy, err := strconv.ParseFloat(rest[2], 32)
		if err != nil {
			die("bad delta %q: %v", rest[2], err)
		}
		move(atoi(rest[0]), atoi(rest[1]))
		settle()
		if err := ctx.SendRequest(vp, 3, t(), uint32(0), float32(dy)); err != nil {
			die("axis: %v", err)
		}
		_ = ctx.SendRequest(vp, 4)
		_ = roundtrip(d)
		time.Sleep(60 * time.Millisecond)
	case "dwell":
		// Rest the pointer on X Y for MS milliseconds, keeping the
		// virtual device alive the whole time: dwell-dependent behavior
		// (tooltip delay) needs the pointer to stay put with focus.
		if len(rest) != 3 {
			die("dwell needs X Y MS")
		}
		ms, err := strconv.Atoi(rest[2])
		if err != nil {
			die("bad duration %q: %v", rest[2], err)
		}
		move(atoi(rest[0]), atoi(rest[1]))
		settle()
		time.Sleep(time.Duration(ms) * time.Millisecond)
	default:
		die("unknown command %q", cmd)
	}

	_ = ctx.SendRequest(vp, 8)
	_ = ctx.SendRequest(mgr, 1)
}
