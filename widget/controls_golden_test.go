package widget

import (
	"fmt"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The control set in both palettes: every button variant at
// rest, hovered, pressed and disabled, the size steps, and the inputs
// and toggles at rest and focused (#193).
func TestGoldenControls(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			button := func(label string, classes ...string) *Button {
				b := NewButton(NewLabel(face, 14, label, 0), 6, 4)
				b.AddClass(classes...)
				return b
			}
			grid := NewBox(Column, 8, 0)
			for _, variant := range [][]string{nil, {css.SuggestedAction}, {css.DestructiveAction}, {css.Outline}, {css.Ghost}, {css.Link}} {
				row := NewBox(Row, 8, 0)
				rest, hover, pressed, disabled := button("Button", variant...), button("Hover", variant...), button("Pressed", variant...), button("Disabled", variant...)
				hover.SetHovered(true)
				pressed.Pressed = true
				disabled.SetEnabled(false)
				row.Append(rest, false).Append(hover, false).Append(pressed, false).Append(disabled, false)
				grid.Append(row, false)
			}
			sizes := NewBox(Row, 8, 0)
			sizes.Append(button("Small", css.Small), false).Append(button("Default"), false).Append(button("Large", css.Large), false)
			grid.Append(sizes, false)
			NewGolden(t, grid, "buttons-"+tc.name, goldenTheme(tc.th))

			entry := NewEntry(face, 14, tc.th.Text)
			entry.SetPlaceholder("Email")
			focused := NewEntry(face, 14, tc.th.Text)
			focused.SetText("name@example.com")
			inputs := NewBox(Column, 8, 0)
			inputs.Append(entry, false).Append(focused, false).
				Append(NewDropdown(face, 14, []string{"Apple", "Banana"}, 0), false)
			toggles := NewBox(Row, 12, 0)
			toggles.Append(NewCheckButton(false), false).Append(NewCheckButton(true), false).
				Append(NewSwitch(false), false).Append(NewSwitch(true), false)
			inputs.Append(toggles, false).Append(NewSlider(0, 100, 1, 40), false).Append(NewProgressBar(0.6), false)
			h := inputs.Measure(Constraints{Max: Size{W: 260, H: 4096}}).H
			NewGolden(t, inputs, "inputs-"+tc.name, goldenTheme(tc.th), goldenFrame(260, h), goldenFocus(focused))
		})
	}
}

// The embedded UI faces render the same on every machine: Geist at
// regular, medium and semibold, and Geist Mono (#194).
func TestGoldenUIFont(t *testing.T) {
	ui, err := sysfont.UI()
	if err != nil {
		t.Fatal(err)
	}
	mono, err := sysfont.UIMono()
	if err != nil {
		t.Fatal(err)
	}
	col := NewBox(Column, 4, 0)
	for _, w := range []int{400, 500, 600} {
		face, err := sysfont.Weighted(ui.Family(), 15, w, false)
		if err != nil {
			t.Fatal(err)
		}
		col.Append(NewLabel(face, 15, fmt.Sprintf("Geist %d - The quick brown fox", w), 0), false)
	}
	col.Append(NewLabel(mono, 14, "Geist Mono: func main() {}", 0), false)
	NewGolden(t, col, "ui-font")
}

// .glass over a busy backdrop, in both palettes: the stripes under the
// card blur and saturate inside its rounded shape and stay sharp
// around it (#195).
func TestGoldenGlass(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			backdrop := NewBox(Column, 0, 0)
			backdrop.AddClass("stripes")
			card := NewBox(Column, 4, 16)
			card.AddClass(css.Glass)
			card.Append(NewLabel(face, 15, "Clear glass", 0), false).
				Append(NewLabel(face, 13, "blurred, saturated backdrop", 0), false)
			stage := NewOverlay().Append(backdrop).AppendAligned(card, AlignCenter, AlignCenter)
			cssShot(t, `.stripes { background-image: repeating-linear-gradient(45deg, #e05561 0px, #e05561 10px, #3fb68b 10px, #3fb68b 20px, #4f7cff 20px, #4f7cff 30px); }`,
				stage, "glass-"+tc.name, goldenTheme(tc.th), goldenFrame(260, 140))
		})
	}
}

