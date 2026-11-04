package wlsession

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xcursor"
)

// defaultCursor is shown whenever the app has not asked for another
// shape: the classic arrow.
const defaultCursor = CursorLeftPtr

// CursorHidden is the shape name that hides the pointer over a surface
// (GTK's "none"): set_cursor with no surface, no theme lookup. A
// frozen-screen color picker draws its own loupe in its place.
const CursorHidden = "none"

// cursorThemeSize is the nominal cursor size in pixels; XCURSOR_SIZE
// overrides it.
const cursorThemeSize = 24

var errCursorPending = errors.New("cursor still being read")

// cursorState is everything the cursor path owns. The mutex
// serializes event-loop calls (SetCursor, pointer enter/leave)
// against the frame-timer goroutine that advances animated cursors.
type cursorState struct {
	mu sync.Mutex

	desired string // requested shape, as SetCursor received it
	shown   string // theme name the last apply resolved to
	surface *wl.Surface

	// Animation bookkeeping: the shown theme cursor, its per-frame
	// delays and loop total, when the loop began, the pending frame
	// timer, and a generation that drops stale timers once another
	// apply superseded them.
	cur     *themedCursor
	delays  []uint32
	totalMs uint32
	start   time.Time
	gen     uint64
	timer   *time.Timer
	armed   bool

	// pool holds the shapes read so far. requested is what was asked of
	// the background goroutine, reader its own; waiting says the last
	// apply stopped short on a read in flight.
	pool      *cursorTheme
	requested map[string]bool
	reader    *cursorReader
	waiting   bool

	// now, after, theme, push, readFrames and readFile stand in for the
	// animation clock, the frame timer, the xcursor theme, the wire push
	// and the file reads in tests; nil in production.
	now        func() time.Time
	after      func(time.Duration, func())
	theme      func(desired string) (*themedCursor, string, error)
	push       func(*cursorImage) error
	readFrames func(name string) ([]xcursor.Image, error)
	readFile   func(path string) (xcursor.Image, error)
}

