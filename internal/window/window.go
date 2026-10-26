// Package window maps the xdg-shell role onto a wl_surface: real
// toplevel windows with titles, the configure handshake, the
// compositor-confirmed state set, ping/pong liveness, and the close
// signal.
package window

import (
	"errors"
	"fmt"
	"strings"

	xdeco "github.com/neurlang/wayland/unstable/xdg-decoration-v1"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/wlnull"
)

// ErrNotConfigured reports a draw attempt before the first configure
// event. The compositor rejects buffers committed before it.
var ErrNotConfigured = errors.New("window: not configured yet")

// ErrClosed reports that the window was closed; the objects must not be
// used again.
var ErrClosed = errors.New("window: closed")

// Config describes the window before the first commit.
type Config struct {
	// Title and AppID identify the window to the compositor.
	Title, AppID string
	// Width and Height in surface (logical) pixels. Zeros let the
	// compositor pick.
	Width, Height uint32
	// MinWidth and MinHeight constrain how far the compositor may
	// resize the window down; zero axes are unconstrained. The app
	// clamps layout to the same limits (SizeLimits) in case a
	// compositor configures outside them.
	MinWidth, MinHeight uint32
	// MaxWidth and MaxHeight constrain how far the compositor may
	// resize the window up; zero axes are unconstrained.
	MaxWidth, MaxHeight uint32
}

// State is the compositor-confirmed toplevel state: the state array of
// the most recent configure event, never the set of requests the client
// sent. A request (Maximize and friends) is a hint the compositor may
// refuse, delay, or undo in a later configure; only what arrives in the
// state array is true, so UI chrome reads this, not the requests.
//
// There is no minimized bit: the protocol reports no state for a
// minimized (unmapped) window, and State keeps reporting what was last
// confirmed before it.
type State struct {
	Activated  bool // keyboard focus
	Maximized  bool
	Fullscreen bool
	Resizing   bool // an interactive resize (Resize grab) is streaming
	Suspended  bool // the compositor stopped the window (not visible)
	// The tiled_* bits: the compositor snapped the window to an edge.
	TiledLeft, TiledRight, TiledTop, TiledBottom bool
}

// dictatesSize reports a state whose size the compositor sets: the
// window then has no size of its own to keep.
func (s State) dictatesSize() bool {
	return s.Maximized || s.Fullscreen || s.TiledLeft || s.TiledRight || s.TiledTop || s.TiledBottom
}

// String renders the confirmed state set ("activated+maximized"),
// "normal" when it is empty. The order is fixed so traces and dumps
// are comparable.
func (s State) String() string {
	parts := make([]string, 0, 9)
	add := func(on bool, name string) {
		if on {
			parts = append(parts, name)
		}
	}
	add(s.Activated, "activated")
	add(s.Maximized, "maximized")
	add(s.Fullscreen, "fullscreen")
	add(s.Resizing, "resizing")
	add(s.Suspended, "suspended")
	add(s.TiledLeft, "tiled-left")
	add(s.TiledRight, "tiled-right")
	add(s.TiledTop, "tiled-top")
	add(s.TiledBottom, "tiled-bottom")
	if len(parts) == 0 {
		return "normal"
	}
	return strings.Join(parts, "+")
}

// parseStates decodes a configure event's state array (one 32-bit
// xdg_toplevel.state value per entry) into the confirmed set. Unknown
// values are ignored: newer compositors may carry states this build
// does not know.
func parseStates(states []int32) State {
	var s State
	for _, v := range states {
		switch intconv.Uint32(v) {
		case xdg.ToplevelStateActivated:
			s.Activated = true
		case xdg.ToplevelStateMaximized:
			s.Maximized = true
		case xdg.ToplevelStateFullscreen:
			s.Fullscreen = true
		case xdg.ToplevelStateResizing:
			s.Resizing = true
		case xdg.ToplevelStateSuspended:
			s.Suspended = true
		case xdg.ToplevelStateTiledLeft:
			s.TiledLeft = true
		case xdg.ToplevelStateTiledRight:
			s.TiledRight = true
		case xdg.ToplevelStateTiledTop:
			s.TiledTop = true
		case xdg.ToplevelStateTiledBottom:
			s.TiledBottom = true
		}
	}
	return s
}

