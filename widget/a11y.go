// Accessibility semantics for the widget tree: semantic roles plus a
// state snapshot that assistive technology can consume. This is the
// plain-Go layer only - how it reaches a screen reader is recorded in
// docs/a11y.md.
package widget

import (
	"github.com/stubbedev/lys-lib/render"
)

// Role classifies a widget for assistive technology. The names follow
// the AT-SPI roles an out-of-process bridge would translate to, so
// callers can switch on them without lys-ui gaining a dbus dependency.
type Role uint8

// Semantic roles. RoleNone is zero: layout containers, spacers, and
// unclassified leaves; the rest name the interactive and text widgets,
// translated to their AT-SPI names by String.
const (
	RoleNone Role = iota
	RoleButton
	RoleLabel
	RoleEntry
	RoleTextArea
	RoleSlider
	RoleSwitch
	RoleCheckBox
	RoleProgressBar
	RoleLevelBar
	RoleScrollArea
	RoleList
	RoleMenu
	RoleTabList
	RoleComboBox
	RoleSplitter
	RoleCalendar
	RoleColorChooser
	RoleScrollBar
	// RoleToggleButton is a button that stays pressed (GTK's
	// toggle-button role); A11yState.Pressed carries its state.
	RoleToggleButton
	// RoleAlert is an inline callout assistive technology announces
	// (an Alert).
	RoleAlert
	// RoleChart is a chart, named by a summary of its data.
	RoleChart
)

// String returns the lowercase role name.
func (r Role) String() string {
	switch r {
	case RoleButton:
		return "button"
	case RoleLabel:
		return "label"
	case RoleEntry:
		return "entry"
	case RoleTextArea:
		return "text-area"
	case RoleSlider:
		return "slider"
	case RoleSwitch:
		return "switch"
	case RoleCheckBox:
		return "check-box"
	case RoleProgressBar:
		return "progress-bar"
	case RoleLevelBar:
		return "level-bar"
	case RoleScrollArea:
		return "scroll-area"
	case RoleList:
		return "list"
	case RoleMenu:
		return "menu"
	case RoleTabList:
		return "tab-list"
	case RoleComboBox:
		return "combo-box"
	case RoleSplitter:
		return "splitter"
	case RoleCalendar:
		return "calendar"
	case RoleColorChooser:
		return "color-chooser"
	case RoleScrollBar:
		return "scroll-bar"
	case RoleToggleButton:
		return "toggle-button"
	case RoleAlert:
		return "alert"
	case RoleChart:
		return "chart"
	default:
		return "none"
	}
}

// SetAccessibleName names the widget for assistive technology, over
// its tooltip and the name its content gives it: an icon button's
// "Close", a page link's "Go to page 3". Empty restores those.
func (n *node) SetAccessibleName(name string) { n.accName = name }

// AccessibleName returns the name SetAccessibleName set.
func (n *node) AccessibleName() string { return n.accName }

// Roleer is implemented by widgets carrying a semantic role.
type Roleer interface {
	// Role returns the widget's semantic role.
	Role() Role
}

// RoleOf returns w's role, or RoleNone when w is nil or carries none
// (plain containers and third-party Widget implementations).
func RoleOf(w Widget) Role {
	if r, ok := w.(Roleer); ok {
		return r.Role()
	}
	return RoleNone
}

// A11yState is a point-in-time snapshot of what assistive technology
// needs about one widget: role, name, current value, text contents
// with caret and selection, and state flags.
type A11yState struct {
	Role Role
	// Name is the accessible name: SetAccessibleName's when set, else
	// the tooltip text, else the visible label text for labeled
	// controls.
	Name string
	// Text is the current contents for text-bearing roles.
	Text string
	// Caret is the text cursor as a rune offset into Text.
	Caret int
	// SelStart and SelEnd bound the selected rune range; HasSelection
	// reports whether one is active.
	SelStart, SelEnd int
	HasSelection     bool
	// Checked is the Switch and CheckButton state.
	Checked bool
	// Pressed is a ToggleButton's active state.
	Pressed bool
	// Min, Max, and Value carry the ranged roles (slider, progress
	// bar); Step is the keyboard increment (slider only, 0 when the
	// slider divides the range itself).
	Min, Max, Value, Step float64
	// Editable reports whether the text can be changed by the user:
	// false for a read-only field, and for a disabled one — the same
	// contract the widget input paths enforce.
	Editable bool
	// Enabled reports whether the widget accepts input at all, folding
	// its ancestors in (widget.IsEnabled): a widget inside a disabled
	// container reports false, exactly as input treats it.
	Enabled bool
	// Multiline distinguishes text areas from single-line entries.
	Multiline bool
	// Focusable reports whether focus can land on the widget, by Tab,
	// a click, SetFocus or an assistive technology: an enabled, rendered
	// KeyActionHandler.
	Focusable bool
	// TabStop reports whether Tab traversal lands on it: focusable, and
	// not a row of a list, whose rows are one stop with the list.
	TabStop bool
	// Bounds is the arranged rect, so a bridge can report extents.
	Bounds render.Rect
}

