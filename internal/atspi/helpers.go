// Snapshot lookups, properties, text boundaries, and introspection —
// the read-side helpers behind the exported tables. Everything reads
// the published tree under the bridge mutex and returns copies; no
// handler can observe a half-built pass.
package atspi

import (
	"runtime/debug"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/widget"
)

// readToolkitVersion digs the module version out of the build info.
func readToolkitVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}

// node returns the published node for id (nil for the app node's
// never-seen case or an id that left the tree).
func (b *Bridge) node(id int32) *anode {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cur.nodes[id]
}

// childAt returns the index-th child's reference, or the null
// reference when the index is out of range.
func (b *Bridge) childAt(id, index int32) objRef {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.cur.nodes[id]
	if n == nil || index < 0 || index >= intconv.Int32(len(n.children)) {
		return objRef{Name: "", Path: nullPath}
	}
	c := n.children[index]
	return objRef{Name: b.name, Path: pathOf(c)}
}

// childrenOf lists the node's children as references.
func (b *Bridge) childrenOf(id int32) []objRef {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.cur.nodes[id]
	out := make([]objRef, 0, len(n.children))
	for _, c := range n.children {
		out = append(out, objRef{Name: b.name, Path: pathOf(c)})
	}
	return out
}

// indexInParent is the node's position under its parent; -1 for the
// application root.
func (b *Bridge) indexInParent(id int32) int32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.cur.nodes[id]
	if n == nil {
		return -1
	}
	return intconv.Int32(n.index)
}

// roleOf is the AT-SPI role enum: APPLICATION for the root, the mapped
// semantic role for widgets.
func (b *Bridge) roleOf(id int32) uint32 {
	if id == 0 {
		return roleApplication
	}
	if n := b.node(id); n != nil {
		return atspiRole(n.st.Role)
	}
	return roleInvalid
}

// roleNameOf is the role's name.
func (b *Bridge) roleNameOf(id int32) string {
	if id == 0 {
		return "application"
	}
	if n := b.node(id); n != nil {
		return atspiRoleName(n.st.Role)
	}
	return "invalid"
}

// stateOf is the AT-SPI state set: the enabled/focusable/focused core
// every node answers, the checked state for toggles, and the text
// shape for text roles. The application root is always enabled.
func (b *Bridge) stateOf(id int32) []uint32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return stateSet(b.statesOf(b.cur, id))
}

// stateSet packs state enum values into AT-SPI's wire state set: a
// 64-bit mask as two uint32 words, low word first (Accessible.GetState
// and the Cache items carry it as "au").
func stateSet(states []uint32) []uint32 {
	var set [2]uint32
	for _, s := range states {
		set[s/32] |= 1 << (s % 32)
	}
	return set[:]
}

// statesOf lists node id's state enum values in tr; the root reads as
// the active application. Callers hold the bridge mutex.
func (b *Bridge) statesOf(tr *tree, id int32) []uint32 {
	if id == 0 {
		return []uint32{stateEnabled, stateActive, stateShowing, stateVisible}
	}
	n := tr.nodes[id]
	if n == nil {
		return nil
	}
	states := []uint32{stateEnabled, stateSensitive, stateShowing, stateVisible}
	if n.st.Enabled {
		if n.st.Focusable {
			states = append(states, stateFocusable)
		}
		if n.id == tr.focus {
			states = append(states, stateFocused)
		}
	} else {
		// A disabled widget reports neither enabled nor sensitive, the
		// greyed-out reading.
		states = []uint32{stateShowing, stateVisible}
	}
	if n.st.Checked {
		states = append(states, stateChecked)
	}
	if n.st.Pressed {
		states = append(states, statePressed)
	}
	if n.st.Editable {
		states = append(states, stateEditable)
	}
	if n.st.Multiline {
		states = append(states, stateMultiLine)
	} else if n.st.Role == widget.RoleEntry {
		states = append(states, stateSingleLine)
	}
	return states
}

// interfacesOf lists the D-Bus interfaces the node serves (the AT-SPI
// ones; the standard Properties/Introspectable ride along implicitly).
// The Application interface rides the root only.
func (b *Bridge) interfacesOf(id int32) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return ifacesOf(b.cur.nodes[id])
}

// ifacesOf is interfacesOf for a node in hand.
func ifacesOf(n *anode) []string {
	out := []string{ifaceAccessible, ifaceComponent}
	if n == nil {
		return out
	}
	if n.text {
		out = append(out, ifaceText)
	}
	if n.action {
		out = append(out, ifaceAction)
	}
	if n.value {
		out = append(out, ifaceValue)
	}
	if n.id == 0 {
		out = append(out, ifaceApplication)
	}
	return out
}

// nameOf is a node's accessible name; the application root's is the
// toolkit's.
func nameOf(n *anode) string {
	switch {
	case n == nil:
		return ""
	case n.id == 0:
		return "lys-ui"
	default:
		return n.st.Name
	}
}

