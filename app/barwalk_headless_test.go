package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	evdevEsc   = 1
	evdevEnter = 28
	evdevLeft  = 105
	evdevRight = 106
)

// A bar walked by keyboard takes the keys, rings its first control,
// moves along its edge with the arrows, presses the focused control
// with Enter, and on Esc clears the focus, gives the keyboard back and
// says the walk ended.
func TestHeadlessBarWalksItsControlsByKeyboard(t *testing.T) {
	wltest.Require(t)
	type shown struct {
		a       *app.Application
		w       *app.BarWindow
		buttons []*widget.Button
	}
	ready := make(chan shown, 1)
	pressed := make(chan int, 4)
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		bar := widget.NewBar(widget.BarTop)
		var buttons []*widget.Button
		for i := range 3 {
			b := widget.NewButton(widget.NewSpacer(24, 24), 2, 0)
			b.OnClick = func() { pressed <- i }
			bar.Center().Append(b, false)
			buttons = append(buttons, b)
		}
		w, err := a.NewBar(app.BarConfig{Output: out, Bar: bar, Layer: app.LayerTop, Namespace: "lys-ui-bar-walk-test"})
		ready <- shown{a, w, buttons}
		return err
	})
	s := <-ready
	onLoop := func(fn func()) {
		ran := make(chan struct{})
		s.a.Invoke(func() {
			fn()
			close(ran)
		})
		<-ran
	}
	ringed := func(want int) func() error {
		return func() error {
			var at []int
			onLoop(func() {
				for i, b := range s.buttons {
					if widget.FocusVisible(b) {
						at = append(at, i)
					}
				}
			})
			if want < 0 && len(at) == 0 || len(at) == 1 && at[0] == want {
				return nil
			}
			return fmt.Errorf("the ring is on %v, want %d", at, want)
		}
	}
	wltest.Eventually(t, "the bar laid out", func() error {
		var w int
		onLoop(func() { w, _ = s.w.Size() })
		if w == 0 {
			return errors.New("no size yet")
		}
		return nil
	})
	kb, err := headlesstest.Dial("")
	if err != nil {
		t.Fatal(err)
	}
	defer kb.Close()
	first, ended := make(chan struct{}, 1), make(chan struct{}, 1)
	for _, done := range []chan struct{}{first, ended} {
		onLoop(func() { err = s.w.WalkKeyboard(func() { done <- struct{}{} }) })
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-first:
	default:
		t.Fatal("a second walk left the first one's done unrun")
	}
	wltest.Eventually(t, "the first control ringed", ringed(0))
	onLoop(func() {
		if mode := s.w.KeyboardMode(); mode != app.KeyboardExclusive {
			t.Errorf("the walking bar holds keyboard mode %v, want exclusive", mode)
		}
	})
	tap := func(code uint32) {
		t.Helper()
		if err := kb.Tap(code); err != nil {
			t.Fatal(err)
		}
	}
	wltest.Eventually(t, "the bar holding the keyboard", func() error {
		var focus, host any
		var mode app.KeyboardMode
		onLoop(func() { focus, host, mode = s.a.Session().KeyboardFocus(), s.w.HostSurface(), s.w.KeyboardMode() })
		if focus != host {
			return fmt.Errorf("the compositor focuses %v, not the bar %v in mode %v", focus, host, mode)
		}
		return nil
	})
	tap(evdevRight)
	wltest.Eventually(t, "Right moving the ring to the second control", ringed(1))
	tap(evdevRight)
	wltest.Eventually(t, "Right moving the ring to the third control", ringed(2))
	tap(evdevLeft)
	wltest.Eventually(t, "Left moving it back", ringed(1))
	tap(evdevEnter)
	wltest.Eventually(t, "Enter pressing the second control", func() error {
		select {
		case i := <-pressed:
			if i != 1 {
				return fmt.Errorf("Enter pressed control %d, want 1", i)
			}
			return nil
		default:
			return errors.New("nothing pressed yet")
		}
	})
	tap(evdevEsc)
	wltest.Eventually(t, "Esc ending the walk", func() error {
		select {
		case <-ended:
			return nil
		default:
			return errors.New("the walk has not ended")
		}
	})
	if err := ringed(-1)(); err != nil {
		t.Error(err)
	}
	onLoop(func() {
		if s.w.Walking() || s.w.KeyboardMode() != app.KeyboardNone {
			t.Errorf("after Esc the bar walks %v in keyboard mode %v, want no walk and no keyboard", s.w.Walking(), s.w.KeyboardMode())
		}
	})
}
