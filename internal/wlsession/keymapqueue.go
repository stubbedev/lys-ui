package wlsession

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/keymapfd"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

// maxQueuedKeyEvents bounds the keyboard events waiting on a keymap. A
// compile is finite, so a queue this long means a client flooded with
// events it could not have typed.
const maxQueuedKeyEvents = 4096

// compiledKeymap is what one wl_keyboard.keymap event becomes on the
// background goroutine.
type compiledKeymap struct {
	keymap *xkb.Keymap
	state  *xkb.State
	err    error
}

// pendingKeymap is one keymap event on its way to being installed.
type pendingKeymap struct {
	ready    bool
	compiled compiledKeymap
}

// queuedKeyEvent is one entry of the keyboard's wire order while a keymap
// compiles: a keymap to install at this point, or an event to deliver.
type queuedKeyEvent struct {
	keymap  *pendingKeymap
	deliver func()
}

// HandleKeyboardKeymap implements wl.KeyboardKeymapHandler: the keymap
// fd is mapped (keymapfd.Read), compiled, and closed on the background
// goroutine, with the loop free to draw meanwhile.
//
// Key codes mean what the latest keymap says, so what follows this event
// on the wire must not be read through the one before it, and the old
// keymap is still what applies to everything before it. Every keyboard
// event arriving before the compile ends therefore waits behind it, in
// wire order (enter, leave, key, modifiers, the seat's loss of the
// keyboard), and is delivered when the keymap is installed: late, never
// lost, never mistranslated. Keymaps in a row install in turn, each with
// the events that belong to it. A keymap that fails to compile leaves
// keys untranslated, as dropKeymap says. Synthesized repeats hold off
// while KeymapPending reports true.
func (s *Session) HandleKeyboardKeymap(ev wl.KeyboardKeymapEvent) {
	pending := &pendingKeymap{}
	s.keyQueue = append(s.keyQueue, queuedKeyEvent{keymap: pending})
	if ev.FdError != nil || ev.Fd == 0 {
		s.settleKeymap(pending, compiledKeymap{err: errors.Join(fmt.Errorf("a keymap event without an fd (format %d)", ev.Format), ev.FdError)})
		return
	}
	compile := s.compileKeymap
	if compile == nil {
		compile = s.compileKeymapFd
	}
	fd, size, format := ev.Fd, ev.Size, ev.Format
	posted := s.background().Post(func() {
		compiled := compile(fd, size, format)
		s.postToLoop(func() { s.settleKeymap(pending, compiled) })
	})
	if !posted {
		keymapfd.Close(fd)
		s.settleKeymap(pending, compiledKeymap{err: fmt.Errorf("the background queue refused a %d byte keymap", size)})
	}
}

// compileKeymapFd reads and compiles a keymap on the background goroutine,
// the only user of xkbContext, and closes its fd.
func (s *Session) compileKeymapFd(fd uintptr, size, format uint32) compiledKeymap {
	data, err := keymapfd.Read(fd, size)
	keymapfd.Close(fd)
	if err != nil {
		return compiledKeymap{err: fmt.Errorf("read the keymap: %w", err)}
	}
	if s.xkbContext == nil {
		s.xkbContext = xkb.NewContext(os.Getenv)
	}
	km, err := s.xkbContext.NewKeymapFromString(data)
	if err != nil {
		return compiledKeymap{err: fmt.Errorf("compile a %d byte keymap (format %d): %w", len(data), format, err)}
	}
	state := km.NewState()
	debug.Log("input", "keymap (format %d, %d bytes) compiled: KEY_A=%v", format, len(data), state.KeyGetOneSym(30+8))
	return compiledKeymap{keymap: km, state: state}
}

// settleKeymap records a keymap's outcome and delivers what no longer has
// to wait.
func (s *Session) settleKeymap(pending *pendingKeymap, compiled compiledKeymap) {
	pending.ready, pending.compiled = true, compiled
	s.drainKeyQueue()
}

// drainKeyQueue walks the keyboard's wire order up to the first keymap
// still compiling. A delivery may dispatch and append behind itself, so
// the head comes off before it runs.
func (s *Session) drainKeyQueue() {
	for len(s.keyQueue) > 0 {
		head := s.keyQueue[0]
		if head.keymap != nil && !head.keymap.ready {
			return
		}
		s.keyQueue[0] = queuedKeyEvent{}
		s.keyQueue = s.keyQueue[1:]
		if head.keymap != nil {
			s.installKeymap(head.keymap.compiled)
			continue
		}
		head.deliver()
	}
	s.keyQueue = nil
}

func (s *Session) installKeymap(compiled compiledKeymap) {
	if compiled.err != nil {
		s.dropKeymap(compiled.err)
		return
	}
	s.xkbKeymap, s.xkbState = compiled.keymap, compiled.state
}

// KeymapPending reports whether a keymap the compositor sent is still on
// its way: keys read now would go through the one it replaces.
func (s *Session) KeymapPending() bool { return s.keyboardQueued() }

// keyboardQueued reports whether a keyboard event must wait in keyQueue:
// the handlers ask first, so a key with nothing compiling builds no
// closure.
func (s *Session) keyboardQueued() bool { return len(s.keyQueue) > 0 }

// queueKeyEvent puts deliver behind what already waits.
func (s *Session) queueKeyEvent(deliver func()) {
	if len(s.keyQueue) >= maxQueuedKeyEvents {
		logutil.L().Error("wlsession: dropped a keyboard event behind a keymap still compiling", slog.Int("queued", len(s.keyQueue)))
		return
	}
	s.keyQueue = append(s.keyQueue, queuedKeyEvent{deliver: deliver})
}
