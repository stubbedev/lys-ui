package app

import (
	"errors"
	"testing"
)

func TestASurfaceFadeScalesTheAppsOpacity(t *testing.T) {
	var o surfaceOpacity
	var sent []float64
	send := func(o surfaceOpacity) error {
		v, _ := o.value()
		sent = append(sent, v)
		return nil
	}
	o.setApp(0.8)
	for _, a := range []float64{0, 0.5, 1} {
		if !fadeOpacity(&o, a, send) {
			t.Fatalf("the fade to %v was refused", a)
		}
	}
	want := []float64{0, 0.4, 0.8}
	for i, v := range want {
		if sent[i] != v {
			t.Errorf("fade %d sent %v, want %v", i, sent[i], v)
		}
	}
	o.setApp(0.5)
	if v, _ := o.value(); v != 0.5 {
		t.Errorf("a landed fade left the app's opacity at %v", v)
	}
}

func TestARefusedSurfaceFadeIsNotKept(t *testing.T) {
	o := surfaceOpacity{}
	if fadeOpacity(&o, 0.3, func(surfaceOpacity) error { return errors.New("no alpha modifier") }) {
		t.Fatal("a fade the compositor refused was taken")
	}
	if v, set := o.value(); set || v != 1 {
		t.Errorf("a refused fade left opacity %v (set %v)", v, set)
	}
}
