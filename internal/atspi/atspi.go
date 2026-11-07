// Package atspi is the in-process AT-SPI bridge (#65): it serves the
// widget tree's accessibility semantics (widget.Describe and the
// semantic roles, the plain-Go model of widget/a11y.go) over D-Bus in
// the AT-SPI2 shape, so screen readers and tools like accerciser can
// walk a lys-ui application. It ships in every build; the application
// starts it when the desktop asks for assistive technologies
// (org.a11y.Status, status.go), so without one it costs nothing.
//
// The bridge is snapshot-driven and nothing it does on the loop waits on
// a bus. Serve runs off the loop. A poller goroutine watches the scene's
// accessibility generation and only when it moved does a sampling pass run on
// the application's loop goroutine (Scene.Invoke), rebuilding the tree
// of widget.A11yState snapshots keyed by stable widget identity. The
// emitter goroutine takes the finished tree, publishes it, and diffs it
// against the previous one: every difference becomes the AT-SPI events
// it sends: focus (and state-changed focused), state-changed
// (checked, enabled, sensitive, editable), text-changed,
// text-selection-changed, text-caret-moved, property-change (name,
// value), bounds-changed, children-changed for tree updates, and the
// Cache's add/remove. Handlers answer D-Bus method calls from the published
// snapshot only, never touching widgets off the loop goroutine.
package atspi

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/atspi/proto"
	"github.com/stubbedev/lys-ui/widget"
)

// AT-SPI2 role enum values (atspi-constants.h AtspiRole) for the roles
// the semantic model carries; everything else maps to FILLER, the
// container role AT-SPI uses for unlabeled layout widgets.
const (
	roleInvalid      uint32 = 0
	roleApplication  uint32 = 75
	roleCalendar     uint32 = 5
	roleCheckBox     uint32 = 7
	roleColorChooser uint32 = 9
	roleComboBox     uint32 = 11
	roleEntry        uint32 = 79
	roleFiller       uint32 = 20
	roleLabel        uint32 = 29
	roleLevelBar     uint32 = 103
	roleList         uint32 = 31
	roleMenu         uint32 = 33
	roleNotification uint32 = 101
	roleChart        uint32 = 80
	roleProgressBar  uint32 = 42
	rolePushButton   uint32 = 43
	roleScrollBar    uint32 = 48
	roleScrollPane   uint32 = 49
	roleSlider       uint32 = 51
	roleSplitPane    uint32 = 53
	rolePageTabList  uint32 = 38
	roleSwitch       uint32 = 130
	roleToggleButton uint32 = 62
	roleTextArea     uint32 = 60 // terminal; multi-line text in lys-ui
)

// AT-SPI2 state enum values (atspi-constants.h AtspiStateType).
const (
	stateActive     uint32 = 1
	stateChecked    uint32 = 4
	stateEditable   uint32 = 7
	stateEnabled    uint32 = 8
	stateFocusable  uint32 = 11
	stateFocused    uint32 = 12
	stateMultiLine  uint32 = 17
	statePressed    uint32 = 20
	stateSensitive  uint32 = 24
	stateShowing    uint32 = 25
	stateSingleLine uint32 = 26
	stateVisible    uint32 = 30
)

// atspiRoleInfo is a role's AT-SPI enum value and D-Bus name
// (GetRoleName), the enum's lowercase spelling.
type atspiRoleInfo struct {
	id   uint32
	name string
}

// atspiRoles maps each semantic role to its AT-SPI role, so the enum
// value and the name never disagree; RoleNone's zero entry is FILLER.
var atspiRoles = [...]atspiRoleInfo{
	widget.RoleButton:       {rolePushButton, "push button"},
	widget.RoleLabel:        {roleLabel, "label"},
	widget.RoleEntry:        {roleEntry, "entry"},
	widget.RoleTextArea:     {roleTextArea, "text frame"},
	widget.RoleSlider:       {roleSlider, "slider"},
	widget.RoleSwitch:       {roleSwitch, "switch"},
	widget.RoleCheckBox:     {roleCheckBox, "check box"},
	widget.RoleProgressBar:  {roleProgressBar, "progress bar"},
	widget.RoleLevelBar:     {roleLevelBar, "level bar"},
	widget.RoleScrollArea:   {roleScrollPane, "scroll pane"},
	widget.RoleList:         {roleList, "list"},
	widget.RoleMenu:         {roleMenu, "menu"},
	widget.RoleTabList:      {rolePageTabList, "page tab list"},
	widget.RoleComboBox:     {roleComboBox, "combo box"},
	widget.RoleSplitter:     {roleSplitPane, "split pane"},
	widget.RoleCalendar:     {roleCalendar, "calendar"},
	widget.RoleColorChooser: {roleColorChooser, "color chooser"},
	widget.RoleScrollBar:    {roleScrollBar, "scroll bar"},
	widget.RoleToggleButton: {roleToggleButton, "toggle button"},
	widget.RoleAlert:        {roleNotification, "notification"},
	widget.RoleChart:        {roleChart, "chart"},
}

