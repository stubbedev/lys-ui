// Package wlsession owns the wl_display connection: registry discovery,
// globals binding, output scale tracking, seat input, and the blocking
// event dispatch.
package wlsession

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	text "github.com/stubbedev/lys-lib/third_party/wayland/unstable/text-input-v3"
	deco "github.com/stubbedev/lys-lib/third_party/wayland/unstable/xdg-decoration-v1"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlclient"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/compose"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/worker"
)

// requiredGlobals are the interfaces no lys-ui surface can exist without, in
// the order they are reported as missing: surfaces and the shm buffers every
// renderer can fall back to. Everything else is feature-detected. A role's
// global fails at the call asking for that role, so a kiosk without layer
// shell runs toplevels, and outputs come and go at runtime, so a compositor
// with none connected is not an error.
var requiredGlobals = []string{"wl_compositor", "wl_shm"}

// minCompositorVersion is the wl_surface version SetBufferScale needs
// (set_buffer_scale is version 3).
const minCompositorVersion = 3

// minDataDeviceVersion is the wl_data_device_manager version the dnd
// action requests and events need (set_actions, finish, dnd_finished).
const minDataDeviceVersion = 3

// Output is one wl_output and its current integer scale.
type Output struct {
	WL    *wl.Output
	Scale int

	// ModeW and ModeH are the current mode's pixel size.
	ModeW, ModeH int

	// Transform is the output's rotation/flip as wl_output.geometry
	// reports it (a wl_output.transform enum); surfaces pinned to the
	// output publish it with wl_surface.set_buffer_transform.
	Transform int32

	// Name and Description are the output's stable xdg-output identity
	// (DP-1, HDMI-A-1) — what compositor configs are written against.
	// Empty when the compositor lacks xdg_output or advertises it
	// below version 2, in which case outputs are only identifiable by
	// registry order.
	Name, Description string

	// LogicalX/Y and LogicalW/H are the output's position and size in
	// the global compositor space, as xdg_output.logical_position and
	// logical_size report them.
	LogicalX, LogicalY, LogicalW, LogicalH int32

	// name is the registry global name, for hotplug removal.
	name uint32

	// xdg is the output's xdg_output object (see xdgoutput.go), nil
	// until the manager is bound or the object is created.
	xdg xdgOutputAPI
}

// Mods is a bitmask of held keyboard modifiers, mirroring the low bits
// of the wayland ModsDepressed map.
type Mods uint8

// Modifier bits: xkb's core modifier indices (Shift, Lock, Control,
// Mod1 for Alt, Mod4 for Super).
const (
	ModShift Mods = 1 << iota
	ModCapsLock
	ModCtrl
	ModAlt
	// ModSuper is Mod4, the logo key.
	ModSuper Mods = 1 << 6
)

// Locks is which lock keys are on, read from the keymap's indicators,
// so a layout that moves Num Lock to another modifier still reads true.
type Locks struct {
	Caps, Num, Scroll bool
}

// KeyboardLayout is the keymap's layouts by name and the one keys type in.
// Names belongs to the session and is never modified.
type KeyboardLayout struct {
	Names  []string
	Active uint32
}

// Session is a connected display with the globals lys-ui needs bound.
type Session struct {
	Display *wl.Display

	registry      *wl.Registry
	compositor    *wl.Compositor
	shm           *wl.Shm
	layerShell    *wlr.ZwlrLayerShellV1
	viewporter    *wlr.WpViewporter
	presentation  *wlr.WpPresentation
	syncobj       *wlr.WpDrmSyncobjManagerV1
	subcompositor *wl.Subcompositor
	dmabuf        *wlr.ZwpDmabufV1
	dmabufVersion uint32
	dmabufLayouts map[DmabufLayout]bool
	// dmabufDevice is the compositor's main device (dev_t) from the
	// linux-dmabuf v4 default feedback; hasDmabufDevice says one came.
	dmabufDevice     uint64
	hasDmabufDevice  bool
	fracScaleManager *wlr.WpScaleManagerV1
	seat             *wl.Seat
	seatVersion      uint32
	seatDev          seatDevices
	pointer          pointerAPI
	keyboard         keyboardAPI
	// touch is the bound wl_touch, touchFocus each contact's surface
	// (touch.go).
	touch      touchAPI
	touchFocus map[int32]*wl.Surface
	// gestures is the touchpad gesture manager and its per-pointer
	// objects; gestureSurface is where the gesture in progress began.
	gestures        *wlr.ZwpGesturesV1
	gesturesVersion uint32
	swipe           *wlr.ZwpGestureSwipeV1
	pinch           *wlr.ZwpGesturePinchV1
	hold            *wlr.ZwpGestureHoldV1
	gestureSurface  *wl.Surface
	// tabletMgr and tabletSeat bind tablet tools (tablet.go).
	tabletMgr  *wlr.ZwpTabletManagerV2
	tabletSeat *wlr.ZwpTabletSeatV2
	// shapeMgr and shapeDevice draw cursors compositor-side (cursorshape.go).
	shapeMgr    *wlr.WpCursorShapeManagerV1
	shapeDevice shapeAPI
	// constraintsMgr locks and confines the pointer, constraints holding
	// each surface's; relativeMgr and relative read raw motion
	// (constraints.go).
	constraintsMgr *wlr.ZwpPointerConstraintsV1
	constraints    map[*wl.Surface]*PointerConstraint
	relativeMgr    *wlr.ZwpRelativePointerManagerV1
	relative       *wlr.ZwpRelativePointerV1
	// idleNotifier watches the seat's idleness (idlenotify.go).
	idleNotifier        *wlr.ExtIdleNotifierV1
	idleNotifierVersion uint32
	// The per-surface hints and the bell (surfacehints.go).
	contentTypeMgr *wlr.WpContentTypeManagerV1
	contentTypes   map[*wl.Surface]*wlr.WpContentTypeV1
	alphaMgr       *wlr.WpAlphaModifierV1
	// blurMgr is ext_background_effect_manager_v1; blurCapable is its
	// last capabilities event's blur bit, and blurSurfaces the
	// per-surface effect objects.
	blurMgr             *wlr.ExtBackgroundEffectManagerV1
	blurCapable         bool
	blurSurfaces        map[*wl.Surface]*wlr.ExtBackgroundEffectSurfaceV1
	alphaSurfaces       map[*wl.Surface]*wlr.WpAlphaModifierSurfaceV1
	bell                *wlr.XdgSystemBellV1
	wmBase              *xdg.WmBase
	wmBaseVersion       uint32
	layerShellVersion   uint32
	compositorVersion   uint32
	outputs             []*Output
	hasArgb             bool
	globals             map[string]bool
	globalVersions      map[string]uint32
	ifaceNames          map[uint32]string
	mods                uint32
	locks               Locks
	layout              KeyboardLayout
	repeatRate          uint32
	repeatDelay         uint32
	xkbContext          *xkb.Context
	xkbKeymap           *xkb.Keymap
	xkbState            *xkb.State
	dataDeviceManager   *wl.DataDeviceManager
	dataDeviceVersion   uint32
	dataDevice          *wl.DataDevice
	inputSerial         uint32
	decorationManager   *deco.ZxdgDecorationManagerV1
	textInputMgr        *text.ZwpInputManagerV3
	textInput           *text.ZwpInputV3
	tiSerial            uint32 // commits sent on textInput; done events compare against it
	tiPending           tiPending
	primarySelectionMgr *wlr.ZwpPrimarySelectionDeviceManagerV1
	primarySelectionDev *wlr.ZwpPrimarySelectionDeviceV1
	// Data-control managers (datacontrol.go); internal/datacontrol
	// creates the device. nil without the protocol.
	extDataControlMgr     *wlr.DataControlManagerV1
	wlrDataControlMgr     *wlr.ZwlrDataControlManagerV1
	wlrDataControlVersion uint32
	globalShortcutsMgr    *wlr.GlobalShortcutsManagerV1
	// comp is the seat's dead-key compose state (compose.go); nil
	// without a compose file, which disables compose entirely.
	comp     *compose.State
	compLoad <-chan *compose.Table

	// Shell-integration protocols (toplevel.go, activation.go,
	// idleinhibit.go, shortinhibit.go, xdgoutput.go, dialog.go), each
	// seen through a narrow interface so tests can substitute recorders;
	// nil on the no-protocol path. The xdg-dialog manager is a plain
	// proxy (its only consumer, window.SetModal, takes it as-is).
	foreignToplevelMgr  foreignToplevelManagerAPI
	toplevels           []*Toplevel
	activation          activationAPI
	activationTokens    []*activationToken
	idleInhibitMgr      idleInhibitAPI
	shortcutsInhibitMgr shortcutsInhibitAPI
	escapeMgr           escapeManagerAPI
	xdgOutputMgr        xdgOutputMaker
	dialogMgr           *wlr.WmDialogV1
	exporter            *wlr.ZxdgExporterV2
	// toplevelIconMgr (toplevelicon.go) posts window icons; iconSizes
	// are the compositor's preferred sizes, complete at iconSizesDone.
	toplevelIconMgr *wlr.ToplevelIconManagerV1
	iconSizes       []int
	iconSizesDone   bool
	// sessionLockMgr (sessionlock.go) is the optional lock-screen
	// protocol; outputWatchers fan output hotplug out to subscribers
	// beyond the OnOutputAdded/OnOutputRemoved hooks.
	sessionLockMgr *wlr.SessionLockManagerV1
	outputWatchers []*outputWatcher

	pointerEnterSerial uint32

	// crs is the cursor state (shape, animation timing, frame
	// timer); see cursor.go.
	crs cursorState

	// OnOutputAdded fires when a wl_output global appears, including
	// after Connect for hotplug; OnOutputRemoved fires when its global
	// goes away. Set them after Connect to drive per-output windows.
	OnOutputAdded   func(*Output)
	OnOutputRemoved func(*Output)

	// Pointer routing state. surfaceHandlers maps a wl_surface to the
	// handler that receives pointer events targeting it; pointerFocus
	// is the surface the compositor says the pointer is over, and
	// grabSurface holds implicit-grab routing while a button is down.
	surfaceHandlers map[*wl.Surface]SurfacePointerHandler
	dropHandlers    map[*wl.Surface]SurfaceDropHandler
	pointerFocus    *wl.Surface
	grabSurface     *wl.Surface

	// keyboardFocus is the surface the compositor gives keyboard input
	// to; keyboard events route through KeyboardFocus().
	keyboardFocus *wl.Surface

	// wheel120 accumulates axis_value120 units per axis (vertical,
	// horizontal) between pointer frames; the frame flushes them as
	// whole-notch scroll deltas.
	wheel120 [2]int32
	// axisValue accumulates the frame's wl_pointer.axis values per
	// axis; axisSource is its axis_source (axisSourced once one came).
	axisValue   [2]float64
	axisSource  uint32
	axisSourced bool
	// axisStopped marks the frame carrying axis_stop: the fingers left
	// the touchpad, the end a kinetic scroll glides from.
	axisStopped bool

	// closed is set by Close, before the connection goes.
	closed atomic.Bool
	// wakeMu guards wakes, the deadlines WakeAfter has armed, and makes
	// arming one atomic with respect to Close.
	wakes    []time.Time
	deadline time.Time
	armed    time.Time
	wakeMu   sync.Mutex

	stalled       bool
	watchingWrite atomic.Bool
	writeWatchers sync.WaitGroup

	// protoErr carries the compositor's fatal wl_display.error, if one
	// arrived, so dispatch failures name the compositor's verdict
	// instead of a bare connection reset.
	protoErr error

	// OnKey fires on key presses (never releases) with the evdev
	// keycode and the held modifiers. Keyboard focus is seat-wide,
	// unlike pointer events which route per surface.
	OnKey func(keycode uint32, mods Mods)
	// OnLocks fires on the loop when a lock key's state changes, the
	// first modifiers event after the keyboard's enter included: Caps
	// Lock already on at startup arrives this way.
	OnLocks func(Locks)

	OnKeyboardLayout func(KeyboardLayout)

	// OnKeyUp fires on key releases with the evdev keycode.
	OnKeyUp func(keycode uint32)

	keyQueue      []queuedKeyEvent
	bg            *worker.Queue
	postedMu      sync.Mutex
	posted        []func()
	hasPosted     atomic.Bool
	compileKeymap func(fd uintptr, size, format uint32) compiledKeymap
	// OnKeyboardFocus fires when keyboard focus moves: an enter, a
	// leave, the keyboard going away, or the focused surface being
	// unregistered (a client-side destroy, which the compositor never
	// answers with a leave). A key held across the move releases
	// wherever focus went, so no release follows here.
	OnKeyboardFocus func()
	// OnIME fires when the input method applies a batch of changes (a
	// done event) — commit, preedit, and surrounding-text deletion in
	// protocol order. Nil without the text-input protocol, or when no
	// host consumes the events.
	OnIME func(IMEEvent)
	// OnIMEFocus fires when text-input focus enters or leaves a
	// surface; committed state is invalidated and must be re-pushed
	// for whichever surface the input method now targets.
	OnIMEFocus func()

	// OnToplevelAdded fires when the compositor advertises a foreign
	// toplevel — including our own windows; OnToplevelRemoved fires
	// when one closes, OnToplevelUpdated when a batch (title, app-id,
	// state) applied. Nil without the foreign-toplevel protocol or
	// when no host consumes the events.
	OnToplevelAdded   func(*Toplevel)
	OnToplevelRemoved func(*Toplevel)
	OnToplevelUpdated func(*Toplevel)

	// OnOutputIdentity fires when an output's stable xdg-output name
	// arrives or changes — the point where an `output "DP-1"` config
	// section can be matched to a hotplugged monitor.
	OnOutputIdentity func(*Output)
}

