package widget

import (
	"fmt"
	"slices"
	"unicode"

	"github.com/stubbedev/lys-lib/render"
)

// OTPCharset is what an InputOTP accepts.
type OTPCharset uint8

// The code character sets.
const (
	// OTPDigits accepts 0-9.
	OTPDigits OTPCharset = iota
	// OTPAlphanumeric accepts letters and digits, uppercased.
	OTPAlphanumeric
)

// accept maps r into the set, reporting false when it is not in it.
func (c OTPCharset) accept(r rune) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return r, true
	case c == OTPAlphanumeric && unicode.IsLetter(r):
		return unicode.ToUpper(r), true
	}
	return 0, false
}

// InputOTP is a one-time-code input: a fixed
// number of slots in groups, one character each, the groups split by
// a dash. It takes the focus as one field: typing fills the active
// slot and advances, Backspace and Delete clear, Left, Right, Home and
// End move, a paste fills from the active slot, and a click picks a
// slot. The focus ring goes around the active slot. It styles as the
// `inputotp` element with .slot boxes (the active one :checked) and
// .separator dashes.
type InputOTP struct {
	composite
	row     *Box
	slots   []*Box
	labels  []*Label
	charset OTPCharset
	code    []rune
	caret   int

	// OnChanged fires with the code after every edit; OnComplete when
	// an edit fills the last empty slot.
	OnChanged  func(code string)
	OnComplete func(code string)
}

// otpSlotSize is a slot's side.
const otpSlotSize = 36

// NewInputOTP returns an empty code input of the groups' slot counts:
// NewInputOTP(face, OTPDigits, 3, 3) is two groups of three. Every
// group holds at least one slot, or the call panics.
func NewInputOTP(face render.Font, charset OTPCharset, groups ...int) *InputOTP {
	face = requireFace("widget.NewInputOTP", face)
	if len(groups) == 0 || slices.ContainsFunc(groups, func(n int) bool { return n < 1 }) {
		panic(fmt.Sprintf("widget: NewInputOTP: groups %v, want one or more of at least one slot", groups))
	}
	o := &InputOTP{row: NewBox(Row, 8, 0), charset: charset}
	o.SetElement("inputotp")
	for g, n := range groups {
		if g > 0 {
			sep := NewSymbol(SymbolMinus, 24)
			sep.AddClass("separator")
			o.row.AppendAligned(sep, false, AlignCenter)
		}
		group := NewBox(Row, 0, 0)
		for range n {
			label := NewLabel(face, 14, "", 0)
			label.SetAlignment(render.AlignCenter)
			slot := NewBox(Column, 0, 0)
			slot.AddClass("slot")
			slot.SetSizeRequest(otpSlotSize, otpSlotSize)
			slot.Append(NewSpacer(0, 0), true).Append(label, false).Append(NewSpacer(0, 0), true)
			group.Append(slot, false)
			o.slots = append(o.slots, slot)
			o.labels = append(o.labels, label)
		}
		o.row.AppendAligned(group, false, AlignCenter)
	}
	o.initComposite(o, o.row)
	o.boxed = true
	o.sync()
	return o
}

// Len is the number of slots.
func (o *InputOTP) Len() int { return len(o.slots) }

// Code returns the characters entered so far.
func (o *InputOTP) Code() string { return string(o.code) }

// SetCode replaces the code with s's accepted characters, as many as
// fit, and puts the caret after them. It reports nothing.
func (o *InputOTP) SetCode(s string) {
	o.code = o.code[:0]
	o.caret = 0
	o.insert(s)
	o.sync()
}

// active is the slot the caret points at: past a full code it stays
// on the last.
func (o *InputOTP) active() int { return min(o.caret, len(o.slots)-1) }

