// The bridge's end-to-end tests: a private dbus-daemon plays the
// accessibility bus, a second connection plays the registry (its
// Socket.Embed) and the assistive technology (walking the tree, reading
// interfaces, listening for events), and a real widget tree plays the
// scene. No compositor, no at-spi2-core — the wire contract is pinned
// against the interface definitions the bridge serves.
package atspi

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/dbustest"
	"github.com/stubbedev/lys-ui/widget"
)

// ref mirrors the (so) object reference for Store targets.
type ref struct {
	Name string
	Path dbus.ObjectPath
}

// mockOrgA11yBus plays org.a11y.Bus: its GetAddress reports the test
// bus as the accessibility bus.
type mockOrgA11yBus struct{ addr string }

func (m *mockOrgA11yBus) GetAddress() (string, *dbus.Error) { return m.addr, nil }

// testScene is a synchronous Scene over a real widget tree: Invoke
// runs inline, so Refresh and Serve's first sample are deterministic.
// loop stands in for the loop goroutine: the bridge's D-Bus handlers
// and the test body both touch the widgets only while holding it.
type testScene struct {
	mu    sync.Mutex
	loop  sync.Mutex
	roots []widget.Widget
	focus widget.Widget
}

func (s *testScene) Roots() []widget.Widget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]widget.Widget(nil), s.roots...)
}

func (s *testScene) Focused() widget.Widget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focus
}

func (s *testScene) Invoke(fn func()) {
	s.loop.Lock()
	defer s.loop.Unlock()
	fn()
}

func (s *testScene) SetFocus(w widget.Widget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focus = w
}

// startA11yBus launches a private bus standing in for the
// accessibility bus.
func startA11yBus(t *testing.T) string {
	t.Helper()
	address, _ := dbustest.Start(t)
	return address
}

// fakeRegistry plays the accessibility registry on the bus: it answers
// Socket.Embed, records the plug, and collects the signals the bridge
// emits into a recorded list — order-independent lookups, unlike a
// raw channel that drops skipped members.
type fakeRegistry struct {
	conn *dbus.Conn

	mu       sync.Mutex
	plug     ref
	recorded []*dbus.Signal
	signal   chan *dbus.Signal
}

// startFakeRegistry connects a second connection, claims the registry
// name, and serves the Socket.
func startFakeRegistry(t *testing.T, address string) *fakeRegistry {
	t.Helper()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatalf("registry connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.RequestName("org.a11y.atspi.Registry", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("registry name: %v", err)
	}
	r := &fakeRegistry{conn: conn, signal: make(chan *dbus.Signal, 256)}
	if err := conn.ExportAll(r, RootPath, "org.a11y.atspi.Socket"); err != nil {
		t.Fatalf("registry socket export: %v", err)
	}
	for _, iface := range []string{"org.a11y.atspi.Event.Object", "org.a11y.atspi.Event.Focus", "org.a11y.atspi.Cache"} {
		if err := conn.AddMatchSignal(dbus.WithMatchInterface(iface)); err != nil {
			t.Fatalf("match rule for %s: %v", iface, err)
		}
	}
	conn.Signal(r.signal)
	go r.record()
	return r
}

// record moves delivered signals into the recorded list.
func (r *fakeRegistry) record() {
	for sig := range r.signal {
		r.mu.Lock()
		r.recorded = append(r.recorded, sig)
		r.mu.Unlock()
	}
}

// Embed implements org.a11y.atspi.Socket: record the plug, answer with
// the registry's root.
func (r *fakeRegistry) Embed(plug ref) ref {
	r.mu.Lock()
	r.plug = plug
	r.mu.Unlock()
	return ref{Name: "org.a11y.atspi.Registry", Path: "/org/a11y/atspi/accessible/0"}
}

// Unembed implements org.a11y.atspi.Socket.
func (r *fakeRegistry) Unembed(plug ref) {}