// Describe snapshots w's accessibility state. Focus is deliberately
// absent: it lives on the Router, and callers compare against
// Router.Focused directly.
func Describe(w Widget) A11yState {
	if w == nil {
		return A11yState{}
	}
	st := A11yState{Role: RoleOf(w), Enabled: IsEnabled(w), Focusable: isFocusable(w), TabStop: tabStop(w)}
	if n := nodeOf(w); n != nil && n.accName != "" {
		st.Name = n.accName
	} else if t, ok := w.(TooltipTexter); ok {
		st.Name = t.TooltipText()
	}
	if b, ok := w.(Boundser); ok {
		st.Bounds = b.Bounds()
	}
	// The derived name goes through an interface, not the type switch
	// below, so a consumer type embedding a Button (a toggle that adds
	// behavior) keeps the name GTK would give it.
	if n, ok := w.(a11yNamer); ok && st.Name == "" {
		st.Name = n.a11yName()
	}
	if p, ok := w.(a11yPresser); ok {
		st.Pressed = p.a11yPressed()
	}
	switch v := w.(type) {
	case *Label:
		st.Text = v.Text()
	case *RichLabel:
		st.Text = v.Text()
	case *Entry:
		st.Editable = v.editable()
		st.Text = v.Text()
		st.Caret = v.Cursor()
		st.SelStart, st.SelEnd, st.HasSelection = v.Selection()
	case *TextArea:
		st.Editable = v.editable()
		st.Multiline = true
		st.Text = v.Text()
		st.Caret, st.SelStart, st.SelEnd, st.HasSelection = textAreaOffsets(v)
	case *InputOTP:
		st.Editable = IsEnabled(v)
		st.Text = v.Code()
		st.Caret = v.caret
	case *Slider:
		st.Min, st.Max, st.Value, st.Step = v.min, v.max, v.value, v.step
	case *Switch:
		st.Checked = v.On()
	case *CheckButton:
		st.Checked = v.Checked()
	case *ProgressBar:
		st.Min, st.Max, st.Value = 0, 1, v.Value()
	case *LevelBar:
		st.Min, st.Max, st.Value = 0, 1, v.Value()
	case *ProgressRing:
		st.Min, st.Max, st.Value = 0, 1, v.Value()
	case *Scroll:
		st.Role = RoleScrollArea
	case *List:
		st.Role = RoleList
	case *Menu:
		st.Role = RoleMenu
	case *Notebook:
		st.Role = RoleTabList
	}
	return st
}

// a11yNamer is a widget whose accessible name derives from its own
// content when no tooltip names it. The method is unexported, so only
// lys-ui's widgets implement it - and every type embedding one.
type a11yNamer interface{ a11yName() string }

// A bare label's text is its accessible name.
func (l *Label) a11yName() string { return l.Text() }

func (l *RichLabel) a11yName() string { return l.Text() }

// A button is named by the label it wraps.
func (b *Button) a11yName() string { return labelText(b.child) }

// a11yPresser is a toggle reporting its pressed state, promoted like
// a11yNamer to the types that embed one.
type a11yPresser interface{ a11yPressed() bool }

func (t *ToggleButton) a11yPressed() bool { return t.active }

// A combo box's accessible name is its current selection.
func (d *Dropdown) a11yName() string { return d.Selection() }

// focusOptOut is a key handler that takes the focus only some of the
// time: an Item while it activates.
type focusOptOut interface{ focusable() bool }

// isFocusable reports whether focus can land on w, by Tab traversal or
// SetFocus: it handles key actions (and has not opted out), is
// enabled, and is rendered — a widget hidden by SetVisible, display:
// none or visibility: hidden takes no focus, the same as a disabled
// one.
func isFocusable(w Widget) bool {
	if _, ok := w.(KeyActionHandler); !ok {
		return false
	}
	if o, ok := w.(focusOptOut); ok && !o.focusable() {
		return false
	}
	return IsEnabled(w) && rendered(w)
}

// rendered reports whether w is on screen for the user: laid out
// (IsVisible) and shown by its computed visibility.
func rendered(w Widget) bool { return IsVisible(w) && shown(w) }

// DescribeTree snapshots root and every rendered descendant in paint
// order, the same pre-order focus traversal uses: a widget hidden from
// the screen is hidden from assistive technology too.
func DescribeTree(root Widget) []A11yState {
	var out []A11yState
	focusWalker(root, func(w Widget) {
		if rendered(w) {
			out = append(out, Describe(w))
		}
	})
	return out
}

// labelText returns the first label text found under w, for controls
// that wrap a label as their visible name.
func labelText(w Widget) string {
	if l, ok := w.(*Label); ok {
		return l.Text()
	}
	type childser interface{ Children() []Widget }
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			if s := labelText(k); s != "" {
				return s
			}
		}
	}
	return ""
}

// textAreaOffsets flattens a TextArea's cursor and selection into rune
// offsets into Text: every line contributes its runes plus a newline.
func textAreaOffsets(t *TextArea) (caret, selStart, selEnd int, active bool) {
	start, end := t.ordered()
	return t.offsetOf(t.cursor), t.offsetOf(start), t.offsetOf(end), start != end
}