// Window is one toplevel window and its configure handshake.
type Window struct {
	WLSurface  *wl.Surface
	XdgSurface *xdg.Surface
	Toplevel   *xdg.Toplevel

	decoration *xdeco.ZxdgToplevelDecorationV1
	// dialog is this toplevel's xdg_dialog_v1 object, created on the
	// first SetModal; dialogModalled tracks the live hint; closing the
	// window unsets and destroys it.
	dialog         *wlr.DialogV1
	dialogModalled bool
	exported       *wlr.ZxdgExportedV2
	exportHandle   string

	// onCloseRequest vetoes the compositor's close request when it
	// returns false (an unsaved-changes prompt, for instance); nil
	// means every close request is accepted.
	onCloseRequest func() bool

	// caps is the compositor's window-management capabilities
	// (wm_capabilities, toplevel v5); OnCapabilities hears changes.
	caps           Capabilities
	OnCapabilities func(Capabilities)

	closed     bool
	configured bool
	width      uint32
	height     uint32
	minW, minH uint32
	maxW, maxH uint32
	// floatW and floatH are the size the window last had while nothing
	// dictated it (no fullscreen, maximize, or tiling): what a zero axis
	// returns to once such a state ends, GTK's unconstrained size.
	floatW, floatH uint32
	// state is the compositor-confirmed set from the last configure;
	// requests never write it.
	state State
	// title and parent are the last title and parent set, what a
	// rebuild on a new connection reads back.
	title  string
	parent *Window
}

// New assigns the xdg_toplevel role and sends the initial state. The
// caller must still commit the wl_surface; the compositor answers with
// the first configure round.
func New(wmBase *xdg.WmBase, surf *wl.Surface, cfg Config) (*Window, error) {
	xdgSurf, err := wmBase.GetSurface(surf)
	if err != nil {
		return nil, fmt.Errorf("window: get_xdg_surface: %w", err)
	}
	tl, err := xdgSurf.GetToplevel()
	if err != nil {
		return nil, fmt.Errorf("window: get_toplevel: %w", err)
	}
	w := &Window{
		WLSurface: surf, XdgSurface: xdgSurf, Toplevel: tl,
		width: cfg.Width, height: cfg.Height,
		floatW: cfg.Width, floatH: cfg.Height,
		minW: cfg.MinWidth, minH: cfg.MinHeight,
		maxW: cfg.MaxWidth, maxH: cfg.MaxHeight,
		title: cfg.Title,
	}
	xdgSurf.AddConfigureHandler(w)
	tl.AddConfigureHandler(w)
	tl.AddCloseHandler(w)
	tl.AddWmCapabilitiesHandler(w)

	if err := tl.SetTitle(cfg.Title); err != nil {
		return nil, fmt.Errorf("window: set_title: %w", err)
	}
	if err := tl.SetAppId(cfg.AppID); err != nil {
		return nil, fmt.Errorf("window: set_app_id: %w", err)
	}
	if cfg.MinWidth != 0 || cfg.MinHeight != 0 {
		if err := tl.SetMinSize(intconv.Int32(cfg.MinWidth), intconv.Int32(cfg.MinHeight)); err != nil {
			return nil, fmt.Errorf("window: set_min_size: %w", err)
		}
	}
	if cfg.MaxWidth != 0 || cfg.MaxHeight != 0 {
		if err := tl.SetMaxSize(intconv.Int32(cfg.MaxWidth), intconv.Int32(cfg.MaxHeight)); err != nil {
			return nil, fmt.Errorf("window: set_max_size: %w", err)
		}
	}
	return w, nil
}

// HandleSurfaceConfigure completes the configure round: it marks the
// window configured and acknowledges the serial. Without a wire surface
// (tests) only the state changes.
func (w *Window) HandleSurfaceConfigure(ev xdg.SurfaceConfigureEvent) {
	w.configured = true
	if w.XdgSurface != nil {
		_ = w.XdgSurface.AckConfigure(ev.Serial)
	}
}