// nextSignal waits for a matching recorded signal: path and member
// (the full name, e.g. org.a11y.atspi.Event.Object.StateChanged).
// Matched signals are consumed; lookups are order-independent.
func (r *fakeRegistry) nextSignal(t *testing.T, path dbus.ObjectPath, member string) *dbus.Signal {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		r.mu.Lock()
		for i, sig := range r.recorded {
			if sig.Path == path && sig.Name == member {
				r.recorded = append(r.recorded[:i], r.recorded[i+1:]...)
				r.mu.Unlock()
				return sig
			}
		}
		r.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s on %s", member, path)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// drainSignals empties the recorded list.
func (r *fakeRegistry) drainSignals() {
	r.mu.Lock()
	r.recorded = nil
	r.mu.Unlock()
}

// fixture is one served scene: the bus, the fake registry, the bridge,
// and the widget tree with handles for the assertions.
type fixture struct {
	bus    *fakeRegistry
	br     *Bridge
	name   string
	face   render.Font
	root   *widget.Box
	box    *widget.Box
	btn    *widget.Button
	sw     *widget.Switch
	entry  *widget.Entry
	label  *widget.Label
	slider *widget.Slider
	scene  *testScene
}

// faceOf loads the standard test face.
func faceOf(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// newFixture serves a window tree: a column holding a button, a
// switch, an entry, a label, and a slider, arranged so every widget
// carries bounds.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	address := startA11yBus(t)
	bus := startFakeRegistry(t, address)
	name := fmt.Sprintf("org.a11y.atspi.lysui.test.p%d", os.Getpid())

	face := faceOf(t)
	f := &fixture{bus: bus, name: name, face: face}
	f.btn = widget.NewButton(widget.NewLabel(face, 13, "press me", render.RGB(0, 0, 0)), 8, 4)
	f.sw = widget.NewSwitch(false)
	f.entry = widget.NewEntry(face, 13, render.RGB(0, 0, 0))
	f.entry.SetText("hello")
	f.label = widget.NewLabel(face, 13, "just a label", render.RGB(0, 0, 0))
	f.slider = widget.NewSlider(0, 100, 5, 40)
	f.box = widget.NewBox(widget.Column, 8, 8)
	// The button expands, so the bounds-change pass has a widget whose
	// rect actually moves when the window relayouts.
	f.box.Append(f.btn, true)
	f.box.Append(f.sw, false)
	f.box.Append(f.entry, false)
	f.box.Append(f.label, false)
	f.box.Append(f.slider, false)
	f.root = widget.NewBox(widget.Column, 0, 0)
	// The box expands in the window and the button in the box, so a
	// relayout moves the button and BoundsChanged has a subject.
	f.root.Append(f.box, true)
	arrange(t, f.root, 300, 400)

	f.scene = &testScene{roots: []widget.Widget{f.root}}
	br, err := Serve(f.scene, Options{Address: address, Name: name, Poll: -1})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	t.Cleanup(br.Stop)
	f.br = br
	br.Refresh() // the deterministic first sample (Poll: -1, sync scene)
	return f
}

// arrange measures under w x h and then arranges exactly w x h — the
// resize is the point, so the arrangement is the requested size, not
// the natural measure.
func arrange(t *testing.T, root widget.Widget, w, h int) {
	t.Helper()
	root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
}

// obj is the bridge's object for a path on the registry's connection.
func (f *fixture) obj(path dbus.ObjectPath) dbus.BusObject {
	return f.bus.conn.Object(f.name, path)
}

// childrenOf asks the bridge for a node's children.
func (f *fixture) childrenOf(t *testing.T, path dbus.ObjectPath) []ref {
	t.Helper()
	var out []ref
	if err := f.obj(path).Call("org.a11y.atspi.Accessible.GetChildren", 0).Store(&out); err != nil {
		t.Fatalf("GetChildren on %s: %v", path, err)
	}
	return out
}

// pathOfWidget finds a widget's object path by walking from the root
// (the test's own map of the served tree).
func (f *fixture) walk(t *testing.T) map[dbus.ObjectPath]widget.Widget {
	t.Helper()
	out := map[dbus.ObjectPath]widget.Widget{}
	var visit func(p dbus.ObjectPath, w widget.Widget)
	visit = func(p dbus.ObjectPath, w widget.Widget) {
		out[p] = w
		type childser interface{ Children() []widget.Widget }
		if c, ok := w.(childser); ok {
			kids := c.Children()
			refs := f.childrenOf(t, p)
			if len(refs) != len(kids) {
				t.Fatalf("children of %s: %d served, %d in the tree", p, len(refs), len(kids))
			}
			for i, k := range kids {
				visit(refs[i].Path, k)
			}
		}
	}
	refs := f.childrenOf(t, RootPath)
	if len(refs) != 1 || refs[0].Name != f.name {
		t.Fatalf("root children = %v, want one window under our name", refs)
	}
	visit(refs[0].Path, f.root)
	return out
}

// TestRegistrationAndEmbed pins the handshake (#65): the bridge claims
// its name, exports the root, and Embeds with the registry carrying
// exactly the root path.
func TestRegistrationAndEmbed(t *testing.T) {
	f := newFixture(t)
	f.bus.mu.Lock()
	plug := f.bus.plug
	f.bus.mu.Unlock()
	if plug.Name != f.name || plug.Path != RootPath {
		t.Fatalf("Embed plug = %+v, want (%s, %s)", plug, f.name, RootPath)
	}

	// The application node answers Application and Properties.
	var toolkit string
	if err := f.obj(RootPath).Call("org.a11y.atspi.Application.GetToolkitName", 0).Store(&toolkit); err != nil {
		t.Fatalf("GetToolkitName: %v", err)
	}
	if toolkit != "lys-ui" {
		t.Errorf("toolkit = %q, want lys-ui", toolkit)
	}
	var props map[string]dbus.Variant
	if err := f.obj(RootPath).Call("org.freedesktop.DBus.Properties.GetAll", 0,
		"org.a11y.atspi.Accessible").Store(&props); err != nil {
		t.Fatalf("GetAll on root: %v", err)
	}
	if got, ok := props["ChildCount"]; !ok || got.Value() != any(int32(1)) {
		t.Errorf("root ChildCount = %v, want 1 window", props["ChildCount"])
	}
	if got, ok := props["Parent"]; !ok || got.Value() == nil {
		t.Error("root Parent property missing")
	}

	// Introspection names the interfaces the root serves.
	var xml string
	if err := f.obj(RootPath).Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	for _, iface := range []string{
		"org.a11y.atspi.Accessible", "org.a11y.atspi.Application",
		"org.a11y.atspi.Component", "org.freedesktop.DBus.Properties",
	} {
		if !strings.Contains(xml, iface) {
			t.Errorf("root introspection lacks %s", iface)
		}
	}
}

// TestTreeWalk pins the served tree: every widget in the scene has an
// object whose role, name, and states are the semantic model's, in
// paint order, with the AT-SPI role enum and names.
func TestTreeWalk(t *testing.T) {
	f := newFixture(t)
	paths := f.walk(t)

	want := map[widget.Widget]struct {
		role     uint32
		roleName string
		name     string
	}{
		f.btn:    {rolePushButton, "push button", "press me"},
		f.sw:     {roleSwitch, "switch", ""},
		f.entry:  {roleEntry, "entry", ""},
		f.label:  {roleLabel, "label", "just a label"},
		f.slider: {roleSlider, "slider", ""},
	}
	var entryPath, btnPath dbus.ObjectPath
	for path, w := range paths {
		ww, ok := want[w]
		if !ok {
			continue // the containers themselves
		}
		var role uint32
		var roleName, name string
		obj := f.obj(path)
		if err := obj.Call("org.a11y.atspi.Accessible.GetRole", 0).Store(&role); err != nil {
			t.Fatalf("GetRole on %s: %v", path, err)
		}
		if err := obj.Call("org.a11y.atspi.Accessible.GetRoleName", 0).Store(&roleName); err != nil {
			t.Fatalf("GetRoleName: %v", err)
		}
		if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0,
			"org.a11y.atspi.Accessible", "Name").Store(&name); err != nil {
			t.Fatalf("Name property: %v", err)
		}
		if role != ww.role || roleName != ww.roleName {
			t.Errorf("%s role = %d/%q, want %d/%q", path, role, roleName, ww.role, ww.roleName)
		}
		if name != ww.name {
			t.Errorf("%s name = %q, want %q", path, name, ww.name)
		}
		switch w {
		case f.entry:
			entryPath = path
		case f.btn:
			btnPath = path
		}
	}
	if entryPath == "" || btnPath == "" {
		t.Fatal("entry or button never walked")
	}

	// The button is enabled and focusable; the entry single-line.
	var states []uint32
	if err := f.obj(btnPath).Call("org.a11y.atspi.Accessible.GetState", 0).Store(&states); err != nil {
		t.Fatalf("GetState on the button: %v", err)
	}
	has := func(s uint32) bool { return hasState(states, s) }
	if !has(stateEnabled) || !has(stateFocusable) {
		t.Errorf("button states = %v, want enabled and focusable", states)
	}
	if has(stateFocused) {
		t.Error("button focused with no focus in the scene")
	}
	if err := f.obj(entryPath).Call("org.a11y.atspi.Accessible.GetState", 0).Store(&states); err != nil {
		t.Fatalf("GetState on the entry: %v", err)
	}
	for _, want := range []uint32{stateSingleLine, stateEditable} {
		if !has(want) {
			t.Errorf("entry states %v lack %d", states, want)
		}
	}

	// Component extents are the arranged bounds.
	var ext atspiRect
	if err := f.obj(btnPath).Call("org.a11y.atspi.Component.GetExtents", 0, uint32(1)).Store(&ext); err != nil {
		t.Fatalf("GetExtents: %v", err)
	}
	var b render.Rect
	f.scene.Invoke(func() { b = f.btn.Bounds() })
	if int(ext.X) != b.X || int(ext.Y) != b.Y || int(ext.W) != b.W || int(ext.H) != b.H {
		t.Errorf("extents = (%d,%d,%d,%d), want the arranged %+v", ext.X, ext.Y, ext.W, ext.H, b)
	}
	// Contains agrees with the extents.
	var inside bool
	if err := f.obj(btnPath).Call("org.a11y.atspi.Component.Contains", 0,
		int32(b.X+b.W/2), int32(b.Y+b.H/2), uint32(1)).Store(&inside); err != nil {
		t.Fatalf("Contains: %v", err)
	}
	if !inside {
		t.Error("Contains disagrees with GetExtents at the center")
	}
}