// atspiRoleOf is r's AT-SPI role: its table entry, FILLER for RoleNone.
func atspiRoleOf(r widget.Role) atspiRoleInfo {
	if int(r) < len(atspiRoles) && atspiRoles[r].id != roleInvalid {
		return atspiRoles[r]
	}
	return atspiRoleInfo{roleFiller, "filler"}
}

// atspiRole maps a semantic role to its AT-SPI enum value.
func atspiRole(r widget.Role) uint32 { return atspiRoleOf(r).id }

// atspiRoleName is the role's D-Bus name (GetRoleName).
func atspiRoleName(r widget.Role) string { return atspiRoleOf(r).name }

// Scene is what the bridge serves: the accessible roots (one per
// window, in window order) and their focus, plus the loop bridge.
// app.Application implements it (app/atspi.go); Roots and Focused are
// only ever called inside Invoke — on the loop goroutine, where the
// widget tree and its routers are consistent.
type Scene interface {
	// Roots returns the window roots in window order; the bridge's
	// synthetic application node parents them.
	Roots() []widget.Widget
	// Focused returns the focused widget of the topmost window, nil
	// when nothing holds focus.
	Focused() widget.Widget
	// Invoke runs fn on the application's loop goroutine, asynchronously.
	Invoke(fn func())
	A11yGeneration() uint64
	// SetFocus moves keyboard focus to w through its window's router
	// (AT-driven focus), failing as widget.Router.SetFocus does; called
	// on the loop goroutine.
	SetFocus(w widget.Widget) error
}

// Options tune Serve. The zero value is the production default.
type Options struct {
	// Address is the accessibility bus address; empty discovers it
	// through org.a11y.Bus.GetAddress on the session bus (which
	// dbus-activates at-spi-bus-launcher on a real desktop).
	Address string
	// Name is the bus name to claim; empty picks
	// org.a11y.atspi.lysui.p<pid> (a bus element may not lead with a
	// digit).
	Name string
	// Poll is the sampling interval. Zero means the default (100ms);
	// negative means no ticker — the embedder drives Refresh itself.
	Poll time.Duration

	emit func(ev event) error
}

// DefaultPoll is the cadence the scene's generation is checked
// at: fast enough that a screen reader hears state changes promptly.
// A check that finds no change costs one atomic load, so an idle scene
// never wakes the loop.
const DefaultPoll = 100 * time.Millisecond

const (
	busTimeout  = 5 * time.Second
	stopTimeout = 3 * time.Second
)

// Bridge is a served scene. Serve starts it; Stop tears it down.
// Every method is safe from any goroutine; snapshots are immutable and
// swapped under the bridge mutex.
type Bridge struct {
	conn  *dbus.Conn
	name  string
	scene Scene
	poll  time.Duration

	mu       sync.Mutex
	ids      map[widget.Widget]int32
	nextID   int32
	exported map[dbus.ObjectPath]bool
	cur      *tree

	// appID is the Id the registry set on the Application interface
	// (its Embed handshake); zero until then.
	appID int32
	// desktop is the registry's desktop as its Embed answered, the parent
	// ATs walk up to from the application root.
	desktop objRef

	stop   chan struct{}
	closed sync.Once
	emit   func(ev event) error

	// sampledGen is the generation the last pass saw, and
	// sampleQueued says a pass waits in the scene's Invoke queue, so a
	// stalled loop accumulates one at most. builds counts the passes.
	// taken is the announcements a pass drained, reused across passes.
	taken []widget.Announcement

	sampledGen   atomic.Uint64
	sampleQueued atomic.Bool
	builds       atomic.Uint64

	// handoff guards what crosses from the loop's pass to the emitter:
	// the finished tree, and the retired nodes coming back. A pass that
	// finds pending taken builds nothing, so a stuck bus costs the loop
	// nothing and the next pass still diffs against what was published.
	handoff  sync.Mutex
	settled  *sync.Cond
	pending  *tree
	applying bool
	exited   bool
	spare    *tree
	kick     chan struct{}
	done     chan struct{}
}