// SurfacePointerHandler receives the pointer events whose compositor
// focus belongs to one registered surface, with coordinates in that
// surface's logical space. Hosts and popup surfaces register one
// handler each, so multiple surfaces never fight over one callback.
type SurfacePointerHandler interface {
	// HandlePointerEnter reports the pointer entering the surface.
	HandlePointerEnter(x, y float64)
	// HandlePointerMotion reports movement within the surface; while
	// a grab is active it may carry coordinates outside the surface.
	HandlePointerMotion(x, y float64)
	// HandlePointerButton reports a press or release: the wayland
	// button code, 1 pressed / 0 released, and the event serial.
	HandlePointerButton(button, state, serial uint32)
	// HandlePointerAxis reports scroll motion: dx from the horizontal
	// axis (tilt wheels, trackpads), dy from the vertical, both
	// positive right/down.
	HandlePointerAxis(dx, dy float64)
	// HandlePointerLeave reports the pointer leaving the surface.
	HandlePointerLeave()
}

// SurfacePreciseScroller is a SurfacePointerHandler that takes finger
// and continuous scrolling as exact surface pixels (positive
// right/down) instead of axis motion: a touchpad scrolls by what the
// fingers moved, not in wheel steps.
type SurfacePreciseScroller interface {
	HandlePointerScrollPixels(dx, dy float64)
}

// SurfaceScrollEnder is a SurfacePreciseScroller told when a finger
// scroll ends (wl_pointer.axis_stop): the fingers lifted, so a kinetic
// scroller may glide on from the gesture's velocity.
type SurfaceScrollEnder interface {
	HandlePointerScrollEnd()
}

// SurfaceDropHandler receives the wl_data_device drag-and-drop events
// targeting one registered surface, with coordinates in that surface's
// logical space. The offer travels with the enter event; its advertised
// mime types are collected by whoever tracks the offer object
// (internal/dragdrop), leaving the selection bookkeeping of
// internal/clipboard untouched.
type SurfaceDropHandler interface {
	// HandleDragEnter reports a drag entering the surface: position in
	// surface coordinates, the enter serial (the one accept replies
	// with), and the offered data; a nil offer means no transfer.
	HandleDragEnter(x, y float64, serial uint32, offer *wl.DataOffer)
	// HandleDragMotion reports drag movement within the surface.
	HandleDragMotion(x, y float64)
	// HandleDragLeave reports the drag leaving without a drop.
	HandleDragLeave()
	// HandleDrop reports the drop; the payload transfers on request
	// through the offer afterward.
	HandleDrop()
}

// Connect binds the display, waits for the initial registry burst and the
// shm format list, and fails when a required global, the HiDPI-capable
// compositor version, or the ARGB8888 shm format is missing.
func Connect() (*Session, error) {
	d, err := wl.Connect("")
	if err != nil {
		return nil, fmt.Errorf("wlsession: connect: %w", err)
	}
	s := &Session{
		Display:         d,
		globals:         make(map[string]bool),
		globalVersions:  make(map[string]uint32),
		ifaceNames:      make(map[uint32]string),
		surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler),
		dropHandlers:    make(map[*wl.Surface]SurfaceDropHandler),
	}
	d.Context().SetBuffered(true)
	s.loadCompose(compose.Load)

	reg, err := d.GetRegistry()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: registry: %w", err)
	}
	s.registry = reg
	// Record the compositor's fatal protocol errors, so a killed
	// connection reports what the compositor objected to.
	d.AddErrorHandler(s)
	wlclient.RegistryAddListener(reg, s)

	if err := s.Roundtrip(); err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: initial roundtrip: %w", err)
	}
	if missing := missingGlobals(s.globals); len(missing) > 0 {
		s.Close()
		return nil, fmt.Errorf("wlsession: missing wayland globals: %v", missing)
	}
	if s.compositorVersion < minCompositorVersion {
		s.Close()
		return nil, fmt.Errorf("wlsession: wl_compositor v%d lacks set_buffer_scale (need v%d)",
			s.compositorVersion, minCompositorVersion)
	}
	s.logOptionalGlobals()

	if err := s.Roundtrip(); err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: globals roundtrip: %w", err)
	}
	if !s.hasArgb {
		s.Close()
		return nil, errors.New("wlsession: compositor lacks ARGB8888 wl_shm support")
	}
	if err := s.readDmabufFeedback(); err != nil {
		if errors.Is(err, ErrDisconnected) {
			s.Close()
			return nil, fmt.Errorf("wlsession: linux-dmabuf feedback: %w", err)
		}
		logutil.L().Warn("wlsession: linux-dmabuf feedback unreadable; no dmabuf imports", "err", err)
		s.dmabuf = nil
	}
	return s, nil
}