// cursorAliases maps requested shape names to the xcursor names to
// try, best first. Window edges and corners go by the toolkit's
// resize_* spellings; the CSS spellings themes ship themselves and
// the classic core names follow. Whatever the loaded theme lacks is
// skipped, and a shape with no candidate at all degrades to the
// arrow.
var cursorAliases = map[string][]string{
	// Edge and corner resize, toolkit spellings.
	"resize_n":    {"top_side", "ns-resize", "sb_v_double_arrow", "size_ver"},
	"resize_s":    {"bottom_side", "ns-resize", "sb_v_double_arrow", "size_ver"},
	"resize_e":    {"right_side", "ew-resize", "sb_h_double_arrow", "size_hor"},
	"resize_w":    {"left_side", "ew-resize", "sb_h_double_arrow", "size_hor"},
	"resize_nw":   {"top_left_corner", "nwse-resize", "fd_double_arrow"},
	"resize_se":   {"bottom_right_corner", "nwse-resize", "fd_double_arrow"},
	"resize_ne":   {"top_right_corner", "nesw-resize", "bd_double_arrow"},
	"resize_sw":   {"bottom_left_corner", "nesw-resize", "bd_double_arrow"},
	"resize_ns":   {"ns-resize", "sb_v_double_arrow", "size_ver"},
	"resize_ew":   {"ew-resize", "sb_h_double_arrow", "size_hor"},
	"resize_nwse": {"nwse-resize", "fd_double_arrow"},
	"resize_nesw": {"nesw-resize", "bd_double_arrow"},
	// The CSS spellings, in case a caller prefers them.
	"n-resize":    {"top_side", "ns-resize", "sb_v_double_arrow"},
	"s-resize":    {"bottom_side", "ns-resize", "sb_v_double_arrow"},
	"e-resize":    {"right_side", "ew-resize", "sb_h_double_arrow"},
	"w-resize":    {"left_side", "ew-resize", "sb_h_double_arrow"},
	"nw-resize":   {"top_left_corner", "nwse-resize", "fd_double_arrow"},
	"se-resize":   {"bottom_right_corner", "nwse-resize", "fd_double_arrow"},
	"ne-resize":   {"top_right_corner", "nesw-resize", "bd_double_arrow"},
	"sw-resize":   {"bottom_left_corner", "nesw-resize", "bd_double_arrow"},
	"ns-resize":   {"ns-resize", "sb_v_double_arrow", "size_ver"},
	"ew-resize":   {"ew-resize", "sb_h_double_arrow", "size_hor"},
	"nwse-resize": {"nwse-resize", "fd_double_arrow"},
	"nesw-resize": {"nesw-resize", "bd_double_arrow"},
	"col-resize":  {"col-resize", "sb_h_double_arrow"},
	"row-resize":  {"row-resize", "sb_v_double_arrow"},
	"all-scroll":  {"fleur", "move", "all-scroll"},
	// The busy shapes are the animated ones in most themes; the frame
	// timer below advances them.
	"wait":     {"watch"},
	"progress": {"left_ptr_watch", "progress", "watch"},
	// Hand shapes and friends under either spelling.
	"pointer":  {"hand1", "pointer"},
	"text":     {"xterm", "text"},
	"default":  {defaultCursor},
	"move":     {"move", "fleur"},
	"grab":     {"grab", "hand1"},
	"grabbing": {"grabbing", "closedhand", "hand2"},
	// The remaining CSS keywords under their own spellings, with the
	// classic core names the themes ship as fallbacks.
	"crosshair":     {"crosshair", "cross"},
	"context-menu":  {"context-menu", "contextmenu"},
	"help":          {"question_arrow", "help"},
	"cell":          {"plus", "cell"},
	"vertical-text": {"vertical-text", "xterm"},
	"no-drop":       {"no-drop", "no_drop"},
	"not-allowed":   {"not-allowed", "forbidden", "crossed_circle"},
	"zoom-in":       {"zoom-in", "zoom_in"},
	"zoom-out":      {"zoom-out", "zoom_out"},
}

// cursorCandidates lists the xcursor names a requested shape can
// resolve to, best first; empty means the default arrow. Unlisted
// names pass through unchanged.
func cursorCandidates(desired string) []string {
	if desired == "" {
		return []string{defaultCursor}
	}
	if c, ok := cursorAliases[desired]; ok {
		return c
	}
	return []string{desired}
}

// effectiveCursor resolves a requested name to a real shape.
func effectiveCursor(desired string) string {
	if desired == "" {
		return defaultCursor
	}
	return desired
}

// SetCursor shows the named xcursor shape (CursorLeftPtr, "xterm",
// "grabbing", ...) on the pointer. Window-edge interactions can use the
// toolkit aliases - "resize_e", "resize_nw", the CSS spellings
// ("ns-resize", "e-resize", ...) - which resolve to the shapes the
// loaded theme actually carries and degrade to the arrow when it has
// none. Animated shapes (watch, left_ptr_watch) advance their frames
// on a timer for as long as they show. CursorHidden ("none") hides
// the pointer. The choice sticks across pointer enters; "" or an
// unknown name falls back to the arrow. It is a no-op before the
// pointer or the compositor surfaces exist.
func (s *Session) SetCursor(name string) error {
	s.crs.mu.Lock()
	s.crs.desired = effectiveCursor(name)
	s.crs.mu.Unlock()
	return s.applyCursor()
}

// restoreCursor drops any special shape back to the default arrow.
// It runs on pointer leave: xcursor shapes stick across surfaces of
// one client, so the shape a surface asked for would otherwise bleed
// onto whichever surface the pointer enters next.
func (s *Session) restoreCursor() error { return s.SetCursor("") }

