package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
)

func newTestNavMenu(t *testing.T) (*NavigationMenu, *fakeSurfaceHost, *int) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	docs := 0
	m := NewNavigationMenu(face,
		NavigationMenuItem{Label: "Getting started", Content: NewNavigationLinks(face, 2,
			NavigationLink{Title: "Introduction", Description: "Re-usable components built using Radix UI and Tailwind CSS."},
			NavigationLink{Title: "Installation", Description: "How to install dependencies and structure your app."},
		)},
		NavigationMenuItem{Label: "Components", Content: NewNavigationLinks(face, 1,
			NavigationLink{Title: "Alert Dialog", Description: "A modal dialog that interrupts the user."},
		)},
		NavigationMenuItem{Label: "Docs", OnActivate: func() { docs++ }},
	)
	host := &fakeSurfaceHost{}
	SetSurfaceHost(m, host)
	arrangeTree(t, m, 600, 40)
	return m, host, &docs
}

// Clicking triggers swaps their panels in one viewport and moves the
// caret; a link runs and closes; clicking the open trigger closes.
func TestNavigationMenuClicks(t *testing.T) {
	m, host, docs := newTestNavMenu(t)
	m.triggers[0].OnClick()
	if m.Active() != 0 || len(host.popovers) != 1 || !host.opts[0].NoGrab {
		t.Fatalf("active %d with %d popovers", m.Active(), len(host.popovers))
	}
	m.triggers[1].OnClick()
	if m.Active() != 1 || len(host.popovers) != 1 || m.viewport.Children()[0] != m.items[1].Content {
		t.Fatalf("the second trigger opened another popover (%d) or kept the first panel", len(host.popovers))
	}
	b := m.triggers[1].Bounds()
	if m.at[0] != float64(b.X+b.W/2) || m.at[1] != 1 {
		t.Errorf("caret at %v, want under the second trigger", m.at)
	}
	m.triggers[2].OnClick()
	if m.Active() != -1 || *docs != 1 || host.closed != 1 {
		t.Errorf("the link: active %d, docs %d, closes %d", m.Active(), *docs, host.closed)
	}
}

// Resting the pointer on a trigger opens it; leaving the menu and the
// panel closes it.
func TestNavigationMenuHover(t *testing.T) {
	m, host, _ := newTestNavMenu(t)
	r := &Router{Root: m}
	b := m.triggers[1].Bounds()
	r.Move(Point{X: b.X + 2, Y: b.Y + 2})
	if m.Active() != 1 {
		t.Fatalf("hovering the trigger opened %d", m.Active())
	}
	r.Leave()
	if m.Active() != -1 || host.closed != 1 {
		t.Error("leaving the menu kept the panel open")
	}
}

// Enter on a focused trigger opens its panel with the keyboard in it;
// Right steps to the next trigger; Escape closes.
func TestNavigationMenuKeyboard(t *testing.T) {
	m, host, _ := newTestNavMenu(t)
	r := &Router{Root: m}
	r.SetFocus(m.triggers[0])
	r.KeyAction(KeyRight, 0)
	if r.Focused() != Widget(m.triggers[1]) {
		t.Fatalf("Right focused %T", r.Focused())
	}
	r.KeyAction(KeyEnter, 0)
	if m.Active() != 1 || host.opts[0].NoGrab || host.opts[0].Focus == nil {
		t.Fatalf("Enter: active %d, opts %+v", m.Active(), host.opts)
	}
	r.KeyAction(KeyDismiss, 0)
	if m.Active() != -1 {
		t.Error("Escape left the panel open")
	}
}

// TestGoldenNavigationMenu pins the menu with its first panel
// open and the panel itself, in both palettes.
func TestGoldenNavigationMenu(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			m, host, _ := newTestNavMenu(t)
			m.triggers[0].OnClick()
			col := NewBox(Column, 8, 0).Append(m, false).Append(host.popovers[0], false)
			NewGolden(t, col, "navmenu-"+tc.name, goldenTheme(tc.th))
		})
	}
}
