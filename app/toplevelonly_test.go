package app_test

import (
	"encoding/binary"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wltest"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	wlDisplayID                = 1
	displaySync                = 0
	displayGetRegistry         = 1
	displayEventDeleteID       = 1
	registryBind               = 0
	registryEventGlobal        = 0
	compositorCreateSurface    = 0
	surfaceAttach              = 1
	surfaceCommit              = 6
	wmBaseGetXdgSurface        = 2
	xdgSurfaceGetToplevel      = 1
	layerShellGetSurface       = 0
	configureEvent             = 0
	shmEventFormat             = 0
	shmFormatArgb8888          = 0
	callbackEventDone          = 0
	headerSize                 = 8
	fakeSurfaceW, fakeSurfaceH = 64, 48
)

type fakeGlobal struct {
	iface   string
	version uint32
}

var (
	coreGlobals      = []fakeGlobal{{"wl_compositor", 4}, {"wl_shm", 1}}
	xdgShellGlobal   = fakeGlobal{"xdg_wm_base", 2}
	layerShellGlobal = fakeGlobal{"zwlr_layer_shell_v1", 4}
	kioskGlobals     = slices.Concat(coreGlobals, []fakeGlobal{xdgShellGlobal})
)

// fakeCompositor is the far end of a real socket offering exactly globals:
// it answers what a client needs to map a surface and reports each commit
// that carries a buffer on mapped.
type fakeCompositor struct {
	conn     *net.UnixConn
	globals  []fakeGlobal
	objects  map[uint32]string
	roles    map[uint32]func()
	pending  map[uint32]bool
	toplevel map[uint32]uint32
	serial   uint32
	mapped   chan struct{}
}

func startFakeCompositor(t *testing.T, globals []fakeGlobal) *fakeCompositor {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, "wayland-fake")
	t.Cleanup(func() { _ = ln.Close() })
	f := &fakeCompositor{
		globals:  globals,
		objects:  map[uint32]string{wlDisplayID: "wl_display"},
		roles:    map[uint32]func(){},
		pending:  map[uint32]bool{},
		toplevel: map[uint32]uint32{},
		mapped:   make(chan struct{}, 1),
	}
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		f.conn = conn
		f.serve()
	}()
	return f
}

func (f *fakeCompositor) serve() {
	buf := make([]byte, 0, 1<<16)
	chunk := make([]byte, 1<<16)
	oob := make([]byte, unix.CmsgSpace(4*28))
	for {
		n, oobn, _, _, err := f.conn.ReadMsgUnix(chunk, oob)
		if err != nil {
			return
		}
		closeReceivedFds(oob[:oobn])
		buf = append(buf, chunk[:n]...)
		for len(buf) >= headerSize {
			sizeOpcode := binary.NativeEndian.Uint32(buf[4:8])
			size := int(sizeOpcode >> 16)
			if len(buf) < size {
				break
			}
			f.handle(binary.NativeEndian.Uint32(buf[:4]), sizeOpcode&0xffff, buf[headerSize:size])
			buf = buf[size:]
		}
	}
}

func closeReceivedFds(oob []byte) {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for i := range msgs {
		fds, err := unix.ParseUnixRights(&msgs[i])
		if err != nil {
			continue
		}
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	}
}

func word(body []byte, i int) uint32 { return binary.NativeEndian.Uint32(body[4*i:]) }