// tree is one sampling pass: the synthetic application root, every
// snapshot keyed by its stable id, and the focused id (-1 when none).
// Immutable once built, until it retires.
type tree struct {
	root  *anode
	nodes map[int32]*anode
	focus int32
	free  []*anode
	// announce are the announcements this pass speaks, on the objects
	// they come from.
	announce []event
}

// anode is one accessible object: the stable id (its object path),
// where it sits, the snapshot it serves, and the interfaces it carries.
type anode struct {
	id     int32
	parent *anode
	index  int
	st     widget.A11yState
	w      widget.Widget
	// iface flags: which AT-SPI subinterfaces the node serves beyond
	// Accessible (Component is always served — every arranged widget
	// has bounds).
	text, action, value bool
	children            []*anode
}

// childser is the tree walk (the widget package's own walkTree is
// private); containers implement it.
type childser interface {
	Children() []widget.Widget
}

// Serve connects to the accessibility bus, exports the scene, embeds
// with the registry, and starts sampling. It waits on the bus, so call
// it off the loop goroutine. The returned error is fatal for the
// bridge; a registry without a Socket.Embed (older at-spi2 watches for
// new bus names instead) is not — it logs and idles.
func Serve(scene Scene, opts Options) (*Bridge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), busTimeout)
	defer cancel()
	address := opts.Address
	if address == "" {
		discovered, err := discoverBus(ctx)
		if err != nil {
			return nil, err
		}
		address = discovered
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		return nil, fmt.Errorf("atspi: connect: %w", err)
	}
	name := opts.Name
	if name == "" {
		// A bus name element may not start with a digit, so the pid is
		// prefixed; the registry tracks apps by unique name anyway.
		name = fmt.Sprintf("org.a11y.atspi.lysui.p%d", os.Getpid())
	}
	var reply uint32
	if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.RequestName", 0, name, uint32(dbus.NameFlagDoNotQueue)).Store(&reply); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("atspi: request name %s: %w", name, err)
	}
	poll := DefaultPoll
	switch {
	case opts.Poll > 0:
		poll = opts.Poll
	case opts.Poll < 0:
		poll = 0
	}
	b := &Bridge{
		conn:     conn,
		name:     name,
		scene:    scene,
		poll:     poll,
		ids:      make(map[widget.Widget]int32),
		exported: make(map[dbus.ObjectPath]bool),
		cur:      &tree{root: &anode{id: 0, index: -1}, nodes: map[int32]*anode{}},
		desktop:  desktopRef,
		stop:     make(chan struct{}),
		kick:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		emit:     opts.emit,
	}
	if b.emit == nil {
		b.emit = func(ev event) error { return conn.Emit(ev.path, ev.name, ev.args...) }
	}
	b.settled = sync.NewCond(&b.handoff)
	// The application node serves its tables from the start; children
	// gain theirs as sampling first sees them.
	b.cur.nodes[0] = b.cur.root
	b.exportNode(0, false, false, false, true)
	b.exportCache()
	go b.emitter()

	// The first sample queues through Invoke — before Run it drains
	// Announcements are heard from now on; the toggle runs on the loop,
	// which owns the queue.
	scene.Invoke(func() { widget.SetAnnouncementsHeard(true) })
	// once the loop starts, which is fine: the registry probes the
	// root asynchronously after the Embed below, and ATs enumerate
	// after that.
	scene.Invoke(b.sample)

	// Embed with the registry; an absent or older registry is not an
	// error (it watches for names and probes the root itself). The
	// socket lives at the desktop root path, where at-spi2-core's
	// registryd and GTK's bridge both speak it.
	registry := conn.Object(proto.RegistryName, proto.RootPath)
	var socket objRef
	if err := registry.CallWithContext(ctx, "org.a11y.atspi.Socket.Embed", 0,
		objRef{Name: name, Path: proto.RootPath}).Store(&socket); err != nil {
		debug.Log("a11y", "registry embed: %v (older registry probes roots itself)", err)
	} else {
		b.mu.Lock()
		b.desktop = socket
		b.mu.Unlock()
	}

	if b.poll > 0 {
		go b.loop()
	}
	return b, nil
}

