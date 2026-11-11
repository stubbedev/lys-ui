package app

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/fdtest"
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
	handout *dbus.UnixFD
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
	unicastFrom(d.conn, dest, path, member, body...)
}

func unicastFrom(conn *dbus.Conn, dest string, path dbus.ObjectPath, member string, body ...any) {
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
	conn.Send(msg, nil)
}

func (d *mockDesktop) respond(sender dbus.Sender, path dbus.ObjectPath, code uint32, results map[string]dbus.Variant) {
	d.mu.Lock()
	if d.handout != nil {
		results["handout"] = dbus.MakeVariant(*d.handout)
	}
	d.mu.Unlock()
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
	inhibit := func(flags InhibitFlags, reason string) (func(), error) {
		t.Helper()
		var (
			release func()
			err     error
			done    bool
		)
		a.Inhibit(flags, reason, func(r func(), e error) { release, err, done = r, e, true })
		pumpUntil(t, a, func() bool { return done })
		return release, err
	}
	release, err := inhibit(InhibitLogout|InhibitSuspend, "saving the document")
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	flags, reasons := slices.Clone(d.flags), slices.Clone(d.reasons)
	d.mu.Unlock()
	if !slices.Equal(flags, []uint32{1 | 4}) || !slices.Equal(reasons, []string{"saving the document"}) {
		t.Errorf("portal saw flags %v reasons %v", flags, reasons)
	}
	release()
	release()
	if !a.desktop().queue.Settle(5 * time.Second) {
		t.Fatal("the release never ran")
	}
	d.mu.Lock()
	closed := len(d.closed)
	d.mu.Unlock()
	if closed != 1 {
		t.Errorf("release closed %d request handles, want 1", closed)
	}
	if _, err := inhibit(InhibitIdle, "deny"); !errors.Is(err, ErrPortalDenied) {
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
	statusDone := false
	a.SetBackgroundStatus("syncing 3 files", func(err error) {
		if err != nil {
			t.Error(err)
		}
		statusDone = true
	})
	pumpUntil(t, a, func() bool { return statusDone })
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
	monitoring := false
	a.OnSessionStateChange(func(s SessionState, _ bool) { states = append(states, s) }, func(err error) {
		if err != nil {
			t.Error(err)
		}
		monitoring = true
	})
	pumpUntil(t, a, func() bool { return monitoring })
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

func stuckPortals(t *testing.T, a *Application) (release func()) {
	t.Helper()
	held, entered := make(chan struct{}), make(chan struct{})
	if !a.desktop().queue.Post(func() {
		close(entered)
		<-held
	}) {
		t.Fatal("the worker refused the holding job")
	}
	<-entered
	return sync.OnceFunc(func() { close(held) })
}

func TestPortalCallsReturnWhileThePortalIsStuck(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	release := stuckPortals(t, a)
	defer release()
	var answers []string
	a.Invoke(func() {
		a.Inhibit(InhibitIdle, "stuck", func(_ func(), err error) {
			if err != nil {
				t.Error(err)
			}
			answers = append(answers, "inhibit")
		})
		a.OnSessionStateChange(func(SessionState, bool) {}, func(err error) {
			if err != nil {
				t.Error(err)
			}
			answers = append(answers, "monitor")
		})
		a.SetBackgroundStatus("stuck", func(err error) {
			if err != nil {
				t.Error(err)
			}
			answers = append(answers, "status")
		})
	})
	a.pump(time.Now())
	d.mu.Lock()
	seen := len(d.flags)
	d.mu.Unlock()
	if seen != 0 || len(answers) != 0 {
		t.Fatalf("the stuck worker let through %d bus calls and %v", seen, answers)
	}
	release()
	pumpUntil(t, a, func() bool { return len(answers) == 3 })
	if !slices.Equal(answers, []string{"inhibit", "monitor", "status"}) {
		t.Errorf("answers %v, want posting order", answers)
	}
}

func TestQueryEndAnswerFollowsTheInhibitMadeForIt(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	monitoring := false
	objected := false
	a.OnSessionStateChange(func(s SessionState, _ bool) {
		if s != SessionQueryEnd {
			return
		}
		a.Inhibit(InhibitLogout, "unsaved work", func(_ func(), err error) {
			if err != nil {
				t.Error(err)
			}
			objected = true
		})
	}, func(err error) {
		if err != nil {
			t.Error(err)
		}
		monitoring = true
	})
	pumpUntil(t, a, func() bool { return monitoring })
	d.mu.Lock()
	monitor, owner := d.monitor, d.owner
	d.mu.Unlock()
	d.unicast(owner, "/org/freedesktop/portal/desktop", portalInhibitIface+"|StateChanged", monitor, map[string]dbus.Variant{
		"session-state":      dbus.MakeVariant(uint32(SessionQueryEnd)),
		"screensaver-active": dbus.MakeVariant(false),
	})
	flagsAtQuery := -1
	pumpUntil(t, a, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		if len(d.queried) > 0 && flagsAtQuery < 0 {
			flagsAtQuery = len(d.flags)
		}
		return objected && len(d.queried) == 1
	})
	if flagsAtQuery != 1 {
		t.Errorf("the portal saw %d inhibits when the query-end was answered, want the objection", flagsAtQuery)
	}
}

func TestPortalBusyWhenTheQueueIsFull(t *testing.T) {
	a := testApp(nil)
	d := a.desktop()
	defer d.close()
	release := stuckPortals(t, a)
	defer release()
	for d.queue.Post(func() {}) {
	}
	var got error
	a.SetBackgroundStatus("x", func(err error) { got = err })
	a.pump(time.Now())
	if !errors.Is(got, ErrPortalBusy) {
		t.Errorf("status over a full queue returned %v", got)
	}
}

func TestPortalCloseEndsAStuckRequest(t *testing.T) {
	startMockDesktop(t)
	a := testApp(nil)
	d := a.desktop()
	result := make(chan error, 1)
	d.post("hold", func() error {
		if err := d.connect(); err != nil {
			return err
		}
		_, _, err := d.request(portalRequestBg+"NoSuchMethodThatNeverAnswers", map[string]dbus.Variant{})
		return err
	}, nil)
	go func() {
		d.close()
		result <- nil
	}()
	select {
	case <-result:
	case <-time.After(portalRequestTimeout / 2):
		t.Fatal("close waited on the portal")
	}
}

// Every portal signal the application is sent closes its descriptors:
// a Response's results, a StateChanged's state and the arguments after
// those it declares, and a StateChanged another peer sends in the
// portal's place, which reaches no listener.
func TestPortalSignalsCloseTheirDescriptors(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	impostor, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = impostor.Close() }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	fd := dbus.UnixFD(w.Fd())
	d.mu.Lock()
	d.handout = &fd
	d.mu.Unlock()
	var states []SessionState
	monitoring := false
	a.OnSessionStateChange(func(s SessionState, _ bool) { states = append(states, s) }, func(err error) {
		if err != nil {
			t.Error(err)
		}
		monitoring = true
	})
	pumpUntil(t, a, func() bool { return monitoring })
	d.mu.Lock()
	monitor, owner := d.monitor, d.owner
	d.mu.Unlock()
	change := func(from *dbus.Conn, state SessionState) {
		unicastFrom(from, owner, portalPath, portalInhibitIface+"|StateChanged", monitor, map[string]dbus.Variant{
			"session-state": dbus.MakeVariant(uint32(state)),
			"handout":       dbus.MakeVariant(fd),
		}, fd)
	}
	background := func() {
		t.Helper()
		answered := false
		a.RequestBackground(BackgroundRequest{Reason: "sync"}, func(_ BackgroundResult, err error) {
			if err != nil {
				t.Error(err)
			}
			answered = true
		})
		pumpUntil(t, a, func() bool { return answered })
	}
	change(d.conn, SessionRunning)
	pumpUntil(t, a, func() bool { return len(states) == 1 })
	background()
	before := fdtest.Own(t)
	const events = 16
	for range events {
		change(impostor, SessionEnding)
	}
	if err := impostor.BusObject().Call("org.freedesktop.DBus.GetId", 0).Err; err != nil {
		t.Fatal(err)
	}
	for i := range events {
		change(d.conn, SessionRunning)
		pumpUntil(t, a, func() bool { return len(states) == 2+i })
		background()
	}
	if slices.Contains(states, SessionEnding) {
		t.Errorf("states %v: another peer's StateChanged reached the listener", states)
	}
	fdtest.RequireNoneBeyond(t, before, "the application read the portal's Responses and StateChanged, and another peer's")
}