func (f *fakeCompositor) handle(obj, opcode uint32, body []byte) {
	switch iface := f.objects[obj]; {
	case iface == "wl_display" && opcode == displaySync:
		id := word(body, 0)
		f.send(id, callbackEventDone, 0)
		f.send(wlDisplayID, displayEventDeleteID, id)
	case iface == "wl_display" && opcode == displayGetRegistry:
		id := word(body, 0)
		f.objects[id] = "wl_registry"
		for i, g := range f.globals {
			f.sendGlobal(id, uint32(i+1), g)
		}
	case iface == "wl_registry" && opcode == registryBind:
		f.bind(body)
	case iface == "wl_compositor" && opcode == compositorCreateSurface:
		f.objects[word(body, 0)] = "wl_surface"
	case iface == "xdg_wm_base" && opcode == wmBaseGetXdgSurface:
		id, surface := word(body, 0), word(body, 1)
		f.objects[id] = "xdg_surface"
		f.roles[surface] = func() { f.configureXdg(id) }
	case iface == "xdg_surface" && opcode == xdgSurfaceGetToplevel:
		id := word(body, 0)
		f.objects[id] = "xdg_toplevel"
		f.toplevel[obj] = id
	case iface == "zwlr_layer_shell_v1" && opcode == layerShellGetSurface:
		id, surface := word(body, 0), word(body, 1)
		f.objects[id] = "zwlr_layer_surface_v1"
		f.roles[surface] = func() {
			f.serial++
			f.send(id, configureEvent, f.serial, fakeSurfaceW, fakeSurfaceH)
		}
	case iface == "wl_surface" && opcode == surfaceAttach:
		f.pending[obj] = word(body, 0) != 0
	case iface == "wl_surface" && opcode == surfaceCommit:
		f.commit(obj)
	}
}

func (f *fakeCompositor) bind(body []byte) {
	nameLen := int(word(body, 1))
	padded := (nameLen + 3) &^ 3
	iface := string(body[8 : 8+nameLen-1])
	id := word(body, 2+padded/4+1)
	f.objects[id] = iface
	if iface == "wl_shm" {
		f.send(id, shmEventFormat, shmFormatArgb8888)
	}
}

func (f *fakeCompositor) configureXdg(xdgSurface uint32) {
	f.send(f.toplevel[xdgSurface], configureEvent, fakeSurfaceW, fakeSurfaceH, 0)
	f.serial++
	f.send(xdgSurface, configureEvent, f.serial)
}

func (f *fakeCompositor) commit(surface uint32) {
	if configure, ok := f.roles[surface]; ok {
		delete(f.roles, surface)
		configure()
	}
	if f.pending[surface] {
		f.pending[surface] = false
		select {
		case f.mapped <- struct{}{}:
		default:
		}
	}
}

func (f *fakeCompositor) send(obj, opcode uint32, args ...uint32) {
	msg := make([]byte, headerSize, headerSize+4*len(args))
	for _, a := range args {
		msg = binary.NativeEndian.AppendUint32(msg, a)
	}
	f.write(obj, opcode, msg)
}

func (f *fakeCompositor) sendGlobal(registry, name uint32, g fakeGlobal) {
	msg := make([]byte, headerSize, 64)
	msg = binary.NativeEndian.AppendUint32(msg, name)
	msg = binary.NativeEndian.AppendUint32(msg, uint32(len(g.iface)+1))
	msg = append(msg, g.iface...)
	msg = append(msg, make([]byte, 4-len(g.iface)%4)...)
	msg = binary.NativeEndian.AppendUint32(msg, g.version)
	f.write(registry, registryEventGlobal, msg)
}

func (f *fakeCompositor) write(obj, opcode uint32, msg []byte) {
	binary.NativeEndian.PutUint32(msg, obj)
	binary.NativeEndian.PutUint32(msg[4:], uint32(len(msg))<<16|opcode)
	_, _ = f.conn.Write(msg)
}

func (f *fakeCompositor) awaitMapped(t *testing.T, what string) {
	t.Helper()
	select {
	case <-f.mapped:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s never committed a buffer", what)
	}
}

func connectFake(t *testing.T, globals []fakeGlobal) (*fakeCompositor, *app.Application) {
	t.Helper()
	t.Setenv("LYS_UI_RENDERER", "cpu")
	f := startFakeCompositor(t, globals)
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect with %v: %v", globals, err)
	}
	a := app.NewApplication(sess)
	t.Cleanup(sess.Close)
	return f, a
}