// TestTextInterface pins the Text service over the entry: contents,
// caret, character probes, selections, and the granularity readers.
func TestTextInterface(t *testing.T) {
	f := newFixture(t)
	paths := f.walk(t)
	var entryPath dbus.ObjectPath
	for path, w := range paths {
		if w == widget.Widget(f.entry) {
			entryPath = path
		}
	}
	if entryPath == "" {
		t.Fatal("entry not walked")
	}
	obj := f.obj(entryPath)

	var count int32
	if err := obj.Call("org.a11y.atspi.Text.GetCharacterCount", 0).Store(&count); err != nil {
		t.Fatalf("GetCharacterCount: %v", err)
	}
	if count != 5 {
		t.Fatalf("character count = %d, want 5 (hello)", count)
	}
	var text string
	if err := obj.Call("org.a11y.atspi.Text.GetText", 0, int32(1), int32(4)).Store(&text); err != nil {
		t.Fatalf("GetText: %v", err)
	}
	if text != "ell" {
		t.Errorf("GetText(1,4) = %q, want ell", text)
	}
	var ch int32
	if err := obj.Call("org.a11y.atspi.Text.GetCharacterAtOffset", 0, int32(0)).Store(&ch); err != nil {
		t.Fatalf("GetCharacterAtOffset: %v", err)
	}
	if ch != 'h' {
		t.Errorf("character 0 = %d, want 'h'", ch)
	}
	// Word granularity over "hello".
	var s string
	var start, end int32
	if err := obj.Call("org.a11y.atspi.Text.GetStringAtOffset", 0, int32(2), uint32(1)).Store(&s, &start, &end); err != nil {
		t.Fatalf("GetStringAtOffset: %v", err)
	}
	if s != "hello" || start != 0 || end != 5 {
		t.Errorf("word at 2 = (%q,%d,%d), want (hello,0,5)", s, start, end)
	}
	// The setters reach the widget: caret, selection, collapse.
	call := func(method string, args ...any) bool {
		t.Helper()
		var ok bool
		if err := obj.Call("org.a11y.atspi.Text."+method, 0, args...).Store(&ok); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return ok
	}
	var caret int
	if !call("SetCaretOffset", int32(2)) {
		t.Error("SetCaretOffset declined")
	}
	if f.scene.Invoke(func() { caret = f.entry.Cursor() }); caret != 2 {
		t.Errorf("SetCaretOffset left the caret at %d, want 2", caret)
	}
	if !call("SetSelection", int32(0), int32(1), int32(4)) {
		t.Error("SetSelection declined")
	}
	var st, en int
	var on bool
	if f.scene.Invoke(func() { st, en, on = f.entry.Selection() }); !on || st != 1 || en != 4 {
		t.Errorf("selection = %d..%d %v, want 1..4", st, en, on)
	}
	f.br.Refresh()
	var n int32
	if err := obj.Call("org.a11y.atspi.Text.GetNSelections", 0).Store(&n); err != nil {
		t.Fatalf("GetNSelections: %v", err)
	}
	if n != 1 {
		t.Errorf("selections = %d, want 1", n)
	}
	if call("AddSelection", int32(0), int32(1)) {
		t.Error("a second selection was accepted")
	}
	if !call("RemoveSelection", int32(0)) {
		t.Error("RemoveSelection declined")
	}
	if f.scene.Invoke(func() { _, _, on = f.entry.Selection() }); on {
		t.Error("RemoveSelection left a selection")
	}

	// Per-character geometry: a box inside the entry, and the point at
	// its center maps back to the character.
	var x, y, w, h int32
	if err := obj.Call("org.a11y.atspi.Text.GetCharacterExtents", 0, int32(1), uint32(1)).Store(&x, &y, &w, &h); err != nil {
		t.Fatalf("GetCharacterExtents: %v", err)
	}
	var b render.Rect
	if f.scene.Invoke(func() { b = f.entry.Bounds() }); w <= 0 || h <= 0 || !b.Contains(int(x), int(y)) {
		t.Errorf("extents of 1 = (%d,%d,%d,%d), want a box inside %+v", x, y, w, h, b)
	}
	var off int32
	if err := obj.Call("org.a11y.atspi.Text.GetOffsetAtPoint", 0, x+w/2, y+h/2, uint32(1)).Store(&off); err != nil {
		t.Fatalf("GetOffsetAtPoint: %v", err)
	}
	if off != 1 {
		t.Errorf("offset at the box's center = %d, want 1", off)
	}
	var rw int32
	if err := obj.Call("org.a11y.atspi.Text.GetRangeExtents", 0, int32(0), int32(5), uint32(1)).Store(&x, &y, &rw, &h); err != nil {
		t.Fatalf("GetRangeExtents: %v", err)
	}
	if rw < 4*w {
		t.Errorf("range width %d, want the five characters' (char width %d)", rw, w)
	}

	// AT-driven focus lands through the scene.
	var ok bool
	if err := f.obj(entryPath).Call("org.a11y.atspi.Component.GrabFocus", 0).Store(&ok); err != nil || !ok {
		t.Fatalf("GrabFocus: %v %v", ok, err)
	}
	if f.scene.Focused() != widget.Widget(f.entry) {
		t.Error("GrabFocus did not focus the entry")
	}
}

