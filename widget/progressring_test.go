package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The value clamps, the sweep follows it, and assistive technology
// reads it as a progress bar.
func TestProgressRingValue(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	r := NewProgressRing(1.5)
	if r.Value() != 1 {
		t.Fatalf("value %v, want it clamped to 1", r.Value())
	}
	r.SetValue(0.4)
	if r.Value() != 0.4 || r.shown != 0.4 {
		t.Errorf("value %v shown %v, want 0.4 both", r.Value(), r.shown)
	}
	st := Describe(r)
	if st.Role != RoleProgressBar || st.Max != 1 || st.Value != 0.4 {
		t.Errorf("described as %+v, want a progress bar at 0.4 of 1", st)
	}
}

// The ring's min sizes set the diameter, and the child centers in it.
func TestProgressRingSizesFromCSS(t *testing.T) {
	r := NewProgressRing(0.5)
	if sz := SizeWithin(r, Size{W: 200, H: 200}); sz != (Size{W: progressRingSize, H: progressRingSize}) {
		t.Errorf("unstyled ring measures %v, want %d square", sz, progressRingSize)
	}
	loadCSS(t, `progressring { min-width: 48px; }`)
	label := NewLabel(goldenFace(t), 12, "50%", 0)
	r.SetChild(label)
	arrangeTree(t, NewBox(Column, 0, 0).Append(r, false), 200, 200)
	if b := r.Bounds(); b.H != 48 {
		t.Errorf("styled ring is %d tall, want 48", b.H)
	}
	rb, lb := r.trough.bounds, label.Bounds()
	if rb.W != 48 || lb.X+lb.W/2 != rb.X+rb.W/2 || lb.Y+lb.H/2 != rb.Y+rb.H/2 {
		t.Errorf("ring square %v, label %v; want a 48 square with the label centered", rb, lb)
	}
}

// TestGoldenProgressRing pins the ring empty, partway with a centered
// label, full, and in the status variants, in both palettes.
func TestGoldenProgressRing(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			row := NewBox(Row, 16, 0)
			row.Append(NewProgressRing(0), false)
			labelled := NewProgressRing(0.66)
			labelled.SetChild(NewLabel(face, 11, "66%", 0))
			row.Append(labelled, false)
			row.Append(NewProgressRing(1), false)
			for _, class := range []string{css.Success, css.Warning, css.Error} {
				ring := NewProgressRing(0.4)
				ring.AddClass(class)
				row.Append(ring, false)
			}
			NewGolden(t, row, "progressring-"+tc.name, goldenTheme(tc.th), goldenFrame(300, 60))
		})
	}
}