// discoverBus asks the session bus for the accessibility bus address —
// org.a11y.Bus.GetAddress, which dbus-activates at-spi-bus-launcher on
// a real desktop.
func discoverBus(ctx context.Context) (string, error) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return "", fmt.Errorf("atspi: session bus: %w", err)
	}
	defer func() { _ = bus.Close() }()
	obj := bus.Object("org.a11y.Bus", "/org/a11y/bus")
	var address string
	if err := obj.CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&address); err != nil {
		return "", fmt.Errorf("atspi: org.a11y.Bus.GetAddress: %w", err)
	}
	return address, nil
}

// loop checks the scene at the poll cadence until Stop.
func (b *Bridge) loop() {
	t := time.NewTicker(b.poll)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.check()
		}
	}
}

// check queues a pass when the scene's generation moved since the last one,
// and leaves the loop asleep otherwise.
func (b *Bridge) check() {
	if b.scene.A11yGeneration() == b.sampledGen.Load() || !b.sampleQueued.CompareAndSwap(false, true) {
		return
	}
	b.scene.Invoke(func() {
		b.sampleQueued.Store(false)
		b.sample()
	})
}

// Stop tears the bridge down: the poller and the emitter stop, and the
// bus connection closes. An emitter stuck in a write to a bus that
// stopped reading keeps its goroutine until the write fails. Safe to
// call twice; Serve's error path never returns a bridge.
func (b *Bridge) Stop() {
	b.closed.Do(func() {
		b.scene.Invoke(func() { widget.SetAnnouncementsHeard(false) })
		close(b.stop)
		t := time.NewTimer(stopTimeout)
		defer t.Stop()
		select {
		case <-b.done:
			_ = b.conn.Close()
		case <-t.C:
			debug.Log("a11y", "the emitter is stuck on the bus; closing without it")
			go func() { _ = b.conn.Close() }()
		}
	})
}

// Refresh runs one sampling pass whatever the generation and returns once
// its tree is published and its events sent: for embedders that drive
// sampling themselves (Options.Poll < 0) and for tests. It queues
// through the scene's Invoke like the poller does, so it must not run
// on the loop goroutine.
func (b *Bridge) Refresh() {
	b.settle()
	sampled := make(chan struct{})
	b.scene.Invoke(func() {
		b.sample()
		close(sampled)
	})
	select {
	case <-sampled:
	case <-b.stop:
		return
	}
	b.settle()
}

func (b *Bridge) settle() {
	b.handoff.Lock()
	defer b.handoff.Unlock()
	for (b.pending != nil || b.applying) && !b.exited {
		b.settled.Wait()
	}
}

// sample rebuilds the snapshot tree and hands it to the emitter. Runs
// inside scene.Invoke — the loop goroutine — where reads of the widget
// tree are consistent. While the emitter is behind it builds nothing and
// leaves the generation unseen, so the poller comes back.
func (b *Bridge) sample() {
	gen := b.scene.A11yGeneration()
	b.handoff.Lock()
	idle := b.pending == nil && !b.exited
	b.handoff.Unlock()
	if !idle {
		return
	}
	nt := b.build()
	b.announcements(nt)
	b.builds.Add(1)
	b.sampledGen.Store(gen)
	b.handoff.Lock()
	b.pending = nt
	b.handoff.Unlock()
	select {
	case b.kick <- struct{}{}:
	default:
	}
}

func (b *Bridge) emitter() {
	defer func() {
		b.handoff.Lock()
		b.exited = true
		b.settled.Broadcast()
		b.handoff.Unlock()
		close(b.done)
	}()
	for {
		select {
		case <-b.stop:
			return
		case <-b.kick:
		}
		b.handoff.Lock()
		nt := b.pending
		b.pending = nil
		b.applying = nt != nil
		b.handoff.Unlock()
		if nt == nil {
			continue
		}
		b.apply(nt)
		b.handoff.Lock()
		b.applying = false
		b.settled.Broadcast()
		b.handoff.Unlock()
	}
}