// applyCursor pushes the current cursor shape to the compositor: one
// shm buffer from the theme, attached to a dedicated cursor surface.
// Animated cursors leave a frame timer armed behind it.
func (s *Session) applyCursor() error {
	c := &s.crs
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopTimer()
	c.waiting = false
	if s.pointer == nil || s.pointerEnterSerial == 0 {
		return nil
	}
	if c.surface == nil {
		surf, err := s.Compositor().CreateSurface()
		if err != nil {
			return fmt.Errorf("cursor surface: %w", err)
		}
		c.surface = surf
		debug.Log("input", "cursor surface %d created", surf.Id())
	}
	c.gen++
	if fc, ok := parseFileCursor(c.desired); ok {
		c.shown, c.cur = c.desired, nil
		return s.pushFileCursorLocked(fc)
	}
	return s.applyNamedCursorLocked(c.desired)
}

// applyNamedCursorLocked shows the named shape: through the
// cursor-shape protocol when the compositor offers it, hidden for
// CursorHidden, else the theme's (animated) cursor. A file cursor that
// cannot load lands here with its CSS fallback keyword. Called with
// cursorState.mu held.
func (s *Session) applyNamedCursorLocked(desired string) error {
	c := &s.crs
	if s.applyShapeLocked(desired) {
		c.shown, c.cur = desired, nil
		return nil
	}
	if desired == CursorHidden {
		c.shown, c.cur = CursorHidden, nil
		return s.pushCursorLocked(nil)
	}
	cur, name, err := s.themeCursor(desired)
	if errors.Is(err, errCursorPending) {
		c.waiting = true
		return nil
	}
	if err != nil {
		return err
	}
	if len(cur.images) == 0 {
		return nil
	}
	if c.shown != name || c.cur != cur {
		c.shown, c.cur = name, cur
		c.delays, c.totalMs = cursorDelays(cur)
		c.start = s.cursorClock()
	}
	img, rest := c.frame(s.cursorClock)
	if err := s.pushCursorLocked(img); err != nil {
		return err
	}
	debug.Log("input", "cursor %q -> %q (%d frames, %dms loop)", desired, name, len(cur.images), c.totalMs)
	s.armCursorLocked(rest)
	return nil
}

// themeCursor resolves the theme cursor for a requested shape: the
// first candidate the theme carries, else the default arrow. It is
// the only theme-facing step of the cursor path, so tests substitute
// it through cursorState.theme. It reads nothing: a shape not yet read
// asks the background goroutine for it and reports errCursorPending,
// and the apply runs again when the shape arrives. Called with
// cursorState.mu held.
func (s *Session) themeCursor(desired string) (*themedCursor, string, error) {
	if s.crs.theme != nil {
		return s.crs.theme(desired)
	}
	theme, err := s.cursorPoolLocked()
	if err != nil {
		return nil, "", fmt.Errorf("cursor theme: %w", err)
	}
	name, settled := theme.resolve(desired)
	if !settled {
		s.requestCursorLocked(theme, desired)
		return nil, "", errCursorPending
	}
	cur, _, err := theme.lookup(name)
	if err != nil {
		return nil, "", fmt.Errorf("default cursor: %w", err)
	}
	return cur, name, nil
}

func (s *Session) cursorPoolLocked() (*cursorTheme, error) {
	if s.crs.pool != nil {
		return s.crs.pool, nil
	}
	pool, err := newCursorTheme(s.shm)
	if err != nil {
		return nil, err
	}
	s.crs.pool = pool
	return pool, nil
}

func (s *Session) requestCursorLocked(theme *cursorTheme, desired string) {
	c := &s.crs
	if c.requested[desired] {
		return
	}
	names := theme.unread(desired)
	if c.requested == nil {
		c.requested = make(map[string]bool)
	}
	c.requested[desired] = true
	posted := s.background().Post(func() {
		for _, name := range names {
			frames, err := s.readCursorFrames(name)
			s.postToLoop(func() { s.cursorRead(func(t *cursorTheme) { t.addCursor(name, frames, err) }) })
			if err == nil && name != defaultCursor {
				return
			}
		}
	})
	if !posted {
		delete(c.requested, desired)
		debug.Log("input", "cursor %q: the background queue is full", desired)
	}
}

