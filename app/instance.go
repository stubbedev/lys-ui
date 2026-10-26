package app

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/mailbox"
)

// Single-instance activation, GApplication/relm4 parity (#79): the
// first process to claim an AppID becomes the primary and listens on a
// unix socket keyed by it; every later process forwards its invocation
// - argv, open requests, working directory, activation token - and
// exits. The primary delivers forwarded invocations on its loop
// goroutine through the same mailbox shape the typed messengers use,
// and spends the activation token, when the launcher provided one, to
// focus itself.

// Invocation is what one process run forwards to the primary: the
// command-line arguments, the paths it asks to open, the working
// directory they came from, and the XDG activation token the launcher
// provided, if any.
type Invocation struct {
	Args  []string
	Open  []string
	Cwd   string
	Token string
}

// OSInvocation builds the Invocation of this process from args (os.Args
// without the program): "--open PATH" pairs move into Open, and a token
// in XDG_ACTIVATION_TOKEN rides along - lys-ui's launch convention, so
// every single-instance app understands --open without parsing it
// twice.
func OSInvocation(args []string) Invocation {
	inv := Invocation{Token: os.Getenv("XDG_ACTIVATION_TOKEN")}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--open" {
			inv.Open = append(inv.Open, args[i+1])
		}
	}
	if len(inv.Open) == 0 {
		inv.Args = args
		return inv
	}
	kept := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--open" {
			i++
			continue
		}
		kept = append(kept, args[i])
	}
	inv.Args = kept
	return inv
}

// InstanceConfig declares the single-instance contract. The zero
// config guards nothing; an empty AppID is an error, because every
// un-keyed app would collide on one socket.
type InstanceConfig struct {
	// AppID keys the instance: one primary per AppID per runtime
	// directory. The window AppID is the natural key.
	AppID string
	// AllowMultipleInstances skips the guard: every process runs as its
	// own primary, relm4's allow_multiple_instances. The socket, the
	// forwarding, and the hooks are all inert.
	AllowMultipleInstances bool
	// OnCommandLine receives a secondary's argv and working directory
	// on the loop goroutine; empty argv is the plain-activate case (the
	// secondary ran with no arguments). It fires once per forwarded
	// invocation.
	OnCommandLine func(args []string, cwd string)
	// OnOpen receives the paths a secondary asked to open, in its
	// stead, on the loop goroutine. When it is set and the invocation
	// carries paths, it replaces OnCommandLine for that invocation -
	// GApplication's open signal replacing command-line.
	OnOpen func(paths []string, cwd string)
}

// Instance is a claimed single-instance guard: the primary's listener,
// or an inert holder when multiple instances are allowed. The zero
// value is not usable; ClaimInstance builds one.
type Instance struct {
	cfg  InstanceConfig
	path string
	ln   net.Listener
	mu   sync.Mutex
	app  *Application
	id   int
	box  mailbox.Box[Invocation]
}

// invocationWire is the on-socket form of Invocation.
type invocationWire struct {
	Args  []string `json:"args"`
	Open  []string `json:"open"`
	Cwd   string   `json:"cwd"`
	Token string   `json:"token"`
}

// ClaimInstance runs the single-instance protocol for cfg, forwarding
// inv when a primary already holds the AppID. It reports the Instance
// and true for the primary, nil and false for a secondary that has
// forwarded and should exit cleanly - the GApplication remote exit,
// conventionally status 0. A non-nil error means the claim itself
// failed (empty AppID, or the runtime directory accepts no socket);
// callers that prefer availability over uniqueness may treat it as
// primary.
//
// Claim before connecting the session: a secondary never touches the
// compositor. Forwarded invocations buffer until Bind wires the hooks
// onto a live application.
func ClaimInstance(cfg InstanceConfig, inv Invocation) (*Instance, bool, error) {
	if cfg.AppID == "" {
		return nil, false, errors.New("app: single-instance AppID is required")
	}
	if cfg.AllowMultipleInstances {
		return &Instance{cfg: cfg}, true, nil
	}
	path, err := instanceSocketPath(cfg.AppID)
	if err != nil {
		return nil, false, err
	}
	if forwardInvocation(path, inv) {
		return nil, false, nil
	}
	os.Remove(path) //nolint:gosec,errcheck // reclaiming a stale socket: nothing to do when it is gone
	ln, err := net.Listen("unix", path)
	if err != nil {
		// Another process won the claim race while we cleaned the
		// stale socket: fall back to forwarding.
		if forwardInvocation(path, inv) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("app: claim instance socket: %w", err)
	}
	ins := &Instance{cfg: cfg, path: path, ln: ln}
	debug.Log("shell", "instance %s: primary", cfg.AppID)
	go ins.accept()
	return ins, true, nil
}

