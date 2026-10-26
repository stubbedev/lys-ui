package headlesstest

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/stubbedev/lys-lib/vinput"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// usKeymap is a flattened US xkb keymap (see testdata/us.xkb), the
// keymap uploaded to the virtual keyboard when the seat has none of
// its own - the protocol refuses key events before a keymap is set.
//
//go:embed testdata/us.xkb
var usKeymap string

// Pointer button codes (linux/input-event-codes.h), the values
// wl_pointer.button carries.
const (
	BTNLeft  = 0x110
	BTNRight = 0x111
)

// evdev key codes the suite drives (linux/input-event-codes.h). The
// compositor maps them through the seat's keymap when it has one; the
// client under test decodes keycodes with its built-in US fallback
// regardless, so codes behave as they would from a physical US
// keyboard.
const (
	KeyEscape = 1
	KeyTab    = 15
	KeyEnter  = 28
	KeyA      = 30
	KeyD      = 32
	KeyC      = 46
	KeyV      = 47
	KeySpace  = 57
	KeyHome   = 102
	KeyLeft   = 105
	KeyRight  = 106
	KeyEnd    = 107
	KeyDown   = 108
	KeyUp     = 103
)

// settleTimes mirror wlpointer's pacing: after each action a roundtrip
// plus a short sleep lets the compositor deliver the event and the
// client react before the next step (assertions still wait on traces,
// so the pacing only orders actions, it does not carry correctness).
const (
	settleDelay = 80 * time.Millisecond
	pressDelay  = 60 * time.Millisecond
)

// VirtualInput is a connected synthetic-seat client over a
// vinput.Device. One instance per test keeps focus, grab and keymap
// state from bleeding between tests.
type VirtualInput struct {
	dev *vinput.Device
}

// Dial connects to the compositor socket named by WAYLAND_DISPLAY in
// the process's XDG_RUNTIME_DIR, binds the seat and the virtual-input
// managers, and creates a pointer and a keyboard. If the seat offers a
// keymap it is relayed to the virtual keyboard; when it does not (a
// virtual-only seat may never have compiled one), keys are injected
// raw and modifiers are pushed explicitly through
// zwp_virtual_keyboard_v1.modifiers - the client under test decodes
// keycodes with its own US fallback either way.
func Dial(display string) (*VirtualInput, error) {
	return dial(display, true)
}

// DialPointerOnly connects with a virtual pointer and no keyboard: the
// shape of a seat whose input devices are pointers only. Widgets that
// open surfaces on hover (tooltips, popups) must survive and behave
// there exactly as with a keyboard present.
func DialPointerOnly(display string) (*VirtualInput, error) {
	return dial(display, false)
}

func dial(display string, keyboard bool) (*VirtualInput, error) {
	dev, err := vinput.Dial(display, vinput.Options{Keyboard: keyboard, FallbackKeymap: []byte(usKeymap)})
	if err != nil {
		return nil, err
	}
	return &VirtualInput{dev: dev}, nil
}

func (v *VirtualInput) roundtrip() error { return v.dev.Roundtrip() }

func (v *VirtualInput) settle() {
	_ = v.roundtrip()
	time.Sleep(settleDelay)
}

// MoveTo moves the pointer to compositor coordinates (x, y).
func (v *VirtualInput) MoveTo(x, y int) error {
	w, h := v.dev.OutputSize()
	if x < 0 || y < 0 || x > int(w) || y > int(h) {
		return fmt.Errorf("move to (%d,%d): outside the %dx%d output", x, y, w, h)
	}
	if err := v.dev.MotionAbsolute(intconv.Uint32(x), intconv.Uint32(y), intconv.Uint32(w), intconv.Uint32(h)); err != nil {
		return err
	}
	v.settle()
	return nil
}

// press presses or releases button and waits for the compositor.
func (v *VirtualInput) press(button uint32, pressed bool) error {
	if err := v.dev.Button(button, pressed); err != nil {
		return err
	}
	return v.roundtrip()
}

// MoveOnto moves the pointer onto (x, y) so the compositor always
// sees motion arriving there, from a pixel off: a window can map under
// a pointer that already rests on the spot (the client traces its
// layout before its first frame lands), or a retry can aim at the
// spot the last attempt left the pointer on - and a compositor that
// sends no enter for a surface appearing under a resting cursor
// (Hyprland) then leaves the press with no surface at all.
func (v *VirtualInput) MoveOnto(x, y int) error {
	if err := v.MoveTo(x+1, y); err != nil {
		return err
	}
	return v.MoveTo(x, y)
}

// ClickAt presses and releases button at (x, y), arriving there with
// motion (MoveOnto).
func (v *VirtualInput) ClickAt(x, y int, button uint32) error {
	if err := v.MoveOnto(x, y); err != nil {
		return err
	}
	if err := v.press(button, true); err != nil {
		return err
	}
	time.Sleep(pressDelay)
	if err := v.press(button, false); err != nil {
		return err
	}
	time.Sleep(settleDelay)
	return nil
}