func (s *Session) requestFileCursorLocked(path string) {
	c := &s.crs
	key := fileCursorPrefix + path
	if c.requested[key] {
		return
	}
	if c.requested == nil {
		c.requested = make(map[string]bool)
	}
	c.requested[key] = true
	posted := s.background().Post(func() {
		img, err := s.readCursorFile(path)
		s.postToLoop(func() { s.cursorRead(func(t *cursorTheme) { t.addFile(path, img, err) }) })
	})
	if !posted {
		delete(c.requested, key)
		debug.Log("input", "file cursor %q: the background queue is full", path)
	}
}

func (s *Session) cursorRead(add func(*cursorTheme)) {
	c := &s.crs
	c.mu.Lock()
	if c.pool == nil {
		c.mu.Unlock()
		return
	}
	add(c.pool)
	waiting := c.waiting
	c.mu.Unlock()
	if !waiting {
		return
	}
	if err := s.applyCursor(); err != nil {
		debug.Log("input", "cursor after its read: %v", err)
	}
}

func (s *Session) readCursorFrames(name string) ([]xcursor.Image, error) {
	if s.crs.readFrames != nil {
		return s.crs.readFrames(name)
	}
	if s.crs.reader == nil {
		s.crs.reader = newCursorReaderFromEnv(cursorThemeSize)
	}
	return s.crs.reader.frames(name)
}

func (s *Session) readCursorFile(path string) (xcursor.Image, error) {
	if s.crs.readFile != nil {
		return s.crs.readFile(path)
	}
	return xcursor.ReadWidest(path)
}

var preloadedCursors = []string{"", "pointer", "text", "resize_n", "resize_s", "resize_e", "resize_w", "resize_nw", "resize_ne", "resize_sw", "resize_se"}

func (s *Session) preloadCursors() {
	if s.shapeDevice != nil || s.shm == nil {
		return
	}
	c := &s.crs
	c.mu.Lock()
	defer c.mu.Unlock()
	theme, err := s.cursorPoolLocked()
	if err != nil {
		debug.Log("input", "cursor preload: %v", err)
		return
	}
	for _, desired := range preloadedCursors {
		s.requestCursorLocked(theme, desired)
	}
}

func (s *Session) closeCursorPool() {
	c := &s.crs
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pool != nil {
		_ = c.pool.file.Close()
	}
}

// pushCursorLocked attaches one frame of the shown cursor to the
// cursor surface and hands it to the compositor for the current enter
// serial. Called with cursorState.mu held.
func (s *Session) pushCursorLocked(img *cursorImage) error {
	if s.crs.push != nil {
		return s.crs.push(img)
	}
	if img == nil {
		// Hidden: set_cursor with a null surface. The zero Surface has
		// id 0, which is how the wire spells null; a nil *wl.Surface
		// would crash the binding's new-id bookkeeping instead.
		if err := s.pointer.SetCursor(s.pointerEnterSerial, &wl.Surface{}, 0, 0); err != nil {
			return fmt.Errorf("hide cursor: %w", err)
		}
		return nil
	}
	if err := s.crs.surface.Attach(img.buffer, 0, 0); err != nil {
		return fmt.Errorf("cursor attach: %w", err)
	}
	if err := s.crs.surface.DamageBuffer(0, 0, img.width, img.height); err != nil {
		return fmt.Errorf("cursor damage: %w", err)
	}
	if err := s.crs.surface.Commit(); err != nil {
		return fmt.Errorf("cursor commit: %w", err)
	}
	if err := s.pointer.SetCursor(s.pointerEnterSerial, s.crs.surface, img.hotspotX, img.hotspotY); err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	return nil
}

// cursorClock is the animation clock; tests inject a fixed one.
func (s *Session) cursorClock() time.Time {
	if s.crs.now != nil {
		return s.crs.now()
	}
	return time.Now()
}