// TestActionAndValue pin the activatable and ranged services: the
// button's click action fires the widget's own handler through the
// loop, and the slider's value round-trips with a working setter.
func TestActionAndValue(t *testing.T) {
	f := newFixture(t)
	paths := f.walk(t)
	var btnPath, sliderPath dbus.ObjectPath
	for path, w := range paths {
		switch w {
		case widget.Widget(f.btn):
			btnPath = path
		case widget.Widget(f.slider):
			sliderPath = path
		}
	}
	if btnPath == "" || sliderPath == "" {
		t.Fatal("button or slider not walked")
	}

	clicked := make(chan struct{}, 1)
	f.scene.Invoke(func() { f.btn.OnClick = func() { clicked <- struct{}{} } })

	var n int32
	var name string
	if err := f.obj(btnPath).Call("org.a11y.atspi.Action.GetNActions", 0).Store(&n); err != nil {
		t.Fatalf("GetNActions: %v", err)
	}
	if n != 1 {
		t.Fatalf("actions = %d, want 1", n)
	}
	if err := f.obj(btnPath).Call("org.a11y.atspi.Action.GetName", 0, int32(0)).Store(&name); err != nil {
		t.Fatalf("GetName: %v", err)
	}
	if name != "click" {
		t.Errorf("action name = %q, want click", name)
	}
	var ok bool
	if err := f.obj(btnPath).Call("org.a11y.atspi.Action.DoAction", 0, int32(0)).Store(&ok); err != nil {
		t.Fatalf("DoAction: %v", err)
	}
	if !ok {
		t.Fatal("DoAction declined a healthy button")
	}
	select {
	case <-clicked:
	case <-time.After(time.Second):
		t.Fatal("DoAction never clicked the button")
	}

	// Value: the slider's range and current value, and the setter
	// applies through the widget.
	var min, max, cur float64
	if err := f.obj(sliderPath).Call("org.a11y.atspi.Value.GetMinimumValue", 0).Store(&min); err != nil {
		t.Fatalf("GetMinimumValue: %v", err)
	}
	if err := f.obj(sliderPath).Call("org.a11y.atspi.Value.GetMaximumValue", 0).Store(&max); err != nil {
		t.Fatalf("GetMaximumValue: %v", err)
	}
	if min != 0 || max != 100 {
		t.Fatalf("slider range = [%v,%v], want [0,100]", min, max)
	}
	if err := f.obj(sliderPath).Call("org.a11y.atspi.Value.GetCurrentValue", 0).Store(&cur); err != nil {
		t.Fatalf("GetCurrentValue: %v", err)
	}
	if cur != 40 {
		t.Errorf("current = %v, want 40", cur)
	}
	if err := f.obj(sliderPath).Call("org.a11y.atspi.Value.SetCurrentValue", 0, 60.0).Store(&ok); err != nil {
		t.Fatalf("SetCurrentValue: %v", err)
	}
	var value float64
	if f.scene.Invoke(func() { value = f.slider.Value() }); !ok || value != 60 {
		t.Errorf("SetCurrentValue(60): ok=%v value=%v", ok, value)
	}
}