func (b *Bridge) apply(nt *tree) {
	b.mu.Lock()
	old := b.cur
	b.cur = nt
	export := b.exportNew(nt)
	cache := b.cacheEvents(old, nt)
	var gone []int32
	for id := range old.nodes {
		if _, live := nt.nodes[id]; !live {
			gone = append(gone, id)
			delete(b.exported, widgetPath(id))
		}
	}
	b.mu.Unlock()
	for _, ex := range export {
		b.exportNode(ex.id, ex.text, ex.action, ex.value, false)
	}
	for _, ev := range cache {
		b.send(ev)
	}
	for _, ev := range diff(old, nt, b.name) {
		b.send(ev)
	}
	for _, ev := range nt.announce {
		b.send(ev)
	}
	for _, id := range gone {
		b.unexportNode(id)
	}
	b.retire(old)
}

func (b *Bridge) retire(old *tree) {
	clear(old.announce)
	old.announce = old.announce[:0]
	for _, n := range old.nodes {
		kids := n.children
		clear(kids)
		*n = anode{children: kids[:0]}
		old.free = append(old.free, n)
	}
	clear(old.nodes)
	old.root = nil
	b.handoff.Lock()
	defer b.handoff.Unlock()
	if b.spare == nil {
		b.spare = old
	}
}

func (tr *tree) newNode() *anode {
	last := len(tr.free) - 1
	if last < 0 {
		return new(anode)
	}
	n := tr.free[last]
	tr.free[last] = nil
	tr.free = tr.free[:last]
	return n
}

// exportNew describes the nodes the pass saw for the first time.
type exportJob struct {
	id                  int32
	text, action, value bool
}

// build snapshots the scene into a tree, assigning stable ids to
// first-seen widgets.
func (b *Bridge) build() *tree {
	b.handoff.Lock()
	tr := b.spare
	b.spare = nil
	b.handoff.Unlock()
	if tr == nil {
		tr = &tree{nodes: map[int32]*anode{}}
	}
	tr.root, tr.focus = tr.newNode(), -1
	tr.root.index = -1
	tr.nodes[0] = tr.root
	for _, w := range b.scene.Roots() {
		tr.root.children = append(tr.root.children, b.buildFrom(w, tr.root, len(tr.root.children), tr))
	}
	if f := b.scene.Focused(); f != nil {
		if id, ok := b.ids[f]; ok {
			tr.focus = id
		}
	}
	if len(b.ids) != len(tr.nodes)-1 {
		for w, id := range b.ids {
			if _, live := tr.nodes[id]; !live {
				delete(b.ids, w)
			}
		}
	}
	return tr
}

// buildFrom snapshots one widget and its descendants in paint order.
func (b *Bridge) buildFrom(w widget.Widget, parent *anode, index int, tr *tree) *anode {
	id, seen := b.ids[w]
	if !seen {
		b.nextID++
		id = b.nextID
		b.ids[w] = id
	}
	st := widget.Describe(w)
	n := tr.newNode()
	n.id, n.parent, n.index, n.st, n.w = id, parent, index, st, w
	n.text = st.Role == widget.RoleLabel || st.Role == widget.RoleEntry || st.Role == widget.RoleTextArea
	n.action = st.Role == widget.RoleButton || st.Role == widget.RoleToggleButton || st.Role == widget.RoleSwitch || st.Role == widget.RoleCheckBox
	n.value = st.Role == widget.RoleSlider || st.Role == widget.RoleProgressBar
	tr.nodes[id] = n
	if c, ok := w.(childser); ok {
		for i, k := range c.Children() {
			n.children = append(n.children, b.buildFrom(k, n, i, tr))
		}
	}
	return n
}

// exportNew collects the nodes of nt that have no tables yet.
func (b *Bridge) exportNew(nt *tree) []exportJob {
	var out []exportJob
	for id, n := range nt.nodes {
		if id == 0 {
			continue
		}
		path := widgetPath(id)
		if b.exported[path] {
			continue
		}
		b.exported[path] = true
		out = append(out, exportJob{id: id, text: n.text, action: n.action, value: n.value})
	}
	return out
}

