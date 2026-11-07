package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/appearance"
	"github.com/stubbedev/lys-ui/gtksettings"
)

func TestOnLoopDeliversOnPumpAndDropsAfterOff(t *testing.T) {
	a := testApp(nil)
	var emit func(appearance.ColorScheme)
	unregistered := 0
	register := func(fn func(appearance.ColorScheme)) func() {
		emit = fn
		return func() { unregistered++ }
	}
	var got []appearance.ColorScheme
	off := onLoop(a, register, func(s appearance.ColorScheme) { got = append(got, s) })

	done := make(chan struct{})
	go func() {
		emit(appearance.Dark)
		emit(appearance.Light)
		close(done)
	}()
	<-done
	if len(got) != 0 {
		t.Fatalf("delivered %v before the loop pumped", got)
	}
	a.pump(time.Now())
	if len(got) != 2 || got[0] != appearance.Dark || got[1] != appearance.Light {
		t.Fatalf("delivered %v, want [dark light] in order", got)
	}

	emit(appearance.Dark)
	off()
	off()
	a.pump(time.Now())
	if len(got) != 2 {
		t.Errorf("a change queued before off was delivered: %v", got)
	}
	if unregistered != 1 {
		t.Errorf("unregistered %d times, want 1", unregistered)
	}
}

func TestAccentColor(t *testing.T) {
	if _, ok := (appearance.Accent{}).Color(); ok {
		t.Error("an unknown accent reported a color")
	}
	c, ok := appearance.Accent{R: 1, G: 0.5, B: 0, Known: true}.Color()
	if !ok || c.R() != 255 || c.G() != 128 || c.B() != 0 || c.A() != 255 {
		t.Errorf("accent color = %v, want opaque rgb(255,128,0)", c)
	}
}

func TestReducedMotionComesFromThePortalThenSettingsIni(t *testing.T) {
	for _, c := range []struct {
		portal appearance.Motion
		ini    gtksettings.Toggle
		want   bool
	}{
		{appearance.MotionUnknown, gtksettings.ToggleUnset, false},
		{appearance.MotionUnknown, gtksettings.ToggleOn, false},
		{appearance.MotionUnknown, gtksettings.ToggleOff, true},
		{appearance.MotionReduced, gtksettings.ToggleUnset, true},
		{appearance.MotionReduced, gtksettings.ToggleOn, true},
	} {
		if got := resolveReducedMotion(c.portal, c.ini); got != c.want {
			t.Errorf("portal %v, ini %v: %v", c.portal, c.ini, got)
		}
	}
}