// insert puts s's accepted characters at the caret until the code is
// full, and reports whether any landed.
func (o *InputOTP) insert(s string) bool {
	landed := false
	for _, r := range s {
		r, ok := o.charset.accept(r)
		if !ok {
			continue
		}
		if len(o.code) == len(o.slots) {
			break
		}
		o.code = slices.Insert(o.code, o.caret, r)
		o.caret++
		landed = true
	}
	return landed
}

// edited shows an edit and reports it.
func (o *InputOTP) edited(wasFull bool) {
	o.sync()
	if o.OnChanged != nil {
		o.OnChanged(o.Code())
	}
	if !wasFull && len(o.code) == len(o.slots) && o.OnComplete != nil {
		o.OnComplete(o.Code())
	}
}

// sync shows the code and the active slot.
func (o *InputOTP) sync() {
	for i, l := range o.labels {
		text := ""
		if i < len(o.code) {
			text = string(o.code[i])
		}
		l.SetText(text)
		o.slots[i].SetState(StateChecked, i == o.active())
	}
	o.Invalidate()
}

// InsertRune implements RuneHandler: an accepted character fills the
// active slot and advances; in a full code it overwrites the active
// slot.
func (o *InputOTP) InsertRune(r rune) {
	r, ok := o.charset.accept(r)
	if !ok {
		return
	}
	if len(o.code) < len(o.slots) {
		o.Insert(string(r))
		return
	}
	o.code[o.active()] = r
	o.caret = min(o.caret+1, len(o.slots))
	o.edited(true)
}

// Insert implements TextInserter: a paste fills from the active slot.
func (o *InputOTP) Insert(s string) {
	wasFull := len(o.code) == len(o.slots)
	if o.insert(s) {
		o.edited(wasFull)
	}
}

// KeyAction implements KeyActionHandler: clearing and moving.
func (o *InputOTP) KeyAction(a KeyAction, _ Mods) {
	switch a {
	case KeyBackspace:
		at := o.caret - 1
		if len(o.code) == len(o.slots) && o.caret == len(o.slots) {
			at = len(o.code) - 1
		}
		if at < 0 {
			return
		}
		o.code = slices.Delete(o.code, at, at+1)
		o.caret = at
		o.edited(true)
		return
	case KeyDelete:
		if o.caret >= len(o.code) {
			return
		}
		o.code = slices.Delete(o.code, o.caret, o.caret+1)
		o.edited(true)
		return
	case KeyLeft:
		o.caret = max(0, min(o.caret, len(o.slots)-1)-1)
	case KeyRight:
		o.caret = min(o.caret+1, len(o.code))
	case KeyHome:
		o.caret = 0
	case KeyEnd:
		o.caret = len(o.code)
	default:
		return
	}
	o.sync()
}

// ClickAt implements Clicker: a click picks the slot under it, or the
// first empty one past the code.
func (o *InputOTP) ClickAt(p Point) {
	for i, s := range o.slots {
		if s.Bounds().Contains(p.X, p.Y) {
			o.caret = min(i, len(o.code))
			o.sync()
			return
		}
	}
}

// HitTest takes every press inside the bounds, so a click on a slot
// focuses the input.
func (o *InputOTP) HitTest(p Point) Widget { return o.HitLeaf(o, p) }

// Paint draws the slots, then the caret in an empty active slot while
// the input has the focus.
func (o *InputOTP) Paint(cv *render.Canvas) {
	o.composite.Paint(cv)
	if !o.focused || o.caret != len(o.code) || len(o.code) == len(o.slots) {
		return
	}
	b := o.slots[o.active()].Bounds()
	h := b.H * 4 / 9
	cv.FillRect(render.Rect{X: b.X + b.W/2, Y: b.Y + (b.H-h)/2, W: 1, H: h}, Current().Text)
}

// focusRingTarget draws the focus ring around the active slot.
func (o *InputOTP) focusRingTarget() Widget { return o.slots[o.active()] }

// Role is RoleEntry: a field whose text is the code.
func (o *InputOTP) Role() Role { return RoleEntry }