// missingGlobals returns every required interface absent from have.
func missingGlobals(have map[string]bool) []string {
	var missing []string
	for _, g := range requiredGlobals {
		if !have[g] {
			missing = append(missing, g)
		}
	}
	return missing
}

// bindVersion caps a bind at the version we implement.
func bindVersion(advertised, want uint32) uint32 {
	if advertised < want {
		return advertised
	}
	return want
}

// HandleRegistryGlobal implements wl.RegistryGlobalHandler: record the
// global and bind what we need immediately.
func (s *Session) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	s.globals[ev.Interface] = true
	s.globalVersions[ev.Interface] = ev.Version
	s.ifaceNames[ev.Name] = ev.Interface

	switch ev.Interface {
	case "wl_compositor":
		s.compositorVersion = ev.Version
		s.compositor = wlclient.RegistryBindCompositorInterface(s.registry, ev.Name, bindVersion(ev.Version, 4))
	case "wl_shm":
		s.shm = wlclient.RegistryBindShmInterface(s.registry, ev.Name, 1)
		wlclient.ShmAddListener(s.shm, s)
	case "wl_output":
		wlo := wlclient.RegistryBindOutputInterface(s.registry, ev.Name, bindVersion(ev.Version, 2))
		out := &Output{WL: wlo, Scale: 1, name: ev.Name}
		wlclient.OutputAddListener(wlo, &outputEvents{sess: s, out: out})
		s.trackOutput(out)
		s.ensureXdgOutputs()
	case "zwlr_layer_shell_v1":
		ctx := s.registry.Context()
		shell := wlr.NewZwlrLayerShellV1(ctx)
		// v4: set_layer (v2), on-demand keyboard interactivity (v4).
		s.layerShellVersion = bindVersion(ev.Version, 4)
		_ = s.registry.Bind(ev.Name, ev.Interface, s.layerShellVersion, shell)
		s.layerShell = shell
	case "wp_viewporter":
		ctx := s.registry.Context()
		vp := wlr.NewWpViewporter(ctx)
		if !s.bindOptional(ev, 1, vp) {
			return
		}
		s.viewporter = vp
	case "wp_presentation":
		p := wlr.NewWpPresentation(s.registry.Context())
		if !s.bindOptional(ev, 1, p) {
			return
		}
		s.presentation = p
	case "wp_linux_drm_syncobj_manager_v1":
		m := wlr.NewWpDrmSyncobjManagerV1(s.registry.Context())
		if !s.bindOptional(ev, 1, m) {
			return
		}
		s.syncobj = m
	case "wp_fractional_scale_manager_v1":
		ctx := s.registry.Context()
		mgr := wlr.NewWpScaleManagerV1(ctx)
		if !s.bindOptional(ev, 1, mgr) {
			return
		}
		s.fracScaleManager = mgr
	case "wl_seat":
		s.seat = wlclient.RegistryBindSeatInterface(s.registry, ev.Name, bindVersion(ev.Version, 7))
		s.seatVersion = bindVersion(ev.Version, 7)
		s.seatDev = wireSeat{seat: s.seat}
		wlclient.SeatAddListener(s.seat, s)
		s.ensureDataDevice()
		s.ensureTextInput()
		s.ensurePrimarySelectionDevice()
		s.ensureTabletSeat()
	case "xdg_wm_base":
		ctx := s.registry.Context()
		wmBase := xdg.NewShell(ctx)
		s.wmBaseVersion = bindVersion(ev.Version, 5)
		_ = s.registry.Bind(ev.Name, ev.Interface, s.wmBaseVersion, wmBase)
		xdg.WmBaseAddListener(wmBase, s)
		s.wmBase = wmBase
	case "wl_data_device_manager":
		ctx := s.registry.Context()
		s.dataDeviceManager = wl.NewDataDeviceManager(ctx)
		s.dataDeviceVersion = bindVersion(ev.Version, minDataDeviceVersion)
		_ = s.registry.Bind(ev.Name, ev.Interface, s.dataDeviceVersion, s.dataDeviceManager)
		s.ensureDataDevice()
	case "zxdg_decoration_manager_v1":
		ctx := s.registry.Context()
		s.decorationManager = deco.NewZxdgDecorationManagerV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 2), s.decorationManager)
	case "zwp_text_input_manager_v3":
		s.bindTextInputManager(ev)
	case "zwp_primary_selection_device_manager_v1":
		s.bindPrimarySelectionManager(ev)
	case "zwlr_foreign_toplevel_manager_v1":
		s.bindForeignToplevelManager(ev)
	case "xdg_activation_v1":
		s.bindActivation(ev)
	case "zwp_idle_inhibit_manager_v1":
		s.bindIdleInhibitManager(ev)
	case "zwp_pointer_gestures_v1":
		s.bindPointerGestures(ev)
	case "zwp_tablet_manager_v2":
		s.bindTabletManager(ev)
	case "wp_cursor_shape_manager_v1":
		s.bindCursorShapeManager(ev)
	case "zwp_pointer_constraints_v1":
		s.bindPointerConstraints(ev)
	case "zwp_relative_pointer_manager_v1":
		s.bindRelativePointer(ev)
	case "ext_idle_notifier_v1":
		s.bindIdleNotifier(ev)
	case "wp_content_type_manager_v1", "wp_alpha_modifier_v1", "xdg_system_bell_v1", "ext_background_effect_manager_v1":
		s.bindSurfaceHint(ev)
	case "zwp_keyboard_shortcuts_inhibit_manager_v1":
		s.bindShortcutsInhibitManager(ev)
	case "lys_escape_manager_v1":
		s.bindEscapeManager(ev)
	case "zxdg_output_manager_v1":
		s.bindXdgOutputManager(ev)
	case "xdg_wm_dialog_v1":
		s.bindDialogManager(ev)
	case "zxdg_exporter_v2":
		s.bindExporter(ev)
	case "wl_subcompositor":
		s.bindSubcompositor(ev)
	case "zwp_linux_dmabuf_v1":
		s.bindDmabuf(ev)
	case "ext_session_lock_manager_v1":
		s.bindSessionLockManager(ev)
	case "xdg_toplevel_icon_manager_v1":
		s.bindToplevelIconManager(ev)
	case "ext_data_control_manager_v1":
		s.bindExtDataControlManager(ev)
	case "zwlr_data_control_manager_v1":
		s.bindWlrDataControlManager(ev)
	case "hyprland_global_shortcuts_manager_v1":
		s.bindGlobalShortcutsManager(ev)
	}
}

// ensureDataDevice creates the seat's data device once both the manager
// and the seat are bound, whichever arrives first.
func (s *Session) ensureDataDevice() {
	if s.dataDevice != nil || s.dataDeviceManager == nil || s.seat == nil {
		return
	}
	dev, err := s.dataDeviceManager.GetDataDevice(s.seat)
	if err != nil {
		logutil.L().Debug("wlsession: wl_data_device bind failed; drag-and-drop and clipboard transfer disabled",
			slog.Any("err", err))
		return
	}
	// The session routes the drag-and-drop events per surface; the
	// selection and data_offer events go to internal/clipboard and
	// internal/dragdrop, which register their own handlers.
	dev.AddEnterHandler(s)
	dev.AddLeaveHandler(s)
	dev.AddMotionHandler(s)
	dev.AddDropHandler(s)
	s.dataDevice = dev
}

// HandleRegistryGlobalRemove implements wl.RegistryGlobalRemoveHandler:
// drop the global and, for outputs, run the hotplug hook so hosts on
// that output can tear themselves down.
func (s *Session) HandleRegistryGlobalRemove(ev wl.RegistryGlobalRemoveEvent) {
	iface, ok := s.ifaceNames[ev.Name]
	if !ok {
		return
	}
	if iface == "wl_output" {
		for i, out := range s.outputs {
			if out.name != ev.Name {
				continue
			}
			s.outputs = append(s.outputs[:i], s.outputs[i+1:]...)
			delete(s.ifaceNames, ev.Name)
			s.notifyOutputRemoved(out)
			return
		}
	}
	delete(s.globals, iface)
	delete(s.globalVersions, iface)
	delete(s.ifaceNames, ev.Name)
}