// widgetPath is a widget node's object path: the root plus its stable
// id.
func widgetPath(id int32) dbus.ObjectPath {
	return dbus.ObjectPath(string(proto.RootPath) + "/" + strconv.FormatInt(int64(id), 10))
}

// pathOf returns n's object path (the application node's is proto.RootPath).
func pathOf(n *anode) dbus.ObjectPath {
	if n.id == 0 {
		return proto.RootPath
	}
	return widgetPath(n.id)
}

// event is one queued AT-SPI signal: the emitting object's path, the
// full signal name (interface + member), and the argument tuple —
// always (string, int32, int32, variant, dict), the historical AT-SPI
// event shape. The minimal set lives in Event.Object except Focus,
// which the spec keeps in its own Event.Focus interface (deprecated
// in favor of StateChanged focused, still emitted for older
// listeners).
type event struct {
	path dbus.ObjectPath
	name string // full signal name: interface + "." + member
	args []any
}

// diff derives the events between two passes, keyed by the stable ids.
// Order: focus first, then per-node state (stable ascending id), then
// tree changes — a deterministic sequence tests can pin. busName is
// the bridge's own name, carried in child references.
func diff(old, cur *tree, busName string) []event {
	if old == cur {
		return nil
	}
	var out []event

	// Focus: the previous holder loses it, the new one gains it, and
	// the deprecated Focus event rides along for older listeners.
	if old.focus != cur.focus {
		if on, ok := old.nodes[old.focus]; ok && old.focus >= 0 {
			out = append(out, stateEvent(on, "focused", false))
		}
		if on, ok := cur.nodes[cur.focus]; ok && cur.focus >= 0 {
			out = append(out, stateEvent(on, "focused", true),
				event{path: pathOf(on), name: "org.a11y.atspi.Event.Focus.Focus", args: plainArgs()})
		}
	}

	for _, id := range sortedIDs(cur) {
		now, ok := cur.nodes[id]
		if !ok {
			continue
		}
		before, had := old.nodes[id]
		if !had {
			continue
		}
		if now.st.Checked != before.st.Checked {
			out = append(out, stateEvent(now, "checked", now.st.Checked))
		}
		if now.st.Pressed != before.st.Pressed {
			out = append(out, stateEvent(now, "pressed", now.st.Pressed))
		}
		if now.st.Enabled != before.st.Enabled {
			// AT-SPI carries both: enabled is the application state,
			// sensitive the interacts-with-input reading. They move
			// together in lys-ui (IsEnabled is the effective state).
			out = append(out, stateEvent(now, "enabled", now.st.Enabled),
				stateEvent(now, "sensitive", now.st.Enabled))
		}
		if now.st.Editable != before.st.Editable {
			out = append(out, stateEvent(now, "editable", now.st.Editable))
		}
		if now.st.Name != before.st.Name {
			out = append(out, propertyEvent(now, "accessible-name", dbus.MakeVariant(now.st.Name)))
		}
		if now.st.Description != before.st.Description {
			out = append(out, propertyEvent(now, "accessible-description", dbus.MakeVariant(now.st.Description)))
		}
		if now.value && now.st.Value != before.st.Value {
			out = append(out, propertyEvent(now, "accessible-value", dbus.MakeVariant(now.st.Value)))
		}
		if now.text && now.st.Text != before.st.Text {
			out = append(out, textChangedEvents(now, before.st.Text, now.st.Text)...)
		}
		if now.text && selectionOf(now.st) != selectionOf(before.st) {
			out = append(out, event{path: pathOf(now), name: eventObject("TextSelectionChanged"), args: plainArgs()})
		}
		if now.text && now.st.Caret != before.st.Caret {
			out = append(out, event{
				path: pathOf(now), name: eventObject("TextCaretMoved"),
				args: []any{"", intconv.Clamp[int32](now.st.Caret), int32(0), dbus.MakeVariant(int32(0)), map[string]dbus.Variant{}},
			})
		}
		if now.st.Bounds != before.st.Bounds {
			r := now.st.Bounds
			out = append(out, event{
				path: pathOf(now), name: eventObject("BoundsChanged"),
				args: []any{
					"", int32(0), int32(0),
					dbus.MakeVariant(rectOf(r)),
					map[string]dbus.Variant{},
				},
			})
		}
		// Tree updates: children added or removed under this node.
		for i, c := range now.children {
			if i < len(before.children) && before.children[i].id == c.id {
				continue
			}
			out = append(out, childrenEvent(busName, now, "add", i, c))
		}
		if len(before.children) > len(now.children) {
			for i := len(now.children); i < len(before.children); i++ {
				out = append(out, childrenEvent(busName, now, "remove", i, before.children[i]))
			}
		}
	}
	return out
}

