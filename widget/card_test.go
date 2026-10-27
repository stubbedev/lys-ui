package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// newProjectCard is the card the gallery shows: a title, a
// description, a field for content and two footer actions.
func newProjectCard(t *testing.T) (*Card, *Button) {
	t.Helper()
	face := goldenFace(t)
	c := NewCard(face, "Create project")
	c.SetDescription("Deploy your new project in one click.")
	deploy := NewButton(NewLabel(face, 14, "Deploy", 0), 0, 0)
	deploy.AddClass(css.SuggestedAction)
	cancel := NewButton(NewLabel(face, 14, "Cancel", 0), 0, 0)
	cancel.AddClass(css.Outline)
	c.AppendFooter(cancel)
	c.AppendFooter(deploy)
	entry := NewEntry(face, 14, 0)
	entry.SetPlaceholder("Name of your project")
	c.SetContent(entry)
	return c, deploy
}

// The sections keep their order whatever order they are set in, the
// padding comes from the theme, and the footer's actions sit at the
// right edge.
func TestCardLayout(t *testing.T) {
	SetTheme(DarkTheme().WithPadding(4))
	defer SetTheme(nil)
	c, deploy := newProjectCard(t)
	arrangeTree(t, NewBox(Column, 0, 0).Append(c, false), 400, 400)
	kids := c.column.Children()
	if len(kids) != 3 || kids[0] != Widget(c.header) || kids[1] != Widget(c.contentSlot) || kids[2] != Widget(c.footer) {
		t.Fatalf("sections %v, want header, content, footer", kids)
	}
	border := CascadeBorder(c)
	pad := 3 * Current().Padding
	if got, want := c.title.Bounds().X-c.Bounds().X, border.Left+pad; got != want {
		t.Errorf("title inset %d, want the border and three theme paddings, %d", got, want)
	}
	if got, want := deploy.Bounds().X+deploy.Bounds().W, c.Bounds().X+c.Bounds().W-border.Right-pad; got != want {
		t.Errorf("last action ends at %d, want the content edge %d", got, want)
	}
}

// Empty parts leave the tree: no description, action, content or
// footer is laid out until set, and nil or clearing takes them out.
func TestCardOptionalParts(t *testing.T) {
	face := goldenFace(t)
	c := NewCard(face, "Notifications")
	if len(c.titles.Children()) != 1 || len(c.column.Children()) != 1 {
		t.Fatal("a bare card lays out more than its title")
	}
	more := NewButton(NewLabel(face, 14, "More", 0), 0, 0)
	c.SetAction(more)
	c.SetContent(NewLabel(face, 14, "You have 3 unread messages.", 0))
	if c.Action() != Widget(more) || len(c.header.Children()) != 2 || len(c.column.Children()) != 2 {
		t.Fatalf("action %v, header %d, sections %d", c.Action(), len(c.header.Children()), len(c.column.Children()))
	}
	c.SetAction(nil)
	c.SetContent(nil)
	c.SetDescription("")
	c.ClearFooter()
	if c.Action() != nil || c.Content() != nil || len(c.header.Children()) != 1 || len(c.column.Children()) != 1 {
		t.Errorf("cleared card keeps action %v, content %v, %d header parts, %d sections",
			c.Action(), c.Content(), len(c.header.Children()), len(c.column.Children()))
	}
	c.SetClasses("settings-card")
	arrangeTree(t, c, 300, 200)
	if CascadeBorder(c).Left != 1 {
		t.Error("the card lost its surface to a class list of its user's")
	}
	if Describe(c).Name != "Notifications" {
		t.Errorf("announced as %q, want its title", Describe(c).Name)
	}
}

// TestGoldenCard pins the project card in both palettes.
func TestGoldenCard(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			c, _ := newProjectCard(t)
			c.SetSizeRequest(360, 0)
			frame := NewBox(Column, 0, 20)
			frame.AppendAligned(c, false, AlignStart)
			NewGolden(t, frame, "card-"+tc.name, goldenTheme(tc.th), goldenFrame(400, 280))
		})
	}
}

// The title is the theme's TitleSize, tracking its TextSize.
func TestCardTitleIsTheThemeTitleSize(t *testing.T) {
	SetTheme(DarkTheme().WithTextSize(16))
	defer SetTheme(nil)
	c := NewCard(goldenFace(t), "Settings")
	if got, want := c.title.SizePx(), Current().TitleSize(); got != want || want != 18 {
		t.Errorf("title at %v, want the theme's title size %v (18 over 16)", got, want)
	}
}

// The header's action keeps its natural width at the header's end and
// the titles wrap in what is left: a wrapping description's natural
// width is its whole paragraph, which pushed the action past the
// card's edge or squeezed it to an ellipsis.
func TestCardActionKeepsItsWidthBesideWrappingText(t *testing.T) {
	face := goldenFace(t)
	for _, ellipsize := range []bool{false, true} {
		c := NewCard(face, "Screen sharing")
		c.SetDescription("An application wants to share your screen. Choose what it can see and whether it may keep the selection.")
		caption := NewLabel(face, 14, "An application", 0)
		if ellipsize {
			caption.SetEllipsize(EllipsizeEnd)
		}
		action := NewBox(Row, 6, 0)
		action.Append(NewSpacer(16, 16), false)
		action.Append(caption, false)
		c.SetAction(action)
		c.SetSizeRequest(420, 0)
		c.SetMaximumSize(420, 0)
		arrangeTree(t, NewBox(Column, 0, 0).Append(c, false), 800, 600)
		want := SizeWithin(caption, Size{W: 800, H: 100}).W
		cb, ab, lb := c.Bounds(), action.Bounds(), caption.Bounds()
		if lb.W < want {
			t.Errorf("ellipsize %v: caption %d wide, want its natural %d", ellipsize, lb.W, want)
		}
		if ab.X+ab.W > cb.X+cb.W-CascadeBorder(c).Right-3*Current().Padding {
			t.Errorf("ellipsize %v: action ends at %d, past the card's content edge (card %v)", ellipsize, ab.X+ab.W, cb)
		}
		if tb := c.titles.Bounds(); ab.X-(tb.X+tb.W) != 2*Current().Spacing {
			t.Errorf("ellipsize %v: titles %v and action %v, want the theme's header gap between", ellipsize, tb, ab)
		}
		if c.desc.Bounds().H <= c.title.Bounds().H {
			t.Errorf("ellipsize %v: the description did not wrap beside the action", ellipsize)
		}
	}
}