// TestEvents pins the minimal event set (#65): focus (and the focused
// state change), state-changed for checked and enabled, caret moves,
// bounds changes, and children-changed for tree updates.
func TestEvents(t *testing.T) {
	f := newFixture(t)
	// First sample already happened in the fixture; the first Refresh
	// below diffs against it. Walk to find the paths.
	f.br.Refresh() // settle the tree under the event listener
	time.Sleep(50 * time.Millisecond)
	f.bus.drainSignals()

	paths := f.walk(t)
	var btnPath, swPath, entryPath, boxPath dbus.ObjectPath
	for path, w := range paths {
		switch w {
		case widget.Widget(f.btn):
			btnPath = path
		case widget.Widget(f.sw):
			swPath = path
		case widget.Widget(f.entry):
			entryPath = path
		case widget.Widget(f.box):
			boxPath = path
		}
	}
	if btnPath == "" || swPath == "" || entryPath == "" || boxPath == "" {
		t.Fatal("tree walk missed a fixture widget")
	}

	t.Run("focus moves fire Focus and focused", func(t *testing.T) {
		f.scene.SetFocus(f.btn)
		f.br.Refresh()
		sig := f.bus.nextSignal(t, btnPath, "org.a11y.atspi.Event.Focus.Focus")
		_ = sig
		st := f.bus.nextSignal(t, btnPath, "org.a11y.atspi.Event.Object.StateChanged")
		if st.Body[0] != "focused" || st.Body[1] != int32(1) {
			t.Errorf("focused event = %v, want focused/1", st.Body)
		}
		// The served state agrees.
		var states []uint32
		if err := f.obj(btnPath).Call("org.a11y.atspi.Accessible.GetState", 0).Store(&states); err != nil {
			t.Fatalf("GetState: %v", err)
		}
		if !hasState(states, stateFocused) {
			t.Error("button not serving FOCUSED after the focus event")
		}
	})

	t.Run("a check toggle fires checked", func(t *testing.T) {
		f.scene.Invoke(func() { f.sw.SetOn(true) })
		f.br.Refresh()
		st := f.bus.nextSignal(t, swPath, "org.a11y.atspi.Event.Object.StateChanged")
		if st.Body[0] != "checked" || st.Body[1] != int32(1) {
			t.Errorf("checked event = %v, want checked/1", st.Body)
		}
	})

	t.Run("a caret move fires TextCaretMoved", func(t *testing.T) {
		f.scene.Invoke(func() { f.entry.InsertRune('!') })
		f.br.Refresh()
		ev := f.bus.nextSignal(t, entryPath, "org.a11y.atspi.Event.Object.TextCaretMoved")
		if ev.Body[1] != int32(6) {
			t.Errorf("caret event position = %v, want 6", ev.Body[1])
		}
	})

	t.Run("an edit fires TextChanged, a selection TextSelectionChanged", func(t *testing.T) {
		time.Sleep(50 * time.Millisecond) // the caret test's insert event lands, then goes
		f.bus.drainSignals()
		f.scene.Invoke(func() {
			f.entry.Select(0, 6)
			f.entry.Insert("yo")
		})
		f.br.Refresh()
		del := f.bus.nextSignal(t, entryPath, "org.a11y.atspi.Event.Object.TextChanged")
		if del.Body[0] != "delete" || del.Body[1] != int32(0) || del.Body[2] != int32(6) || del.Body[3].(dbus.Variant).Value() != "hello!" {
			t.Errorf("delete event = %v", del.Body)
		}
		ins := f.bus.nextSignal(t, entryPath, "org.a11y.atspi.Event.Object.TextChanged")
		if ins.Body[0] != "insert" || ins.Body[2] != int32(2) || ins.Body[3].(dbus.Variant).Value() != "yo" {
			t.Errorf("insert event = %v", ins.Body)
		}
		f.scene.Invoke(func() { f.entry.Select(0, 1) })
		f.br.Refresh()
		f.bus.nextSignal(t, entryPath, "org.a11y.atspi.Event.Object.TextSelectionChanged")
	})

	t.Run("a value change fires PropertyChange", func(t *testing.T) {
		f.scene.Invoke(func() { f.slider.SetValue(70) })
		f.br.Refresh()
		var sliderPath dbus.ObjectPath
		for path, w := range paths {
			if w == widget.Widget(f.slider) {
				sliderPath = path
			}
		}
		ev := f.bus.nextSignal(t, sliderPath, "org.a11y.atspi.Event.Object.PropertyChange")
		if ev.Body[0] != "accessible-value" || ev.Body[3].(dbus.Variant).Value() != 70.0 {
			t.Errorf("value event = %v", ev.Body)
		}
	})

	t.Run("a relayout fires BoundsChanged", func(t *testing.T) {
		f.scene.Invoke(func() { arrange(t, f.root, 500, 600) })
		f.br.Refresh()
		f.bus.nextSignal(t, btnPath, "org.a11y.atspi.Event.Object.BoundsChanged")
	})

	t.Run("tree updates fire ChildrenChanged", func(t *testing.T) {
		added := widget.NewLabel(f.face, 13, "added", render.RGB(0, 0, 0))
		f.scene.Invoke(func() { f.box.Append(added, false) })
		f.scene.Invoke(func() { arrange(t, f.root, 300, 400) })
		f.br.Refresh()
		ev := f.bus.nextSignal(t, boxPath, "org.a11y.atspi.Event.Object.ChildrenChanged")
		if ev.Body[0] != "add" {
			t.Errorf("children event = %v, want add", ev.Body)
		}
		add := f.bus.nextSignal(t, CachePath, "org.a11y.atspi.Cache.AddAccessible")
		if item, ok := add.Body[0].([]any); !ok || len(item) != 10 || item[6] != "added" {
			t.Errorf("AddAccessible = %v, want the label's entry", add.Body)
		}
		f.scene.Invoke(func() { f.box.Remove(added) })
		f.scene.Invoke(func() { arrange(t, f.root, 300, 400) })
		f.br.Refresh()
		ev = f.bus.nextSignal(t, boxPath, "org.a11y.atspi.Event.Object.ChildrenChanged")
		if ev.Body[0] != "remove" {
			t.Errorf("children event = %v, want remove", ev.Body)
		}
		f.bus.nextSignal(t, CachePath, "org.a11y.atspi.Cache.RemoveAccessible")
	})

	t.Run("disabling fires enabled and sensitive", func(t *testing.T) {
		f.scene.Invoke(func() { f.btn.SetEnabled(false) })
		f.br.Refresh()
		st := f.bus.nextSignal(t, btnPath, "org.a11y.atspi.Event.Object.StateChanged")
		if st.Body[0] != "enabled" || st.Body[1] != int32(0) {
			t.Errorf("enabled event = %v, want enabled/0", st.Body)
		}
		st = f.bus.nextSignal(t, btnPath, "org.a11y.atspi.Event.Object.StateChanged")
		if st.Body[0] != "sensitive" || st.Body[1] != int32(0) {
			t.Errorf("sensitive event = %v, want sensitive/0", st.Body)
		}
	})
}