// trackOutput records a discovered output in arrival order and fires
// the hotplug hook. Separated from the registry bind so the
// bookkeeping is testable without a live connection.
func (s *Session) trackOutput(out *Output) {
	s.outputs = append(s.outputs, out)
	s.notifyOutputAdded(out)
}

// HandleShmFormat implements wl.ShmFormatHandler.
func (s *Session) HandleShmFormat(ev wl.ShmFormatEvent) {
	if ev.Format == wl.ShmFormatArgb8888 {
		s.hasArgb = true
	}
}

// outputEvents tracks one output's state; the wayland handlers carry no
// back-reference, so each output gets its own listener.
type outputEvents struct {
	sess *Session
	out  *Output
}

// HandleOutputScale implements wl.OutputScaleHandler.
func (e *outputEvents) HandleOutputScale(ev wl.OutputScaleEvent) {
	if ev.Factor > 0 {
		e.out.Scale = int(ev.Factor)
	}
}

// HandleOutputGeometry implements wl.OutputGeometryHandler: the event's
// transform is what rotated outputs need surfaces to publish through
// wl_surface.set_buffer_transform.
func (e *outputEvents) HandleOutputGeometry(ev wl.OutputGeometryEvent) {
	e.out.Transform = ev.Transform
}

// HandleOutputMode implements wl.OutputModeHandler: the current mode
// (flag bit 0) records the output's pixel size, the fallback for layer
// surfaces with automatic axes.
func (e *outputEvents) HandleOutputMode(ev wl.OutputModeEvent) {
	if ev.Flags&1 != 0 {
		e.out.ModeW = int(ev.Width)
		e.out.ModeH = int(ev.Height)
	}
}

// HandleOutputDone implements wl.OutputDoneHandler.
func (e *outputEvents) HandleOutputDone(wl.OutputDoneEvent) {}

// seat capability bits.
const (
	capPointer  = 1
	capKeyboard = 2
	// capTouch binds wl_touch (touch.go). This overturns the earlier
	// recorded non-goal of no touch pipeline (#105).
	capTouch = 4
)

// minSeatReleaseVersion is the wl_seat version that introduced the
// wl_pointer.release and wl_keyboard.release destructor requests. On
// older seats the objects are dropped without a release request; the
// compositor destroys them with the capability either way.
const minSeatReleaseVersion = 5

// seatDevices is the wl_seat request side the capability path drives:
// acquiring the pointer and keyboard objects as their capabilities
// arrive. wireSeat adapts the generated proxy; tests substitute a
// recorder so unplug-replug cycles run without a live connection.
type seatDevices interface {
	GetPointer() (pointerAPI, error)
	GetKeyboard() (keyboardAPI, error)
}

// pointerAPI is the request and listener side of wl_pointer the
// session drives: the cursor push, listener wiring at bind time, and
// the release the spec asks for on capability loss. wirePointer
// adapts the generated proxy; tests substitute a recorder.
type pointerAPI interface {
	SetCursor(serial uint32, surface *wl.Surface, hotspotX, hotspotY int32) error
	Release() error
	AddListener(h wlclient.PointerListener)
}

// keyboardAPI is the listener-plus-release side of wl_keyboard;
// wireKeyboard adapts the generated proxy, tests substitute a
// recorder.
type keyboardAPI interface {
	Release() error
	AddListener(h wlclient.KeyboardListener)
}

// wireSeat adapts the generated wl_seat proxy to seatDevices: the
// generated GetPointer/GetKeyboard return concrete proxies, the wrap
// hands back the narrow interfaces.
type wireSeat struct{ seat *wl.Seat }

// GetPointer implements seatDevices.
func (w wireSeat) GetPointer() (pointerAPI, error) {
	p, err := w.seat.GetPointer()
	if err != nil {
		return nil, err
	}
	return wirePointer{p: p}, nil
}

// GetKeyboard implements seatDevices.
func (w wireSeat) GetKeyboard() (keyboardAPI, error) {
	k, err := w.seat.GetKeyboard()
	if err != nil {
		return nil, err
	}
	return wireKeyboard{k: k}, nil
}

// wirePointer adapts the generated wl_pointer proxy to pointerAPI;
// the wlclient listener helper becomes a method.
type wirePointer struct{ p *wl.Pointer }

// SetCursor implements pointerAPI.
func (w wirePointer) SetCursor(serial uint32, surface *wl.Surface, hotspotX, hotspotY int32) error {
	return w.p.SetCursor(serial, surface, hotspotX, hotspotY)
}

// Release implements pointerAPI.
func (w wirePointer) Release() error { return w.p.Release() }

// AddListener implements pointerAPI.
func (w wirePointer) AddListener(h wlclient.PointerListener) { wlclient.PointerAddListener(w.p, h) }

// wireKeyboard adapts the generated wl_keyboard proxy to keyboardAPI.
type wireKeyboard struct{ k *wl.Keyboard }

// Release implements keyboardAPI.
func (w wireKeyboard) Release() error { return w.k.Release() }

// AddListener implements keyboardAPI.
func (w wireKeyboard) AddListener(h wlclient.KeyboardListener) { wlclient.KeyboardAddListener(w.k, h) }

// HandleSeatCapabilities implements wl.SeatCapabilitiesHandler. The
// set arrives at bind time and again whenever it changes — a USB
// mouse unplugged, a keyboard switched, libinput re-probing. The
// pointer and keyboard objects track it: when a capability disappears
// the compositor destroys the matching object, so the stale proxy is
// released and dropped, and a fresh one is created on the next gain,
// or input goes silent after an unplug-replug.
func (s *Session) HandleSeatCapabilities(ev wl.SeatCapabilitiesEvent) {
	s.handleCapabilities(ev.Capabilities&capPointer != 0, ev.Capabilities&capKeyboard != 0)
	s.handleTouchCapability(ev.Capabilities&capTouch != 0)
}

// handleCapabilities is the capability transition state machine,
// split from the event so tests can drive synthetic unplug-replug
// cycles through a recorder seat. Idempotent: the compositor may
// re-send an unchanged set at any time.
func (s *Session) handleCapabilities(hasPointer, hasKeyboard bool) {
	if s.seatDev == nil {
		return
	}
	if !hasPointer && s.pointer != nil {
		debug.Log("seat", "pointer capability lost")
		s.pointerLost()
	}
	if hasPointer && s.pointer == nil {
		p, err := s.seatDev.GetPointer()
		if err != nil {
			logutil.L().Debug("wlsession: get_pointer failed; pointer input disabled", slog.Any("err", err))
		} else {
			p.AddListener(s)
			s.pointer = p
			s.ensurePointerGestures()
			s.ensureCursorShape()
			s.ensureRelativePointer()
			s.preloadCursors()
			debug.Log("seat", "pointer capability gained")
		}
	}
	if !hasKeyboard && s.keyboard != nil {
		debug.Log("seat", "keyboard capability lost")
		s.keyboardLost()
	}
	if hasKeyboard && s.keyboard == nil {
		k, err := s.seatDev.GetKeyboard()
		if err != nil {
			logutil.L().Debug("wlsession: get_keyboard failed; keyboard input disabled", slog.Any("err", err))
		} else {
			k.AddListener(s)
			s.keyboard = k
			// The text-input object hangs off the seat proxy, not the
			// keyboard, so the IME enable state survives this loss;
			// when the compositor re-establishes focus for the new
			// keyboard it sends text-input enter, and the OnIMEFocus
			// hook makes the host re-push its state.
			debug.Log("seat", "keyboard capability gained")
		}
	}
	// Re-enter the seat-attached setup paths after every transition.
	// All three are guarded no-ops while their objects are bound; they
	// act only when an earlier attempt failed or the manager global
	// arrived after the seat. Deliberately eager rather than lazy at
	// next use: the setup is cheap and idempotent, and laziness would
	// push capability awareness into the clipboard and IME paths.
	s.ensureDataDevice()
	s.ensureTextInput()
	s.ensurePrimarySelectionDevice()
}