// HandleToplevelConfigure records the new size. A zero axis leaves the
// size to the client, per the protocol: the window keeps its size, or
// returns to its unconstrained size when the configure ends a
// fullscreen, maximize, or tiling that dictated it. The state array is
// authoritative: it replaces the confirmed set wholesale, so a
// configure without a state the client asked for reports the request
// as refused (or withdrawn).
func (w *Window) HandleToplevelConfigure(ev xdg.ToplevelConfigureEvent) {
	state := parseStates(ev.States)
	free := !state.dictatesSize()
	released := free && w.state.dictatesSize()
	switch {
	case ev.Width != 0:
		w.width = intconv.Uint32(ev.Width)
	case released && w.floatW != 0:
		w.width = w.floatW
	}
	switch {
	case ev.Height != 0:
		w.height = intconv.Uint32(ev.Height)
	case released && w.floatH != 0:
		w.height = w.floatH
	}
	if free {
		w.floatW, w.floatH = w.width, w.height
	}
	if state != w.state {
		w.state = state
		debug.Log("shell", "toplevel state %s, %dx%d", state, w.width, w.height)
	}
}

// Capabilities is what the compositor's window management offers a
// toplevel. Known is false until the compositor said (wm_capabilities,
// toplevel v5); until then every capability counts as available, the
// protocol's rule.
type Capabilities struct {
	Known                                      bool
	WindowMenu, Maximize, Fullscreen, Minimize bool
}

// all is the assumed set before the compositor speaks.
func (c Capabilities) all() Capabilities {
	if c.Known {
		return c
	}
	return Capabilities{WindowMenu: true, Maximize: true, Fullscreen: true, Minimize: true}
}

// Capabilities reports the compositor's window-management offer.
func (w *Window) Capabilities() Capabilities { return w.caps.all() }

// HandleToplevelWmCapabilities records the compositor's offer.
func (w *Window) HandleToplevelWmCapabilities(ev xdg.ToplevelWmCapabilitiesEvent) {
	c := Capabilities{Known: true}
	for _, v := range ev.Capabilities {
		switch v {
		case xdg.ToplevelWmCapabilitiesWindowMenu:
			c.WindowMenu = true
		case xdg.ToplevelWmCapabilitiesMaximize:
			c.Maximize = true
		case xdg.ToplevelWmCapabilitiesFullscreen:
			c.Fullscreen = true
		case xdg.ToplevelWmCapabilitiesMinimize:
			c.Minimize = true
		}
	}
	w.caps = c
	debug.Log("shell", "wm capabilities %+v", c)
	if w.OnCapabilities != nil {
		w.OnCapabilities(c)
	}
}

// State returns the compositor-confirmed state: what the last
// configure's state array said, not what the client asked for.
func (w *Window) State() State { return w.state }

// Maximize asks the compositor to maximize the window. Like every
// state request it is a hint: it becomes visible only through a
// configure carrying the maximized state, which the compositor is free
// to refuse - judge the outcome through State. A wireless window
// (tests) sends nothing.
func (w *Window) Maximize() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMaximized()
}

// Unmaximize asks the compositor to restore the window. See Maximize.
func (w *Window) Unmaximize() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.UnsetMaximized()
}

// SetTitle updates the compositor-visible title at runtime; the
// xdg_toplevel title is a hint surfaces show in their own chrome
// (taskbars, alt-tab) - CSD apps paint HeaderBar themselves.
func (w *Window) SetTitle(title string) error {
	w.title = title
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetTitle(title)
}

// Title is the title last set (at creation or by SetTitle).
func (w *Window) Title() string { return w.title }

// Parent is the toplevel last set as this one's parent, nil for none.
func (w *Window) Parent() *Window { return w.parent }

// Minimize asks the compositor to minimize the window; a hint
// compositors without a minimized concept ignore.
func (w *Window) Minimize() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMinimized()
}

// Move starts the compositor's xdg_toplevel.move grab: the pointer
// drags the window from the point of the press with the given serial.
// Like resize, the grab replaces the widget press.
func (w *Window) Move(seat *wl.Seat, serial uint32) error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.Move(seat, serial)
}

// Fullscreen asks the compositor to show the window fullscreen; a nil
// output lets the compositor pick (the usual choice). The confirmed
// fullscreen state arrives with the output-sized configure. See
// Maximize.
func (w *Window) Fullscreen(output *wl.Output) error {
	if w.Toplevel == nil {
		return nil
	}
	if output == nil {
		// The protocol's NULL: the compositor picks the output. A plain
		// typed nil cannot ride SendRequest (its ordering pass calls
		// Id() on every proxy argument and panics on a nil receiver), so
		// the zero-value stand-in writes the 0 object word instead.
		output = &wl.Output{}
	}
	return w.Toplevel.SetFullscreen(output)
}

// Unfullscreen asks the compositor to leave fullscreen. See Maximize.
func (w *Window) Unfullscreen() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.UnsetFullscreen()
}