// parentRef is a node's parent reference: the registry's desktop for
// the application root, the null reference for a node with none.
func (b *Bridge) parentRef(n *anode) objRef {
	switch {
	case n != nil && n.id == 0:
		return desktopRef
	case n == nil || n.parent == nil:
		return objRef{Name: "", Path: nullPath}
	default:
		return objRef{Name: b.name, Path: pathOf(n.parent)}
	}
}

// accessibleAtPoint finds the deepest node under id whose bounds
// contain the point, in paint order (later siblings win, matching the
// painter). The node itself counts.
func (b *Bridge) accessibleAtPoint(id int32, x, y int) objRef {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.cur.nodes[id]
	if n == nil {
		return objRef{Name: "", Path: nullPath}
	}
	deepest := deepestAt(n, x, y)
	if deepest == nil {
		return objRef{Name: "", Path: nullPath}
	}
	return objRef{Name: b.name, Path: pathOf(deepest)}
}

// deepestAt walks n's subtree for the deepest containing node.
func deepestAt(n *anode, x, y int) *anode {
	if !n.st.Bounds.Contains(x, y) {
		return nil
	}
	// Later children paint on top, so they get the first claim.
	for _, v := range slices.Backward(n.children) {
		if hit := deepestAt(v, x, y); hit != nil {
			return hit
		}
	}
	return n
}

// property answers one org.freedesktop.DBus.Properties Get.
func (b *Bridge) property(id int32, iface, prop string) (dbus.Variant, bool) {
	if v, ok := b.properties(id, iface)[prop]; ok {
		return v, true
	}
	return dbus.Variant{}, false
}

// properties answers a Properties GetAll for one interface.
func (b *Bridge) properties(id int32, iface string) map[string]dbus.Variant {
	switch iface {
	case ifaceAccessible:
		b.mu.Lock()
		defer b.mu.Unlock()
		n := b.cur.nodes[id]
		childCount := 0
		if n != nil {
			childCount = len(n.children)
		}
		return map[string]dbus.Variant{
			"Name":         dbus.MakeVariant(nameOf(n)),
			"Description":  dbus.MakeVariant(""),
			"Parent":       dbus.MakeVariant(b.parentRef(n)),
			"ChildCount":   dbus.MakeVariant(int32(childCount)),
			"Locale":       dbus.MakeVariant(""),
			"AccessibleId": dbus.MakeVariant(""),
			"HelpText":     dbus.MakeVariant(""),
			"version":      dbus.MakeVariant(uint32(1)),
		}
	case ifaceApplication:
		b.mu.Lock()
		appID := b.appID
		b.mu.Unlock()
		return map[string]dbus.Variant{
			"ToolkitName":      dbus.MakeVariant("lys-ui"),
			"ToolkitVersion":   dbus.MakeVariant(toolkitVersion),
			"Version":          dbus.MakeVariant(toolkitVersion),
			"AtspiVersion":     dbus.MakeVariant("2.1"),
			"Id":               dbus.MakeVariant(appID),
			"InterfaceVersion": dbus.MakeVariant(uint32(1)),
		}
	case ifaceText:
		if n := b.node(id); n != nil {
			return map[string]dbus.Variant{
				"CharacterCount": dbus.MakeVariant(intconv.Int32(len([]rune(n.st.Text)))),
				"CaretOffset":    dbus.MakeVariant(intconv.Int32(n.st.Caret)),
				"version":        dbus.MakeVariant(uint32(1)),
			}
		}
	case ifaceValue:
		if n := b.node(id); n != nil {
			return map[string]dbus.Variant{
				"MinimumValue":     dbus.MakeVariant(n.st.Min),
				"MaximumValue":     dbus.MakeVariant(n.st.Max),
				"MinimumIncrement": dbus.MakeVariant(n.st.Step),
				"CurrentValue":     dbus.MakeVariant(n.st.Value),
				"Text":             dbus.MakeVariant(""),
				"version":          dbus.MakeVariant(uint32(1)),
			}
		}
	case ifaceAction:
		return map[string]dbus.Variant{
			"NActions": dbus.MakeVariant(int32(1)),
			"version":  dbus.MakeVariant(uint32(1)),
		}
	case ifaceComponent:
		return map[string]dbus.Variant{"version": dbus.MakeVariant(uint32(1))}
	}
	return map[string]dbus.Variant{}
}