// pointerLost tears the pointer down because its capability went
// away: wl_pointer.release (seat v5+) tells the compositor the client
// is done with the doomed object, and every routing state that depends
// on it ends cleanly — the implicit grab drops, the surfaces holding
// pointer state hear a leave (hover clears; the host cancels any
// in-flight widget drag), and the cursor frame timer stops. The
// desired shape survives so the next enter re-applies it.
func (s *Session) pointerLost() {
	s.dropPointerGestures()
	s.dropCursorShape()
	s.dropRelativePointer()
	if p := s.pointer; p != nil {
		s.pointer = nil
		if s.seatVersion >= minSeatReleaseVersion {
			_ = p.Release()
		}
	}
	s.crs.mu.Lock()
	s.pointerEnterSerial = 0
	s.crs.mu.Unlock()
	s.stopCursorAnim()
	s.closeCursorPool()
	grab := s.grabSurface
	s.grabSurface = nil
	s.resetAxisFrame()
	focus := s.pointerFocus
	s.pointerFocus = nil
	if focus != nil {
		if h := s.surfaceHandlers[focus]; h != nil {
			h.HandlePointerLeave()
		}
	}
	// The grabbed surface hears the loss too when it differs from the
	// focus: a drag whose pointer died must end on the surface that
	// holds it, not only on wherever focus had drifted.
	if grab != nil && grab != focus {
		if h := s.surfaceHandlers[grab]; h != nil {
			h.HandlePointerLeave()
		}
	}
}

// keyboardLost tears the keyboard down because its capability went
// away: release (seat v5+), drop the proxy, and clear the state that
// belongs to the device — the held modifiers and the focus surface.
// The keymap state is kept: the compositor announces a fresh keymap
// when the next keyboard arrives, and keeping it keeps KeyUTF8
// answering in the gap.
func (s *Session) keyboardLost() {
	if k := s.keyboard; k != nil {
		s.keyboard = nil
		if s.seatVersion >= minSeatReleaseVersion {
			_ = k.Release()
		}
	}
	if s.keyboardQueued() {
		s.queueKeyEvent(s.forgetKeyboard)
		return
	}
	s.forgetKeyboard()
}

func (s *Session) forgetKeyboard() {
	s.mods = 0
	s.loseKeyboardFocus()
}

// loseKeyboardFocus forgets the focused surface: a half-typed compose
// sequence must not commit into the next surface's widgets, and the
// keyboard hook hears the move.
func (s *Session) loseKeyboardFocus() {
	s.keyboardFocus = nil
	s.comp.Cancel()
	s.keyboardFocusMoved()
}

func (s *Session) keyboardFocusMoved() {
	if s.OnKeyboardFocus != nil {
		s.OnKeyboardFocus()
	}
}

// SetSurfaceInput registers h as the receiver of pointer events
// targeting surf; h == nil unregisters. Handlers must be registered
// before the surface can receive input and removed when the surface
// is destroyed.
func (s *Session) SetSurfaceInput(surf *wl.Surface, h SurfacePointerHandler) {
	if h == nil {
		s.releaseConstraint(surf)
		s.releaseSurfaceHints(surf)
		delete(s.surfaceHandlers, surf)
		s.forgetFocus(surf)
		return
	}
	s.surfaceHandlers[surf] = h
}

// forgetFocus drops surf from every piece of seat focus as it goes away:
// the compositor names a destroyed surface as nil in the leave that may
// follow, which could not match it any more.
func (s *Session) forgetFocus(surf *wl.Surface) {
	if surf == s.keyboardFocus {
		s.loseKeyboardFocus()
	}
	if surf == s.pointerFocus {
		s.pointerFocus = nil
	}
	if surf == s.grabSurface {
		s.grabSurface = nil
	}
	if surf == s.gestureSurface {
		s.gestureSurface = nil
	}
	maps.DeleteFunc(s.touchFocus, func(_ int32, focus *wl.Surface) bool { return focus == surf })
}

// surfaceID is surf's wire id for a trace line; an event names a surface
// the client already destroyed as nil.
func surfaceID(surf *wl.Surface) wl.ProxyId {
	if surf == nil {
		return 0
	}
	return surf.Id()
}

// SetSurfaceDrop registers h as the receiver of drag-and-drop events
// targeting surf; h == nil unregisters. It mirrors SetSurfaceInput for
// the wl_data_device half of the input model.
func (s *Session) SetSurfaceDrop(surf *wl.Surface, h SurfaceDropHandler) {
	if h == nil {
		delete(s.dropHandlers, surf)
		return
	}
	if s.dropHandlers == nil {
		s.dropHandlers = make(map[*wl.Surface]SurfaceDropHandler)
	}
	s.dropHandlers[surf] = h
}

// HandleDataDeviceEnter implements wl.DataDeviceEnterHandler: a drag
// entered a surface; route it to that surface's drop handler.
func (s *Session) HandleDataDeviceEnter(ev wl.DataDeviceEnterEvent) {
	debug.Log("input", "wire dnd enter surf=%d (%.1f,%.1f) serial=%d",
		surfaceID(ev.Surface), ev.X, ev.Y, ev.Serial)
	if h := s.dropHandlers[ev.Surface]; h != nil {
		h.HandleDragEnter(float64(ev.X), float64(ev.Y), ev.Serial, ev.Id)
	}
}

// HandleDataDeviceLeave implements wl.DataDeviceLeaveHandler: the drag
// ended over every surface. The event names no surface, so every
// registered handler hears it; only the one holding a drag acts.
func (s *Session) HandleDataDeviceLeave(wl.DataDeviceLeaveEvent) {
	debug.Log("input", "wire dnd leave")
	for _, h := range s.dropHandlers {
		h.HandleDragLeave()
	}
}

// HandleDataDeviceMotion implements wl.DataDeviceMotionHandler.
func (s *Session) HandleDataDeviceMotion(ev wl.DataDeviceMotionEvent) {
	for _, h := range s.dropHandlers {
		h.HandleDragMotion(float64(ev.X), float64(ev.Y))
	}
	debug.Log("input", "wire dnd motion (%.1f,%.1f)", ev.X, ev.Y)
}

// HandleDataDeviceDrop implements wl.DataDeviceDropHandler.
func (s *Session) HandleDataDeviceDrop(wl.DataDeviceDropEvent) {
	debug.Log("input", "wire dnd drop")
	for _, h := range s.dropHandlers {
		h.HandleDrop()
	}
}

// pointerTarget returns the handler pointer events route to right
// now: the grabbed surface while a button is held, else the surface
// under the pointer. Nil when neither applies or nothing is
// registered.
func (s *Session) pointerTarget() SurfacePointerHandler {
	surf := s.grabSurface
	if surf == nil {
		surf = s.pointerFocus
	}
	if surf == nil {
		return nil
	}
	return s.surfaceHandlers[surf]
}

// HandleSeatName implements wl.SeatNameHandler.
func (s *Session) HandleSeatName(wl.SeatNameEvent) {}

// HandleWmBasePing implements xdg.WmBasePingHandler: the compositor is
// asking whether the client is alive, and a client that does not pong
// is treated as hung (Hyprland raises its not-responding dialog over
// it). The answer goes out from the dispatch itself, so it only fails
// to arrive when the loop really is stuck.
func (s *Session) HandleWmBasePing(ev xdg.WmBasePingEvent) {
	if err := s.wmBase.Pong(ev.Serial); err != nil {
		logutil.L().Debug("wlsession: pong failed", slog.Any("serial", ev.Serial), slog.Any("err", err))
	}
}