// SetMinimized asks the compositor to minimize the window. The
// protocol defines no minimized state event, so State keeps reporting
// whatever was confirmed before; treat the window as invisible from
// here on. See Maximize.
func (w *Window) SetMinimized() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMinimized()
}

// SetCloseRequest installs a veto callback: returning false keeps the
// window open when the compositor asks it to close (an unsaved-changes
// prompt decides itself when to call Window.Close). Nil accepts every
// close request.
func (w *Window) SetCloseRequest(veto func() bool) { w.onCloseRequest = veto }

// SetMinSize publishes the smallest size the compositor may resize the
// window to; zero axes are unconstrained. The limits are also tracked
// client-side (SizeLimits) so layout clamps to them when a compositor
// configures outside them anyway. A wireless window (tests) records
// the limits without sending anything.
func (w *Window) SetMinSize(width, height uint32) error {
	w.minW, w.minH = width, height
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMinSize(intconv.Int32(width), intconv.Int32(height))
}

// SetMaxSize publishes the largest size the compositor may resize the
// window to; zero axes are unconstrained. See SetMinSize.
func (w *Window) SetMaxSize(width, height uint32) error {
	w.maxW, w.maxH = width, height
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMaxSize(intconv.Int32(width), intconv.Int32(height))
}

// SizeLimits returns the current min/max size in surface (logical)
// pixels; a zero axis is unconstrained.
func (w *Window) SizeLimits() (minW, minH, maxW, maxH int) {
	return int(w.minW), int(w.minH), int(w.maxW), int(w.maxH)
}

// Resize hands the pointer grab to the compositor for an interactive
// resize: it streams configure events as the user drags, and this
// window repaints at every configured size. seat and serial come from
// the button press that started the gesture; edges is a combination of
// the xdg_toplevel.resize_edge bits. A none (zero) edges value or a
// missing seat is a no-op: there is nothing to resize with.
func (w *Window) Resize(seat *wl.Seat, serial uint32, edges uint32) error {
	if w.Toplevel == nil || seat == nil || edges == xdg.ToplevelResizeEdgeNone {
		return nil
	}
	return w.Toplevel.Resize(seat, serial, edges)
}

// ServerDecorated reports whether server-side decorations were
// requested: the compositor then owns the window's borders and resize
// handles, and the client keeps its own edge handles passive. The
// answer reflects the request; the compositor may still decide
// otherwise (mode events are not tracked).
func (w *Window) ServerDecorated() bool { return w.decoration != nil }

// SetParent parents this toplevel to another - a dialog, a toolbox
// that stays above its owner; the compositor keeps it above the parent
// and may group them. A nil parent clears the association.
func (w *Window) SetParent(parent *Window) {
	w.parent = parent
	if w.Toplevel == nil {
		return
	}
	if parent == nil || parent.Toplevel == nil {
		_ = w.Toplevel.Context().SendRequest(w.Toplevel, 1, wlnull.Null)
		return
	}
	_ = w.Toplevel.SetParent(parent.Toplevel)
}

// HandleToplevelClose marks the window closed unless a close-request
// veto rejects it.
func (w *Window) HandleToplevelClose(ev xdg.ToplevelCloseEvent) {
	if w.onCloseRequest != nil && !w.onCloseRequest() {
		return
	}
	w.Close()
}

// EnsureUsable gates drawing: the window must have completed the first
// configure round and must not be closed.
func (w *Window) EnsureUsable() error {
	if w.closed {
		return ErrClosed
	}
	if !w.configured {
		return ErrNotConfigured
	}
	return nil
}

// Closed reports whether the window was closed.
func (w *Window) Closed() bool { return w.closed }

// Close closes the window, from the client side or on the compositor's
// close request: it destroys the toplevel role, which unmaps the
// surface, and marks the window closed for the next loop check. The
// wl_surface stays with its owner. Idempotent.
func (w *Window) Close() {
	w.teardownDialog()
	w.teardownExport()
	w.teardownRole()
	w.closed = true
}

// teardownRole destroys the toplevel's role objects in the order
// xdg-shell requires: the decoration and the toplevel before their
// xdg_surface. Without them the window is wireless, which every
// request method already tolerates.
func (w *Window) teardownRole() {
	if w.decoration != nil {
		_ = w.decoration.Destroy()
		w.decoration = nil
	}
	if w.Toplevel != nil {
		_ = w.Toplevel.Destroy()
		w.Toplevel = nil
	}
	if w.XdgSurface != nil {
		_ = w.XdgSurface.Destroy()
		w.XdgSurface = nil
	}
}

