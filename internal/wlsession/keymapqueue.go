package wlsession

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/keymapfd"
	"github.com/stubbedev/lys-lib/logutil"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xkb"
)

const maxQueuedKeyEvents = 4096

type compiledKeymap struct {
	keymap *xkb.Keymap
	state  *xkb.State
	err    error
}

type pendingKeymap struct {
	ready    bool
	compiled compiledKeymap
}

type queuedKeyEvent struct {
	keymap  *pendingKeymap
	deliver func()
}

// HandleKeyboardKeymap implements wl.KeyboardKeymapHandler: the keymap
// fd is mapped (keymapfd.Read), compiled, and closed on the background
// goroutine, with the loop free to draw meanwhile. The fd is the
// handler's whatever becomes of the keymap: closed once compiled, or at
// once when the background queue refuses it.
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
	if ev.FdError != nil {
		s.settleKeymap(pending, compiledKeymap{err: fmt.Errorf("a keymap event without an fd (format %d): %w", ev.Format, ev.FdError)})
		return
	}
	compile := s.compileKeymap
	if compile == nil {
		compile = s.compileKeymapFd
	}
	fd, size, format := ev.Fd, ev.Size, ev.Format
	posted := s.background().Post(func() {
		compiled := compile(fd, size, format)
		keymapfd.Close(fd)
		s.postToLoop(func() { s.settleKeymap(pending, compiled) })
	})
	if !posted {
		keymapfd.Close(fd)
		s.settleKeymap(pending, compiledKeymap{err: fmt.Errorf("the background queue refused a %d byte keymap", size)})
	}
}

func (s *Session) compileKeymapFd(fd uintptr, size, format uint32) compiledKeymap {
	data, err := keymapfd.Read(fd, size)
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

func (s *Session) settleKeymap(pending *pendingKeymap, compiled compiledKeymap) {
	pending.ready, pending.compiled = true, compiled
	s.drainKeyQueue()
}

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
	s.syncLayoutNames()
}

// KeymapPending reports whether a keymap the compositor sent is still on
// its way: keys read now would go through the one it replaces.
func (s *Session) KeymapPending() bool { return s.keyboardQueued() }

func (s *Session) keyboardQueued() bool { return len(s.keyQueue) > 0 }

func (s *Session) queueKeyEvent(deliver func()) {
	if len(s.keyQueue) >= maxQueuedKeyEvents {
		logutil.L().Error("wlsession: dropped a keyboard event behind a keymap still compiling", slog.Int("queued", len(s.keyQueue)))
		return
	}
	s.keyQueue = append(s.keyQueue, queuedKeyEvent{deliver: deliver})
}