// textBoundary implements the deprecated GetText{Before,At,After}Offset
// trio over the boundary types 0..6 (char, word-start, word-end,
// sentence-start, sentence-end, line-start, line-end), returning the
// delimited string and its offsets. All offsets are rune offsets into
// the model's text.
func textBoundary(r []rune, offset, kind int32) (string, int32, int32) {
	n := intconv.Int32(len(r))
	offset = max32(0, min32(offset, n))
	switch kind {
	case 0: // CHAR
		if offset >= n {
			return "", n, n
		}
		return string(r[offset]), offset, offset + 1
	case 1: // WORD_START
		s, e := wordSpan(r, offset)
		return string(r[min32(s, n):min32(e, n)]), s, e
	case 2: // WORD_END
		s, e := wordSpan(r, offset)
		// end-bounded: shift to the previous word's end when the
		// offset sits at a boundary.
		if s >= e && s > 0 {
			ps, pe := wordSpan(r, s-1)
			s, e = ps, pe
		}
		return string(r[min32(s, n):min32(e, n)]), s, e
	case 3: // SENTENCE_START
		s, e := sentenceSpan(r, offset)
		return string(r[min32(s, n):min32(e, n)]), s, e
	case 4: // SENTENCE_END
		s, e := sentenceSpan(r, offset)
		if s >= e && s > 0 {
			ps, pe := sentenceSpan(r, s-1)
			s, e = ps, pe
		}
		return string(r[min32(s, n):min32(e, n)]), s, e
	case 5: // LINE_START
		s, e := lineSpan(r, offset)
		return string(r[min32(s, n):min32(e, n)]), s, e
	case 6: // LINE_END
		s, e := lineSpan(r, offset)
		if s >= e && s > 0 {
			ps, pe := lineSpan(r, s-1)
			s, e = ps, pe
		}
		return string(r[min32(s, n):min32(e, n)]), s, e
	default:
		return "", offset, offset
	}
}

// wordAt is the GetStringAtOffset word granularity: the word at or
// before the offset.
func wordAt(r []rune, offset int32) (string, int32, int32) {
	s, e := wordSpan(r, max32(0, min32(offset, intconv.Int32(len(r)))))
	return string(r[s:e]), s, e
}

// sentenceAt is the sentence granularity.
func sentenceAt(r []rune, offset int32) (string, int32, int32) {
	s, e := sentenceSpan(r, max32(0, min32(offset, intconv.Int32(len(r)))))
	return string(r[s:e]), s, e
}

// lineAt is the line granularity: newline-delimited.
func lineAt(r []rune, offset int32) (string, int32, int32) {
	s, e := lineSpan(r, max32(0, min32(offset, intconv.Int32(len(r)))))
	return string(r[s:e]), s, e
}

// wordSpan finds the whitespace-delimited word containing offset.
func wordSpan(r []rune, offset int32) (int32, int32) {
	n := intconv.Int32(len(r))
	if n == 0 {
		return 0, 0
	}
	s := min32(offset, n-1)
	for s > 0 && !isSpace(r[s-1]) {
		s--
	}
	e := s
	for e < n && !isSpace(r[e]) {
		e++
	}
	if s == e {
		// Whitespace itself: the span is the single rune.
		return offset, min32(offset+1, n)
	}
	return s, e
}

// sentenceSpan finds the sentence (terminated by . ! or ?) containing
// offset.
func sentenceSpan(r []rune, offset int32) (int32, int32) {
	n := intconv.Int32(len(r))
	if n == 0 {
		return 0, 0
	}
	s := min32(offset, n-1)
	for s > 0 && !isSentenceEnd(r[s-1]) {
		s--
	}
	e := s
	for e < n && !isSentenceEnd(r[e]) {
		e++
	}
	if e < n {
		e++ // include the terminator
	}
	return s, e
}

// lineSpan finds the newline-delimited line containing offset.
func lineSpan(r []rune, offset int32) (int32, int32) {
	n := intconv.Int32(len(r))
	if n == 0 {
		return 0, 0
	}
	s := min32(offset, n-1)
	for s > 0 && r[s-1] != '\n' {
		s--
	}
	e := s
	for e < n && r[e] != '\n' {
		e++
	}
	if e < n {
		e++ // include the newline, per LINE_START semantics
	}
	return s, e
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

// introspectionXML builds the node's Introspect answer from the
// interface set. The XML mirrors the at-spi2-core specs the tables
// implement, so real AT tooling (which introspects before calling)
// marshals correctly.
func introspectionXML(text, action, value, app bool) string {
	ifaces := []string{xmlAccessible, xmlComponent}
	if text {
		ifaces = append(ifaces, xmlText)
	}
	if action {
		ifaces = append(ifaces, xmlAction)
	}
	if value {
		ifaces = append(ifaces, xmlValue)
	}
	if app {
		ifaces = append(ifaces, xmlApplication)
	}
	return introspectionDoc(ifaces...)
}

// introspectionDoc is a node document holding ifaces plus the standard
// Properties and Introspectable.
func introspectionDoc(ifaces ...string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<node>
`)
	for _, x := range append(ifaces, xmlProperties, xmlIntrospectable) {
		writeIFace(&sb, x)
	}
	sb.WriteString("</node>\n")
	return sb.String()
}

func writeIFace(sb *strings.Builder, xml string) {
	sb.WriteString(xml)
	sb.WriteString("\n")
}