// advanceCursor shows the frame due now for the animated cursor armed
// by gen and re-arms for that frame's remaining delay. A stale
// generation - the shape changed since the timer was armed - or a
// lost pointer drops the chain instead, so the timer stops entirely
// once no animated cursor shows.
func (s *Session) advanceCursor(gen uint64) {
	if !s.pushNextCursorFrame(gen) || s.Display == nil {
		return
	}
	pending, err := s.Display.Context().Flush()
	if err != nil {
		debug.Log("wire", "wlsession: flush a cursor frame: %v", err)
	}
	if pending {
		s.WakeAfter(0)
	}
}

func (s *Session) pushNextCursorFrame(gen uint64) bool {
	c := &s.crs
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen || s.pointer == nil || s.pointerEnterSerial == 0 || c.cur == nil {
		return false
	}
	c.armed = false
	img, rest := c.frame(s.cursorClock)
	if img == nil {
		return false
	}
	if err := s.pushCursorLocked(img); err != nil {
		return false
	}
	s.armCursorLocked(rest)
	return true
}

// stopCursorAnim drops the pending frame timer, if any. It runs on
// capability loss: there is no pointer to push frames to until it
// returns, and the next enter re-applies the desired shape, timer
// included.
func (s *Session) stopCursorAnim() {
	s.crs.mu.Lock()
	defer s.crs.mu.Unlock()
	s.crs.stopTimer()
}

// frame picks the frame of the shown cursor due at now and reports
// when it ends. It reads only cursorState fields, so callers hold the
// mutex.
func (c *cursorState) frame(now func() time.Time) (*cursorImage, time.Duration) {
	if c.cur == nil {
		return nil, 0
	}
	elapsed := uint32(0)
	if t := now(); t.After(c.start) {
		elapsed = intconv.Clamp[uint32](t.Sub(c.start).Milliseconds())
	}
	idx, rest := cursorFrame(c.totalMs, c.delays, elapsed)
	return c.cur.images[idx], rest
}

// armCursorLocked schedules the next frame advance rest from now;
// static cursors are never armed, which is what stops the animation
// chain once no animated cursor shows. Called with the mutex held.
func (s *Session) armCursorLocked(rest time.Duration) {
	c := &s.crs
	if rest <= 0 {
		return
	}
	gen := c.gen
	c.armed = true
	fire := func() { s.advanceCursor(gen) }
	if c.after != nil {
		c.after(rest, fire)
		return
	}
	c.timer = time.AfterFunc(rest, fire)
}

// stopTimer drops a pending frame timer. Called with the mutex held.
func (c *cursorState) stopTimer() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.armed = false
}

// cursorDelays collects the per-frame delays (ms) of cur and their
// sum; static cursors report no delays.
func cursorDelays(cur *themedCursor) ([]uint32, uint32) {
	if cur == nil || len(cur.images) < 2 {
		return nil, 0
	}
	delays := make([]uint32, len(cur.images))
	var total uint32
	for i, img := range cur.images {
		delays[i] = img.delay
		total += img.delay
	}
	return delays, total
}

// cursorFrame reports which frame of an animated cursor loop is due
// elapsedMs into it and how long that frame still runs - the pair the
// timer needs to advance right on time. It mirrors
// wlcursor.Cursor.FrameAndDuration: elapsed wraps around the total
// loop duration. Static cursors (a single frame or a zero total) show
// frame 0 and never come due.
func cursorFrame(totalMs uint32, delays []uint32, elapsedMs uint32) (index int, rest time.Duration) {
	if totalMs == 0 || len(delays) < 2 {
		return 0, 0
	}
	m := elapsedMs % totalMs
	for i, d := range delays {
		if m < d {
			return i, time.Duration(d-m) * time.Millisecond
		}
		m -= d
	}
	// Unreachable with a consistent delay list; show the last frame
	// for the smallest schedulable slice rather than spin.
	return len(delays) - 1, time.Millisecond
}