func runUntilCleanup(t *testing.T, a *app.Application) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = a.Run()
	}()
	t.Cleanup(func() {
		a.Invoke(a.Quit)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("application did not quit")
		}
	})
}

func square() widget.Widget { return widget.NewDrawingArea(fakeSurfaceW, fakeSurfaceH) }

func layerConfig() app.LayerConfig {
	return app.LayerConfig{Anchor: app.AnchorTop, Width: fakeSurfaceW, Height: fakeSurfaceH, Namespace: "lys-ui-fake", Root: square()}
}

func TestToplevelMapsWithoutLayerShellOrOutputs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height uint32
	}{
		{"fixed size", fakeSurfaceW, fakeSurfaceH},
		{"sized by its content", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, a := connectFake(t, kioskGlobals)
			if outs := len(a.Session().Outputs()); outs != 0 {
				t.Fatalf("the compositor has no outputs, the session lists %d", outs)
			}
			if _, err := a.NewWindow(app.WindowConfig{Title: "t", Width: tc.width, Height: tc.height, Root: square()}); err != nil {
				t.Fatalf("NewWindow: %v", err)
			}
			runUntilCleanup(t, a)
			f.awaitMapped(t, "the toplevel")
		})
	}
}

func TestLayerRequestsWithoutLayerShellFailAtTheCall(t *testing.T) {
	_, a := connectFake(t, kioskGlobals)
	l, err := a.NewLayer(layerConfig())
	if !errors.Is(err, app.ErrLayerShellUnavailable) {
		t.Errorf("NewLayer: err = %v, want ErrLayerShellUnavailable", err)
	}
	if l != nil {
		t.Errorf("NewLayer returned a surface with its error: %v", l)
	}
	bar, err := a.NewBar(app.BarConfig{Bar: widget.NewBar(widget.BarTop), Namespace: "lys-ui-fake"})
	if !errors.Is(err, app.ErrLayerShellUnavailable) {
		t.Errorf("NewBar: err = %v, want ErrLayerShellUnavailable", err)
	}
	if bar != nil {
		t.Errorf("NewBar returned a bar with its error: %v", bar)
	}
}

func TestLayerMapsWhereLayerShellIsOffered(t *testing.T) {
	f, a := connectFake(t, slices.Concat(kioskGlobals, []fakeGlobal{layerShellGlobal}))
	if _, err := a.NewLayer(layerConfig()); err != nil {
		t.Fatalf("NewLayer: %v", err)
	}
	runUntilCleanup(t, a)
	f.awaitMapped(t, "the layer surface")
}

func TestToplevelRequestWithoutXdgShellFailsAtTheCall(t *testing.T) {
	f, a := connectFake(t, slices.Concat(coreGlobals, []fakeGlobal{layerShellGlobal}))
	if _, err := a.NewWindow(app.WindowConfig{Title: "t", Root: square()}); err == nil {
		t.Error("NewWindow succeeded on a compositor without xdg_wm_base")
	}
	if _, err := a.NewLayer(layerConfig()); err != nil {
		t.Fatalf("NewLayer without xdg_wm_base: %v", err)
	}
	runUntilCleanup(t, a)
	f.awaitMapped(t, "the layer surface")
}

func TestConnectFailsWithoutARequiredGlobal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		globals []fakeGlobal
	}{
		{"wl_compositor", []fakeGlobal{{"wl_shm", 1}, xdgShellGlobal}},
		{"wl_shm", []fakeGlobal{{"wl_compositor", 4}, xdgShellGlobal}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startFakeCompositor(t, tc.globals)
			sess, err := app.Connect()
			if err == nil {
				sess.Close()
				t.Fatal("Connect succeeded")
			}
			if want := "wlsession: missing wayland globals: [" + tc.name + "]"; err.Error() != want {
				t.Errorf("err = %q, want %q", err, want)
			}
		})
	}
}
