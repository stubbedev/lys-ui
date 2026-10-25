package widget

import (
	"slices"
	"testing"
)

// Typing fills and advances, rejects what the charset does not take,
// completes once, and Backspace steps back.
func TestInputOTPTyping(t *testing.T) {
	o := NewInputOTP(goldenFace(t), OTPDigits, 3, 3)
	var changes, completes []string
	o.OnChanged = func(c string) { changes = append(changes, c) }
	o.OnComplete = func(c string) { completes = append(completes, c) }
	for _, r := range "12a3" {
		o.InsertRune(r)
	}
	if o.Code() != "123" || o.active() != 3 || len(changes) != 3 {
		t.Fatalf("code %q, active %d, %d changes; want 123 on slot 3 after three", o.Code(), o.active(), len(changes))
	}
	o.Insert("45-6789")
	if o.Code() != "123456" || !slices.Equal(completes, []string{"123456"}) {
		t.Fatalf("after the paste code %q, completes %v", o.Code(), completes)
	}
	o.KeyAction(KeyBackspace, 0)
	o.KeyAction(KeyBackspace, 0)
	if o.Code() != "1234" || o.active() != 4 {
		t.Errorf("after two backspaces code %q, active %d; want 1234 on slot 4", o.Code(), o.active())
	}
	o.KeyAction(KeyHome, 0)
	o.InsertRune('9')
	if o.Code() != "91234" {
		t.Errorf("typing at the start gave %q, want 91234", o.Code())
	}
	o.Insert("876")
	if o.Code() != "981234" || len(completes) != 2 {
		t.Errorf("code %q with %d completes, want 981234 and a second completion", o.Code(), len(completes))
	}
}

// An alphanumeric code uppercases letters; a full code overwrites the
// active slot.
func TestInputOTPAlphanumeric(t *testing.T) {
	o := NewInputOTP(goldenFace(t), OTPAlphanumeric, 4)
	o.Insert("ab1c")
	o.KeyAction(KeyLeft, 0)
	o.InsertRune('z')
	if o.Code() != "ABZC" {
		t.Errorf("code %q, want ABZC", o.Code())
	}
	o.KeyAction(KeyEnd, 0)
	o.InsertRune('q')
	if o.Code() != "ABZQ" {
		t.Errorf("code %q, want the last slot overwritten: ABZQ", o.Code())
	}
}

// The focus ring goes around the active slot, and moves with it.
func TestInputOTPRingFollowsTheSlot(t *testing.T) {
	o := NewInputOTP(goldenFace(t), OTPDigits, 4)
	arrangeTree(t, o, 300, 60)
	o.Insert("12")
	if FocusRingOwner(o) != Widget(o.slots[2]) {
		t.Fatal("the ring is not on the active slot")
	}
	o.KeyAction(KeyLeft, 0)
	if FocusRingRect(o) != FocusRingRect(o.slots[1]) {
		t.Error("the ring did not move with the caret")
	}
}

// TestGoldenInputOTP pins a focused, half-filled code of two
// groups in both palettes.
func TestGoldenInputOTP(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			o := NewInputOTP(face, OTPDigits, 3, 3)
			o.Insert("123")
			r := &Router{Root: o}
			arrangeTree(t, o, 300, 60)
			r.SetFocus(o)
			NewGolden(t, o, "inputotp-"+tc.name, goldenTheme(tc.th), goldenFocus(o))
		})
	}
}