// DoubleClickAt clicks button twice at (x, y) as one burst: one move
// (MoveOnto), then both press/release pairs back to back with a single
// roundtrip, so the pair stays inside any double-click interval however
// slow the compositor's roundtrips are.
func (v *VirtualInput) DoubleClickAt(x, y int, button uint32) error {
	if err := v.MoveOnto(x, y); err != nil {
		return err
	}
	for range 2 {
		if err := v.dev.Button(button, true); err != nil {
			return err
		}
		if err := v.dev.Button(button, false); err != nil {
			return err
		}
	}
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(settleDelay)
	return nil
}

// DragTo presses at (x0, y0), moves in steps to (x1, y1), and
// releases there: the gesture drag-driven widgets need (a plain click
// neither moves a slider nor drags a scrollbar).
func (v *VirtualInput) DragTo(x0, y0, x1, y1 int, steps int) error {
	if err := v.MoveTo(x0, y0); err != nil {
		return err
	}
	if err := v.press(BTNLeft, true); err != nil {
		return err
	}
	time.Sleep(pressDelay)
	steps = max(steps, 1)
	for i := 1; i <= steps; i++ {
		if err := v.MoveTo(x0+(x1-x0)*i/steps, y0+(y1-y0)*i/steps); err != nil {
			return err
		}
	}
	if err := v.press(BTNLeft, false); err != nil {
		return err
	}
	time.Sleep(settleDelay)
	return nil
}

// ScrollAt sends a wheel tick at (x, y); dy is in wayland axis units,
// positive down.
func (v *VirtualInput) ScrollAt(x, y int, dy float64) error {
	if err := v.MoveTo(x, y); err != nil {
		return err
	}
	if err := v.dev.Axis(vinput.SourceWheel, vinput.AxisVertical, dy); err != nil {
		return err
	}
	v.settle()
	return nil
}

// FingerScrollAt scrolls by dy at (x, y) as a touchpad does: finger
// frames carrying dy, then the frame that lifts the fingers.
func (v *VirtualInput) FingerScrollAt(x, y int, dy float64) error {
	if err := v.MoveTo(x, y); err != nil {
		return err
	}
	if err := v.dev.Axis(vinput.SourceFinger, vinput.AxisVertical, dy); err != nil {
		return err
	}
	if err := v.dev.AxisStop(vinput.SourceFinger, vinput.AxisVertical); err != nil {
		return err
	}
	v.settle()
	return nil
}

// Tap presses and releases one key.
func (v *VirtualInput) Tap(code uint32) error {
	// Press and release go out together and settle with one roundtrip:
	// a roundtrip between them measures the compositor's speed, and on
	// a slow one (the software-rendered Hyprland VM, ~0.5-2s) that hold
	// outlasts the key-repeat delay the compositor gives a virtual
	// keyboard - its configured repeat_delay applies to real keyboards
	// only - so a tap became a held, repeating key ("hi" typed "hhii").
	if err := v.dev.Key(code, true); err != nil {
		return err
	}
	if err := v.dev.Key(code, false); err != nil {
		return err
	}
	return v.roundtrip()
}

// TypeText types s: lowercase letters, digits and spaces. Anything
// else is a harness bug (a test asking for a key the table does not
// know), so it errors instead of guessing.
func (v *VirtualInput) TypeText(s string) error {
	for _, r := range s {
		code, shift, err := KeyFor(r)
		if err != nil {
			return err
		}
		if shift {
			if err := v.mods(ModShift); err != nil {
				return err
			}
		}
		if err := v.Tap(code); err != nil {
			return err
		}
		if shift {
			if err := v.mods(0); err != nil {
				return err
			}
		}
	}
	return v.roundtrip()
}

// xkb modifier masks, the values wl_keyboard.modifiers and
// zwp_virtual_keyboard_v1.modifiers exchange (Shift, Lock, Control,
// Alt from bit 0).
const (
	ModShift uint32 = 1 << 0
	ModCtrl  uint32 = 1 << 2
)

// mods pushes an explicit modifier state: on a seat without a keymap
// the compositor cannot derive modifiers from key presses, so the
// virtual keyboard protocol lets us declare them.
func (v *VirtualInput) mods(depressed uint32) error {
	if err := v.dev.Modifiers(depressed, 0, 0, 0); err != nil {
		return err
	}
	return v.roundtrip()
}

// Combo holds the ctrl modifier, taps key, releases it: ctrl+c and
// friends.
func (v *VirtualInput) Combo(code uint32) error {
	if err := v.mods(ModCtrl); err != nil {
		return err
	}
	if err := v.Tap(code); err != nil {
		return err
	}
	if err := v.mods(0); err != nil {
		return err
	}
	v.settle()
	return nil
}

// Close destroys the virtual devices and disconnects.
func (v *VirtualInput) Close() { v.dev.Close() }