// TestBusDiscovery pins the org.a11y.Bus path: with no explicit
// address, Serve asks the session bus (DBUS_SESSION_BUS_ADDRESS) for
// the accessibility bus and dials what it is told.
func TestBusDiscovery(t *testing.T) {
	address := startA11yBus(t)
	// The session bus and the a11y bus are the same daemon here — the
	// mock org.a11y.Bus just reports it.
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.RequestName("org.a11y.Bus", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("mock bus name: %v", err)
	}
	// ExportAll, not the map form of Export: godbus only dispatches
	// struct handlers (the appearance tests record the same).
	if err := conn.ExportAll(&mockOrgA11yBus{addr: address}, "/org/a11y/bus", "org.a11y.Bus"); err != nil {
		t.Fatalf("mock bus export: %v", err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)

	root := widget.NewBox(widget.Column, 0, 0)
	scene := &testScene{roots: []widget.Widget{root}}
	br, err := Serve(scene, Options{Name: fmt.Sprintf("org.a11y.atspi.lysui.disc.p%d", os.Getpid()), Poll: -1})
	if err != nil {
		t.Fatalf("Serve through discovery: %v", err)
	}
	t.Cleanup(br.Stop)
	br.Refresh()
	var v dbus.Variant
	if err := br.conn.Object(br.name, RootPath).Call(
		"org.freedesktop.DBus.Properties.Get", 0,
		"org.a11y.atspi.Accessible", "ChildCount").Store(&v); err != nil {
		t.Fatalf("root ChildCount over discovery: %v", err)
	}
	if got, ok := v.Value().(int32); !ok || got != 1 {
		t.Errorf("discovered bridge ChildCount = %v, want 1", v.Value())
	}
}

// TestServeDefaultNameIsValid pins the default bus name against the
// D-Bus naming rule the reference daemon enforces: an element may not
// start with a digit, so the raw pid (org.a11y.atspi.lysui.1234) is
// rejected by a real bus and the bridge died on startup.
func TestServeDefaultNameIsValid(t *testing.T) {
	address := startA11yBus(t)
	startFakeRegistry(t, address)
	root := widget.NewBox(widget.Column, 0, 0)
	scene := &testScene{roots: []widget.Widget{root}}
	br, err := Serve(scene, Options{Address: address, Poll: -1})
	if err != nil {
		t.Fatalf("Serve with the default name: %v", err)
	}
	t.Cleanup(br.Stop)
	if !strings.HasPrefix(br.name, "org.a11y.atspi.lysui.p") {
		t.Fatalf("default name %q lacks the digit-safe prefix", br.name)
	}
	if _, err := br.conn.RequestName(br.name, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("default name %q is not claimable: %v", br.name, err)
	}
}

// hasState reads one state out of an AT-SPI wire state set.
func hasState(set []uint32, s uint32) bool {
	return len(set) == 2 && set[s/32]&(1<<(s%32)) != 0
}

// TestCacheGetItems pins the Cache batch: every object once, root
// first with the application role, entries agreeing with the
// Accessible interface (name, role, child count, state set).
func TestCacheGetItems(t *testing.T) {
	f := newFixture(t)
	var items []cacheItem
	if err := f.bus.conn.Object(f.name, CachePath).Call("org.a11y.atspi.Cache.GetItems", 0).Store(&items); err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	walked := f.walk(t)
	if len(items) != len(walked)+1 {
		t.Fatalf("cache holds %d items, the tree %d objects plus the root", len(items), len(walked))
	}
	root := items[0]
	if root.Path.Path != RootPath || root.Role != roleApplication || root.Parent != desktopRef {
		t.Errorf("root item = %+v", root)
	}
	for _, it := range items[1:] {
		w := walked[it.Path.Path]
		if w == nil {
			t.Errorf("cache item %s not in the tree", it.Path.Path)
			continue
		}
		st := widget.Describe(w)
		if it.Name != st.Name || it.Role != atspiRole(st.Role) || len(it.States) != 2 {
			t.Errorf("item %s = %q role %d states %v, want %q role %d", it.Path.Path, it.Name, it.Role, it.States, st.Name, atspiRole(st.Role))
		}
	}
}