// plainArgs is the empty detail tuple (unused string/ints/variant).
func plainArgs() []any {
	return []any{"", int32(0), int32(0), dbus.MakeVariant(int32(0)), map[string]dbus.Variant{}}
}

// eventObject builds an Event.Object signal name.
func eventObject(member string) string {
	return "org.a11y.atspi.Event.Object." + member
}

// propertyEvent is one PropertyChange: the property's name and its new
// value.
func propertyEvent(n *anode, prop string, value dbus.Variant) event {
	return event{
		path: pathOf(n), name: eventObject("PropertyChange"),
		args: []any{prop, int32(0), int32(0), value, map[string]dbus.Variant{}},
	}
}

// textChangedEvents describes an edit as TextChanged events: the
// differing middle between the common prefix and suffix, deleted then
// inserted, at rune offsets.
func textChangedEvents(n *anode, before, after string) []event {
	b, a := []rune(before), []rune(after)
	pre := 0
	for pre < len(b) && pre < len(a) && b[pre] == a[pre] {
		pre++
	}
	suf := 0
	for suf < len(b)-pre && suf < len(a)-pre && b[len(b)-1-suf] == a[len(a)-1-suf] {
		suf++
	}
	var out []event
	change := func(op string, rs []rune) {
		if len(rs) > 0 {
			out = append(out, event{
				path: pathOf(n), name: eventObject("TextChanged"),
				args: []any{op, intconv.Clamp[int32](pre), intconv.Clamp[int32](len(rs)), dbus.MakeVariant(string(rs)), map[string]dbus.Variant{}},
			})
		}
	}
	change("delete", b[pre:len(b)-suf])
	change("insert", a[pre:len(a)-suf])
	return out
}

// selection is the part of a snapshot TextSelectionChanged watches.
type selection struct {
	start, end int
	on         bool
}

func selectionOf(st widget.A11yState) selection {
	return selection{st.SelStart, st.SelEnd, st.HasSelection}
}

// stateEvent is one StateChanged: the state's name, gained or lost.
func stateEvent(n *anode, state string, on bool) event {
	v := int32(0)
	if on {
		v = 1
	}
	return event{
		path: pathOf(n), name: eventObject("StateChanged"),
		args: []any{state, v, int32(0), dbus.MakeVariant(int32(0)), map[string]dbus.Variant{}},
	}
}

// childrenEvent is one ChildrenChanged: add or remove at an index,
// carrying the child's reference in the variant.
func childrenEvent(busName string, parent *anode, op string, index int, child *anode) event {
	return event{
		path: pathOf(parent), name: eventObject("ChildrenChanged"),
		args: []any{
			op, intconv.Clamp[int32](index), int32(0),
			dbus.MakeVariant(objRef{Name: busName, Path: pathOf(child)}),
			map[string]dbus.Variant{},
		},
	}
}

// send emits one event signal on the bus.
func (b *Bridge) send(ev event) {
	if err := b.emit(ev); err != nil {
		debug.Log("a11y", "emit %s: %v", ev.name, err)
	}
}

// announcements turns what the widgets announced since the last pass
// into Announcement events on their objects (at-spi2-core 2.46's
// object:announcement, the politeness as detail1, the text as any_data):
// one each, after the pass's state changes. An announcement from a
// widget no window shows is not spoken.
func (b *Bridge) announcements(nt *tree) {
	b.taken = widget.TakeAnnouncements(b.taken[:0])
	for _, a := range b.taken {
		id, ok := b.ids[a.Widget]
		if !ok {
			continue
		}
		n, ok := nt.nodes[id]
		if !ok {
			continue
		}
		nt.announce = append(nt.announce, event{
			path: pathOf(n), name: eventObject("Announcement"),
			args: []any{"", int32(a.Politeness), int32(0), dbus.MakeVariant(a.Text), map[string]dbus.Variant{}},
		})
	}
	clear(b.taken)
}