// Bind wires the forwarded invocations onto an application's loop
// goroutine: every hook runs with event-callback guarantees, and
// anything buffered between the claim and Bind flushes through. The
// listener dies with the loop, like every other loop-owned resource.
func (ins *Instance) Bind(a *Application) {
	ins.mu.Lock()
	ins.app = a
	ins.mu.Unlock()
	id, ok := a.watchers.add(ins.stop)
	if ok {
		ins.id = id
	} else {
		ins.box.Stop()
	}
	if ins.box.Pending() > 0 {
		a.Invoke(ins.drain)
	}
}

// Close stops the guard early: the socket goes away and the loop stops
// accepting secondaries. The loop's end closes it too.
func (ins *Instance) Close() {
	ins.mu.Lock()
	a := ins.app
	id := ins.id
	ins.mu.Unlock()
	ins.stop()
	if a != nil && id != 0 {
		a.watchers.remove(id)
	}
}

// drain is the closure routed through Invoke: it takes the batch and
// delivers each invocation to the hooks, activating afterwards so a
// window the hook just created can be the one focused.
func (ins *Instance) drain() {
	for _, inv := range ins.box.Take() {
		ins.deliver(inv)
	}
}

func (ins *Instance) deliver(inv Invocation) {
	debug.Log("shell", "instance %s: forwarded invocation (%d args, %d open)", ins.cfg.AppID, len(inv.Args), len(inv.Open))
	if len(inv.Open) > 0 && ins.cfg.OnOpen != nil {
		ins.cfg.OnOpen(inv.Open, inv.Cwd)
	} else if ins.cfg.OnCommandLine != nil {
		ins.cfg.OnCommandLine(inv.Args, inv.Cwd)
	}
	if inv.Token != "" {
		ins.activate(inv.Token)
	}
}

// activate spends a forwarded activation token on the first live
// window's surface, focusing the application after the hooks ran.
func (ins *Instance) activate(token string) {
	ins.mu.Lock()
	a := ins.app
	ins.mu.Unlock()
	if a == nil || a.sess == nil {
		return
	}
	for _, w := range a.windows {
		if surf := w.host.HostSurface(); surf != nil {
			a.sess.Activate(surf, token)
			return
		}
	}
}

// receive queues a forwarded invocation and routes the drain onto the
// loop when one is bound; before Bind the mailbox buffers.
func (ins *Instance) receive(inv Invocation) {
	ins.mu.Lock()
	route := ins.box.Put(inv) && ins.app != nil
	app := ins.app
	ins.mu.Unlock()
	if route {
		app.Invoke(ins.drain)
	}
}

// accept serves secondaries until the listener closes: one forwarded
// invocation per connection, acknowledged after queueing so the
// secondary can tell a forward from a dead socket. The order matters:
// once the secondary holds the ack the invocation must already be in
// the mailbox, or a Bind or pump that the ack happens-before can miss
// it and the hooks fire a loop iteration late.
func (ins *Instance) accept() {
	for {
		conn, err := ins.ln.Accept()
		if err != nil {
			return
		}
		inv, err := readInvocation(conn)
		if err == nil {
			ins.receive(inv)
			_, _ = conn.Write([]byte{'k'})
		}
		_ = conn.Close()
	}
}

func (ins *Instance) stop() {
	ins.box.Stop()
	if ins.ln != nil {
		_ = ins.ln.Close()
		_ = os.Remove(ins.path)
	}
}

// forwardInvocation offers the primary this run's invocation: true when
// a primary took it, false when nobody holds the AppID (or the holder
// never answered - a stale socket the caller may reclaim).
func forwardInvocation(path string, inv Invocation) bool {
	conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err := writeInvocation(conn, inv); err != nil {
		return false
	}
	ack := make([]byte, 1)
	_, err = io.ReadFull(conn, ack)
	return err == nil
}

func writeInvocation(conn net.Conn, inv Invocation) error {
	payload, err := json.Marshal(invocationWire(inv))
	if err != nil {
		return err
	}
	n, err := intconv.CheckedUint32(len(payload))
	if err != nil {
		return fmt.Errorf("app: invocation frame length: %w", err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], n)
	if _, err := conn.Write(header[:]); err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

// maxInvocationBytes caps one forwarded invocation; a command line is
// kilobytes, never megabytes.
const maxInvocationBytes = 1 << 20

func readInvocation(conn net.Conn) (Invocation, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return Invocation{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > maxInvocationBytes {
		return Invocation{}, errors.New("app: forwarded invocation too large")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return Invocation{}, err
	}
	var wire invocationWire
	if err := json.Unmarshal(payload, &wire); err != nil {
		return Invocation{}, err
	}
	return Invocation(wire), nil
}

// instanceSocketPath keys the AppID under the runtime directory (the
// per-user, tmpfs-backed XDG_RUNTIME_DIR, falling back to the temp dir
// where it is unset).
func instanceSocketPath(appID string) (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	safe := make([]rune, 0, len(appID))
	for _, r := range appID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			safe = append(safe, r)
		default:
			safe = append(safe, '_')
		}
	}
	if len(safe) == 0 {
		return "", errors.New("app: single-instance AppID is required")
	}
	return filepath.Join(dir, "lys-ui-"+string(safe)+".sock"), nil
}