// A .glass window root whose compositor blur is not in effect turns
// opaque: the card color instead of the translucent glass (#196).
func TestGlassBlurUnavailableIsOpaque(t *testing.T) {
	SetTheme(DarkTheme())
	defer SetTheme(nil)
	glass := NewBox(Column, 0, 0)
	glass.AddClass(css.Glass)
	if v := glass.style(glass); v.Background == Current().CardColor() {
		t.Fatal("plain .glass is already opaque; the test proves nothing")
	}
	glass.AddClass(css.BlurUnavailable)
	if v := glass.style(glass); v.Background != Current().CardColor() {
		t.Errorf("fallback background %08x, want the opaque card color %08x", uint32(v.Background), uint32(Current().CardColor()))
	}
	frosted := NewBox(Column, 0, 0)
	frosted.AddClass(css.Frosted, css.BlurUnavailable)
	if v := frosted.style(frosted); v.Background != Current().CardColor() {
		t.Errorf("frosted fallback background %08x, want the opaque card color", uint32(v.Background))
	}
}

// .liquid-glass over a busy backdrop, in both palettes: the stripes
// bend through the rim and run straight through the middle (#197).
func TestGoldenLiquidGlass(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			backdrop := NewBox(Column, 0, 0)
			backdrop.AddClass("stripes")
			pill := NewBox(Column, 4, 18)
			pill.AddClass(css.LiquidGlass)
			pill.Append(NewLabel(face, 15, "Liquid glass", 0), false).
				Append(NewLabel(face, 13, "the rim bends what is behind it", 0), false)
			stage := NewOverlay().Append(backdrop).AppendAligned(pill, AlignCenter, AlignCenter)
			cssShot(t, `.stripes { background-image: repeating-linear-gradient(90deg, #e05561 0px, #e05561 10px, #3fb68b 10px, #3fb68b 20px, #4f7cff 20px, #4f7cff 30px); }`,
				stage, "liquid-glass-"+tc.name, goldenTheme(tc.th), goldenFrame(300, 150))
		})
	}
}

// .frosted over a busy backdrop, in both palettes: a heavier blur, a
// whiter tint and the grain that makes it read as etched (#198).
func TestGoldenFrosted(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			backdrop := NewBox(Column, 0, 0)
			backdrop.AddClass("stripes")
			card := NewBox(Column, 4, 16)
			card.AddClass(css.Frosted)
			card.Append(NewLabel(face, 15, "Frosted glass", 0), false).
				Append(NewLabel(face, 13, "blur, tint and grain", 0), false)
			stage := NewOverlay().Append(backdrop).AppendAligned(card, AlignCenter, AlignCenter)
			cssShot(t, `.stripes { background-image: repeating-linear-gradient(45deg, #e05561 0px, #e05561 10px, #3fb68b 10px, #3fb68b 20px, #4f7cff 20px, #4f7cff 30px); }`,
				stage, "frosted-"+tc.name, goldenTheme(tc.th), goldenFrame(260, 140))
		})
	}
}

// A popover mid-enter (#200): grown out of its top edge - scaled, its
// rounded clip half unrolled, the shadow and plate drawn for the
// unrolled shape - at reveal 0.5.
func TestGoldenMorphMidEnter(t *testing.T) {
	face := goldenFace(t)
	box := NewBox(Column, 4, 10)
	box.Append(NewLabel(face, 13, "Open", 0), false).Append(NewLabel(face, 13, "Save as...", 0), false).Append(NewLabel(face, 13, "Quit", 0), false)
	m := NewMorph(box)
	m.SetOrigin(0.5, 0)
	m.SetRadius(8)
	m.SetPlate(12, func(cv *render.Canvas, c render.Rect) {
		Current().DrawShadow(cv, c, 8)
		cv.RoundedRect(c, 8, Current().PopoverColor())
	})
	m.SetReveal(0.5, true)
	NewGolden(t, m, "morph-mid-enter", goldenTheme(DarkTheme()))
}