// Size returns the last configured size in surface (logical) pixels. An
// unconfigured window reports zeros.
func (w *Window) Size() (int, int) {
	return int(w.width), int(w.height)
}

// HostSurface returns the underlying wl_surface.
func (w *Window) HostSurface() *wl.Surface { return w.WLSurface }

// TooltipSurface returns the xdg_surface hover tooltips anchor to.
func (w *Window) TooltipSurface() *xdg.Surface { return w.XdgSurface }

// Decorate requests server-side title bars and borders through the
// xdg-decoration manager. A nil manager (compositor without the
// global) is a silent no-op: the window simply stays undecorated.
func (w *Window) Decorate(mgr *xdeco.ZxdgDecorationManagerV1) error {
	if mgr == nil {
		return nil
	}
	dec, err := mgr.GetToplevelDecoration(w.Toplevel)
	if err != nil {
		return err
	}
	w.decoration = dec
	return dec.SetMode(xdeco.ZxdgToplevelDecorationV1ModeServerSide)
}

// SetModal hints, through the optional xdg-dialog-v1 protocol, that
// this parented toplevel is a modal dialog of its parent: the
// compositor then blocks input to the parent itself, where the
// toolkit's application-level block only covers this client's
// windows. The hint rides the surface's next commit, so it should be
// sent before the first one for the compositor to map the dialog
// modal from the start.
//
// A nil manager (compositor without the global) or a wireless window
// (tests) is a silent no-op — modality degrades to the
// application-level block, which callers keep as the floor. The
// dialog object is created lazily on the first call and reused after;
// closing the window unsets and destroys it.
func (w *Window) SetModal(mgr *wlr.WmDialogV1, modal bool) error {
	if mgr == nil || w.Toplevel == nil {
		return nil
	}
	if w.dialog == nil {
		d, err := mgr.GetDialog(w.Toplevel)
		if err != nil {
			return fmt.Errorf("window: get_xdg_dialog: %w", err)
		}
		w.dialog = d
	}
	if modal {
		if err := w.dialog.SetModal(); err != nil {
			return err
		}
		w.dialogModalled = true
		return nil
	}
	w.dialogModalled = false
	return w.dialog.UnsetModal()
}

// Export publishes the toplevel through xdg-foreign so other clients,
// xdg-desktop-portal above all, can name it. The compositor answers
// with the handle asynchronously; ExportHandle reports it once it has
// arrived. A nil exporter (no xdg-foreign v2) is a no-op, and a second
// call keeps the first export.
func (w *Window) Export(exp *wlr.ZxdgExporterV2) error {
	if exp == nil || w.exported != nil {
		return nil
	}
	e, err := exp.ExportToplevel(w.WLSurface)
	if err != nil {
		return fmt.Errorf("window: export_toplevel: %w", err)
	}
	e.AddHandleHandler(exportHandler{w})
	w.exported = e
	return nil
}

// ExportHandle returns the toplevel's xdg-foreign handle, or "" before
// the compositor sent it or without an export.
func (w *Window) ExportHandle() string { return w.exportHandle }

type exportHandler struct{ w *Window }

func (h exportHandler) HandleZxdgExportedV2Handle(ev wlr.ZxdgExportedV2HandleEvent) {
	h.w.exportHandle = ev.Handle
	debug.Log("shell", "toplevel exported as %q", ev.Handle)
}

func (w *Window) teardownExport() {
	if w.exported == nil {
		return
	}
	_ = w.exported.Destroy()
	w.exported = nil
	w.exportHandle = ""
}

// ModalHinted reports whether the xdg-dialog modality hint is set on
// this window (SetModal succeeded with modal true). The compositor's
// own presentation of the hint is not observable.
func (w *Window) ModalHinted() bool { return w.dialog != nil && w.dialogModalled }

// teardownDialog drops the xdg-dialog hint on window teardown: unset,
// then destroy (destroying the object before the toplevel also
// unapplies its effects, per the protocol). Idempotent.
func (w *Window) teardownDialog() {
	if w.dialog == nil {
		return
	}
	_ = w.dialog.UnsetModal()
	_ = w.dialog.Destroy()
	w.dialog = nil
	w.dialogModalled = false
}
