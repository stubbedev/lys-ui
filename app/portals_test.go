package app

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

type mockDesktop struct {
	t       *testing.T
	conn    *dbus.Conn
	mu      sync.Mutex
	flags   []uint32
	reasons []string
	closed  []dbus.ObjectPath
	status  []string
	queried []dbus.ObjectPath
	monitor dbus.ObjectPath
	owner   string
	print   mockPrint
}

type mockRequest struct {
	d    *mockDesktop
	path dbus.ObjectPath
}

func (r *mockRequest) Close() *dbus.Error {
	r.d.mu.Lock()
	r.d.closed = append(r.d.closed, r.path)
	r.d.mu.Unlock()
	return nil
}

func (d *mockDesktop) handle(sender dbus.Sender, opts map[string]dbus.Variant) dbus.ObjectPath {
	token, _ := opts["handle_token"].Value().(string)
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + strings.NewReplacer(":", "", ".", "_").Replace(string(sender)) + "/" + token)
	if err := d.conn.Export(&mockRequest{d: d, path: path}, path, "org.freedesktop.portal.Request"); err != nil {
		d.t.Error(err)
	}
	return path
}

func (d *mockDesktop) unicast(dest string, path dbus.ObjectPath, member string, body ...any) {
	iface, name, _ := strings.Cut(member, "|")
	msg := &dbus.Message{
		Type: dbus.TypeSignal,
		Headers: map[dbus.HeaderField]dbus.Variant{
			dbus.FieldPath:        dbus.MakeVariant(path),
			dbus.FieldInterface:   dbus.MakeVariant(iface),
			dbus.FieldMember:      dbus.MakeVariant(name),
			dbus.FieldDestination: dbus.MakeVariant(dest),
		},
		Body: body,
	}
	if len(body) > 0 {
		msg.Headers[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(body...))
	}
	d.conn.Send(msg, nil)
}

func (d *mockDesktop) respond(sender dbus.Sender, path dbus.ObjectPath, code uint32, results map[string]dbus.Variant) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		d.unicast(string(sender), path, "org.freedesktop.portal.Request|Response", code, results)
	}()
}

type inhibitIface struct{ d *mockDesktop }

func (i inhibitIface) Inhibit(sender dbus.Sender, _ string, flags uint32, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	reason, _ := opts["reason"].Value().(string)
	i.d.mu.Lock()
	i.d.flags = append(i.d.flags, flags)
	i.d.reasons = append(i.d.reasons, reason)
	i.d.mu.Unlock()
	path := i.d.handle(sender, opts)
	code := uint32(0)
	if reason == "deny" {
		code = 1
	}
	i.d.respond(sender, path, code, map[string]dbus.Variant{})
	return path, nil
}

func (i inhibitIface) CreateMonitor(sender dbus.Sender, _ string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	path := i.d.handle(sender, opts)
	token, _ := opts["session_handle_token"].Value().(string)
	session := dbus.ObjectPath("/org/freedesktop/portal/desktop/session/x/" + token)
	i.d.mu.Lock()
	i.d.monitor, i.d.owner = session, string(sender)
	i.d.mu.Unlock()
	i.d.respond(sender, path, 0, map[string]dbus.Variant{"session_handle": dbus.MakeVariant(string(session))})
	return path, nil
}

func (i inhibitIface) QueryEndResponse(session dbus.ObjectPath) *dbus.Error {
	i.d.mu.Lock()
	i.d.queried = append(i.d.queried, session)
	i.d.mu.Unlock()
	return nil
}

type backgroundIface struct{ d *mockDesktop }

func (b backgroundIface) RequestBackground(sender dbus.Sender, _ string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	path := b.d.handle(sender, opts)
	auto, _ := opts["autostart"].Value().(bool)
	b.d.respond(sender, path, 0, map[string]dbus.Variant{"background": dbus.MakeVariant(true), "autostart": dbus.MakeVariant(auto)})
	return path, nil
}

func (b backgroundIface) SetStatus(opts map[string]dbus.Variant) *dbus.Error {
	msg, _ := opts["message"].Value().(string)
	b.d.mu.Lock()
	b.d.status = append(b.d.status, msg)
	b.d.mu.Unlock()
	return nil
}

func startMockDesktop(t *testing.T) *mockDesktop {
	t.Helper()
	bus := dbustest.Session(t)
	conn := bus.Conn(t)
	d := &mockDesktop{t: t, conn: conn}
	for iface, v := range map[string]any{"org.freedesktop.portal.Inhibit": inhibitIface{d}, "org.freedesktop.portal.Background": backgroundIface{d}, portalPrintIface: printIface{d}} {
		if err := conn.Export(v, "/org/freedesktop/portal/desktop", iface); err != nil {
			t.Fatal(err)
		}
	}
	if reply, err := conn.RequestName("org.freedesktop.portal.Desktop", 0); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("claim the portal name: %v %v", reply, err)
	}
	return d
}

func pumpUntil(t *testing.T, a *Application, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting on the portal")
		}
		a.pump(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
}

func TestInhibitHoldsUntilReleased(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	release, err := a.Inhibit(InhibitLogout|InhibitSuspend, "saving the document")
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	flags, reasons := slices.Clone(d.flags), slices.Clone(d.reasons)
	d.mu.Unlock()
	if !slices.Equal(flags, []uint32{1 | 4}) || !slices.Equal(reasons, []string{"saving the document"}) {
		t.Errorf("portal saw flags %v reasons %v", flags, reasons)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	_ = release()
	d.mu.Lock()
	closed := len(d.closed)
	d.mu.Unlock()
	if closed != 1 {
		t.Errorf("release closed %d request handles, want 1", closed)
	}
	if _, err := a.Inhibit(InhibitIdle, "deny"); !errors.Is(err, ErrPortalDenied) {
		t.Errorf("a denied inhibit returned %v", err)
	}
}

func TestBackgroundRequestAndStatus(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	var got *BackgroundResult
	a.RequestBackground(BackgroundRequest{Reason: "sync", Autostart: true}, func(r BackgroundResult, err error) {
		if err != nil {
			t.Error(err)
		}
		got = &r
	})
	pumpUntil(t, a, func() bool { return got != nil })
	if !got.Background || !got.Autostart {
		t.Errorf("result %+v", *got)
	}
	if err := a.SetBackgroundStatus("syncing 3 files"); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	status := slices.Clone(d.status)
	d.mu.Unlock()
	if !slices.Equal(status, []string{"syncing 3 files"}) {
		t.Errorf("status %v", status)
	}
}

func TestSessionStateChangesReachTheLoopAndQueryEndIsAnswered(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	var states []SessionState
	if err := a.OnSessionStateChange(func(s SessionState, _ bool) { states = append(states, s) }); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	monitor, owner := d.monitor, d.owner
	d.mu.Unlock()
	d.unicast(owner, "/org/freedesktop/portal/desktop", portalInhibitIface+"|StateChanged", monitor, map[string]dbus.Variant{
		"session-state":      dbus.MakeVariant(uint32(SessionQueryEnd)),
		"screensaver-active": dbus.MakeVariant(false),
	})
	pumpUntil(t, a, func() bool { return len(states) == 1 })
	if states[0] != SessionQueryEnd {
		t.Errorf("state %v", states[0])
	}
	pumpUntil(t, a, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.queried) == 1
	})
	d.mu.Lock()
	queried := d.queried[0]
	d.mu.Unlock()
	if queried != monitor {
		t.Errorf("query-end answered for %q, want the monitor %q", queried, monitor)
	}
}