// The display pieces in both palettes (#204): the badge
// variants (one with an icon), a key combination, skeleton blocks, and
// the typography classes.
func TestGoldenDisplay(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			col := NewBox(Column, 10, 0)
			badges := NewBox(Row, 6, 0)
			for _, v := range []BadgeVariant{BadgeDefault, BadgeSecondary, BadgeDestructive, BadgeOutline} {
				b := NewBadge(face, "Badge")
				b.SetVariant(v)
				badges.AppendAligned(b, false, AlignCenter)
			}
			withIcon := NewBadge(face, "Remove")
			withIcon.SetIcon(NewSymbol(SymbolClose, 10))
			withIcon.SetVariant(BadgeSecondary)
			badges.AppendAligned(withIcon, false, AlignCenter)
			col.Append(badges, false)
			col.Append(NewKbdGroup(face, "Ctrl", "Shift", "P"), false)
			skel := NewBox(Row, 10, 0)
			skel.AppendAligned(NewSkeleton(36, 36), false, AlignCenter)
			lines := NewBox(Column, 6, 0)
			lines.Append(NewSkeleton(160, 12), false).Append(NewSkeleton(120, 12), false)
			skel.Append(lines, false)
			col.Append(skel, false)
			for _, c := range []string{css.H1, css.H2, css.H3, css.H4, css.Lead, css.LargeText, css.SmallText, css.Muted, css.Blockquote, css.InlineCode} {
				l := NewLabel(face, 14, "The "+c+" style", 0)
				l.AddClass(c)
				col.Append(l, false)
			}
			col.Append(NewBulletList(face, 14, "first item", "second item"), false)
			NewGolden(t, col, "display-"+tc.name, goldenTheme(tc.th))
		})
	}
}

// TestGoldenAlertEmpty pins the Alert variants - with an icon,
// destructive, title only - and the status page as an empty state, in
// both palettes.
func TestGoldenAlertEmpty(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			col := NewBox(Column, 12, 0)
			info := NewAlert(face, "Heads up!", "You can add components to your app using the cli.")
			info.SetIcon(NewSymbol(SymbolCrosshair, 16))
			col.Append(info, false)
			bad := NewAlert(face, "Unable to process your payment.", "Please verify your billing information and try again.")
			bad.SetIcon(NewSymbol(SymbolClose, 16))
			bad.SetVariant(AlertDestructive)
			col.Append(bad, false)
			col.Append(NewAlert(face, "Your session has expired.", ""), false)
			empty := NewStatusPage(face, "", "No projects yet", "You haven't created any projects yet. Get started by creating your first project.")
			empty.SetIcon(NewSymbol(SymbolMaximize, 24))
			create := NewButton(NewLabel(face, 14, "Create project", 0), 0, 0)
			create.AddClass(css.SuggestedAction)
			empty.SetChild(create)
			col.Append(empty, true)
			NewGolden(t, col, "alert-empty-"+tc.name, goldenTheme(tc.th), goldenFrame(420, 520))
		})
	}
}

// Glass and frosted cards inside a root whose blur is not in effect
// turn opaque too, not only a glass root itself (#331).
func TestGlassInsideABlurUnavailableRootIsOpaque(t *testing.T) {
	SetTheme(DarkTheme())
	defer SetTheme(nil)
	root := NewBox(Column, 0, 0)
	glass, frosted := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	glass.AddClass(css.Glass)
	frosted.AddClass(css.Frosted)
	root.Append(glass, false)
	root.Append(frosted, false)
	arrangeTree(t, root, 100, 100)
	if v := glass.style(glass); v.Background == Current().CardColor() {
		t.Fatal("plain .glass is already opaque; the test proves nothing")
	}
	root.AddClass(css.BlurUnavailable)
	arrangeTree(t, root, 100, 100)
	for _, w := range []*Box{glass, frosted} {
		if v := w.style(w); v.Background != Current().CardColor() {
			t.Errorf("%v under a blur-unavailable root: background %08x, want the opaque card color", w.Classes(), uint32(v.Background))
		}
	}
}
