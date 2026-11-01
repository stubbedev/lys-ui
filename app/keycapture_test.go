package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

func TestCaptureKey(t *testing.T) {
	tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{28: xkb.KeyReturn, 30: xkb.Keysym('A')}}

	t.Run("a consumed press reports true with the parsed accel", func(t *testing.T) {
		want, err := ParseAccel("Control+Return")
		if err != nil {
			t.Fatal(err)
		}
		var seen Accel
		consumed := captureKey(tr, func(a Accel) bool { seen = a; return true }, 28, wlsession.ModCtrl)
		if !consumed {
			t.Fatal("the hook consumed the press, captureKey must say so")
		}
		if seen != want {
			t.Errorf("hook saw %+v, want %+v (ParseAccel of the binding)", seen, want)
		}
	})

	t.Run("a declined press is not consumed", func(t *testing.T) {
		if captureKey(tr, func(Accel) bool { return false }, 28, 0) {
			t.Error("a hook returning false must leave the press to routing")
		}
	})

	t.Run("no hook consumes nothing", func(t *testing.T) {
		if captureKey(tr, nil, 28, 0) {
			t.Error("a window without KeyCapture must route every press")
		}
	})

	t.Run("letters fold to lowercase and caps lock is not a modifier", func(t *testing.T) {
		want, err := ParseAccel("Shift+a")
		if err != nil {
			t.Fatal(err)
		}
		var seen Accel
		captureKey(tr, func(a Accel) bool { seen = a; return true }, 30, wlsession.ModShift|wlsession.ModCapsLock)
		if seen != want {
			t.Errorf("hook saw %+v, want %+v", seen, want)
		}
	})
}