// HandlePointerEnter implements wl.PointerEnterHandler: the pointer
// gained focus on ev.Surface. The enter is routed to that surface's
// handler and becomes the motion target until a leave or grab says
// otherwise.
func (s *Session) HandlePointerEnter(ev wl.PointerEnterEvent) {
	s.crs.mu.Lock()
	s.pointerEnterSerial = ev.Serial
	s.crs.mu.Unlock()
	_ = s.applyCursor()
	s.pointerFocus = ev.Surface
	debug.Log("input", "wire enter surf=%d (%.1f,%.1f)",
		surfaceID(ev.Surface), ev.SurfaceX, ev.SurfaceY)
	if h := s.surfaceHandlers[ev.Surface]; h != nil {
		h.HandlePointerEnter(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
}

// HandlePointerLeave implements wl.PointerLeaveHandler: pointer focus
// left ev.Surface. A leave always ends the client-side grab: the
// compositor only strips focus when the pointer truly left or the
// grabbing input device went away.
func (s *Session) HandlePointerLeave(ev wl.PointerLeaveEvent) {
	debug.Log("input", "wire leave surf=%d", surfaceID(ev.Surface))
	grabbed := s.grabSurface != nil
	s.grabSurface = nil
	if s.pointerFocus == ev.Surface {
		s.pointerFocus = nil
	}
	// Restore the default shape unless an implicit grab keeps the
	// pointer: a drag (slider, resize) must not see its cursor flip
	// to the arrow just because the pointer crossed the surface edge.
	// Shapes stick across surfaces of one client, so without the
	// restore the next surface would inherit whatever the old one
	// showed.
	if !grabbed {
		_ = s.restoreCursor()
	}
	if h := s.surfaceHandlers[ev.Surface]; h != nil {
		h.HandlePointerLeave()
	}
}

// HandlePointerMotion implements wl.PointerMotionHandler: routed to
// the grabbed surface during a drag, else the focused surface.
func (s *Session) HandlePointerMotion(ev wl.PointerMotionEvent) {
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerMotion(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
	debug.Log("input", "wire motion (%.1f,%.1f)", ev.SurfaceX, ev.SurfaceY)
}

// HandlePointerButton implements wl.PointerButtonHandler: routed like
// motion, and a press opens the client-side implicit grab so drags
// keep feeding the pressed surface after the pointer leaves it. A
// release ends the grab.
func (s *Session) HandlePointerButton(ev wl.PointerButtonEvent) {
	debug.Log("input", "wire button %d state=%d serial=%d", ev.Button, ev.State, ev.Serial)
	if ev.State == 1 {
		s.inputSerial = ev.Serial
	}
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerButton(ev.Button, ev.State, ev.Serial)
	}
	switch ev.State {
	case 1:
		if s.grabSurface == nil {
			s.grabSurface = s.pointerFocus
		}
	case 0:
		s.grabSurface = nil
	}
}

// Axis orientation enum values on the wire.
const (
	axisVertical   = 0
	axisHorizontal = 1
)

// wheelPerStep converts axis_value120 units to the smooth-axis scale
// the app treats as one scroll step (10): one wheel click is 120 units.
const wheelPerStep = 12.0

// HandlePointerAxis implements wl.PointerAxisHandler: the axis value
// of this frame accumulates per axis and the frame routes it (a seat
// before version 5 has no frames, so it routes at once). Scrolling
// follows the grab while dragging.
func (s *Session) HandlePointerAxis(ev wl.PointerAxisEvent) {
	axis := 0
	if ev.Axis != axisVertical {
		axis = 1
	}
	debug.Log("input", "wire axis %d value=%.2f", axis, float64(ev.Value))
	if s.seatVersion < 5 {
		dx, dy := 0.0, 0.0
		if axis == 0 {
			dy = float64(ev.Value)
		} else {
			dx = float64(ev.Value)
		}
		if h := s.pointerTarget(); h != nil {
			h.HandlePointerAxis(dx, dy)
		}
		return
	}
	s.axisValue[axis] += float64(ev.Value)
}

// HandlePointerFrame implements wl.PointerFrameHandler: routes the
// frame's scroll. Wheel notches (axis_discrete, or axis_value120) go
// out as whole steps, several in one frame being the wheel's
// acceleration; finger and continuous scrolling goes out as exact
// pixels to a handler that takes them (SurfacePreciseScroller), the
// rest as plain axis motion.
func (s *Session) HandlePointerFrame(wl.PointerFrameEvent) {
	dv, dh := s.wheel120[0], s.wheel120[1]
	vv, vh := s.axisValue[0], s.axisValue[1]
	source, sourced, stopped := s.axisSource, s.axisSourced, s.axisStopped
	s.resetAxisFrame()
	h := s.pointerTarget()
	if h == nil {
		return
	}
	precise := sourced && (source == wl.PointerAxisSourceFinger || source == wl.PointerAxisSourceContinuous)
	switch {
	case dv != 0 || dh != 0:
		h.HandlePointerAxis(float64(dh)/wheelPerStep, float64(dv)/wheelPerStep)
	case vv == 0 && vh == 0:
	case precise:
		if p, ok := h.(SurfacePreciseScroller); ok {
			p.HandlePointerScrollPixels(vh, vv)
		} else {
			h.HandlePointerAxis(vh, vv)
		}
	default:
		h.HandlePointerAxis(vh, vv)
	}
	// The stop frame carries the finger source and no motion; it ends
	// the gesture for a handler that glides.
	if stopped && (!sourced || source == wl.PointerAxisSourceFinger) {
		if e, ok := h.(SurfaceScrollEnder); ok {
			e.HandlePointerScrollEnd()
		}
	}
}

// resetAxisFrame drops the scroll gathered for a frame.
func (s *Session) resetAxisFrame() {
	s.wheel120 = [2]int32{}
	s.axisValue = [2]float64{}
	s.axisSource, s.axisSourced = 0, false
	s.axisStopped = false
}

// HandlePointerAxisSource implements wl.PointerAxisSourceHandler: what
// the frame's scroll comes from.
func (s *Session) HandlePointerAxisSource(ev wl.PointerAxisSourceEvent) {
	s.axisSource, s.axisSourced = ev.AxisSource, true
}

// HandlePointerAxisStop implements wl.PointerAxisStopHandler.
// The stop lands in the next frame (seats 5+); an older seat has no
// frames and no stop.
func (s *Session) HandlePointerAxisStop(wl.PointerAxisStopEvent) { s.axisStopped = true }

// HandlePointerAxisDiscrete implements wl.PointerAxisDiscreteHandler
// (seats 5-7): wheel notches, counted as 120 units each like
// axis_value120.
func (s *Session) HandlePointerAxisDiscrete(ev wl.PointerAxisDiscreteEvent) {
	s.wheel120[min(ev.Axis, 1)] += ev.Discrete * 120
}

// HandlePointerAxisValue120 implements wl.PointerAxisValue120Handler
// (seats 8+): wheel clicks per axis, one click 120 units, signed as
// the axis is (positive down or right).
func (s *Session) HandlePointerAxisValue120(ev wl.PointerAxisValue120Event) {
	s.wheel120[min(ev.Axis, 1)] += ev.Value120
}

// dropKeymap forgets the keymap a replacement for failed to arrive: key codes
// mean what the new keymap says, so reading them through the old one acts on
// the wrong key. Until a keymap compiles, keys translate to nothing.
func (s *Session) dropKeymap(err error) {
	logutil.L().Error("wlsession: keyboard input is untranslated until a keymap compiles", slog.Any("err", err))
	s.xkbKeymap, s.xkbState = nil, nil
	s.syncLocks()
	s.syncLayoutNames()
}

// HandleKeyboardEnter implements wl.KeyboardEnterHandler: the named
// surface receives keyboard input until a leave or another enter.
func (s *Session) HandleKeyboardEnter(ev wl.KeyboardEnterEvent) {
	if s.keyboardQueued() {
		s.queueKeyEvent(func() { s.deliverKeyboardEnter(ev) })
		return
	}
	s.deliverKeyboardEnter(ev)
}

func (s *Session) deliverKeyboardEnter(ev wl.KeyboardEnterEvent) {
	debug.Log("input", "keyboard enter")
	s.keyboardFocus = ev.Surface
	s.keyboardFocusMoved()
}

// HandleKeyboardLeave implements wl.KeyboardLeaveHandler.
func (s *Session) HandleKeyboardLeave(wl.KeyboardLeaveEvent) {
	if s.keyboardQueued() {
		s.queueKeyEvent(s.loseKeyboardFocus)
		return
	}
	s.loseKeyboardFocus()
}

// KeyboardFocus returns the surface currently holding keyboard input,
// or nil when the compositor gives the seat no keyboard focus.
func (s *Session) KeyboardFocus() *wl.Surface { return s.keyboardFocus }

// HandleKeyboardKey implements wl.KeyboardKeyHandler.
func (s *Session) HandleKeyboardKey(ev wl.KeyboardKeyEvent) {
	if s.keyboardQueued() {
		s.queueKeyEvent(func() { s.deliverKeyboardKey(ev) })
		return
	}
	s.deliverKeyboardKey(ev)
}

func (s *Session) deliverKeyboardKey(ev wl.KeyboardKeyEvent) {
	debug.Log("input", "wire key code=%d state=%d serial=%d mods=%#x", ev.Key, ev.State, ev.Serial, s.mods)
	switch ev.State {
	case 1:
		s.inputSerial = ev.Serial
		if s.OnKey != nil {
			s.OnKey(ev.Key, s.Mods())
		}
	case 0:
		if s.OnKeyUp != nil {
			s.OnKeyUp(ev.Key)
		}
	}
}

// HandleKeyboardModifiers implements wl.KeyboardModifiersHandler.
func (s *Session) HandleKeyboardModifiers(ev wl.KeyboardModifiersEvent) {
	if s.keyboardQueued() {
		s.queueKeyEvent(func() { s.deliverKeyboardModifiers(ev) })
		return
	}
	s.deliverKeyboardModifiers(ev)
}

func (s *Session) deliverKeyboardModifiers(ev wl.KeyboardModifiersEvent) {
	debug.Log("input", "wire modifiers depressed=%#x latched=%#x locked=%#x group=%d", ev.ModsDepressed, ev.ModsLatched, ev.ModsLocked, ev.Group)
	s.mods = ev.ModsDepressed
	if s.xkbState != nil {
		s.xkbState.UpdateMask(xkb.ModMask(ev.ModsDepressed), xkb.ModMask(ev.ModsLatched), xkb.ModMask(ev.ModsLocked), 0, 0, ev.Group)
	}
	s.syncLocks()
	s.syncActiveLayout()
}

func (s *Session) activeLayout() uint32 {
	if s.xkbState == nil {
		return 0
	}
	return s.xkbState.SerializeLayout(xkb.StateLayoutEffective)
}

func (s *Session) syncLayoutNames() {
	var names []string
	if s.xkbKeymap != nil {
		names = make([]string, s.xkbKeymap.NumLayouts())
		for i := range names {
			names[i] = s.xkbKeymap.LayoutName(uint32(i))
		}
	}
	now := KeyboardLayout{Names: names, Active: s.activeLayout()}
	if slices.Equal(now.Names, s.layout.Names) && now.Active == s.layout.Active {
		return
	}
	s.layout = now
	s.notifyLayout()
}

func (s *Session) syncActiveLayout() {
	active := s.activeLayout()
	if active == s.layout.Active {
		return
	}
	s.layout.Active = active
	s.notifyLayout()
}

func (s *Session) notifyLayout() {
	if s.OnKeyboardLayout != nil {
		s.OnKeyboardLayout(s.layout)
	}
}

func lockActive(state *xkb.State, mod, led string) bool {
	return state.ModNameIsActive(mod, xkb.StateModLocked) || state.LEDNameIsActive(led)
}

func (s *Session) syncLocks() {
	var now Locks
	if s.xkbState != nil {
		now = Locks{
			Caps:   lockActive(s.xkbState, "Lock", "Caps Lock"),
			Num:    lockActive(s.xkbState, "NumLock", "Num Lock"),
			Scroll: lockActive(s.xkbState, "ScrollLock", "Scroll Lock"),
		}
	}
	if now == s.locks {
		return
	}
	s.locks = now
	if s.OnLocks != nil {
		s.OnLocks(now)
	}
}

// KeyUTF8 translates an evdev keycode through the compositor's keymap
// and returns the text it produces with the current modifiers (shift,
// AltGr, layout). Empty when no keymap arrived or the key types nothing,
// which holds for Return, Tab, Backspace, Escape and Ctrl chords: their
// control characters are actions, handled through KeySym. Keycodes need the +8 evdev-to-xkb offset.
func (s *Session) KeyUTF8(code uint32) string {
	if s.xkbState == nil {
		return ""
	}
	var buf [16]byte
	text := s.xkbState.AppendKeyUTF8(buf[:0], xkb.Keycode(code+8))
	if len(text) == 1 && (text[0] < 0x20 || text[0] == 0x7f) {
		return ""
	}
	return string(text)
}

// KeySym translates an evdev keycode to its primary keysym under the
// current state; KeyNoSymbol when there is no keymap.
func (s *Session) KeySym(code uint32) xkb.Keysym {
	if s.xkbState == nil {
		return xkb.KeyNoSymbol
	}
	return s.xkbState.KeyGetOneSym(xkb.Keycode(code + 8))
}

// Mods returns the currently held modifiers (shift, ctrl, alt,
// super); locks such as Caps and Num Lock are not held keys.
func (s *Session) Mods() Mods {
	return Mods(s.mods & uint32(ModShift|ModCtrl|ModAlt|ModSuper))
}

// Locks returns which lock keys are on. It reads all off until the
// compositor has sent a keymap and a modifiers event.
func (s *Session) Locks() Locks { return s.locks }

// KeyboardLayout returns the keymap's layouts and the active one; no
// names until the compositor has sent a keymap.
func (s *Session) KeyboardLayout() KeyboardLayout { return s.layout }

// HandleKeyboardRepeatInfo implements wl.KeyboardRepeatInfoHandler: the
// compositor's rate (keys per second) and delay (milliseconds).
func (s *Session) HandleKeyboardRepeatInfo(ev wl.KeyboardRepeatInfoEvent) {
	s.repeatRate = intconv.Clamp[uint32](ev.Rate)
	s.repeatDelay = intconv.Clamp[uint32](ev.Delay)
}

// RepeatInfo returns the compositor's key repeat rate in keys per second
// and the initial delay in milliseconds. Zeros mean the compositor sent
// no repeat info; callers fall back to their own defaults.
func (s *Session) RepeatInfo() (rate, delayMs uint32) {
	return s.repeatRate, s.repeatDelay
}

// Compositor returns the bound wl_compositor.
func (s *Session) Compositor() *wl.Compositor { return s.compositor }

// Shm returns the bound wl_shm.
func (s *Session) Shm() *wl.Shm { return s.shm }

// LayerShell returns the bound zwlr_layer_shell_v1.
func (s *Session) LayerShell() *wlr.ZwlrLayerShellV1 { return s.layerShell }

// Viewporter returns the bound wp_viewporter, or nil when the
// compositor does not provide it; fractional scaling degrades to
// integer set_buffer_scale without it.
func (s *Session) Viewporter() *wlr.WpViewporter { return s.viewporter }

// Presentation returns the bound wp_presentation, or nil when the
// compositor does not provide it.
func (s *Session) Presentation() *wlr.WpPresentation { return s.presentation }

// FractionalScaleManager returns the bound
// wp_fractional_scale_manager_v1, or nil when the compositor does not
// provide it; without it there are no preferred_scale events and
// surfaces keep their integer scale.
func (s *Session) FractionalScaleManager() *wlr.WpScaleManagerV1 { return s.fracScaleManager }

// Outputs returns the bound outputs in registry order.
func (s *Session) Outputs() []*Output { return s.outputs }

// WmBase returns the bound xdg_wm_base, or nil when the compositor does
// not provide it; window support needs it.
func (s *Session) WmBase() *xdg.WmBase { return s.wmBase }

// WmBaseVersion is the bound xdg_wm_base version (popup repositioning
// needs 3); 0 without one.
func (s *Session) WmBaseVersion() uint32 { return s.wmBaseVersion }

// LayerShellVersion is the bound zwlr_layer_shell_v1 version, 0 when
// the compositor has none.
func (s *Session) LayerShellVersion() uint32 { return s.layerShellVersion }

// Seat returns the bound wl_seat, or nil when the compositor has none.
func (s *Session) Seat() *wl.Seat { return s.seat }

// DataDeviceManager returns the bound wl_data_device_manager, or nil
// when the compositor does not provide it; clipboard support needs it.
func (s *Session) DataDeviceManager() *wl.DataDeviceManager { return s.dataDeviceManager }

// DataDevice returns the seat's data device, or nil before both the
// manager and the seat are bound.
func (s *Session) DataDevice() *wl.DataDevice { return s.dataDevice }

// DataDeviceVersion returns the negotiated wl_data_device_manager
// version: three and above have the dnd action requests and events
// (set_actions, finish), below that drags run in the v1 subset.
func (s *Session) DataDeviceVersion() uint32 { return s.dataDeviceVersion }

// PrimarySelectionManager returns the bound primary selection device
// manager, or nil when the compositor does not provide it; primary
// selection support needs it.
func (s *Session) PrimarySelectionManager() *wlr.ZwpPrimarySelectionDeviceManagerV1 {
	return s.primarySelectionMgr
}

// InputSerial returns the serial of the latest user input event the
// seat delivered to this client: a pointer button press, key press,
// touch down, or tablet tool down or button press, on any surface. It
// is the serial requests anchored to a user action carry (popup
// grabs, interactive move and resize, set_selection, activation
// tokens), and 0 before any such event.
func (s *Session) InputSerial() uint32 { return s.inputSerial }

// ErrNoInputEvent reports a request the protocol anchors to a user
// action made before the seat delivered any input event: there is no
// serial for it to carry, and compositors refuse it without one.
var ErrNoInputEvent = errors.New("wlsession: no user input event has arrived to anchor the request")

// DecorationManager returns the bound xdg-decoration manager, or nil
// when the compositor does not provide it; server-side window
// decorations need it.
func (s *Session) DecorationManager() *deco.ZxdgDecorationManagerV1 {
	return s.decorationManager
}

// Roundtrip issues a display sync and dispatches until it completes.
// It waits on its own callback's done handler, so a handler it
// dispatches may roundtrip too (a popup opened from a press that
// arrived while another popup awaited its configure): the nested
// roundtrip dispatching this one's done still ends this wait. Each
// dispatch is a Step, so destroyed proxies and dead connections are
// handled as Step handles them.
func (s *Session) Roundtrip() error {
	cb, err := s.Display.Sync()
	if err != nil {
		return s.asDisconnect(err)
	}
	defer cb.Unregister()
	var synced syncDone
	cb.AddDoneHandler(&synced)
	for !synced {
		if err := s.Step(); err != nil {
			return err
		}
	}
	return nil
}

// AfterSync runs fn from the dispatch of the compositor's answer to a
// display sync sent now: by then the compositor has processed every
// request the client sent before the call. Its done handler is in place
// before the request goes out, so no dispatch can miss the answer. On a
// closed session or a dead connection it fails with ErrDisconnected and
// fn never runs.
func (s *Session) AfterSync(fn func()) error {
	if s.Closed() {
		return fmt.Errorf("wlsession: sync after Close: %w", ErrDisconnected)
	}
	ctx := s.Display.Context()
	cb := wl.NewCallback(ctx)
	cb.AddDoneHandler(&afterSync{cb: cb, fn: fn})
	if err := ctx.SendRequest(s.Display, displaySyncOpcode, cb); err != nil {
		cb.Unregister()
		return s.asDisconnect(err)
	}
	return nil
}

// afterSync runs fn once its sync callback fires.
type afterSync struct {
	cb *wl.Callback
	fn func()
}

func (a *afterSync) HandleCallbackDone(wl.CallbackDoneEvent) {
	a.cb.Unregister()
	a.fn()
}

type syncDone bool

func (d *syncDone) HandleCallbackDone(wl.CallbackDoneEvent) { *d = true }

// Step dispatches one event, blocking until one arrives, then every
// event already pending: those its read buffered and those one more
// non-blocking read brings in. It is the park point of an event-driven
// loop: with nothing to do, a loop calling Step holds no CPU and wakes
// only when the compositor sends something or a WakeAfter kick fires,
// and a burst of events costs the loop one pass, not one per event.
func (s *Session) Step() error {
	err := s.step()
	s.runPosted()
	return err
}

func (s *Session) step() error {
	ctx := s.Display.Context()
	wasStalled := s.stalled
	if pending, err := s.flush(ctx); err != nil {
		return err
	} else if wasStalled && !pending {
		return nil
	}
	if err := runThroughProxyNil(ctx.Run); errors.Is(err, wl.ErrContextRunTimeout) {
		return s.wakeDue(ctx)
	} else if err := s.asDisconnect(err); err != nil {
		return err
	}
	if err := s.dispatchBuffered(ctx); err != nil {
		return err
	}
	read, err := ctx.ReadReady()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return s.wakeDue(ctx)
	}
	if err != nil {
		return s.asDisconnect(err)
	}
	if !read {
		return nil
	}
	return s.dispatchBuffered(ctx)
}

func (s *Session) dispatchBuffered(ctx *wl.Context) error {
	for ctx.Buffered() {
		if err := ctx.Run(); err != nil && !errors.Is(err, wl.ErrContextRunProxyNil) {
			return s.asDisconnect(err)
		}
	}
	return nil
}

func (s *Session) flush(ctx *wl.Context) (bool, error) {
	pending, err := ctx.Flush()
	if err != nil {
		return true, s.asDisconnect(fmt.Errorf("wlsession: flush requests: %w", err))
	}
	s.stalled = pending
	if pending && s.watchingWrite.CompareAndSwap(false, true) {
		s.writeWatchers.Go(func() { s.watchWritable(ctx) })
	}
	return pending, nil
}

func (s *Session) watchWritable(ctx *wl.Context) {
	if err := ctx.WaitWritable(); err != nil && !s.closed.Load() {
		logutil.L().Error("wlsession: wait for room in the socket", slog.Any("err", err))
	}
	s.watchingWrite.Store(false)
	s.WakeAfter(0)
}

// Stalled reports whether the compositor has left requests of an earlier
// pass unread, trying once more to write them: a frame committed now would
// only queue behind them. It never blocks, and a loop parked in Step wakes
// when the socket takes writes again.
func (s *Session) Stalled() (bool, error) {
	if !s.stalled {
		return false, nil
	}
	return s.flush(s.Display.Context())
}

// HandleDisplayError implements wl.DisplayErrorHandler: record the
// compositor's fatal protocol error so the dispatch failure that ends
// the run names the compositor's verdict, trace it, and report it at
// Error on the injected logger — a protocol error is terminal: the
// connection is dead, and reconnecting would replay the fatal
// exchange. Registered in Connect; a closed connection without an
// error event leaves protoErr nil, which is itself diagnostic (the
// compositor dropped us without saying why).
func (s *Session) HandleDisplayError(ev wl.DisplayErrorEvent) {
	s.protoErr = fmt.Errorf("%s: error %d: %s", objectName(ev.ObjectId), ev.Code, ev.Message)
	debug.Log("wire", "compositor fatal: %v", s.protoErr)
	logutil.L().Error("wlsession: compositor fatal protocol error", slog.Any("err", s.protoErr))
}

// objectName names a protocol object for a log line: its proxy type
// and its wire id, as in *wl.Surface@23; nil for an object the client
// no longer knows.
func objectName(p wl.Proxy) string {
	if p == nil {
		return "unknown object"
	}
	return fmt.Sprintf("%T@%d", p, p.Id())
}

// withProtoErr decorates a dispatch failure with the recorded protocol
// error, if any.
func (s *Session) withProtoErr(err error) error {
	if err != nil && s.protoErr != nil {
		return fmt.Errorf("%w (compositor: %w)", err, s.protoErr)
	}
	return err
}

// WakeAfter arranges for a loop parked in Step to return by waiting d,
// through the socket's read deadline: no request reaches the compositor,
// no goroutine waits, and Step returning is the whole cost. Safe from any
// goroutine; after Close it does nothing.
func (s *Session) WakeAfter(d time.Duration) {
	at := time.Now().Add(max(d, 0))
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.closed.Load() || s.Display == nil {
		return
	}
	s.wakes = append(s.wakes, at)
	s.armEarliest(s.Display.Context())
}

// SetDeadline is the loop's one timer: a loop parked in Step returns at at,
// replacing whatever at the last call named, and a zero at means none. Setting
// the same instant again costs nothing.
func (s *Session) SetDeadline(at time.Time) {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if at.Equal(s.deadline) || s.closed.Load() || s.Display == nil {
		return
	}
	s.deadline = at
	s.armEarliest(s.Display.Context())
}

// wakeDue drops the wakes a read deadline just answered and arms the
// earliest one left.
func (s *Session) wakeDue(ctx *wl.Context) error {
	now := time.Now()
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.closed.Load() {
		return nil
	}
	kept := s.wakes[:0]
	for _, at := range s.wakes {
		if at.After(now) {
			kept = append(kept, at)
		}
	}
	s.wakes = kept
	if !s.deadline.After(now) {
		s.deadline = time.Time{}
	}
	s.armed = now
	s.armEarliest(ctx)
	return nil
}

func (s *Session) armEarliest(ctx *wl.Context) {
	next := s.deadline
	for _, at := range s.wakes {
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	if next.Equal(s.armed) {
		return
	}
	s.armed = next
	if err := ctx.SetReadDeadline(next); err != nil {
		logutil.L().Error("wlsession: arm a loop wake", slog.Any("err", err))
	}
}

// FrameDone receives a frame callback's answer: a listener that takes no
// allocation per frame, and the compositor's timestamp for the animation clock.
type FrameDone struct {
	Ready bool
	MS    uint32
}

// HandleCallbackDone implements wl.CallbackDoneHandler. The delete_id that
// follows a done retires the callback for the next frame to reuse.
func (f *FrameDone) HandleCallbackDone(ev wl.CallbackDoneEvent) {
	debug.Log("frame", "frame callback done")
	f.Ready, f.MS = true, ev.CallbackData
}

// displaySyncOpcode is wl_display.sync's request opcode.
const displaySyncOpcode = 0

// Closed reports whether Close ran.
func (s *Session) Closed() bool { return s.closed.Load() }

// Close disconnects from the display and releases the session's shared
// buffer arena: the pool proxy, mapping, and the session's one fd.
func (s *Session) Close() {
	s.stopBackground()
	s.closeWire()
	s.writeWatchers.Wait()
}

func (s *Session) closeWire() {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	if s.closed.Swap(true) {
		return // idempotent: the disconnect teardown and a deferred Close both run
	}
	s.stopCursorAnim()
	buffer.CloseArenas(s.shm)
	if s.Display == nil {
		return
	}
	ctx := s.Display.Context()
	if _, err := ctx.Flush(); err != nil {
		debug.Log("wire", "wlsession: last flush before closing: %v", err)
	}
	_ = ctx.Close()
}

// AxisSteps converts a smooth axis value to scroll steps (one per 10
// units), keeping at least one step when the value is nonzero: what
// every surface's HandlePointerAxis hands its router.
func AxisSteps(v float64) int {
	steps := int(v / 10)
	if v != 0 && steps == 0 {
		steps = 1
		if v < 0 {
			steps = -1
		}
	}
	return steps
}
