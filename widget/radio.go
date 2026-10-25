package widget

// Mutual exclusion (#96): radioGroup links toggles of which at most
// one is active - radio CheckButtons and grouped ToggleButtons alike,
// and the ToggleButtons a ToggleGroup is made of. Activating a member
// clears the others through their own setters, so each fires its own
// change hook exactly like a user toggle would.

// radioMember is one toggle in a group.
type radioMember interface {
	radioClear()
}

// radioGroup is the shared exclusion set.
type radioGroup struct {
	members []radioMember
}

// join adds m to the group.
func (g *radioGroup) join(m radioMember) { g.members = append(g.members, m) }

// activated clears every member but m.
func (g *radioGroup) activated(m radioMember) {
	for _, o := range g.members {
		if o != m {
			o.radioClear()
		}
	}
}

// ToggleButton is a button that stays pressed (GTK ToggleButton):
// clicking flips its active state, painted with the selected fill and
// :checked. In a group (SetGroup) it behaves as a radio: activating it
// releases the others and a click on the active one keeps it active.
// The embedded Button's OnClick drives the toggle; observe OnToggled.
// Assistive technology reads it as a toggle button, pressed while
// active.
type ToggleButton struct {
	Button
	active bool
	group  *radioGroup

	// OnToggled fires after every change of the active state, from a
	// click, a key, SetActive, or a group peer taking over.
	OnToggled func(active bool)
}

// NewToggleButton returns an inactive toggle around child. It styles
// as GTK's toggle, button.toggle.
func NewToggleButton(child Widget, padding, radius int) *ToggleButton {
	t := &ToggleButton{Button: *NewButton(child, padding, radius)}
	t.AddClass("toggle")
	t.OnClick = t.clicked
	return t
}

// Active reports the state.
func (t *ToggleButton) Active() bool { return t.active }

// SetActive changes the state, firing OnToggled when it flipped.
func (t *ToggleButton) SetActive(on bool) { t.setActive(on, true) }

// SetGroup makes t a radio peer of other (joining other's group).
func (t *ToggleButton) SetGroup(other *ToggleButton) {
	if other.group == nil {
		other.group = &radioGroup{}
		other.group.join(other)
	}
	t.joinGroup(other.group)
}

// joinGroup enters g; an active toggle joining releases the others.
func (t *ToggleButton) joinGroup(g *radioGroup) {
	t.group = g
	g.join(t)
	if t.active {
		g.activated(t)
	}
}

// clicked is the click: a grouped toggle only ever activates.
func (t *ToggleButton) clicked() {
	if t.group != nil && t.active {
		return
	}
	t.SetActive(!t.active)
}

// setActive applies the state; notify gates OnToggled (a container
// rebuilding its toggles marks without announcing).
func (t *ToggleButton) setActive(on, notify bool) {
	if t.active == on {
		return
	}
	t.active = on
	// :checked selects the stylesheet's fill; without a rule the
	// button paints the theme's selected shade (Button.Paint).
	t.SetState(StateChecked, on)
	t.Invalidate()
	if on && t.group != nil {
		t.group.activated(t)
	}
	if notify && t.OnToggled != nil {
		t.OnToggled(on)
	}
}

// radioClear implements radioMember.
func (t *ToggleButton) radioClear() { t.SetActive(false) }

// Role implements Roleer.
func (t *ToggleButton) Role() Role { return RoleToggleButton }

// SetGroup makes c a radio button in other's group: drawn as a round
// `radio` indicator, checking it unchecks the others, and a click on
// the checked one keeps it checked.
func (c *CheckButton) SetGroup(other *CheckButton) {
	if other.group == nil {
		other.group = &radioGroup{}
		other.group.join(other)
		other.check.SetElement("radio")
	}
	c.group = other.group
	c.group.join(c)
	c.check.SetElement("radio")
	if c.checked {
		c.group.activated(c)
	}
	c.Invalidate()
}

// radioClear implements radioMember.
func (c *CheckButton) radioClear() { c.SetChecked(false) }
