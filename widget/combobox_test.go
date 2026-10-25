package widget

import (
	"slices"
	"testing"
)

type framework int

const (
	next framework = iota
	svelte
	nuxt
)

func newTestCombobox(t *testing.T) (*Combobox[framework], *fakeSurfaceHost) {
	t.Helper()
	c := NewCombobox(goldenFace(t), "Select framework...",
		ComboboxOption[framework]{next, "Next.js"},
		ComboboxOption[framework]{svelte, "SvelteKit"},
		ComboboxOption[framework]{nuxt, "Nuxt.js"},
	)
	host := &fakeSurfaceHost{}
	SetSurfaceHost(c, host)
	arrangeTree(t, c, 300, 40)
	return c, host
}

// pickLabel opens the combobox and picks the option labeled label
// through its command.
func pickLabel(t *testing.T, c *Combobox[framework], host *fakeSurfaceHost, label string) {
	t.Helper()
	c.trigger.OnClick()
	cmd := host.popovers[len(host.popovers)-1].(*Box).Children()[0].(*Command)
	cmd.SetQuery(label)
	it, ok := cmd.Highlighted()
	if !ok || it.Label != label {
		t.Fatalf("searching %q highlights %q", label, it.Label)
	}
	it.OnSelect()
}

// A single-choice combobox shows its pick, closes, and clears when the
// selected option is picked again.
func TestComboboxSingle(t *testing.T) {
	c, host := newTestCombobox(t)
	var changes [][]framework
	c.OnChanged = func(s []framework) { changes = append(changes, s) }
	if c.text.Text() != "Select framework..." {
		t.Fatalf("trigger reads %q, want the placeholder", c.text.Text())
	}
	pickLabel(t, c, host, "SvelteKit")
	if !slices.Equal(c.Selected(), []framework{svelte}) || c.text.Text() != "SvelteKit" || host.closed != 1 {
		t.Fatalf("selected %v reading %q, %d closes", c.Selected(), c.text.Text(), host.closed)
	}
	pickLabel(t, c, host, "SvelteKit")
	if len(c.Selected()) != 0 || len(changes) != 2 {
		t.Errorf("picking the selected option left %v after %d changes", c.Selected(), len(changes))
	}
}

// A multiple-choice combobox toggles picks, keeps option order, and
// stays open.
func TestComboboxMultiple(t *testing.T) {
	c, host := newTestCombobox(t)
	c.SetMultiple(true)
	pickLabel(t, c, host, "Nuxt.js")
	pickLabel(t, c, host, "Next.js")
	if !slices.Equal(c.Selected(), []framework{next, nuxt}) || c.text.Text() != "Next.js, Nuxt.js" || host.closed != 0 {
		t.Errorf("selected %v reading %q, %d closes", c.Selected(), c.text.Text(), host.closed)
	}
}

func TestComboboxRejectsUnknownValues(t *testing.T) {
	c, _ := newTestCombobox(t)
	defer func() {
		if recover() == nil {
			t.Error("an unknown value did not panic")
		}
	}()
	c.SetSelected(framework(9))
}

// TestGoldenCombobox pins the trigger empty and with a pick, and
// the open list with its check, in both palettes.
func TestGoldenCombobox(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			empty, _ := newTestCombobox(t)
			picked, host := newTestCombobox(t)
			picked.SetSelected(svelte)
			picked.trigger.OnClick()
			col := NewBox(Column, 12, 0)
			col.Append(empty, false).Append(picked, false).Append(host.popovers[0], false)
			NewGolden(t, col, "combobox-"+tc.name, goldenTheme(tc.th), goldenFrame(220, 260))
		})
	}
}

// An option's check trails its row, after the label, and the trigger
// reads its placeholder in the button's own color: the check sat in a
// leading slot that indented every option, and the placeholder was
// muted (#281).
func TestComboboxCheckTrailsTheOption(t *testing.T) {
	c, host := newTestCombobox(t)
	c.trigger.OnClick()
	content := host.popovers[len(host.popovers)-1]
	SetStyleAnchor(content, c)
	arrangeTree(t, content, 300, 300)
	cmd := content.(*Box).Children()[0].(*Command)
	row := cmd.rows[0].Children()
	if row[len(row)-1] != Widget(c.checks[0]) {
		t.Errorf("the first option's row ends with %T, want its check", row[len(row)-1])
	}
	if _, ok := row[0].(*Label); !ok {
		t.Errorf("the first option's row starts with %T, want its label", row[0])
	}
	if got, muted := c.text.textColor(c.text.style(c.text)), Current().DisabledText(); got == muted {
		t.Errorf("the placeholder reads muted %#x, want the trigger's color", uint32(got))
	}
}
